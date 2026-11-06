package prometheus

import (
	"encoding/json"
	"orchestrator/internal/domain/monitoring"
	"strings"
	"testing"
)

func ownedTarget() monitoring.Target {
	return monitoring.Target{Scope: monitoring.Scope{Organization: "org", Application: "app", Environment: "staging", Generation: 2, ClusterUID: "cluster-uid", Namespace: "owned-staging"}, Workload: "backend", Kind: "Deployment", Name: "backend", UID: "controller-uid"}
}
func TestWorkloadQueryAndRuleOwnership(t *testing.T) {
	target := ownedTarget()
	q, e := WorkloadQueries(target)
	if e != nil {
		t.Fatal(e)
	}
	for key, expr := range q {
		if !strings.Contains(expr, `namespace="owned-staging"`) {
			t.Errorf("unscoped %s", key)
		}
	}
	if !strings.Contains(q["restarts"], "increase(") || !strings.Contains(q["unavailable"], "> 0") || !strings.Contains(q["unavailable"], "observed_generation") {
		t.Fatal("rule semantics lost")
	}
	doc, e := RuleDocument(target, monitoring.DevRules())
	if e != nil {
		t.Fatal(e)
	}
	first, _ := json.Marshal(doc)
	again, _ := RuleDocument(target, monitoring.DevRules())
	second, _ := json.Marshal(again)
	if string(first) != string(second) {
		t.Fatal("non-deterministic rule")
	}
	target.Scope.Environment = "production"
	if monitoring.RuleName(target) == monitoring.RuleName(ownedTarget()) {
		t.Fatal("cross-environment rule collision")
	}
	target = ownedTarget()
	target.Scope.Generation++
	if monitoring.RuleName(target) == monitoring.RuleName(ownedTarget()) {
		t.Fatal("generation collision")
	}
	target.Name = `x"} or up`
	if _, e := WorkloadQueries(target); e == nil {
		t.Fatal("unsafe name accepted")
	}
}
