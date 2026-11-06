package monitoring

import "testing"

func TestTypedProfileRejectsQueriesAndUnboundedCollection(t *testing.T) {
	p := WorkloadProfile{Port: "metrics", Path: "/metrics", IntervalSeconds: 30, SampleLimit: 500, Metrics: []MetricDefinition{{Key: "requests", Metric: "http_requests_total", Aggregation: "rate", Unit: "per_second"}}}
	if p.Validate() != nil {
		t.Fatal("valid typed profile rejected")
	}
	p.Metrics[0].Metric = `up} or {__name__=~".*"`
	if p.Validate() == nil {
		t.Fatal("accepted query injection")
	}
	p.Metrics[0].Metric = "http_requests_total"
	p.Metrics[0].Labels = map[string]string{"orchestrator_app": "foreign"}
	if p.Validate() == nil {
		t.Fatal("accepted identity override")
	}
	p.Metrics[0].Labels = nil
	p.SampleLimit = 1000000
	if p.Validate() == nil {
		t.Fatal("unbounded cardinality")
	}
}

func TestRatioRequiresActualDenominatorAndBoundedMatchers(t *testing.T) {
	p := WorkloadProfile{Port: "metrics", Path: "/metrics", IntervalSeconds: 30, SampleLimit: 500, Metrics: []MetricDefinition{{Key: "error_ratio", Metric: "http_requests_total", Aggregation: "rate_ratio", DenominatorMetric: "http_requests_total", MinimumDenominator: 0.1, Matchers: map[string]string{"code": "5.."}, Unit: "ratio"}}}
	if p.Validate() != nil {
		t.Fatal("typed error ratio rejected")
	}
	p.Metrics[0].MinimumDenominator = 0
	if p.Validate() == nil {
		t.Fatal("no-traffic ratio accepted")
	}
	p.Metrics[0].MinimumDenominator = 0.1
	p.Metrics[0].Matchers = map[string]string{"namespace": ".*"}
	if p.Validate() == nil {
		t.Fatal("ratio escaped trusted identity")
	}
}
