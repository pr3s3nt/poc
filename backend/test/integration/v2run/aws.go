package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

// RequiredAWSActions are the create/delete actions simulated read-only before a
// run. A successful STS call proves authentication only.
var RequiredAWSActions = []string{
	"ec2:CreateVpc", "ec2:DeleteVpc", "ec2:CreateSubnet", "ec2:DeleteSubnet", "ec2:CreateSecurityGroup", "ec2:DeleteSecurityGroup",
	"ec2:CreateInternetGateway", "ec2:DeleteInternetGateway", "ec2:CreateRouteTable", "ec2:DeleteRouteTable", "ec2:CreateTags",
	"eks:CreateCluster", "eks:DeleteCluster", "eks:CreateNodegroup", "eks:DeleteNodegroup", "eks:TagResource",
	"iam:CreateRole", "iam:DeleteRole", "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:PassRole", "iam:TagRole",
	"rds:CreateDBCluster", "rds:DeleteDBCluster", "rds:CreateDBInstance", "rds:DeleteDBInstance", "rds:CreateDBSubnetGroup",
	"elasticache:CreateReplicationGroup", "elasticache:DeleteReplicationGroup", "elasticache:CreateCacheSubnetGroup",
	"ecr:CreateRepository", "ecr:DeleteRepository", "ecr:DescribeRepositories", "ecr:DescribeImages", "ecr:ListTagsForResource", "ecr:GetAuthorizationToken",
	"ecr:TagResource", "ecr:PutImage", "ecr:BatchCheckLayerAvailability", "ecr:InitiateLayerUpload", "ecr:UploadLayerPart", "ecr:CompleteLayerUpload",
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
	args := []string{"iam", "simulate-principal-policy", "--policy-source-arn", a.PrincipalARN, "--action-names"}
	args = append(args, RequiredAWSActions...)
	sim, serr, code := cli.Run(ctx, args...)
	if code != 0 {
		add("permission-simulation", false, "cannot be proven read-only: "+firstLine(serr))
		return out
	}
	var doc struct {
		EvaluationResults []struct {
			EvalActionName string
			EvalDecision   string
		}
	}
	if json.Unmarshal(sim, &doc) != nil || len(doc.EvaluationResults) == 0 {
		add("permission-simulation", false, "unreadable simulation answer")
		return out
	}
	var denied []string
	for _, r := range doc.EvaluationResults {
		if r.EvalDecision != "allowed" {
			denied = append(denied, r.EvalActionName)
		}
	}
	sort.Strings(denied)
	add("permission-simulation", len(denied) == 0 && len(doc.EvaluationResults) >= len(RequiredAWSActions), strings.Join(denied, ","))
	return out
}

// AWSGate refuses any AWS effect until I04-11 passed: the evidence directories
// of the k8s-4f runs must cover A01, E01 and E02, each with result PASS and a
// run marker closed CLEAN (the supervisor reported CLEAN and absence was
// verified).
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
		var run struct{ Scenario, Target string }
		if json.Unmarshal(raw, &run) != nil || run.Target != TargetK8s4f {
			return fmt.Errorf("I04-11 evidence %s is not a k8s-4f run", filepath.Base(dir))
		}
		result, err := os.ReadFile(filepath.Join(dir, "result.txt"))
		if err != nil || strings.TrimSpace(string(result)) != "PASS" {
			return fmt.Errorf("I04-11 evidence %s (%s) did not PASS", filepath.Base(dir), run.Scenario)
		}
		marker, err := ReadRunMarker(dir)
		if err != nil || marker.State != MarkerClosedClean {
			return fmt.Errorf("I04-11 evidence %s (%s) is not closed clean", filepath.Base(dir), run.Scenario)
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
