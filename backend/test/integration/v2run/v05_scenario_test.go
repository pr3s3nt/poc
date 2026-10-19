package v2run_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"orchestrator/test/integration/v2run"
)

// V05: EKS created in the same execution with PostgreSQL and Redis running in the cluster. The AWS runner
// machinery (capacity gate, planned set, estimate, ECR images) must know the scenario and must not price or
// plan managed data services it does not create.
func TestV05IsAnAWSScenarioWithInClusterDataServicesOnly(t *testing.T) {
	if !v2run.IsAWSScenario("V05") || !v2run.IsAWSScenario("A02") || !v2run.IsAWSScenario("A03") || v2run.IsAWSScenario("A01") || v2run.IsAWSScenario("E02") {
		t.Fatal("IsAWSScenario must name exactly V05, A02 and A03")
	}
	planned, err := v2run.PlannedFromModules("V05")
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, p := range planned {
		count[p.Kind] = p.Count
	}
	for _, want := range []string{"aws_vpc", "aws_eks_cluster", "aws_eks_node_group", "kubernetes_stateful_set_v1", "kubernetes_namespace_v1"} {
		if count[want] == 0 {
			t.Errorf("V05 plans %s: %v", want, count)
		}
	}
	for _, unwanted := range []string{"aws_rds_cluster", "aws_elasticache_replication_group"} {
		if count[unwanted] != 0 {
			t.Errorf("V05 creates no managed data service, but plans %s", unwanted)
		}
	}
	kinds := v2run.RequiredCostKinds("V05")
	for _, unwanted := range []string{v2run.CostAuroraACU, v2run.CostAuroraStorage, v2run.CostElastiCache} {
		if contains(kinds, unwanted) {
			t.Errorf("V05 must not be priced for %s", unwanted)
		}
	}
	for _, wanted := range []string{v2run.CostEKSCluster, v2run.CostEC2Node, v2run.CostEBSVolume, v2run.CostPublicIPv4, v2run.CostECRStorage} {
		if !contains(kinds, wanted) {
			t.Errorf("V05 must be priced for %s", wanted)
		}
	}
}

func TestV05CapacityCountsTheInClusterPostgresAndRedisPods(t *testing.T) {
	v05, err := v2run.CheckAWSScenario("V05", "t4g.medium")
	if err != nil || !v05.OK() {
		t.Fatalf("t4g.medium must fit V05: %+v %v", v05, err)
	}
	a03, _ := v2run.CheckAWSScenario("A03", "t4g.medium")
	if v05.PodsRequired != a03.PodsRequired+1 || v05.CPURequested <= a03.CPURequested {
		t.Fatalf("V05 adds the PostgreSQL pod to A03's pods: %d vs %d", v05.PodsRequired, a03.PodsRequired)
	}
}

func TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires(t *testing.T) {
	raw, err := os.ReadFile("../../../../frontend/test/e2e/terraform-v2-aws-definitions.mjs")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	block := regexp.MustCompile(`(?s)COMMON_ASSERTIONS = \[(.*?)\];`).FindStringSubmatch(script)
	if block == nil {
		t.Fatal("COMMON_ASSERTIONS not found in the scenario script")
	}
	var js []string
	for _, m := range regexp.MustCompile(`'([a-z0-9.-]+)'`).FindAllStringSubmatch(block[1], -1) {
		js = append(js, m[1])
	}
	sort.Strings(js)
	got := v2run.RequiredAssertions("V05")
	if strings.Join(js, ",") != strings.Join(got, ",") {
		t.Fatalf("the script's COMMON_ASSERTIONS differ from the runner's V05 list:\nscript %v\nrunner %v", js, got)
	}
	a02 := v2run.RequiredAssertions("A02")
	if len(a02) != len(got)+1 {
		t.Fatalf("A02 adds exactly the technical Pod assertion: %d vs %d", len(a02), len(got))
	}
	// Every ID the script lists is really recorded by the script or its shared flows (or by the shell runner):
	// no required ID can be a typo. The list itself is excluded, otherwise the check would be vacuous.
	flows, err := os.ReadFile("../../../../frontend/test/e2e/full-app-flows.mjs")
	if err != nil {
		t.Fatal(err)
	}
	recorded := regexp.MustCompile(`(?s)COMMON_ASSERTIONS = \[.*?\];`).ReplaceAllString(script, "") + string(flows)
	for _, id := range js {
		if strings.HasPrefix(id, "aws.") && id != "aws.preexisting-preserved" || id == "evidence.no-secret-public" || id == "aws.preexisting-preserved" {
			continue // recorded by the shell runner
		}
		if !strings.Contains(recorded, "'"+id+"'") {
			t.Errorf("the script never records %s", id)
		}
	}
}
