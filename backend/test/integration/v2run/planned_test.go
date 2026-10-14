package v2run_test

import (
	"testing"

	"orchestrator/test/integration/v2run"
)

func TestPlannedResourcesComeFromTheShippedModulesPerScenario(t *testing.T) {
	a02, err := v2run.PlannedFromModules("A02")
	if err != nil {
		t.Fatal(err)
	}
	a03, err := v2run.PlannedFromModules("A03")
	if err != nil {
		t.Fatal(err)
	}
	get := func(list []v2run.Planned, kind string) int {
		for _, p := range list {
			if p.Kind == kind {
				return p.Count
			}
		}
		return 0
	}
	for _, kind := range []string{"aws_vpc", "aws_eks_cluster", "aws_eks_node_group", "aws_rds_cluster"} {
		if get(a02, kind) != 1 || get(a03, kind) != 1 {
			t.Fatalf("%s must be planned once in both scenarios", kind)
		}
	}
	if get(a02, "aws_elasticache_replication_group") != 1 || get(a03, "aws_elasticache_replication_group") != 0 {
		t.Fatal("ElastiCache belongs to A02 only")
	}
	if get(a03, "kubernetes_stateful_set_v1") == 0 || get(a02, "kubernetes_stateful_set_v1") != 0 {
		t.Fatal("the in-cluster Redis StatefulSet belongs to A03 only")
	}
	if get(a02, "aws_subnet") < 2 {
		t.Fatalf("per-AZ subnets must be enumerated: %d", get(a02, "aws_subnet"))
	}
	if _, err := v2run.PlannedFromModules("A01"); err == nil {
		t.Fatal("A01 has no AWS plan")
	}
	// The estimate skeleton lists exactly the plan and every required cost kind, with no price or ceiling.
	c := awsConfig(t, "A02", "t4g.medium")
	e, err := v2run.EstimateTemplate(c)
	if err != nil || len(e.Resources) != len(a02) || len(e.Lines) != len(v2run.RequiredCostKinds("A02")) {
		t.Fatalf("template %+v %v", e, err)
	}
	for _, l := range e.Lines {
		if l.PriceUSD != 0 || l.Source != "" {
			t.Fatal("the skeleton must not invent a price or source")
		}
	}
}
