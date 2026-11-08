package deployment

import (
	"context"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

type progressHistory struct {
	persistence.Store
	records   []domain.Deployment
	sets      map[string]environment.DeploymentSet
	snapshots map[string][]domain.WorkloadSnapshot
}

func (s progressHistory) GetDeploymentSet(_ context.Context, id string) (environment.DeploymentSet, error) {
	v, ok := s.sets[id]
	if !ok {
		return v, persistence.ErrNotFound
	}
	return v, nil
}
func (s progressHistory) ListDeployments(context.Context, string, string) ([]domain.Deployment, error) {
	return s.records, nil
}
func (s progressHistory) ListDeploymentWorkloads(_ context.Context, id string) ([]domain.WorkloadSnapshot, error) {
	return s.snapshots[id], nil
}

func TestRetainedProgressRequiresExactHistoricalProof(t *testing.T) {
	for _, name := range []string{"ready", "missing", "failed snapshot", "wrong snapshot owner", "wrong tenant", "wrong generation", "changed module", "no retain intent", "current failure", "ambiguous ancestry", "unrelated success", "later success"} {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC()
			record := domain.Deployment{ID: "frontend-run", OrganizationKey: "org", ApplicationKey: "app", EnvironmentKey: "staging", EnvironmentID: "env", TargetGeneration: 2, BaseDeploymentSetID: "base", StartedAt: now}
			prior := record
			prior.ID = "backend-run"
			prior.WorkloadID = "backend"
			prior.Status = domain.StatusSucceeded
			prior.CandidateDeploymentSet = "base"
			prior.BaseDeploymentSetID = ""
			prior.StartedAt = now.Add(-time.Minute)
			doc := environment.NewDocument()
			doc.Modules["backend"] = environment.Module{Spec: environment.ModuleSpec{Containers: map[string]environment.Container{}}}
			proof := domain.WorkloadSnapshot{DeploymentID: prior.ID, WorkloadID: "backend", Status: domain.InstanceReady, ManifestDigest: "digest", ObservedAt: now.Add(-time.Second)}
			h := progressHistory{records: []domain.Deployment{prior}, sets: map[string]environment.DeploymentSet{"base": {ID: "base", EnvironmentID: "env", Document: doc}}, snapshots: map[string][]domain.WorkloadSnapshot{prior.ID: {proof}}}
			review := map[string]any{"nodes": []any{map[string]any{"id": "workload.custom#modules.backend", "intent": "RETAIN"}}}
			var current []domain.WorkloadSnapshot
			switch name {
			case "missing":
				h.snapshots = nil
			case "failed snapshot":
				h.snapshots[prior.ID][0].Status = domain.InstanceFailed
			case "wrong snapshot owner":
				h.snapshots[prior.ID][0].DeploymentID = "unrelated"
			case "wrong tenant":
				h.records[0].OrganizationKey = "other"
			case "wrong generation":
				h.records[0].TargetGeneration = 1
			case "changed module":
				h.records[0].WorkloadID = "other"
				h.records[0].BaseDeploymentSetID = "older"
				old := prior
				old.ID = "older-run"
				old.CandidateDeploymentSet = "older"
				h.records = append(h.records, old)
				h.sets["older"] = environment.DeploymentSet{ID: "older", EnvironmentID: "env", Document: environment.NewDocument()}
				h.snapshots[old.ID] = []domain.WorkloadSnapshot{proof}
			case "no retain intent":
				review = nil
			case "current failure":
				current = []domain.WorkloadSnapshot{{DeploymentID: record.ID, WorkloadID: "backend", Status: domain.InstanceFailed}}
			case "ambiguous ancestry":
				other := prior
				other.ID = "duplicate"
				h.records = append(h.records, other)
			case "unrelated success":
				h.records[0].CandidateDeploymentSet = "other-set"
			case "later success":
				h.records[0].StartedAt = now.Add(time.Minute)
			}
			out, err := retainedWorkloadProgress(context.Background(), h, record, review, current)
			if err != nil {
				t.Fatal(err)
			}
			if name == "ready" {
				if len(out) != 1 || out[0].DeploymentID != prior.ID {
					t.Fatalf("lost baseline proof: %#v", out)
				}
			} else if name == "current failure" {
				if len(out) != 1 || out[0].Status != domain.InstanceFailed {
					t.Fatalf("masked current failure: %#v", out)
				}
			} else if len(out) != 0 {
				t.Fatalf("fabricated retained readiness: %#v", out)
			}
		})
	}
}

func TestRetainedProgressTraversesUnchangedAncestry(t *testing.T) {
	now := time.Now().UTC()
	r := domain.Deployment{ID: "run", OrganizationKey: "org", ApplicationKey: "app", EnvironmentID: "env", EnvironmentKey: "staging", BaseDeploymentSetID: "set2", StartedAt: now}
	p := r
	p.ID = "peer"
	p.WorkloadID = "peer"
	p.Status = domain.StatusSucceeded
	p.CandidateDeploymentSet = "set2"
	p.BaseDeploymentSetID = "set1"
	b := p
	b.ID = "backend"
	b.WorkloadID = "backend"
	b.CandidateDeploymentSet = "set1"
	b.BaseDeploymentSetID = ""
	doc := environment.NewDocument()
	doc.Modules["backend"] = environment.Module{}
	h := progressHistory{records: []domain.Deployment{p, b}, sets: map[string]environment.DeploymentSet{"set1": {ID: "set1", EnvironmentID: "env", Document: doc}, "set2": {ID: "set2", EnvironmentID: "env", Document: doc}}, snapshots: map[string][]domain.WorkloadSnapshot{"backend": {{DeploymentID: "backend", WorkloadID: "backend", Status: domain.InstanceReady, ManifestDigest: "digest"}}}}
	review := map[string]any{"nodes": []any{map[string]any{"id": "workload.default#modules.backend", "intent": "RETAIN"}}}
	out, err := retainedWorkloadProgress(context.Background(), h, r, review, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("unchanged ancestry lost: %#v %v", out, err)
	}
	graph := map[string]any{"nodes": []any{map[string]any{"descriptor": "workload.default#modules.backend", "kind": "workload", "workloadId": "backend"}}}
	projected := projectNodeProgress(graph, review, nil, nil, out, domain.StatusSucceeded, now)
	if projected[0].State != "RETAINED" {
		t.Fatal(projected)
	}
}
