package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// Two Store values with separate pgx pools stand in for two backend processes
// sharing one PostgreSQL database.
func twoBackends(t *testing.T) (*Store, *Store, string) {
	t.Helper()
	url := persistencetest.FreshPostgresDatabase(t)
	a, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); b.Close() })
	if a.pool == b.pool {
		t.Fatal("the two backends must not share a pool")
	}
	ctx := context.Background()
	if err := a.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"one", "two"} {
		if err := a.CreateSecretStore(ctx, secretstore.Store{Key: key, OrganizationKey: "acme", Name: key, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
			BackendAddress: "http://v:8200", WorkloadAddress: "http://v:8200", Mount: "kv", AuthMount: "kubernetes"}); err != nil {
			t.Fatal(err)
		}
	}
	appID := ids.New()
	if err := a.SaveApplication(ctx, application.Application{ID: appID, Key: appID, OrganizationKey: "acme", Name: "Multi", Subdomain: "multi-" + appID[:8], Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), Key: "staging", ApplicationID: appID, ApplicationKey: appID, Name: "s", Type: "staging", NamespaceIdentity: "app-" + appID + "-staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	return a, b, appID
}

func TestTwoBackendsClaimOneEnvironmentWithOneWinner(t *testing.T) {
	a, b, app := twoBackends(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		store := a
		if i%2 == 1 {
			store = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: app, EnvironmentKey: "staging", Owner: ids.New(), Kind: environment.OpDeploy,
				Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: 1}, Deadline: time.Now().Add(time.Hour)})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrEnvironmentBusy):
		default:
			t.Fatalf("claim: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners across two pools = %d", wins)
	}
}

// A claim admitted on one backend rejects every Settings/config/draft/selection
// write issued on the other backend, and a write committed first makes a claim
// with the old pins stale.
func TestClaimAndSettingsWritesRaceAcrossBackends(t *testing.T) {
	a, b, app := twoBackends(t)
	ctx := context.Background()
	op, err := a.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: app, EnvironmentKey: "staging", Owner: "backend-a", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 3; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			errs <- b.CommitConfigurationRevision(ctx, 0, configuration.Revision{ID: ids.New(), ApplicationKey: app, EnvironmentKey: "staging", Version: 1, Entries: map[string]configuration.Entry{"A": {Kind: configuration.Variable, Value: "1"}}})
		}()
		go func() {
			defer wg.Done()
			errs <- b.SaveWorkloadDraft(ctx, 0, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: "staging", WorkloadID: "web", State: environment.DraftUpsert, Score: map[string]any{"a": "b"}})
		}()
		go func() {
			defer wg.Done()
			_, err := b.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: app, EnvironmentKey: "staging", StoreKey: "one", ExpectedVersion: 1})
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := b.BindEnvironment(ctx, persistence.EnvironmentBinding{ApplicationKey: app, EnvironmentKey: "staging", ConnectionKey: "x", ExpectedVersion: 1})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, persistence.ErrEnvironmentBusy) && !errors.Is(err, persistence.ErrNotFound) {
			t.Fatalf("a write slipped past the claim of another backend: %v", err)
		}
	}
	env, _ := b.GetEnvironment(ctx, app, "staging")
	if env.Version != 1 || env.DraftVersion != 0 || env.SecretStoreKey != "" {
		t.Fatalf("rejected writes mutated state: %+v", env)
	}
	if err := a.ReleaseOperation(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}

	// Write first, claim with the old pins second.
	if _, err := b.SelectSecretStore(ctx, persistence.SecretStoreSelection{ApplicationKey: app, EnvironmentKey: "staging", StoreKey: "two", ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: app, EnvironmentKey: "staging", Owner: "backend-a", Kind: environment.OpDeploy,
		Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: 1}, Deadline: time.Now().Add(time.Hour)}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("a stale pin must lose to the committed write: %v", err)
	}
}
