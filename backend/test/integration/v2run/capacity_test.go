package v2run_test

import (
	"testing"

	"orchestrator/test/integration/v2run"
)

func TestPodBudgetsMatchPublicEC2Limits(t *testing.T) {
	if got := v2run.KnownInstances["t4g.small"].PodBudget(); got != 11 {
		t.Fatalf("t4g.small budget %d", got)
	}
	if got := v2run.KnownInstances["t4g.medium"].PodBudget(); got != 17 {
		t.Fatalf("t4g.medium budget %d", got)
	}
}

// The EKS module's add-ons plus the workloads exceed the t4g.small pod budget
// for A03 (Redis in cluster); t4g.medium fits. The gate fails BEFORE create.
func TestSmallNodeFailsAndMediumPassesWithTheActualAddons(t *testing.T) {
	small, err := v2run.CheckAWSScenario("A03", "t4g.small")
	if err != nil {
		t.Fatal(err)
	}
	if small.OK() || small.PodsRequired <= small.PodBudget {
		t.Fatalf("t4g.small must fail the pod budget: %+v", small)
	}
	medium, _ := v2run.CheckAWSScenario("A03", "t4g.medium")
	if !medium.OK() {
		t.Fatalf("t4g.medium must fit: %+v", medium)
	}
	if medium.PodsRequired != 4+6 || medium.PodBudget != 15 {
		t.Fatalf("pod accounting %+v", medium)
	}
	if _, err := v2run.CheckAWSScenario("A01", "t4g.medium"); err == nil {
		t.Fatal("the gate applies to AWS scenarios only")
	}
	if _, err := v2run.CheckAWSScenario("A03", "m5.large"); err == nil {
		t.Fatal("an unknown instance type was accepted")
	}
}

func TestA02FitsSmallOnPodsButCPUAndMemoryAreChecked(t *testing.T) {
	res, _ := v2run.CheckAWSScenario("A02", "t4g.small")
	t.Logf("A02 small: %+v", res)
	// 6 add-on pods + 3 workloads = 9 <= 9 schedulable (11-2): fits on pods.
	if res.PodsRequired != 9 || res.PodBudget != 9 {
		t.Fatalf("%+v", res)
	}
	over := v2run.CheckCapacity(v2run.KnownInstances["t4g.small"], 1, []v2run.Pod{{Name: "hog", Replicas: 1, Counted: true, MemoryMi: 5000}})
	if over.OK() {
		t.Fatal("memory over-request passed")
	}
}
