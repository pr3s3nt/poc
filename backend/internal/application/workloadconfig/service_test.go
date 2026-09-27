package workloadconfig

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/ports/persistence"
)

func testWorkloadService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st := store.New()
	if err := st.SaveApplication(ctx, application.Application{Key: "app", ConfigurationProvider: "vault"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"staging", "production"} {
		set := environment.DeploymentSet{ID: "set-" + name, EnvironmentKey: "app/" + name, Document: environment.NewDocument(), DocumentHash: "empty"}
		if err := st.SaveDeploymentSet(ctx, set); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveEnvironment(ctx, environment.Environment{Key: name, ApplicationKey: "app", NamespaceIdentity: "app-" + name, CurrentDeploymentSetID: set.ID}); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(st), st
}

func testScore(name string, vars map[string]string, resources map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": name},
		"containers": map[string]any{"main": map[string]any{"image": "example.invalid/app:test", "variables": vars}},
		"resources":  resources,
	}
}

func TestWorkloadDraftRejectsLiteralAndStaleVersion(t *testing.T) {
	ctx := context.Background()
	svc, st := testWorkloadService(t)
	if err := st.CommitConfigurationRevision(ctx, 0, configuration.Revision{ID: "config-1", ApplicationKey: "app", EnvironmentKey: "staging", Version: 1, Entries: map[string]configuration.Entry{"API_URL": {Kind: configuration.Variable, ValueRef: "ref"}}}); err != nil {
		t.Fatal(err)
	}
	resources := map[string]any{"env": map[string]any{"type": "environment"}}
	if _, err := svc.Save(ctx, "app", "staging", "frontend", testScore("frontend", map[string]string{"API_URL": "https://literal.invalid"}, resources), 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("literal binding should fail: %v", err)
	}
	if _, err := svc.Save(ctx, "app", "production", "frontend", testScore("frontend", map[string]string{"API_URL": "${resources.env.API_URL}"}, resources), 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-environment key should fail: %v", err)
	}
	view, err := svc.Save(ctx, "app", "staging", "frontend", testScore("frontend", map[string]string{"API_URL": "${resources.env.API_URL}"}, resources), 0)
	if err != nil || view.DraftVersion != 1 || len(view.Workloads) != 1 {
		t.Fatalf("valid draft failed: %+v, %v", view, err)
	}
	if _, err := svc.Save(ctx, "app", "staging", "frontend", testScore("frontend", nil, nil), 0); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale draft version should fail: %v", err)
	}
}

func TestServiceReferenceUsesLatestDraftPorts(t *testing.T) {
	ctx := context.Background()
	svc, _ := testWorkloadService(t)
	backend := testScore("backend", nil, nil)
	backend["service"] = map[string]any{"ports": map[string]any{"http": map[string]any{"port": 80}}}
	if _, err := svc.Save(ctx, "app", "staging", "backend", backend, 0); err != nil {
		t.Fatal(err)
	}
	resources := map[string]any{"backend_service": map[string]any{"type": "service", "params": map[string]any{"workload": "backend", "port": "http"}}}
	frontend := testScore("frontend", map[string]string{"BACKEND_URL": "${resources.backend_service.url}"}, resources)
	if _, err := svc.Save(ctx, "app", "staging", "frontend", frontend, 1); err != nil {
		t.Fatalf("draft service reference rejected: %v", err)
	}
	backend["service"] = map[string]any{"ports": map[string]any{"grpc": map[string]any{"port": 9000}}}
	if _, err := svc.Save(ctx, "app", "staging", "backend", backend, 2); err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateImport(ctx, "app", "staging", frontend); !errors.Is(err, ErrInvalid) {
		t.Fatalf("old service port should be rejected: %v", err)
	}
	if err := svc.ValidateImport(ctx, "app", "production", frontend); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-environment service should be rejected: %v", err)
	}
}

func TestDeletingUndeployedDraftCancelsPendingAdd(t *testing.T) {
	ctx := context.Background()
	svc, _ := testWorkloadService(t)
	if _, err := svc.Save(ctx, "app", "staging", "scratch", testScore("scratch", nil, nil), 0); err != nil {
		t.Fatal(err)
	}
	view, err := svc.Delete(ctx, "app", "staging", "scratch", 1)
	if err != nil || view.DraftVersion != 2 || len(view.Workloads) != 0 {
		t.Fatalf("deleting a never-deployed workload should cancel its draft: %+v, %v", view, err)
	}
}

func TestReconstructDeployedReferenceWorkload(t *testing.T) {
	raw := testScore("frontend", map[string]string{"TOKEN": "${resources.env.API_TOKEN}"}, map[string]any{"env": map[string]any{"type": "environment"}})
	parsed, err := score.FromMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := parsed.Fragment(nil)
	if err != nil {
		t.Fatal(err)
	}
	set := environment.NewDocument()
	set.Modules["frontend"] = fragment.Module
	reconstructed, err := ReconstructScore("frontend", fragment.Module, set)
	if err != nil {
		t.Fatal(err)
	}
	again, err := score.FromMap(reconstructed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := again.Fragment(nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Module.Spec.Containers["main"].Variables["TOKEN"] != fragment.Module.Spec.Containers["main"].Variables["TOKEN"] {
		t.Fatalf("round trip changed binding: %+v", reconstructed)
	}
	legacy := fragment.Module
	legacy.Spec.Containers["main"] = environment.Container{Image: "frontend:dev", Variables: map[string]string{"PASSWORD": "raw-secret"}}
	if _, err := ReconstructScore("frontend", legacy, set); err == nil {
		t.Fatal("legacy literal should not be exposed in the editor")
	}
}
