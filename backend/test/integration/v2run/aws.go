package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/adapters/awsaccess"
)

// AWSCLI runs read-only `aws` commands with an explicit environment.
type AWSCLI interface {
	Run(ctx context.Context, args ...string) (stdout []byte, stderr string, exit int)
}

// ExecAWS is the real runner. Its environment contains only PATH, HOME, the
// explicit region and the optional host-chain handles of the configuration:
// the operator's preflight and verification identity, never the product's.
type ExecAWS struct {
	Binary string
	Env    []string
}

// NewExecAWS builds the runner of a configuration.
func NewExecAWS(c Config) (ExecAWS, error) {
	b, ok := c.Binaries["aws"]
	if !ok || c.AWS == nil {
		return ExecAWS{}, errors.New("the aws binary is not configured")
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "AWS_REGION=" + AWSRegion, "AWS_DEFAULT_REGION=" + AWSRegion, "AWS_PAGER="}
	for _, kv := range [][2]string{{"AWS_SHARED_CREDENTIALS_FILE", c.AWS.SharedCredentialsFile}, {"AWS_CONFIG_FILE", c.AWS.ConfigFile}, {"AWS_PROFILE", c.AWS.Profile}} {
		if kv[1] != "" {
			env = append(env, kv[0]+"="+kv[1])
		}
	}
	for _, proxy := range []string{"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy", "SSL_CERT_FILE", "AWS_CA_BUNDLE"} {
		if v := os.Getenv(proxy); v != "" {
			env = append(env, proxy+"="+v)
		}
	}
	return ExecAWS{Binary: b.Path, Env: env}, nil
}

// Run implements AWSCLI.
func (a ExecAWS) Run(ctx context.Context, args ...string) ([]byte, string, int) {
	cmd := exec.CommandContext(ctx, a.Binary, append([]string{"--region", AWSRegion, "--output", "json"}, args...)...)
	cmd.Env = a.Env
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	exit := 0
	if err != nil {
		exit = 1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		}
	}
	return []byte(out.String()), errOut.String(), exit
}

const awsActionSimulationChunkSize = 50

var ecrRequiredActions = []string{
	"ecr:CreateRepository", "ecr:DeleteRepository", "ecr:DescribeRepositories", "ecr:DescribeImages", "ecr:ListTagsForResource",
	"ecr:GetAuthorizationToken", "ecr:TagResource", "ecr:PutImage", "ecr:BatchCheckLayerAvailability", "ecr:InitiateLayerUpload",
	"ecr:UploadLayerPart", "ecr:CompleteLayerUpload",
}

// RequiredAWSActions returns the product-module capability set plus the ECR
// actions used by image preparation and exact cleanup. Product permissions are
// owned by awsaccess.RequiredActions so connection verification and execution
// preflight cannot drift apart.
func RequiredAWSActions() []string {
	seen := make(map[string]struct{})
	actions := append(awsaccess.RequiredActions(), ecrRequiredActions...)
	for _, action := range actions {
		seen[action] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for action := range seen {
		out = append(out, action)
	}
	sort.Strings(out)
	return out
}

// PreflightAWS runs the read-only AWS checks before any effect: the STS
// principal and account are the configured ones, the region is fixed, nothing
// carries the run tag yet, the principal may perform the create/delete actions
// (policy simulation) and the node type has the pod capacity of the scenario.
func PreflightAWS(ctx context.Context, cli AWSCLI, c Config) []Check {
	var out []Check
	add := func(name string, ok bool, note string) { out = append(out, Check{Name: name, OK: ok, Note: note}) }
	a := c.AWS
	raw, stderr, exit := cli.Run(ctx, "sts", "get-caller-identity")
	var who struct{ Account, Arn string }
	if exit != 0 || json.Unmarshal(raw, &who) != nil {
		add("sts-identity", false, firstLine(stderr))
		return out
	}
	add("sts-account", who.Account == a.AccountID, "")
	add("sts-principal", who.Arn == a.PrincipalARN, "")
	add("region-fixed", a.Region == AWSRegion, AWSRegion)
	add("executor-egress-/32", strings.HasSuffix(a.ExecutorEgress, "/32"), "")
	capacity, err := CheckAWSScenario(c.Scenario, a.NodeInstanceType)
	add("pod-capacity", err == nil && capacity.OK(), a.NodeInstanceType)
	for _, q := range AWSQueries(c.RunID, AWSLinkage{}) {
		stdout, serr, code := cli.Run(ctx, queryArgs(q)...)
		p := q.Evaluate(stdout, serr, code)
		add("run-tag-absent:"+q.ID, p.Absent(), strings.Join(p.Present, ","))
	}
	if err := simulateAWSActions(ctx, cli, a.PrincipalARN, a.Region, RequiredAWSActions()); err != nil {
		add("permission-simulation", false, err.Error())
	} else {
		add("permission-simulation", true, "")
	}
	return out
}

type awsSimulationResult struct {
	EvaluationResults []struct {
		EvalActionName string   `json:"EvalActionName"`
		EvalDecision   string   `json:"EvalDecision"`
		MissingContext []string `json:"MissingContextValues"`
		Organizations  *struct {
			AllowedByOrganizations *bool `json:"AllowedByOrganizations"`
		} `json:"OrganizationsDecisionDetail"`
	} `json:"EvaluationResults"`
	IsTruncated bool   `json:"IsTruncated"`
	Marker      string `json:"Marker"`
}

// validateAWSActionSimulation requires one and only one allowed result for
// every requested action. Partial, duplicated, extraneous, truncated, and
// context-incomplete replies are not permission evidence.
func validateAWSActionSimulation(expected []string, raw []byte) error {
	var doc awsSimulationResult
	if len(expected) == 0 || json.Unmarshal(raw, &doc) != nil || doc.IsTruncated || doc.Marker != "" {
		return errors.New("permission simulation result is incomplete or unreadable")
	}
	want := make(map[string]struct{}, len(expected))
	for _, action := range expected {
		if action == "" {
			return errors.New("permission simulation action set is invalid")
		}
		if _, exists := want[action]; exists {
			return errors.New("permission simulation action set contains duplicates")
		}
		want[action] = struct{}{}
	}
	seen := make(map[string]struct{}, len(doc.EvaluationResults))
	for _, result := range doc.EvaluationResults {
		if _, ok := want[result.EvalActionName]; !ok {
			return errors.New("permission simulation returned an unexpected action")
		}
		if _, ok := seen[result.EvalActionName]; ok {
			return errors.New("permission simulation returned a duplicate action")
		}
		seen[result.EvalActionName] = struct{}{}
		if result.EvalDecision != "allowed" || len(result.MissingContext) != 0 {
			return fmt.Errorf("permission denied or unproven: %s", result.EvalActionName)
		}
		if result.Organizations != nil && (result.Organizations.AllowedByOrganizations == nil || !*result.Organizations.AllowedByOrganizations) {
			return fmt.Errorf("organization policy denied or did not prove: %s", result.EvalActionName)
		}
	}
	if len(seen) != len(want) {
		return errors.New("permission simulation omitted required actions")
	}
	return nil
}

func simulateAWSActions(ctx context.Context, cli AWSCLI, principalARN, region string, actions []string) error {
	for start := 0; start < len(actions); start += awsActionSimulationChunkSize {
		end := min(start+awsActionSimulationChunkSize, len(actions))
		chunk := actions[start:end]
		args := []string{"iam", "simulate-principal-policy", "--policy-source-arn", principalARN, "--action-names"}
		args = append(args, chunk...)
		args = append(args, "--context-entries", "ContextKeyName=aws:RequestedRegion,ContextKeyType=string,ContextKeyValues="+region)
		raw, _, code := cli.Run(ctx, args...)
		if code != 0 {
			return errors.New("permission simulation could not be proven")
		}
		if err := validateAWSActionSimulation(chunk, raw); err != nil {
			return err
		}
	}
	return nil
}

// AWSGate refuses any AWS effect until I04-11 passed end to end: each k8s-4f
// evidence directory must cover A01, E01 or E02 with browser PASS, a matching
// CLOSED_CLEAN marker, and the final runner receipt after SQL/video/evidence
// checks and local cleanup succeeded.
func AWSGate(c Config) error {
	if c.AWS == nil {
		return errors.New("aws settings are required")
	}
	seen := map[string]bool{}
	for _, dir := range c.AWS.I0411Evidence {
		raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err != nil {
			return fmt.Errorf("I04-11 evidence %s has no run.json", filepath.Base(dir))
		}
		var run struct {
			Scenario string `json:"scenario"`
			RunID    string `json:"runId"`
			Target   string `json:"target"`
		}
		if json.Unmarshal(raw, &run) != nil || run.Target != TargetK8s4f || run.RunID == "" {
			return fmt.Errorf("I04-11 evidence %s is not a k8s-4f run", filepath.Base(dir))
		}
		result, err := os.ReadFile(filepath.Join(dir, "result.txt"))
		if err != nil || strings.TrimSpace(string(result)) != "PASS" {
			return fmt.Errorf("I04-11 evidence %s (%s) did not PASS", filepath.Base(dir), run.Scenario)
		}
		marker, err := ReadRunMarker(dir)
		if err != nil || marker.State != MarkerClosedClean || marker.RunID != run.RunID || marker.Target != run.Target {
			return fmt.Errorf("I04-11 evidence %s (%s) is not closed clean", filepath.Base(dir), run.Scenario)
		}
		completionRaw, err := os.ReadFile(filepath.Join(dir, "runner-completion.json"))
		var completion struct {
			RunID        string `json:"runId"`
			Scenario     string `json:"scenario"`
			Target       string `json:"target"`
			Status       string `json:"status"`
			CleanupState string `json:"cleanupState"`
			ExitCode     *int   `json:"exitCode"`
		}
		if err != nil || json.Unmarshal(completionRaw, &completion) != nil ||
			completion.RunID != run.RunID || completion.Scenario != run.Scenario || completion.Target != run.Target ||
			completion.Status != "PASS" || completion.ExitCode == nil || *completion.ExitCode != 0 || completion.CleanupState != MarkerClosedClean {
			return fmt.Errorf("I04-11 evidence %s (%s) lacks a successful end-to-end runner completion proof", filepath.Base(dir), run.Scenario)
		}
		seen[run.Scenario] = true
	}
	for _, need := range []string{"A01", "E01", "E02"} {
		if !seen[need] {
			return fmt.Errorf("I04-11 is not complete: no passed and cleaned %s run in aws.i0411Evidence", need)
		}
	}
	return nil
}

// AWSLinkageFromManifest builds the exact owner chain from the assets the run
// recorded as CREATED_BY_RUN. Missing IDs leave only the run-tag sweep.
func AWSLinkageFromManifest(m Manifest) AWSLinkage {
	var l AWSLinkage
	for _, a := range m.OwnedByRun() {
		switch a.Kind {
		case "vpc":
			l.VPCID = a.ID
		case "eks-cluster":
			l.ClusterName = a.ID
		case "eks-nodegroup":
			l.NodeGroup = a.ID
		case "rds-cluster":
			l.DBCluster = a.ID
		case "elasticache-group":
			l.CacheGroup = a.ID
		case "ecr-repository":
			l.Repositories = append(l.Repositories, a.ID)
		case "iam-role":
			l.Roles = append(l.Roles, a.ID)
		}
	}
	return l
}

func awsQueryLayer(id string) string {
	switch {
	case id == "rds-cluster" || id == "elasticache-group":
		return LayerAppResources
	case id == "eks-cluster" || id == "eks-nodegroups" || id == "ec2-instances-cluster" || id == "ebs-volumes-cluster":
		return LayerEKS
	case id == "vpc" || id == "subnets" || id == "security-groups" || id == "route-tables" || id == "internet-gateways" || id == "nat-gateways" || id == "network-interfaces" || id == "ec2-instances-vpc":
		return LayerVPC
	}
	return LayerAux
}

// VerifyAWSLayer runs the read-only absence queries of one layer. Anything but
// a verified absence (present, access denied, throttled, timeout) fails.
func VerifyAWSLayer(ctx context.Context, cli AWSCLI, c Config, m Manifest, layer string) error {
	if err := verifyAWSIdentity(ctx, cli, c); err != nil {
		return fmt.Errorf("AWS identity is not verified for absence checks: %w", err)
	}
	var bad []string
	for _, q := range AWSQueries(c.RunID, AWSLinkageFromManifest(m)) {
		if awsQueryLayer(q.ID) != layer {
			continue
		}
		stdout, stderr, exit := cli.Run(ctx, queryArgs(q)...)
		if q.ID == "tagged-resources" {
			if err := verifyTaggedAWSResources(ctx, cli, c, m, stdout, stderr, exit); err != nil {
				bad = append(bad, "tagged-resources: "+err.Error())
			}
			continue
		}
		p := q.Evaluate(stdout, stderr, exit)
		switch {
		case len(p.Present) > 0:
			bad = append(bad, q.ID+" present: "+strings.Join(p.Present, ","))
		case p.Indeterminate != "":
			bad = append(bad, q.ID+" indeterminate: "+p.Indeterminate)
		}
	}
	if len(bad) > 0 {
		return errors.New(strings.Join(bad, "; "))
	}
	return nil
}

var ec2ResourceARN = regexp.MustCompile(`^arn:aws:ec2:([a-z0-9-]+):([0-9]{12}):(security-group|subnet)/(sg-[0-9a-f]+|subnet-[0-9a-f]+)$`)
var exactNotFoundMessage = regexp.MustCompile(`^An error occurred \(([^)]+)\) when calling the ([A-Za-z0-9]+) operation:`)

type taggedAWSResource struct {
	ResourceARN string `json:"ResourceARN"`
	Tags        []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	} `json:"Tags"`
}

// verifyTaggedAWSResources compensates only for the known eventual-consistency
// lag in RGTA's index. A stale entry is absent only when the exact full ARN was
// recorded as a unique run-owned manifest asset and the service's native,
// exact-ID describe call returns its typed not-found response.
func verifyTaggedAWSResources(ctx context.Context, cli AWSCLI, c Config, m Manifest, raw []byte, stderr string, exit int) error {
	if exit != 0 {
		return fmt.Errorf("tagged resource query failed (exit %d)", exit)
	}
	var doc struct {
		Mappings []taggedAWSResource `json:"ResourceTagMappingList"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return errors.New("tagged resource response is malformed")
	}
	if doc.Mappings == nil {
		return errors.New("tagged resource response omitted ResourceTagMappingList")
	}
	if m.RunID != c.RunID || c.Target != TargetAWS || m.Target != TargetAWS || c.AWS == nil || m.Account != c.AWS.AccountID || m.Region != c.AWS.Region || m.Principal != c.AWS.PrincipalARN {
		return errors.New("manifest account, region, target or run does not match the configured AWS run")
	}
	seen := make(map[string]bool, len(doc.Mappings))
	for _, item := range doc.Mappings {
		arn := item.ResourceARN
		if arn == "" || seen[arn] {
			return errors.New("tagged resource response contains a missing or duplicate ARN")
		}
		seen[arn] = true
	}
	for _, item := range doc.Mappings {
		arn := item.ResourceARN
		if !hasExactRunTag(item.Tags, c.RunID) {
			return fmt.Errorf("tagged resource %s lacks the exact run tag", arn)
		}
		kind, id, ok := staleEC2Candidate(arn, c, m)
		if !ok {
			return fmt.Errorf("tagged resource remains present: %s", arn)
		}
		operation, expectedCode, command, flag := "", "", "", ""
		switch kind {
		case "subnet":
			operation, expectedCode, command, flag = "DescribeSubnets", "InvalidSubnetID.NotFound", "describe-subnets", "--subnet-ids"
		case "security-group":
			operation, expectedCode, command, flag = "DescribeSecurityGroups", "InvalidGroup.NotFound", "describe-security-groups", "--group-ids"
		default:
			return fmt.Errorf("tagged resource remains present: %s", arn)
		}
		_, detail, code := cli.Run(ctx, "ec2", command, flag, id)
		if code == 0 {
			return fmt.Errorf("tagged resource remains physically present: %s", arn)
		}
		detail = strings.TrimSpace(detail)
		// AWS CLI 2.35 prefixes its native service error with this exact
		// diagnostic marker. Strip only that known marker; arbitrary wrapper
		// text must not turn an unrecognized failure into proof of absence.
		detail = strings.TrimPrefix(detail, "aws: [ERROR]: ")
		match := exactNotFoundMessage.FindStringSubmatch(detail)
		if len(match) != 3 || match[1] != expectedCode || match[2] != operation {
			return fmt.Errorf("exact native absence check for %s was indeterminate", arn)
		}
	}
	return nil
}

func hasExactRunTag(tags []struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}, runID string) bool {
	found := false
	for _, tag := range tags {
		if tag.Key == "run-id" {
			if found || tag.Value != runID {
				return false
			}
			found = true
		}
	}
	return found
}

func staleEC2Candidate(arn string, c Config, m Manifest) (kind, id string, ok bool) {
	match := ec2ResourceARN.FindStringSubmatch(arn)
	if len(match) != 5 || match[1] != c.AWS.Region || match[2] != c.AWS.AccountID {
		return "", "", false
	}
	kind, id = match[3], match[4]
	if (kind == "security-group" && !strings.HasPrefix(id, "sg-")) || (kind == "subnet" && !strings.HasPrefix(id, "subnet-")) {
		return "", "", false
	}
	count, exact := 0, false
	for _, asset := range m.Assets {
		if asset.ID == arn {
			count++
			exact = asset.Kind == "arn" && asset.Layer == LayerAux && asset.Owner == OwnerCreatedByRun
		}
	}
	return kind, id, count == 1 && exact
}

// queryArgs strips the fixed `aws --region R --output json` prefix of a query.
func queryArgs(q Query) []string { return q.Argv[5:] }

// AWSGuard proves the supervisor acts in the configured account as the
// configured principal. An identity mismatch is unknown, never NOT_OWNED.
func AWSGuard(ctx context.Context, cli AWSCLI, c Config) error {
	return verifyAWSIdentity(ctx, cli, c)
}

func verifyAWSIdentity(ctx context.Context, cli AWSCLI, c Config) error {
	if c.AWS == nil {
		return errors.New("AWS configuration is missing")
	}
	raw, _, exit := cli.Run(ctx, "sts", "get-caller-identity")
	var who struct{ Account, Arn string }
	if exit != 0 || json.Unmarshal(raw, &who) != nil {
		return errors.New("the AWS identity could not be determined")
	}
	if who.Account != c.AWS.AccountID {
		return errors.New("the AWS execution account does not match the configured account")
	}
	if who.Arn != c.AWS.PrincipalARN {
		return errors.New("the AWS execution principal does not match the configured principal")
	}
	return nil
}

// AWSCleanupSteps are the supervisor steps of an AWS run. The Terraform
// instances (namespace, Aurora/ElastiCache, EKS, VPC) are destroyed in reverse
// dependency order by ONE original-engine pass (v2cleanup terraform); the later
// layers then each verify physical absence by exact IDs, exact linkage and the
// run tag. No step deletes with the AWS CLI or kubectl, and no step unlocks.
func AWSCleanupSteps(ctl, cleanup, manifestPath, handlesPath, configPath, vaultSnapshotPath string, c Config) []Step {
	env := map[string]string{"V2_AWS_BINARY": c.Binaries["aws"].Path}
	for k, v := range map[string]string{"AWS_SHARED_CREDENTIALS_FILE": c.AWS.SharedCredentialsFile, "AWS_CONFIG_FILE": c.AWS.ConfigFile, "AWS_PROFILE": c.AWS.Profile} {
		if v != "" {
			env[k] = v
		}
	}
	guard := []string{ctl, "aws-guard", "-config", configPath}
	verify := func(layer string) []string {
		return []string{ctl, "aws-verify", "-config", configPath, "-manifest", manifestPath, "-layer", layer}
	}
	budgets := CleanupStepsFor(c.Cleanup.Duration)
	step := func(layer, action string, timeout time.Duration) Step {
		return Step{Name: layer, Layer: layer, Env: env, Timeout: Duration{Duration: timeout}, MaxAttempts: 2, Guard: guard,
			Argv: []string{cleanup, action, "-handles", handlesPath, "-manifest", manifestPath}, Verify: verify(layer)}
	}
	probeBudget := budgets.Delivery / 2
	ecrBudget := budgets.Aux / 2
	steps := []Step{
		{Name: "technical-pods", Layer: LayerDelivery, Env: env, Timeout: Duration{Duration: probeBudget}, MaxAttempts: 2, Guard: guard,
			Argv:   []string{ctl, "aws-probes-clean", "-config", configPath, "-manifest", manifestPath},
			Verify: []string{ctl, "aws-probes-clean", "-config", configPath, "-manifest", manifestPath, "-verify"}},
		step(LayerDelivery, "delivery", budgets.Delivery-probeBudget),
		step(LayerAppResources, "terraform", budgets.Terraform),
		step(LayerNamespace, "status", budgets.Status),
		step(LayerEKS, "status", budgets.Status),
		step(LayerVPC, "status", budgets.Status),
		{Name: "ecr-repositories", Layer: LayerAux, Env: env, Timeout: Duration{Duration: ecrBudget}, MaxAttempts: 2, Guard: guard,
			Argv:   []string{ctl, "aws-ecr-clean", "-config", configPath, "-manifest", manifestPath},
			Verify: []string{ctl, "aws-ecr-clean", "-config", configPath, "-manifest", manifestPath, "-verify"}},
	}
	aux := Step{Name: "aux", Layer: LayerAux, Env: env, Timeout: Duration{Duration: budgets.Aux - ecrBudget}, MaxAttempts: 2, Guard: guard,
		Argv: []string{cleanup, "status", "-handles", handlesPath, "-manifest", manifestPath}, Verify: verify(LayerAux)}
	steps = append(steps, aux)
	if c.SecretStore != nil && !c.SecretStore.RunOwned {
		steps = append(steps, Step{
			Name: "vault-config", Layer: LayerAux, Env: env, Timeout: Duration{Duration: budgets.Vault}, MaxAttempts: 3, Guard: guard,
			Argv:   []string{ctl, "vault-clean", "-config", configPath, "-snapshot", vaultSnapshotPath},
			Verify: []string{ctl, "vault-clean", "-verify", "-config", configPath, "-snapshot", vaultSnapshotPath},
		})
	}
	return steps
}
