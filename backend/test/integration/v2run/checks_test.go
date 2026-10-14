package v2run_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

func goodEstimate(c v2run.Config, m v2run.Manifest, now time.Time) v2run.Estimate {
	e := v2run.Estimate{RunID: c.RunID, Scenario: c.Scenario, Region: v2run.AWSRegion, Hours: (c.Execution.Duration + c.Cleanup.Duration).Hours()}
	for _, kind := range v2run.RequiredCostKinds(c.Scenario) {
		e.Lines = append(e.Lines, v2run.EstimateLine{Kind: kind, Unit: "unit", PriceUSD: 0.1, Quantity: 10, SubtotalUSD: 1, Source: "https://aws.amazon.com/eks/pricing/", RetrievedAt: now.Add(-time.Hour)})
		e.TotalUSD++
	}
	for _, p := range m.Planned {
		e.Resources = append(e.Resources, p)
	}
	return e
}

func TestEstimateMustBeCompletePublicAndHaveNoInventedCeiling(t *testing.T) {
	c := awsConfig(t, "A03", "t4g.medium")
	now := time.Now()
	m := v2run.NewManifest(c, now)
	_ = m.AddPlanned("aws_eks_cluster", 1)
	if err := goodEstimate(c, m, now).Check(c, m, now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*v2run.Estimate){
		"missing EKS line":       func(e *v2run.Estimate) { e.Lines = e.Lines[1:]; e.TotalUSD-- },
		"hours too short":        func(e *v2run.Estimate) { e.Hours = 1 },
		"non AWS source":         func(e *v2run.Estimate) { e.Lines[0].Source = "https://example.com/price" },
		"http source":            func(e *v2run.Estimate) { e.Lines[0].Source = "http://aws.amazon.com/x" },
		"stale price":            func(e *v2run.Estimate) { e.Lines[0].RetrievedAt = now.Add(-30 * 24 * time.Hour) },
		"subtotal mismatch":      func(e *v2run.Estimate) { e.Lines[0].SubtotalUSD = 99 },
		"total mismatch":         func(e *v2run.Estimate) { e.TotalUSD = 99 },
		"zero price without why": func(e *v2run.Estimate) { e.Lines[0].PriceUSD, e.Lines[0].SubtotalUSD = 0, 0; e.TotalUSD-- },
		"invented NAT": func(e *v2run.Estimate) {
			e.Lines = append(e.Lines, v2run.EstimateLine{Kind: "nat-gateway-hour", Unit: "h", PriceUSD: 0.045, Quantity: 3, SubtotalUSD: 0.135, Source: "https://aws.amazon.com/vpc/pricing/", RetrievedAt: now})
			e.TotalUSD += 0.135
		},
		"resource count mismatch": func(e *v2run.Estimate) { e.Resources[0].Count = 7 },
		"other run":               func(e *v2run.Estimate) { e.RunID = "other" },
	} {
		e := goodEstimate(c, m, now)
		e.Lines = append([]v2run.EstimateLine(nil), e.Lines...)
		e.Resources = append([]v2run.Planned(nil), e.Resources...)
		mutate(&e)
		if err := e.Check(c, m, now); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	raw, _ := json.Marshal(map[string]any{"runId": "x", "ceilingUsd": 50})
	if _, err := v2run.ParseEstimate(raw); err == nil {
		t.Fatal("an invented USD ceiling field was accepted")
	}
}

func TestA02RequiresTheElastiCacheLineA03DoesNot(t *testing.T) {
	if !contains(v2run.RequiredCostKinds("A02"), v2run.CostElastiCache) || contains(v2run.RequiredCostKinds("A03"), v2run.CostElastiCache) {
		t.Fatal("ElastiCache pricing applies to A02 only")
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func TestInventoryOnlyAbsentWhenTheServiceSaysSo(t *testing.T) {
	q := v2run.AWSQueries("run-1", v2run.AWSLinkage{VPCID: "vpc-1", ClusterName: "c1", DBCluster: "db1", CacheGroup: "rg1", Repositories: []string{"repo"}, Roles: []string{"role1"}})
	byID := map[string]v2run.Query{}
	for _, x := range q {
		byID[x.ID] = x
	}
	for _, want := range []string{"tagged-resources", "eks-cluster", "ec2-instances-cluster", "ebs-volumes-cluster", "vpc", "subnets", "security-groups", "route-tables",
		"internet-gateways", "nat-gateways", "network-interfaces", "rds-cluster", "elasticache-group", "ecr-repo", "iam-role-role1", "elastic-ips-run", "ec2-instances-run"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("missing query %s", want)
		}
	}
	for _, x := range q {
		for _, a := range x.Argv {
			if strings.Contains(a, "*") || strings.Contains(a, "begins_with") {
				t.Errorf("%s uses a wildcard/prefix match: %v", x.ID, x.Argv)
			}
		}
	}
	empty := byID["subnets"].Evaluate([]byte(`{"Subnets":[]}`), "", 0)
	if !empty.Absent() {
		t.Fatalf("%+v", empty)
	}
	still := byID["subnets"].Evaluate([]byte(`{"Subnets":[{"SubnetId":"subnet-1"}]}`), "", 0)
	if still.Absent() || still.Present[0] != "subnet-1" {
		t.Fatalf("%+v", still)
	}
	if !byID["vpc"].Evaluate(nil, "An error occurred (InvalidVpcID.NotFound)", 254).Absent() {
		t.Fatal("not-found must be absent")
	}
	denied := byID["vpc"].Evaluate(nil, "An error occurred (UnauthorizedOperation)", 254)
	if denied.Absent() || denied.Indeterminate == "" {
		t.Fatal("AccessDenied must never count as absence")
	}
	if byID["vpc"].Evaluate([]byte("not json"), "", 0).Absent() {
		t.Fatal("an unparsable answer must not count as absence")
	}
	// Terminated instances and deleted NAT gateways no longer count; running ones do.
	inst := byID["ec2-instances-vpc"]
	if !inst.Evaluate([]byte(`{"Reservations":[{"Instances":[{"InstanceId":"i-1","State":{"Name":"terminated"}}]}]}`), "", 0).Absent() {
		t.Fatal("terminated instance counted")
	}
	if inst.Evaluate([]byte(`{"Reservations":[{"Instances":[{"InstanceId":"i-2","State":{"Name":"running"}}]}]}`), "", 0).Absent() {
		t.Fatal("running instance missed")
	}
	if !byID["nat-gateways"].Evaluate([]byte(`{"NatGateways":[{"NatGatewayId":"nat-1","State":"deleted"}]}`), "", 0).Absent() {
		t.Fatal("deleted NAT counted")
	}
	// The tagged-resources sweep catches anything the run created, and a hit stays reportable.
	tagged := byID["tagged-resources"].Evaluate([]byte(`{"ResourceTagMappingList":[{"ResourceARN":"arn:aws:eks:us-east-1:452025861381:cluster/c1"}]}`), "", 0)
	if tagged.Absent() || tagged.Present[0] != "arn:aws:eks:us-east-1:452025861381:cluster/c1" {
		t.Fatalf("%+v", tagged)
	}
}

func TestK8sInventoryAndPVClaims(t *testing.T) {
	c := goodConfig(t)
	m := v2run.NewManifest(c, time.Now())
	_ = m.RecordNamespace("orch-v2-test-a01-0001", 0, "orch-v2-test-a01-0001")
	_ = m.AddAsset(v2run.Asset{Kind: "pv", ID: "pvc-uid-1", Layer: v2run.LayerAppResources, Owner: v2run.OwnerCreatedByRun})
	_ = m.AddAsset(v2run.Asset{Kind: "pv", ID: "someone-elses", Layer: v2run.LayerAppResources, Owner: v2run.OwnerPreexisting})
	qs := v2run.K8sQueries(m, "/k/kubectl", "/p/kc", v2run.KubeContextK8s4f)
	if len(qs) != 2 || qs[0].Argv[len(qs[0].Argv)-4] != "namespace" || !strings.Contains(strings.Join(qs[1].Argv, " "), "get pv pvc-uid-1") {
		t.Fatalf("%v", qs)
	}
	for _, q := range qs {
		if strings.Contains(strings.Join(q.Argv, " "), "someone-elses") {
			t.Fatal("a preexisting PV is queried as owned")
		}
	}
	if !qs[0].Evaluate(nil, `Error from server (NotFound): namespaces "x" not found`, 1).Absent() {
		t.Fatal("namespace not found must be absent")
	}
	if qs[0].Evaluate([]byte("namespace/orch-v2-test-a01-0001\n"), "", 0).Absent() {
		t.Fatal("existing namespace reported absent")
	}
	pvs, err := v2run.PVsClaimedBy("orch-v2-test-a01-0001", []byte(`{"items":[{"metadata":{"name":"a"},"spec":{"claimRef":{"namespace":"orch-v2-test-a01-0001"}}},{"metadata":{"name":"b"},"spec":{"claimRef":{"namespace":"prod"}}},{"metadata":{"name":"c"},"spec":{}}]}`))
	if err != nil || len(pvs) != 1 || pvs[0] != "a" {
		t.Fatalf("%v %v", pvs, err)
	}
}

func TestMarksAndAssertionsMustCoverTheScenarioWithLiveProof(t *testing.T) {
	var marks []v2run.Mark
	for i, p := range v2run.Phases {
		marks = append(marks, v2run.Mark{Phase: p, AtSec: float64(i * 10), Caption: "Giai đoạn " + p})
	}
	if err := v2run.ValidateMarks("A01", marks); err != nil {
		t.Fatal(err)
	}
	if err := v2run.ValidateMarks("E01", []v2run.Mark{marks[0], marks[1], marks[2], marks[7]}); err != nil {
		t.Fatal(err)
	}
	if err := v2run.ValidateMarks("A01", marks[:7]); err == nil {
		t.Fatal("A01 without CLEANUP accepted")
	}
	swapped := append([]v2run.Mark(nil), marks...)
	swapped[2], swapped[3] = swapped[3], swapped[2]
	if err := v2run.ValidateMarks("A01", swapped); err == nil {
		t.Fatal("out-of-order phases accepted")
	}
	uncaptioned := append([]v2run.Mark(nil), marks...)
	uncaptioned[0].Caption = " "
	if err := v2run.ValidateMarks("A01", uncaptioned); err == nil {
		t.Fatal("a mark without a Vietnamese caption was accepted")
	}

	for _, scenario := range []string{"A01", "E01", "E02", "A02", "A03"} {
		var as []v2run.Assertion
		for _, id := range v2run.RequiredAssertions(scenario) {
			as = append(as, v2run.Assertion{ID: id, Phase: "READY", Pass: true, Source: "ui"})
		}
		if err := v2run.ValidateAssertions(scenario, as); err != nil {
			t.Fatalf("%s: %v", scenario, err)
		}
		fake := append([]v2run.Assertion(nil), as...)
		fake[0].Source = "fake"
		if err := v2run.ValidateAssertions(scenario, fake); err == nil {
			t.Errorf("%s: a fake-sourced assertion counted", scenario)
		}
		if err := v2run.ValidateAssertions(scenario, as[1:]); err == nil {
			t.Errorf("%s: a dropped assertion was accepted", scenario)
		}
		failed := append([]v2run.Assertion(nil), as...)
		failed[1].Pass = false
		if err := v2run.ValidateAssertions(scenario, failed); err == nil {
			t.Errorf("%s: a failed assertion was accepted", scenario)
		}
	}
	// The deletion assertions of A01 cannot be dropped for a simpler graph.
	for _, id := range []string{"delete.dependency-selected", "delete.unchecked-candidate-left", "delete.platform-excluded", "delete.root-only-checked"} {
		if !contains(v2run.RequiredAssertions("A01"), id) {
			t.Errorf("A01 lost %s", id)
		}
	}
}
