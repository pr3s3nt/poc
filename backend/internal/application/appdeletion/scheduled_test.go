package appdeletion

import (
	"orchestrator/internal/domain/environment"
	"testing"
)

func TestApplicationAdmissionRequiresAppliedScheduleSuspension(t *testing.T) {
	doc := environment.NewDocument()
	doc.Modules["backup"] = environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Kind: "CronJob", Schedule: &environment.Schedule{Suspend: false}}}}
	doc.Modules["backend"] = environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Kind: "Deployment"}}}
	blockers := scheduledDeletionBlockers(doc)
	if len(blockers) != 1 || blockers[0].Code != BlockerScheduleNotSuspended || blockers[0].Descriptor != "backup" {
		t.Fatalf("unsuspended applied schedule admitted: %+v", blockers)
	}
	// Editing a detached pending clone does not change the applied precondition.
	pending := environment.NewDocument()
	module := doc.Modules["backup"]
	module.Spec.Execution = module.Spec.Execution.Clone()
	pending.Modules["backup"] = module
	pending.Modules["backup"].Spec.Execution.Schedule.Suspend = true
	if len(scheduledDeletionBlockers(doc)) != 1 {
		t.Fatal("pending draft changed applied safety")
	}
	if len(scheduledDeletionBlockers(pending)) != 0 {
		t.Fatal("suspended applied schedule blocked")
	}
}
