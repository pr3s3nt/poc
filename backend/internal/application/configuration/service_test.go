package configuration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

type fixture struct {
	st       *store.Store
	registry *configmemory.Registry
	svc      *Service
	path     string
}

const (
	org    = "acme"
	appKey = "app-1"
)

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := store.NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: org, Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveApplication(ctx, application.Application{Key: appKey, OrganizationKey: org}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: appKey, Key: env, NamespaceIdentity: "app-" + env}); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"vault-a", "vault-b"} {
		if err := st.CreateSecretStore(ctx, secretstore.Store{Key: key, OrganizationKey: org, Name: key, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady, BackendAddress: "http://vault.example:8200", WorkloadAddress: "http://vault.svc:8200", Mount: "kv", AuthMount: "kubernetes"}); err != nil {
			t.Fatal(err)
		}
	}
	registry := configmemory.NewRegistry(st)
	return &fixture{st: st, registry: registry, svc: NewService(st, registry, envops.NewManager(st)), path: path}
}

func (f *fixture) selectStore(t *testing.T, env, key string) environment.Environment {
	t.Helper()
	current, err := f.st.GetEnvironment(context.Background(), appKey, env)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := f.st.GetConfigurationScope(context.Background(), appKey, env)
	result, err := f.svc.SetSecretStore(context.Background(), SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: env, StoreKey: key, ExpectedVersion: current.Version, ExpectedConfigVersion: scope.Version})
	if err != nil {
		t.Fatal(err)
	}
	return result.Environment
}

func TestVariablesAreOrdinaryMetadataAndSecretsNeedASelectedStore(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	secret := "do-not-store-this-password"
	// Ordinary variables work before any store is selected.
	view, err := f.svc.Put(ctx, appKey, "staging", "LOG_LEVEL", domain.Variable, "debug", 0)
	if err != nil || view.Version != 1 || view.Keys[0].Value == nil || *view.Keys[0].Value != "debug" || view.Keys[0].StoreKey != "" {
		t.Fatalf("variable before store: %+v %v", view, err)
	}
	// A secret write without a selected store fails safely, without writing anywhere.
	if _, err := f.svc.Put(ctx, appKey, "staging", "API_TOKEN", domain.Secret, secret, 1); !errors.Is(err, ErrNoStore) {
		t.Fatalf("secret without store: %v", err)
	}
	if f.registry.Values(org, "vault-a") != 0 {
		t.Fatal("a rejected secret write reached a store")
	}
	env := f.selectStore(t, "staging", "vault-a")
	if env.SecretStoreKey != "vault-a" {
		t.Fatalf("selection: %+v", env)
	}
	view, err = f.svc.Put(ctx, appKey, "staging", "API_TOKEN", domain.Secret, secret, 1)
	if err != nil || view.Version != 2 {
		t.Fatalf("secret put: %+v %v", view, err)
	}
	for _, key := range view.Keys {
		if key.Name == "API_TOKEN" && (key.Value != nil || key.StoreKey != "vault-a" || key.Kind != domain.Secret) {
			t.Fatalf("secret leaked or lost its store identity: %+v", key)
		}
	}
	revision, _ := f.st.GetConfigurationRevision(ctx, view.RevisionID)
	if entry := revision.Entries["API_TOKEN"]; entry.StoreKey != "vault-a" || entry.ValueRef == "" || entry.Value != "" {
		t.Fatalf("secret entry: %+v", entry)
	}
	if entry := revision.Entries["LOG_LEVEL"]; entry.Value != "debug" || entry.ValueRef != "" || entry.StoreKey != "" {
		t.Fatalf("variable entry: %+v", entry)
	}
	if _, err := f.svc.Put(ctx, appKey, "staging", "API_TOKEN", domain.Variable, "plain", 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("want cross-type name conflict, got %v", err)
	}
	if _, err := f.svc.Put(ctx, appKey, "staging", "OTHER", domain.Variable, "x", 0); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("want stale version conflict, got %v", err)
	}
	other, err := f.svc.List(ctx, appKey, "production")
	if err != nil || len(other.Keys) != 0 || other.SecretStoreKey != "" {
		t.Fatalf("production changed: %+v, %v", other, err)
	}
	data, _ := os.ReadFile(f.path)
	if strings.Contains(string(data), secret) {
		t.Fatal("secret value reached the state snapshot")
	}
	if !strings.Contains(string(data), "debug") {
		t.Fatal("ordinary variable value must live in orchestrator metadata")
	}
	if view, err = f.svc.Rename(ctx, appKey, "staging", "API_TOKEN", "NEW_TOKEN", 2); err != nil || view.Version != 3 {
		t.Fatalf("rename: %+v %v", view, err)
	}
	if view, err = f.svc.Delete(ctx, appKey, "staging", "NEW_TOKEN", 3); err != nil || view.Version != 4 {
		t.Fatalf("delete: %+v %v", view, err)
	}
}

func TestStoreSwitchCopiesSecretsAtomicallyAndKeepsOldRevisions(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	if _, err := f.svc.Put(ctx, appKey, "staging", "DB_PASSWORD", domain.Secret, "s3cret-one", 0); err != nil {
		t.Fatal(err)
	}
	view, err := f.svc.Put(ctx, appKey, "staging", "MODE", domain.Variable, "blue", 1)
	if err != nil {
		t.Fatal(err)
	}
	oldRevision, _ := f.st.GetConfigurationRevision(ctx, view.RevisionID)
	oldRef := oldRevision.Entries["DB_PASSWORD"]
	env, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")

	// Same store at the current versions is a no-op: nothing copied, no version.
	same, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-a", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version})
	if err != nil || same.Changed || same.Environment.Version != env.Version || f.registry.Values(org, "vault-b") != 0 {
		t.Fatalf("same-store no-op: %+v %v", same, err)
	}
	// Stale versions lose before anything is copied.
	if _, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version - 1, ExpectedConfigVersion: scope.Version}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale environment version: %v", err)
	}
	if _, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version - 1}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale configuration version: %v", err)
	}
	if f.registry.Values(org, "vault-b") != 0 {
		t.Fatal("stale switch copied values")
	}

	result, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version})
	if err != nil || !result.Changed || result.Copied != 1 || result.Environment.SecretStoreKey != "vault-b" || result.Environment.Version != env.Version+1 || result.Environment.Busy() {
		t.Fatalf("switch: %+v %v", result, err)
	}
	newScope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	newRevision, _ := f.st.GetConfigurationRevision(ctx, newScope.DesiredRevisionID)
	entry := newRevision.Entries["DB_PASSWORD"]
	if newScope.Version != scope.Version+1 || entry.StoreKey != "vault-b" || entry.ValueRef == oldRef.ValueRef || entry.ValueRef == "" || newRevision.Entries["MODE"].Value != "blue" {
		t.Fatalf("new revision: version=%d %+v", newScope.Version, newRevision.Entries)
	}
	// The old revision and its store value are immutable and still readable.
	again, _ := f.st.GetConfigurationRevision(ctx, view.RevisionID)
	if again.Entries["DB_PASSWORD"] != oldRef {
		t.Fatalf("old revision was rewritten: %+v", again.Entries["DB_PASSWORD"])
	}
	oldProvider, _ := f.registry.Provider(ctx, org, "vault-a")
	if value, err := oldProvider.ReadValue(ctx, oldRef.ValueRef); err != nil || value != "s3cret-one" {
		t.Fatalf("old store value lost: %q %v", value, err)
	}
	// New secrets now go to the new store.
	if _, err := f.svc.Put(ctx, appKey, "staging", "TOKEN_2", domain.Secret, "second", newScope.Version); err != nil {
		t.Fatal(err)
	}
	if f.registry.Values(org, "vault-b") != 2 {
		t.Fatalf("new store holds %d values", f.registry.Values(org, "vault-b"))
	}
}

func TestStoreSwitchCopyFailureLeavesOldStateAndCleansAttemptObjects(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	for i, name := range []string{"S1", "S2", "S3"} {
		if _, err := f.svc.Put(ctx, appKey, "staging", name, domain.Secret, "v-"+name, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	env, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	f.registry.LimitWrites("vault-b", 2) // the third copy fails
	_, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version})
	if !errors.Is(err, ErrStoreCopy) {
		t.Fatalf("want copy failure, got %v", err)
	}
	if strings.Contains(err.Error(), "v-S") {
		t.Fatal("error carries secret bytes")
	}
	after, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	afterScope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	if after.SecretStoreKey != "vault-a" || after.Version != env.Version || after.Busy() || afterScope != scope {
		t.Fatalf("failed copy changed state: env=%+v scope=%+v", after, afterScope)
	}
	if f.registry.Values(org, "vault-b") != 0 {
		t.Fatalf("attempt objects were not cleaned: %d", f.registry.Values(org, "vault-b"))
	}
	ops, _ := f.st.ListOperations(ctx, appKey, "staging", 5)
	if len(ops) != 2 || ops[0].Status != environment.OpFailed || ops[0].Kind != environment.OpStoreCopy || strings.Contains(ops[0].Failure, "v-S") {
		t.Fatalf("operation record: %+v", ops)
	}
	// An unreadable source secret is also a safe copy failure.
	f.registry.LimitWrites("vault-b", 10)
	f.registry.FailReads("vault-a", true)
	if _, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version}); !errors.Is(err, ErrStoreCopy) {
		t.Fatalf("source read failure: %v", err)
	}
}

func TestBusyEnvironmentBlocksConfigurationWritesAndSwitch(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	env, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	op, err := f.st.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: appKey, EnvironmentKey: "staging", Owner: "other", Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Put(ctx, appKey, "staging", "A", domain.Variable, "1", scope.Version); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("put while busy: %v", err)
	}
	if _, err := f.svc.Put(ctx, appKey, "staging", "A", domain.Secret, "1", scope.Version); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("secret put while busy: %v", err)
	}
	if f.registry.Values(org, "vault-a") != 0 {
		t.Fatal("a busy environment accepted a secret write")
	}
	var busy *envops.ErrBusy
	_, err = f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version})
	if !errors.Is(err, persistence.ErrEnvironmentBusy) || errors.As(err, &busy) && busy.Operation.ID != op.ID {
		t.Fatalf("switch while busy: %v", err)
	}
	// The other Environment is independent.
	if _, err := f.svc.Put(ctx, appKey, "production", "A", domain.Variable, "1", 0); err != nil {
		t.Fatalf("independent environment: %v", err)
	}
}

func TestConcurrentStoreSwitchesHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	if _, err := f.svc.Put(ctx, appKey, "staging", "S", domain.Secret, "v", 0); err != nil {
		t.Fatal(err)
	}
	env, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version})
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
		case errors.Is(err, persistence.ErrEnvironmentBusy), errors.Is(err, persistence.ErrVersionConflict):
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	// Losers that arrive after the winner see the new selection at a stale
	// version, never a second copy. Exactly one new revision exists.
	after, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	if wins < 1 || after.Version != scope.Version+1 {
		t.Fatalf("wins=%d config version=%d", wins, after.Version)
	}
}

func TestStoreSwitchRejectsForeignAndUnknownStores(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if err := f.st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := f.st.CreateSecretStore(ctx, secretstore.Store{Key: "foreign", OrganizationKey: "globex", Name: "f", Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady, BackendAddress: "http://v:8200", WorkloadAddress: "http://v:8200", Mount: "kv", AuthMount: "kubernetes"}); err != nil {
		t.Fatal(err)
	}
	env, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	for _, key := range []string{"foreign", "missing"} {
		_, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: key, ExpectedVersion: env.Version})
		if !errors.Is(err, configport.ErrStoreUnavailable) || strings.Contains(err.Error(), "globex") {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if _, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: "globex", ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "foreign", ExpectedVersion: env.Version}); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign organization session: %v", err)
	}
	if got, _ := f.st.GetEnvironment(ctx, appKey, "staging"); got.SecretStoreKey != "" || got.Version != env.Version {
		t.Fatalf("rejected switch mutated: %+v", got)
	}
}

func TestLegacyVariablesAreReadableAndMaterializeOnEdit(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	provider, _ := f.registry.Provider(ctx, org, "vault-a")
	ref, err := provider.WriteValue(ctx, appKey, "staging", "legacy-value")
	if err != nil {
		t.Fatal(err)
	}
	legacy := domain.Revision{ID: ids.New(), ApplicationKey: appKey, EnvironmentKey: "staging", Version: 1, Entries: map[string]domain.Entry{"OLD": {Kind: domain.Variable, ValueRef: ref, StoreKey: "vault-a"}}}
	if err := f.st.CommitConfigurationRevision(ctx, 0, legacy); err != nil {
		t.Fatal(err)
	}
	view, err := f.svc.List(ctx, appKey, "staging")
	if err != nil || view.Keys[0].Value == nil || *view.Keys[0].Value != "legacy-value" || !view.Keys[0].Legacy {
		t.Fatalf("legacy list: %+v %v", view, err)
	}
	// An unreadable legacy store refuses the edit and preserves state.
	f.registry.FailReads("vault-a", true)
	if _, err := f.svc.Put(ctx, appKey, "staging", "NEW", domain.Variable, "x", 1); !errors.Is(err, ErrLegacyUnreadable) {
		t.Fatalf("unreadable legacy: %v", err)
	}
	if scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging"); scope.Version != 1 || scope.DesiredRevisionID != legacy.ID {
		t.Fatalf("failed edit changed state: %+v", scope)
	}
	f.registry.FailReads("vault-a", false)
	if view, err = f.svc.Put(ctx, appKey, "staging", "NEW", domain.Variable, "x", 1); err != nil {
		t.Fatal(err)
	}
	next, _ := f.st.GetConfigurationRevision(ctx, view.RevisionID)
	if e := next.Entries["OLD"]; e.Value != "legacy-value" || e.ValueRef != "" || e.StoreKey != "" {
		t.Fatalf("legacy variable not materialized: %+v", e)
	}
	// The old revision is untouched.
	if old, _ := f.st.GetConfigurationRevision(ctx, legacy.ID); old.Entries["OLD"].ValueRef != ref {
		t.Fatalf("old revision changed: %+v", old.Entries["OLD"])
	}
}

func TestStoreSwitchCleanupFailureIsVisibleAndStateIsUntouched(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	for i, name := range []string{"S1", "S2"} {
		if _, err := f.svc.Put(ctx, appKey, "staging", name, domain.Secret, "v-"+name, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	env, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	f.registry.LimitWrites("vault-b", 1) // the second copy fails
	f.registry.FailDeletes("vault-b", true)
	_, err := f.svc.SetSecretStore(ctx, SwitchCommand{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: "staging", StoreKey: "vault-b", ExpectedVersion: env.Version, ExpectedConfigVersion: scope.Version})
	if !errors.Is(err, ErrStoreCopy) || !strings.Contains(err.Error(), "could not be removed") || strings.Contains(err.Error(), "v-S") || strings.Contains(err.Error(), "kv2://") {
		t.Fatalf("cleanup failure must be visible and safe: %v", err)
	}
	after, _ := f.st.GetEnvironment(ctx, appKey, "staging")
	afterScope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	if after.SecretStoreKey != "vault-a" || after.Version != env.Version || afterScope != scope || after.Busy() {
		t.Fatalf("selection or revision changed: %+v %+v", after, afterScope)
	}
	ops, _ := f.st.ListOperations(ctx, appKey, "staging", 1)
	if ops[0].Status != environment.OpFailed || !strings.Contains(ops[0].Failure, "1 attempt-owned object(s)") || strings.Contains(ops[0].Failure, "kv2://") {
		t.Fatalf("operation must record the orphaned object count: %+v", ops[0])
	}
	if ids := fmt.Sprint(ops[0].Detail["orphanedObjects"]); strings.Count(ids, "-") != 4 {
		t.Fatalf("orphaned object ids missing from detail: %+v", ops[0].Detail)
	}
}
