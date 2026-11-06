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

func TestTypedPodMonitorAndApplicationQueryIdentity(t *testing.T) {
	target := ownedTarget()
	metric := monitoring.MetricDefinition{Key: "requests", Metric: "http_requests_total", Aggregation: "rate", Unit: "per_second", Labels: map[string]string{"status": "500"}}
	profile := monitoring.WorkloadProfile{Port: "metrics", Path: "/metrics", IntervalSeconds: 30, SampleLimit: 500, Metrics: []monitoring.MetricDefinition{metric}}
	doc, e := PodMonitorDocument(target, profile, map[string]string{"release": "metrics"})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(doc)
	for _, want := range []string{`"honorLabels":false`, `orchestrator_cluster_uid`, `orchestrator_generation`, `"sampleLimit":500`} {
		if !strings.Contains(string(raw), want) {
			t.Fatal("missing monitor fence", want)
		}
	}
	query := MetricQuery(target, metric)
	for _, want := range []string{`orchestrator_app="app"`, `orchestrator_generation="2"`, `orchestrator_cluster_uid="cluster-uid"`, `rate(http_requests_total`, `[5m]`} {
		if !strings.Contains(query, want) {
			t.Fatal("missing application selector", want)
		}
	}
	queries, e := WorkloadQueries(target)
	if e != nil {
		t.Fatal(e)
	}
	for k, q := range queries {
		if !strings.Contains(q, `cluster_uid="cluster-uid"`) {
			t.Fatal("cross-cluster Kubernetes query", k)
		}
	}
}
