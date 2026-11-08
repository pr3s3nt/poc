package workloadconfig

import (
	"context"
	"orchestrator/internal/adapters/store"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"testing"
)

type retainedJobStore struct {
	*store.Store
	retained []persistence.DeliveryExecution
}

func (s *retainedJobStore) ListDeliveryExecutions(context.Context, string, string) ([]persistence.DeliveryExecution, error) {
	return s.retained, nil
}
func TestDeletePublishedFailedDraftRequiresDeployment(t *testing.T) {
	ctx := context.Background()
	_, st := testWorkloadService(t)
	proof := persistence.DeliveryExecution{WorkloadID: "job", UnitID: "unit", Revision: domain.Revision{ID: "revision", Phase: domain.Job, PayloadDigest: "digest"}, Mutation: domain.Mutation{ID: "intent", Action: domain.Submit, Status: domain.IntentFailed}, Receipt: port.Receipt{IntentID: "intent", RevisionID: "revision", PayloadDigest: "digest", Handle: map[string]string{port.TerminalJobFailureKey: "v1"}}, Inventory: []port.AppliedObject{{ObjectRef: execution.ObjectRef{APIVersion: "batch/v1", Kind: "Job", Name: "job"}, UID: "original"}}}
	wrapped := &retainedJobStore{Store: st, retained: []persistence.DeliveryExecution{proof}}
	svc := NewService(wrapped)
	raw := testWorkload("job", nil, nil)
	env, _ := st.GetEnvironment(ctx, "app", "staging")
	if err := st.SaveWorkloadDraft(ctx, env.DraftVersion, environment.WorkloadDraft{ApplicationKey: "app", EnvironmentKey: "staging", WorkloadID: "job", State: environment.DraftUpsert, Workload: raw}); err != nil {
		t.Fatal(err)
	}
	env, _ = st.GetEnvironment(ctx, "app", "staging")
	view, err := svc.Delete(ctx, "app", "staging", "job", env.DraftVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Workloads) != 1 || view.Workloads[0].State != environment.DraftDelete {
		t.Fatal("published failed draft was discarded")
	}
	wrapped.retained = nil
	if _, err = svc.Undo(ctx, "app", "staging", "job", view.DraftVersion); err != nil {
		t.Fatal(err)
	}
	env, _ = st.GetEnvironment(ctx, "app", "staging")
	view, err = svc.Delete(ctx, "app", "staging", "job", env.DraftVersion)
	if err != nil || len(view.Workloads) != 0 {
		t.Fatal("never-published draft should still be discardable", err)
	}
}
