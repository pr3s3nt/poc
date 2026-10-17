package catalog_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

// registeredAurora registers a copy of the seeded Aurora Definition (version 1)
// and returns it with the catalog under test.
func registeredAurora(t *testing.T) (*store.Store, *catalog.Service, string, resource.Definition) {
	t.Helper()
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	for _, key := range []string{"second-aws", "pending-aws"} {
		status := application.ConnectionReady
		if key == "pending-aws" {
			status = application.ConnectionVerifying
		}
		if err := st.SaveConnection(ctx, application.Connection{Name: key, AuthenticationType: application.AuthAWSAccessKey, Key: key, OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionAWS, Status: status, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	def := seededDefinition(t, opts, "postgres-aws-aurora")
	def.Key, def.Name = "aurora-edit", "Aurora để sửa"
	def.Criteria = []resource.Criterion{{ApplicationID: "edit-app"}}
	created, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, def)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("created version = %d", created.Version)
	}
	return st, svc, opts.OrganizationKey, created
}

func stored(t *testing.T, st persistence.CatalogRepository, org, key string) resource.Definition {
	t.Helper()
	defs, err := st.ListResourceDefinitions(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("definition %s not stored", key)
	return resource.Definition{}
}

func edited(def resource.Definition, change func(*resource.Definition)) resource.Definition {
	out := def
	out.DriverInputs = cloneInputs(def.DriverInputs)
	out.Criteria = append([]resource.Criterion(nil), def.Criteria...)
	change(&out)
	return out
}

// UC-03 V02-06: Name, Driver Type inputs, Account, region, criteria and
// provision rules all change in one CAS; the ID and Resource Type do not.
func TestUpdateResourceDefinition_EditsWholeConfigurationAndBumpsVersionOnce(t *testing.T) {
	ctx := context.Background()
	st, svc, org, def := registeredAurora(t)
	next := edited(def, func(d *resource.Definition) {
		d.Name = "Aurora đã sửa"
		d.DriverAccount = "second-aws"
		d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["region"] = "eu-central-1"
		d.Criteria = []resource.Criterion{{ApplicationID: "edit-app", Class: "fast"}, {EnvironmentType: "production"}}
		d.Provision = map[string]resource.ProvisionRule{"postgres.default#sidecar": {IsDependent: true}}
	})
	got, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, next)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Version != 2 || got.Name != "Aurora đã sửa" || got.DriverAccount != "second-aws" || got.SourceFingerpr == "" {
		t.Fatalf("updated = %+v", got)
	}
	back := stored(t, st, org, def.Key)
	if !reflect.DeepEqual(back, got) {
		t.Fatalf("stored %+v differs from returned %+v", back, got)
	}
	if back.Key != def.Key || back.ResourceTypeKey != def.ResourceTypeKey || len(back.Criteria) != 2 || len(back.Provision) != 1 {
		t.Fatalf("stored = %+v", back)
	}
	region, _ := back.DefinitionRegion()
	if region != "eu-central-1" {
		t.Fatalf("region = %q", region)
	}
}

func TestUpdateResourceDefinition_KeyAndResourceTypeAreImmutable(t *testing.T) {
	ctx := context.Background()
	st, svc, org, def := registeredAurora(t)
	for name, change := range map[string]func(*resource.Definition){
		"resource type": func(d *resource.Definition) { d.ResourceTypeKey = "vpc" },
	} {
		if _, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, edited(def, change)); !errors.Is(err, catalog.ErrInvalid) {
			t.Fatalf("%s change: %v", name, err)
		}
	}
	renamed := edited(def, func(d *resource.Definition) { d.Key = "other-id" })
	if _, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, renamed); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("id change: %v", err)
	}
	if got := stored(t, st, org, def.Key); got.Version != 1 || got.ResourceTypeKey != def.ResourceTypeKey {
		t.Fatalf("immutable fields moved: %+v", got)
	}
}

// The edit runs the SAME validation as creation; a rejected edit leaves the
// stored Definition and its version exactly as they were.
func TestUpdateResourceDefinition_SharesCreateValidationAndLeavesStoredUntouched(t *testing.T) {
	ctx := context.Background()
	st, svc, org, def := registeredAurora(t)
	before := stored(t, st, org, def.Key)
	for name, change := range map[string]func(*resource.Definition){
		"account not READY": func(d *resource.Definition) { d.DriverAccount = "pending-aws" },
		"unknown account":   func(d *resource.Definition) { d.DriverAccount = "gone" },
		"no account":        func(d *resource.Definition) { d.DriverAccount = "" },
		"bad region": func(d *resource.Definition) {
			d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["region"] = "mars-1"
		},
		"blank name":     func(d *resource.Definition) { d.Name = " " },
		"empty criteria": func(d *resource.Definition) { d.Criteria = nil },
		"environment binding": func(d *resource.Definition) {
			d.DriverInputs["values"].(map[string]any)["provider_bindings"] = map[string]any{"aws": map[string]any{"target": "environment"}}
		},
		"drifted digest": func(d *resource.Definition) {
			d.DriverInputs["values"].(map[string]any)["source"].(map[string]any)["content_sha256"] = "9999999999999999999999999999999999999999999999999999999999999999"
		},
	} {
		if _, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, edited(def, change)); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if after := stored(t, st, org, def.Key); !reflect.DeepEqual(before, after) {
		t.Fatalf("a rejected edit changed the stored definition: %+v -> %+v", before, after)
	}
}

func TestUpdateResourceDefinition_StaleVersionConflictsAndKeepsTheWinner(t *testing.T) {
	ctx := context.Background()
	st, svc, org, def := registeredAurora(t)
	winner := edited(def, func(d *resource.Definition) { d.Name = "Người thắng" })
	if _, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, winner); err != nil {
		t.Fatal(err)
	}
	loser := edited(def, func(d *resource.Definition) { d.Name = "Người thua" })
	if _, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, loser); !errors.Is(err, catalog.ErrVersionConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	if got := stored(t, st, org, def.Key); got.Name != "Người thắng" || got.Version != 2 {
		t.Fatalf("the winner was overwritten: %+v", got)
	}
	if _, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 0, loser); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("missing version: %v", err)
	}
}

func TestUpdateResourceDefinition_UnknownScopedAndReservedKeysAreNotFound(t *testing.T) {
	ctx := context.Background()
	_, svc, org, def := registeredAurora(t)
	missing := edited(def, func(d *resource.Definition) { d.Key = "never-registered" })
	if _, err := svc.UpdateResourceDefinition(ctx, org, "never-registered", 1, missing); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.UpdateResourceDefinition(ctx, "globex", def.Key, 1, def); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("other organization: %v", err)
	}
}

// Concurrent edits from the same base version: exactly one commits.
func TestUpdateResourceDefinition_ConcurrentEditsHaveExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	st, svc, org, def := registeredAurora(t)
	const writers = 8
	results := make([]error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = svc.UpdateResourceDefinition(ctx, org, def.Key, 1, edited(def, func(d *resource.Definition) { d.Name = "writer" }))
		}(i)
	}
	wg.Wait()
	wins, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, catalog.ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != writers-1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	if got := stored(t, st, org, def.Key); got.Version != 2 {
		t.Fatalf("version = %d", got.Version)
	}
}

// uncertainStore reports an unknown commit outcome from Transact and counts
// calls, so the test can prove the mutation is never replayed.
type uncertainStore struct {
	persistence.Store
	mu    sync.Mutex
	calls int
}

func (s *uncertainStore) Transact(ctx context.Context, fn func(context.Context) error) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if err := s.Store.Transact(ctx, fn); err != nil {
		return err
	}
	return persistence.ErrCommitUncertain
}

func TestDefinitionWritesNeverReplayAnUncertainCommit(t *testing.T) {
	ctx := context.Background()
	st, _, org, def := registeredAurora(t)
	wrapped := &uncertainStore{Store: st}
	svc := catalog.NewService(wrapped, withRegistry(t, st))
	_, err := svc.UpdateResourceDefinition(ctx, org, def.Key, 1, edited(def, func(d *resource.Definition) { d.Name = "không rõ" }))
	if !errors.Is(err, catalog.ErrOutcomeUncertain) || wrapped.calls != 1 {
		t.Fatalf("update: err=%v transactions=%d", err, wrapped.calls)
	}
	fresh := edited(def, func(d *resource.Definition) {
		d.Key, d.Criteria = "aurora-uncertain", []resource.Criterion{{ApplicationID: "x"}}
	})
	wrapped.calls = 0
	if _, err := svc.RegisterResourceDefinition(ctx, org, fresh); !errors.Is(err, catalog.ErrOutcomeUncertain) || wrapped.calls != 1 {
		t.Fatalf("create: err=%v transactions=%d", err, wrapped.calls)
	}
}
