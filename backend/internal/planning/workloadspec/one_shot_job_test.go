package workloadspec

import (
	"encoding/json"
	"testing"
)

func TestOneShotJobIntentRoundTrip(t *testing.T) {
	raw := []byte(`{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"migration"},"containers":{"main":{"image":"migrate:1","command":["/bin/migrate"]}},"workloadExecution":{"version":1,"kind":"Job","job":{"backoffLimit":0,"activeDeadlineSeconds":300},"waitForJobs":["prepare"]}}`)
	doc, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if again.Execution.Job == nil || again.Execution.Job.BackoffLimit == nil || *again.Execution.Job.BackoffLimit != 0 || again.Execution.WaitForJobs[0] != "prepare" {
		t.Fatal(string(encoded))
	}
	for _, raw := range []string{
		`{"version":1,"kind":"Job","job":null}`,
		`{"version":1,"kind":"Job","waitForJobs":null}`,
		`{"version":1,"kind":"Job","waitForJobs":["migration"]}`,
	} {
		doc := map[string]any{}
		_ = json.Unmarshal([]byte(`{"apiVersion":"orchestrator.dev/workload/v1","metadata":{"name":"migration"},"containers":{"main":{"image":"migrate:1"}}}`), &doc)
		var execution any
		_ = json.Unmarshal([]byte(raw), &execution)
		doc["workloadExecution"] = execution
		b, _ := json.Marshal(doc)
		if _, err := Parse(b); err == nil {
			t.Fatal("invalid intent accepted", raw)
		}
	}
}
