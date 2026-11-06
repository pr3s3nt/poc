package prometheus

import (
	"fmt"
	"orchestrator/internal/domain/monitoring"
	"sort"
	"strconv"
	"strings"
)

func IdentityLabels(t monitoring.Target) map[string]string {
	return map[string]string{"orchestrator_org": t.Scope.Organization, "orchestrator_app": t.Scope.Application, "orchestrator_env": t.Scope.Environment, "orchestrator_generation": strconv.FormatInt(t.Scope.Generation, 10), "orchestrator_cluster_uid": t.Scope.ClusterUID, "orchestrator_workload": t.Workload}
}
func MetricQuery(t monitoring.Target, m monitoring.MetricDefinition) string {
	labels := IdentityLabels(t)
	for k, v := range m.Labels {
		labels[k] = v
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+strconv.Quote(labels[k]))
	}
	matcherKeys := []string{}
	for k := range m.Matchers {
		matcherKeys = append(matcherKeys, k)
	}
	sort.Strings(matcherKeys)
	for _, k := range matcherKeys {
		pairs = append(pairs, k+"=~"+strconv.Quote(m.Matchers[k]))
	}
	selector := m.Metric + "{" + strings.Join(pairs, ",") + "}"
	switch m.Aggregation {
	case "rate_ratio", "gauge_ratio":
		aggregation := "rate"
		if m.Aggregation == "gauge_ratio" {
			aggregation = "gauge"
		}
		numerator := m
		numerator.Aggregation = aggregation
		numerator.DenominatorMetric = ""
		numerator.DenominatorLabels = nil
		numerator.MinimumDenominator = 0
		denominator := numerator
		denominator.Metric = m.DenominatorMetric
		denominator.Labels = m.DenominatorLabels
		denominator.Matchers = nil
		n, d := MetricQuery(t, numerator), MetricQuery(t, denominator)
		return "((" + n + " / " + d + ") and on() (" + d + " >= " + strconv.FormatFloat(m.MinimumDenominator, 'g', -1, 64) + "))"
	case "rate":
		return "sum(rate(" + selector + "[5m]))"
	case "p95":
		return "histogram_quantile(0.95, sum by(le) (rate(" + selector + "[5m])))"
	default:
		return "sum(" + selector + ")"
	}
}
func PodMonitorDocument(t monitoring.Target, p monitoring.WorkloadProfile, labels map[string]string) (map[string]any, error) {
	if t.ValidateDesired() != nil || p.Validate() != nil {
		return nil, monitoring.ErrInvalid
	}
	relabels := []any{}
	identity := IdentityLabels(t)
	keys := []string{}
	for k := range identity {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		relabels = append(relabels, map[string]any{"action": "replace", "targetLabel": k, "replacement": identity[k]})
	}
	return map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "PodMonitor", "metadata": map[string]any{"name": monitoring.RuleName(t) + "-scrape", "namespace": t.Scope.Namespace, "labels": labels}, "spec": map[string]any{"sampleLimit": p.SampleLimit, "labelLimit": 40, "labelNameLengthLimit": 128, "labelValueLengthLimit": 256, "selector": map[string]any{"matchLabels": map[string]string{"app.kubernetes.io/name": t.Workload, "orchestrator.io/application": t.Scope.Application, "orchestrator.io/environment": t.Scope.Environment, "orchestrator.io/generation": strconv.FormatInt(t.Scope.Generation, 10)}}, "podMetricsEndpoints": []any{map[string]any{"port": p.Port, "path": p.Path, "interval": fmt.Sprintf("%ds", p.IntervalSeconds), "scrapeTimeout": "10s", "honorLabels": false, "relabelings": relabels}}}}, nil
}
func AddMetricRules(doc map[string]any, t monitoring.Target, p monitoring.WorkloadProfile, version string) {
	group := doc["spec"].(map[string]any)["groups"].([]any)[0].(map[string]any)
	rules := group["rules"].([]any)
	for _, m := range p.Metrics {
		if m.Alert == nil {
			continue
		}
		labels := IdentityLabels(t)
		labels["severity"] = m.Alert.Severity
		labels["metric_key"] = m.Key
		comparison := " > "
		if m.Alert.Comparison == "below" {
			comparison = " < "
		}
		rules = append(rules, map[string]any{"alert": "OrchestratorMetricThreshold", "expr": MetricQuery(t, m) + comparison + strconv.FormatFloat(m.Alert.Above, 'g', -1, 64), "for": fmt.Sprintf("%ds", m.Alert.ForSeconds), "labels": labels, "annotations": map[string]string{"summary": "Configured metric threshold exceeded: " + m.Key, "configuration_source": version}})
	}
	group["rules"] = rules
}
