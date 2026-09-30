package deployment_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"orchestrator/internal/adapters/fake"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/secrets"
	tf "orchestrator/internal/adapters/terraform"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/provisioning"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/platform/clock"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

// markerFailure fails the UNREFERENCED marker write inside the final
// transaction, after CompareVersionAndSetCurrent has already run.
type markerFailure struct {
	persistence.Store
	armed bool
}

func (m *markerFailure) UpsertActiveResource(ctx context.Context, a resource.ActiveResource) (resource.ActiveResource, error) {
	if m.armed && a.Status == resource.StatusUnreferenced {
		return a, errors.New("injected marker write failure")
	}
	return m.Store.UpsertActiveResource(ctx, a)
}

func pgDeployments(t *testing.T, st persistence.Store) *appsvc.Service {
	t.Helper()
	prov := provisioning.NewService(st, fake.Registry{Executor: fake.NewResourceExecutor()}, secrets.NewMemory(), clock.System{})
	return appsvc.NewService(st, planning.NewService(), prov, kubernetes.NewRenderer(), fake.NewWorkloadDeployer(), tf.NewInspector(), clock.System{})
}

func pgSharedDB(t *testing.T, st persistence.Store, opts seed.Options) resource.ActiveResource {
	t.Helper()
	return sharedDB(t, st, opts)
}

// TestPostgresRemove_CommitsSetStatusAndMarkerAtomically runs UC-07 remove on
// a fresh PostgreSQL database: the current set, SUCCEEDED and the
// UNREFERENCED marker commit together and survive a restart; a marker write
// failure after the version compare rolls back all three.
func TestPostgresRemove_CommitsSetStatusAndMarkerAtomically(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	st, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { st.Close() }()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	store := &markerFailure{Store: st}
	service := pgDeployments(t, store)
	backend := seed.AcceptanceScores(opts)["backend"]
	command := func(before, after map[string]any) appsvc.DeployCommand {
		return appsvc.DeployCommand{OrganizationKey: opts.OrganizationKey, ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, WorkloadID: "backend", ScoreBefore: before, ScoreAfter: after, Actor: "test"}
	}
	if _, err := service.DeployWorkload(ctx, command(nil, backend)); err != nil {
		t.Fatal(err)
	}
	db := pgSharedDB(t, st, opts)
	envBefore, _ := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)

	// Rollback: the marker write fails after CompareVersionAndSetCurrent.
	store.armed = true
	if _, err := service.DeployWorkload(ctx, command(backend, nil)); err == nil {
		t.Fatal("remove succeeded despite the injected marker failure")
	}
	envAfter, _ := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if envAfter.CurrentDeploymentSetID != envBefore.CurrentDeploymentSetID || envAfter.Version != envBefore.Version {
		t.Fatal("current set moved although the final transaction failed")
	}
	if got := pgSharedDB(t, st, opts); got.Status != resource.StatusReady || got.Version != db.Version {
		t.Fatalf("marker persisted after rollback: %s v%d", got.Status, got.Version)
	}
	list, _ := st.ListDeployments(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if list[0].Status != domain.StatusFailed {
		t.Fatalf("failed remove recorded as %s", list[0].Status)
	}

	// Success: set, SUCCEEDED and marker commit together.
	store.armed = false
	result, err := service.DeployWorkload(ctx, command(backend, nil))
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	reopened, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	record, err := reopened.GetDeployment(ctx, result.DeploymentID)
	if err != nil || record.Status != domain.StatusSucceeded {
		t.Fatalf("remove deployment after restart: %+v %v", record.Status, err)
	}
	env, _ := reopened.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if env.CurrentDeploymentSetID != record.CandidateDeploymentSet || env.Version != envBefore.Version+1 {
		t.Fatal("current set pointer not committed with SUCCEEDED")
	}
	set, _ := reopened.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if _, ok := set.Document.Modules["backend"]; ok {
		t.Fatal("backend still in the committed current set")
	}
	got := pgSharedDB(t, reopened, opts)
	if got.Status != resource.StatusUnreferenced || got.LastDeploymentID != result.DeploymentID || got.Version != db.Version+1 {
		t.Fatalf("marker after restart: %s last=%s v%d", got.Status, got.LastDeploymentID, got.Version)
	}
	if got.ID != db.ID || got.InputFingerprint != db.InputFingerprint || !reflect.DeepEqual(got.Outputs, db.Outputs) || !reflect.DeepEqual(got.ExecutorState, db.ExecutorState) {
		t.Fatal("marker changed identity, outputs, executor state or fingerprint")
	}
}
