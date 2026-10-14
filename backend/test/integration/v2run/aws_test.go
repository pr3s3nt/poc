package v2run_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

// fakeAWS answers by the first words of the command; unknown commands exit 255.
type fakeAWS struct {
	account, arn string
	simulate     string // "allowed" or "implicitDeny"
	tagged       bool
	calls        []string
}

func (f *fakeAWS) Run(_ context.Context, args ...string) ([]byte, string, int) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch {
	case args[0] == "sts":
		return []byte(`{"Account":"` + f.account + `","Arn":"` + f.arn + `"}`), "", 0
	case args[0] == "resourcegroupstaggingapi":
		if f.tagged {
			return []byte(`{"ResourceTagMappingList":[{"ResourceARN":"arn:aws:ec2:us-east-1:1:vpc/vpc-1"}]}`), "", 0
		}
		return []byte(`{"ResourceTagMappingList":[]}`), "", 0
	case args[0] == "ec2":
		return []byte(`{"Reservations":[],"Volumes":[],"Addresses":[]}`), "", 0
	case args[0] == "iam":
		var results []string
		for _, action := range v2run.RequiredAWSActions {
			results = append(results, `{"EvalActionName":"`+action+`","EvalDecision":"`+f.simulate+`"}`)
		}
		return []byte(`{"EvaluationResults":[` + strings.Join(results, ",") + `]}`), "", 0
	}
	return nil, "unexpected", 255
}

func newAWS() *fakeAWS {
	return &fakeAWS{account: v2run.AWSAccountID, arn: "arn:aws:iam::452025861381:user/x", simulate: "allowed"}
}

func TestAWSPreflightIsReadOnlyAndProvesIdentityTagsAndPermissions(t *testing.T) {
	c := awsConfig(t, "A03", "t4g.medium")
	cli := newAWS()
	checks := v2run.PreflightAWS(context.Background(), cli, c)
	if !v2run.AllOK(checks) {
		t.Fatalf("a good account must pass: %+v", checks)
	}
	readOnly := map[string]bool{"sts get-caller-identity": true, "resourcegroupstaggingapi get-resources": true, "iam simulate-principal-policy": true,
		"ec2 describe-instances": true, "ec2 describe-volumes": true, "ec2 describe-addresses": true}
	for _, call := range cli.calls {
		words := strings.Fields(call)
		if !readOnly[words[0]+" "+words[1]] {
			t.Fatalf("a preflight must only run read-only operations, got %q", words[0]+" "+words[1])
		}
	}
	for name, mutate := range map[string]func(*fakeAWS){
		"other account":       func(f *fakeAWS) { f.account = "111111111111" },
		"other principal":     func(f *fakeAWS) { f.arn = "arn:aws:iam::452025861381:user/other" },
		"tag already present": func(f *fakeAWS) { f.tagged = true },
		"permission denied":   func(f *fakeAWS) { f.simulate = "implicitDeny" },
	} {
		bad := newAWS()
		mutate(bad)
		if v2run.AllOK(v2run.PreflightAWS(context.Background(), bad, c)) {
			t.Fatalf("%s must fail the preflight", name)
		}
	}
	small := awsConfig(t, "A03", "t4g.medium")
	small.AWS.NodeInstanceType = "t4g.small"
	if v2run.AllOK(v2run.PreflightAWS(context.Background(), newAWS(), small)) {
		t.Fatal("an instance type below the pod budget must fail before create")
	}
}

func TestAWSGuardAndAbsenceChecksRequireExactConfiguredIdentity(t *testing.T) {
	c := awsConfig(t, "A02", "t4g.medium")
	for name, mutate := range map[string]func(*fakeAWS){
		"wrong account":   func(f *fakeAWS) { f.account = "111122223333" },
		"wrong principal": func(f *fakeAWS) { f.arn = "arn:aws:iam::452025861381:role/other" },
	} {
		t.Run(name, func(t *testing.T) {
			cli := newAWS()
			mutate(cli)
			if err := v2run.AWSGuard(context.Background(), cli, c); err == nil {
				t.Fatal("an execution identity mismatch was accepted")
			}
			if err := v2run.VerifyAWSLayer(context.Background(), cli, c, v2run.NewManifest(c, time.Now()), v2run.LayerVPC); err == nil {
				t.Fatal("absence was inferred under an unconfigured execution identity")
			}
			if len(cli.calls) != 2 || cli.calls[0] != "sts get-caller-identity" || cli.calls[1] != "sts get-caller-identity" {
				t.Fatalf("resource queries ran despite identity mismatch: %v", cli.calls)
			}
		})
	}
}

func evidenceRun(t *testing.T, scenario, result, marker string) string {
	t.Helper()
	dir := t.TempDir()
	raw, _ := json.Marshal(map[string]string{"scenario": scenario, "target": "k8s-4f"})
	if err := os.WriteFile(filepath.Join(dir, "run.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.txt"), []byte(result+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if marker != "" {
		if err := v2run.WriteRunMarker(dir, v2run.RunMarker{RunID: "r", State: v2run.MarkerStarted}); err != nil {
			t.Fatal(err)
		}
		if marker != v2run.MarkerStarted {
			if err := v2run.WriteRunMarker(dir, v2run.RunMarker{RunID: "r", State: marker}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

func TestAWSGateRefusesAnyEffectUntilI0411PassedAndWasCleaned(t *testing.T) {
	c := awsConfig(t, "A02", "t4g.medium")
	good := []string{evidenceRun(t, "A01", "PASS", v2run.MarkerClosedClean), evidenceRun(t, "E01", "PASS", v2run.MarkerClosedClean), evidenceRun(t, "E02", "PASS", v2run.MarkerClosedClean)}
	c.AWS.I0411Evidence = good
	if err := v2run.AWSGate(c); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"no evidence":      nil,
		"missing E02":      good[:2],
		"failed run":       {good[0], good[1], evidenceRun(t, "E02", "FAIL", v2run.MarkerClosedClean)},
		"not cleaned":      {good[0], good[1], evidenceRun(t, "E02", "PASS", v2run.MarkerArmed)},
		"no marker at all": {good[0], good[1], evidenceRun(t, "E02", "PASS", "")},
	}
	for name, dirs := range cases {
		c.AWS.I0411Evidence = dirs
		if err := v2run.AWSGate(c); err == nil {
			t.Fatalf("%s must keep the AWS gate closed", name)
		}
	}
}

func TestAWSCleanupStepsNeverDeleteWithTheCLIAndVerifyEveryLayerByExactLinkage(t *testing.T) {
	c := awsConfig(t, "A03", "t4g.medium")
	steps := v2run.AWSCleanupSteps("/abs/v2ctl", "/abs/v2cleanup", "/p/manifest.json", "/p/handles.json", "/p/config.json", "/p/vault.json", c)
	layers := map[string]bool{}
	if len(steps) == 0 || steps[0].Name != "technical-pods" || strings.Join(steps[0].Argv, " ") != "/abs/v2ctl aws-probes-clean -config /p/config.json -manifest /p/manifest.json" || strings.Join(steps[0].Verify, " ") != "/abs/v2ctl aws-probes-clean -config /p/config.json -manifest /p/manifest.json -verify" {
		t.Fatalf("the exact run-owned probe cleanup must precede Workload removal and verify absence: %+v", steps)
	}
	var ecrCleanup *v2run.Step
	for _, s := range steps {
		if s.Name == "ecr-repositories" {
			copy := s
			ecrCleanup = &copy
		}
		layers[s.Layer] = true
		joined := strings.Join(append(append([]string{}, s.Argv...), s.Verify...), " ")
		for _, forbidden := range []string{"delete-", "terminate", " delete ", "--all", "force-unlock", "kubectl"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("step %s contains %q: %s", s.Name, forbidden, joined)
			}
		}
		if len(s.Guard) == 0 || len(s.Verify) == 0 {
			t.Fatalf("step %s needs a guard and a verification", s.Name)
		}
	}
	if ecrCleanup == nil || strings.Join(ecrCleanup.Argv, " ") != "/abs/v2ctl aws-ecr-clean -config /p/config.json -manifest /p/manifest.json" || strings.Join(ecrCleanup.Verify, " ") != "/abs/v2ctl aws-ecr-clean -config /p/config.json -manifest /p/manifest.json -verify" {
		t.Fatal("the independent supervisor must delete and verify only manifest-listed ECR repositories")
	}
	for _, want := range []string{v2run.LayerDelivery, v2run.LayerAppResources, v2run.LayerNamespace, v2run.LayerEKS, v2run.LayerVPC, v2run.LayerAux} {
		if !layers[want] {
			t.Fatalf("layer %s has no step", want)
		}
	}
	spec := v2run.Spec{RunID: c.RunID, StatusPath: "/p/s", FinishPath: "/p/f", LogDir: "/p/l", ExpiresAt: time.Now().Add(time.Hour),
		CleanupBudget: v2run.Duration{Duration: time.Hour}, Watch: []v2run.Watch{{Name: "runner", PID: 1}}, Steps: steps}
	if err := spec.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
}

type presentAWS struct{ fakeAWS }

func (f *presentAWS) Run(ctx context.Context, args ...string) ([]byte, string, int) {
	if args[0] == "ec2" && args[1] == "describe-vpcs" {
		return []byte(`{"Vpcs":[{"VpcId":"vpc-1"}]}`), "", 0
	}
	if args[0] == "eks" {
		return nil, "An error occurred (ResourceNotFoundException) when calling the DescribeCluster operation", 254
	}
	if args[0] == "rds" {
		return nil, "An error occurred (AccessDenied) when calling the DescribeDBClusters operation", 254
	}
	return f.fakeAWS.Run(ctx, args...)
}

func TestVerifyAWSLayerNeedsAVerifiedAbsenceNeverAnInference(t *testing.T) {
	c := awsConfig(t, "A02", "t4g.medium")
	m := v2run.NewManifest(c, time.Now())
	for _, a := range []v2run.Asset{
		{Kind: "vpc", ID: "vpc-1", Layer: v2run.LayerVPC, Owner: v2run.OwnerCreatedByRun},
		{Kind: "eks-cluster", ID: "run-eks", Layer: v2run.LayerEKS, Owner: v2run.OwnerCreatedByRun},
		{Kind: "rds-cluster", ID: "run-db", Layer: v2run.LayerAppResources, Owner: v2run.OwnerCreatedByRun},
	} {
		if err := m.AddAsset(a); err != nil {
			t.Fatal(err)
		}
	}
	cli := &presentAWS{fakeAWS: *newAWS()}
	ctx := context.Background()
	if err := v2run.VerifyAWSLayer(ctx, cli, c, m, v2run.LayerVPC); err == nil || !strings.Contains(err.Error(), "vpc present") {
		t.Fatalf("a present VPC must fail: %v", err)
	}
	if err := v2run.VerifyAWSLayer(ctx, cli, c, m, v2run.LayerEKS); err != nil {
		t.Fatalf("a documented not-found is a verified absence: %v", err)
	}
	if err := v2run.VerifyAWSLayer(ctx, cli, c, m, v2run.LayerAppResources); err == nil || !strings.Contains(err.Error(), "indeterminate") {
		t.Fatalf("AccessDenied is never absence: %v", err)
	}
	if err := v2run.VerifyAWSLayer(ctx, cli, c, m, v2run.LayerAux); err != nil {
		t.Fatalf("an untagged account sweep is clean: %v", err)
	}
}

func TestTimeoutsNestAndTheCleanupStepsFitTheBudget(t *testing.T) {
	c := awsConfig(t, "A03", "t4g.medium")
	c.Cleanup = v2run.Duration{Duration: 120 * time.Minute}
	tm, err := v2run.PlanTimeouts(c)
	if err != nil {
		t.Fatal(err)
	}
	if tm.TerraformInvocation != 45*time.Minute || tm.DeployLease != 180*time.Minute || tm.Execution != 180*time.Minute || tm.TerraformReadiness > tm.TerraformInvocation {
		t.Fatalf("AWS bounds %+v", tm)
	}
	k := goodConfig(t)
	ktm, err := v2run.PlanTimeouts(k)
	if err != nil || ktm.TerraformInvocation != 15*time.Minute || ktm.DeployLease > 45*time.Minute {
		t.Fatalf("k8s bounds %+v %v", ktm, err)
	}
	for _, bad := range []v2run.Timeouts{
		{TerraformReadiness: 20 * time.Minute, TerraformInvocation: 15 * time.Minute, DeployLease: time.Hour, Execution: time.Hour, CleanupBudget: time.Hour},
		{TerraformReadiness: time.Minute, TerraformInvocation: 50 * time.Minute, DeployLease: 45 * time.Minute, Execution: time.Hour, CleanupBudget: time.Hour},
		{TerraformReadiness: time.Minute, TerraformInvocation: time.Minute, DeployLease: 2 * time.Hour, Execution: time.Hour, CleanupBudget: time.Hour},
		{TerraformReadiness: time.Minute, TerraformInvocation: time.Minute, DeployLease: time.Hour, Execution: 181 * time.Minute, CleanupBudget: time.Hour},
		{TerraformReadiness: time.Minute, TerraformInvocation: time.Minute, DeployLease: time.Hour, Execution: time.Hour, CleanupBudget: 121 * time.Minute},
		{},
	} {
		if bad.Validate() == nil {
			t.Fatalf("a non-nesting or unlimited bound was accepted: %+v", bad)
		}
	}
	for _, budget := range []time.Duration{10 * time.Minute, 60 * time.Minute, 120 * time.Minute} {
		s := v2run.CleanupStepsFor(budget)
		if s.Sum() > budget || s.Terraform < budget/2 {
			t.Fatalf("budget %s: steps %+v sum %s", budget, s, s.Sum())
		}
	}
	// The AWS supervisor steps use these bounds: one attempt of each fits the budget and the destroy gets the biggest share.
	steps := v2run.AWSCleanupSteps("/abs/v2ctl", "/abs/v2cleanup", "/p/m", "/p/h", "/p/c", "/p/s", c)
	var total, terraform time.Duration
	for _, s := range steps {
		total += s.Timeout.Duration
		if s.Name == "app-resources" {
			terraform = s.Timeout.Duration
		}
	}
	if total > c.Cleanup.Duration || terraform < c.Cleanup.Duration/2 {
		t.Fatalf("AWS step bounds total %s terraform %s of %s", total, terraform, c.Cleanup.Duration)
	}
}
