package kubernetes

import "testing"

func TestCronJobRemovalUsesControllerUIDAndTerminalCondition(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		blocked    bool
	}{
		{"pending owned", `{"items":[{"metadata":{"ownerReferences":[{"uid":"cron-uid","controller":true}]},"status":{}}]}`, true},
		{"complete owned", `{"items":[{"metadata":{"ownerReferences":[{"uid":"cron-uid","controller":true}]},"status":{"conditions":[{"type":"Complete","status":"True"}]}}]}`, false},
		{"unrelated active", `{"items":[{"metadata":{"ownerReferences":[{"uid":"other","controller":true}]},"status":{}}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ensureCronJobIdle([]byte(tc.body), "cron-uid"); (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v error=%v", tc.blocked, err)
			}
		})
	}
}
