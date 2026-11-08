package environment

import "testing"

func TestOneShotJobValidation(t *testing.T) {
	containers := map[string]Container{"main": {Image: "migration:1"}}
	valid := func() *WorkloadExecution {
		return &WorkloadExecution{Version: 1, Kind: "Job", WaitForJobs: []string{"previous"}}
	}
	if err := valid().Validate(containers, 1, false, nil); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*WorkloadExecution){
		"schedule":             func(e *WorkloadExecution) { e.Schedule = &Schedule{} },
		"rollout":              func(e *WorkloadExecution) { e.UpdateStrategy = &UpdateStrategy{Type: "Recreate"} },
		"duplicate dependency": func(e *WorkloadExecution) { e.WaitForJobs = []string{"prepare", "prepare"} },
		"invalid dependency":   func(e *WorkloadExecution) { e.WaitForJobs = []string{"bad/name"} },
		"negative backoff":     func(e *WorkloadExecution) { n := int32(-1); e.Job = &JobPolicy{BackoffLimit: &n} },
		"zero deadline":        func(e *WorkloadExecution) { n := int64(0); e.Job = &JobPolicy{ActiveDeadlineSeconds: &n} },
	} {
		t.Run(name, func(t *testing.T) {
			e := valid()
			change(e)
			if e.Validate(containers, 1, false, nil) == nil {
				t.Fatal("invalid Job accepted")
			}
		})
	}
	if valid().Validate(containers, 1, true, nil) == nil || valid().Validate(containers, 2, false, nil) == nil || valid().Validate(containers, 0, false, nil) == nil {
		t.Fatal("Job accepts Service or replicas")
	}
	n := int32(0)
	e := valid()
	e.Job = &JobPolicy{BackoffLimit: &n}
	if err := e.Validate(containers, 1, false, nil); err != nil {
		t.Fatal(err)
	}
	e.Kind = "Deployment"
	if e.Validate(containers, 1, false, nil) == nil {
		t.Fatal("Job policy accepted on Deployment")
	}
}
