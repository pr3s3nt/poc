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
