package configuration

import (
	"context"
	"errors"
	"sync"
	"testing"

	domain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

// OC-12 create-only (DEVEX-ENV E04): a name that exists as any kind is never
// overwritten, and a rejected create writes nothing to the store or provider.
func TestCreateKeyNeverOverwrites(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	view, err := f.svc.CreateKey(ctx, appKey, "staging", "REGION", domain.Variable, "eu", 0)
	if err != nil || len(view.Keys) != 1 || view.Keys[0].Value == nil || *view.Keys[0].Value != "eu" {
		t.Fatalf("create variable: %+v %v", view, err)
	}
	secret, err := f.svc.CreateKey(ctx, appKey, "staging", "TOKEN", domain.Secret, "first-secret", view.Version)
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}
	for _, key := range secret.Keys {
		if key.Kind == domain.Secret && key.Value != nil {
			t.Fatal("create response exposes a Secret value")
		}
	}
	values := f.registry.Values(org, "vault-a")

	for name, call := range map[string]func() error{
		"same kind variable": func() error {
			_, err := f.svc.CreateKey(ctx, appKey, "staging", "REGION", domain.Variable, "us", secret.Version)
			return err
		},
		"other kind over variable": func() error {
			_, err := f.svc.CreateKey(ctx, appKey, "staging", "REGION", domain.Secret, "us", secret.Version)
			return err
		},
		"same kind secret": func() error {
			_, err := f.svc.CreateKey(ctx, appKey, "staging", "TOKEN", domain.Secret, "second-secret", secret.Version)
			return err
		},
		"other kind over secret": func() error {
			_, err := f.svc.CreateKey(ctx, appKey, "staging", "TOKEN", domain.Variable, "plain", secret.Version)
			return err
		},
	} {
		if err := call(); !errors.Is(err, ErrConflict) {
			t.Fatalf("%s: want ErrConflict, got %v", name, err)
		}
	}
	after, _ := f.svc.List(ctx, appKey, "staging")
	if after.Version != secret.Version || len(after.Keys) != 2 || *after.Keys[0].Value != "eu" {
		t.Fatalf("a rejected create changed the scope: %+v", after)
	}
	if f.registry.Values(org, "vault-a") != values {
		t.Fatal("a duplicate create wrote a provider value")
	}
}

func TestCreateKeyRejectsStaleBusyInvalidAndMissingStore(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.svc.CreateKey(ctx, appKey, "staging", "TOKEN", domain.Secret, "v", 0); !errors.Is(err, ErrNoStore) {
		t.Fatalf("secret without store: %v", err)
	}
	if _, err := f.svc.CreateKey(ctx, appKey, "staging", "A", domain.Variable, "1", 7); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale version: %v", err)
	}
	for name, args := range map[string]struct {
		key   string
		kind  domain.Kind
		value string
	}{
		"bad name":  {"1BAD", domain.Variable, "v"},
		"bad kind":  {"A", domain.Kind("OTHER"), "v"},
		"empty":     {"A", domain.Variable, ""},
		"NUL value": {"A", domain.Variable, "a\x00b"},
	} {
		if _, err := f.svc.CreateKey(ctx, appKey, "staging", args.key, args.kind, args.value, 0); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	f.selectStore(t, "staging", "vault-a")
	scope, _ := f.st.GetConfigurationScope(ctx, appKey, "staging")
	if _, err := f.st.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: appKey, EnvironmentKey: "staging", Owner: "other", Kind: environment.OpDeploy}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateKey(ctx, appKey, "staging", "A", domain.Secret, "v", scope.Version); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("busy: %v", err)
	}
	if f.registry.Values(org, "vault-a") != 0 {
		t.Fatal("a busy environment accepted a secret write")
	}
}

// Concurrent creates of one name at one version have exactly one winner and the
// losers leave neither a revision nor an orphan provider value.
func TestConcurrentCreateKeyHasOneWinner(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.selectStore(t, "staging", "vault-a")
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.CreateKey(ctx, appKey, "staging", "TOKEN", domain.Secret, "value", 0)
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
		case errors.Is(err, persistence.ErrVersionConflict), errors.Is(err, ErrConflict), errors.Is(err, persistence.ErrEnvironmentBusy):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	view, _ := f.svc.List(ctx, appKey, "staging")
	if wins != 1 || len(view.Keys) != 1 || view.Version != 1 {
		t.Fatalf("winners=%d view=%+v", wins, view)
	}
	if got := f.registry.Values(org, "vault-a"); got != 1 {
		t.Fatalf("losing creates left %d provider values, want 1", got)
	}
}
