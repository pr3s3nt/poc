package persistencetest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// RenameFixture is the state DesiredRename leaves seeded.
type RenameFixture struct {
	Options seed.Options
	Command persistence.DesiredRename
}

func renameScore(key string) map[string]any {
	return map[string]any{
		"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
		"containers": map[string]any{"web": map[string]any{"image": "x", "variables": map[string]any{"A": "${resources.env." + key + "}"}}},
		"resources":  map[string]any{"env": map[string]any{"type": "environment"}},
	}
}

// SeedRename seeds revision v1 {OLD}, a draft "api" referencing OLD and a
// pending-delete draft "gone", and returns a valid rename command to NEW.
func SeedRename(t *testing.T, st persistence.Store) RenameFixture {
	t.Helper()
	ctx := context.Background()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	app, env := opts.ApplicationKey, opts.EnvironmentKey
	first := configuration.Revision{ID: ids.New(), ApplicationKey: app, EnvironmentKey: env, Version: 1, Entries: map[string]configuration.Entry{"OLD": {Kind: configuration.Variable, Value: "v"}}}
	if err := st.CommitConfigurationRevision(ctx, 0, first); err != nil {
		t.Fatal(err)
	}
	e, err := st.GetEnvironment(ctx, app, env)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveWorkloadDraft(ctx, e.DraftVersion, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: "api", State: environment.DraftUpsert, Score: renameScore("OLD")}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveWorkloadDraft(ctx, e.DraftVersion+1, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: "gone", State: environment.DraftDelete}); err != nil {
		t.Fatal(err)
	}
	e, err = st.GetEnvironment(ctx, app, env)
	if err != nil {
		t.Fatal(err)
	}
	return RenameFixture{Options: opts, Command: persistence.DesiredRename{
		ApplicationKey: app, EnvironmentKey: env, ExpectedConfigVersion: 1, ExpectedEnvironmentVersion: e.Version,
		ExpectedDraftVersion: e.DraftVersion, ExpectedCurrentSetID: e.CurrentDeploymentSetID,
		Revision: configuration.Revision{ID: ids.New(), ApplicationKey: app, EnvironmentKey: env, Version: 2, Entries: map[string]configuration.Entry{"NEW": {Kind: configuration.Variable, Value: "v"}}},
		Drafts:   []environment.WorkloadDraft{{ApplicationKey: app, EnvironmentKey: env, WorkloadID: "api", State: environment.DraftUpsert, Score: renameScore("NEW")}},
	}}
}

// RenameSnapshot captures everything a rename may change.
type RenameSnapshot struct {
	Env    environment.Environment
	Scope  configuration.Scope
	Drafts []environment.WorkloadDraft
}

func TakeRenameSnapshot(t *testing.T, st persistence.Store, opts seed.Options) RenameSnapshot {
	t.Helper()
	ctx := context.Background()
	e, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := st.GetConfigurationScope(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := st.ListWorkloadDrafts(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	return RenameSnapshot{Env: e, Scope: scope, Drafts: drafts}
}

// DesiredRename is the adapter-neutral contract of the T30 atomic command:
// every guard failure leaves all metadata unchanged, success commits revision,
// scope pointer and drafts together with one draft-version advance, and a
// pending delete or Environment/current-set state is never touched.
func DesiredRename(t *testing.T, st persistence.Store) {
	t.Helper()
	ctx := context.Background()
	fx := SeedRename(t, st)
	before := TakeRenameSnapshot(t, st, fx.Options)

	mutations := map[string]func(*persistence.DesiredRename){
		"config version":      func(c *persistence.DesiredRename) { c.ExpectedConfigVersion = 0; c.Revision.Version = 1 },
		"environment version": func(c *persistence.DesiredRename) { c.ExpectedEnvironmentVersion++ },
		"draft version":       func(c *persistence.DesiredRename) { c.ExpectedDraftVersion++ },
		"current set":         func(c *persistence.DesiredRename) { c.ExpectedCurrentSetID = ids.New() },
		"revision version":    func(c *persistence.DesiredRename) { c.Revision.Version = 5 },
		"pending delete": func(c *persistence.DesiredRename) {
			c.Drafts = append(c.Drafts, environment.WorkloadDraft{ApplicationKey: c.ApplicationKey, EnvironmentKey: c.EnvironmentKey, WorkloadID: "gone", State: environment.DraftUpsert, Score: renameScore("NEW")})
		},
	}
	for name, mutate := range mutations {
		cmd := fx.Command
		cmd.Revision.ID = ids.New()
		cmd.Drafts = append([]environment.WorkloadDraft(nil), fx.Command.Drafts...)
		mutate(&cmd)
		if err := st.CommitDesiredRename(ctx, cmd); !errors.Is(err, persistence.ErrVersionConflict) {
			t.Fatalf("%s guard: err = %v, want ErrVersionConflict", name, err)
		}
		if after := TakeRenameSnapshot(t, st, fx.Options); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s guard changed state:\nbefore %#v\nafter  %#v", name, before, after)
		}
	}
	invalid := fx.Command
	invalid.Revision.ID = ids.New()
	invalid.Drafts = []environment.WorkloadDraft{{ApplicationKey: invalid.ApplicationKey, EnvironmentKey: invalid.EnvironmentKey, WorkloadID: "api", State: environment.DraftUpsert}}
	if err := st.CommitDesiredRename(ctx, invalid); err == nil {
		t.Fatal("draft without a Score was accepted")
	}
	if after := TakeRenameSnapshot(t, st, fx.Options); !reflect.DeepEqual(before, after) {
		t.Fatal("invalid draft changed state")
	}

	if err := st.CommitDesiredRename(ctx, fx.Command); err != nil {
		t.Fatalf("commit: %v", err)
	}
	after := TakeRenameSnapshot(t, st, fx.Options)
	if after.Scope.Version != 2 || after.Scope.DesiredRevisionID != fx.Command.Revision.ID {
		t.Fatalf("scope = %#v", after.Scope)
	}
	if after.Env.DraftVersion != before.Env.DraftVersion+1 || after.Env.Version != before.Env.Version || after.Env.CurrentDeploymentSetID != before.Env.CurrentDeploymentSetID {
		t.Fatalf("environment = %#v, before %#v", after.Env, before.Env)
	}
	byID := map[string]environment.WorkloadDraft{}
	for _, d := range after.Drafts {
		byID[d.WorkloadID] = d
	}
	if !reflect.DeepEqual(byID["api"].Score, renameScore("NEW")) || byID["gone"].State != environment.DraftDelete {
		t.Fatalf("drafts = %#v", after.Drafts)
	}
	if err := st.CommitDesiredRename(ctx, fx.Command); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("replay = %v, want ErrVersionConflict", err)
	}

	// No changed draft: configuration advances once, the draft version stays.
	again := persistence.DesiredRename{ApplicationKey: fx.Command.ApplicationKey, EnvironmentKey: fx.Command.EnvironmentKey, ExpectedConfigVersion: 2,
		ExpectedEnvironmentVersion: after.Env.Version, ExpectedDraftVersion: after.Env.DraftVersion, ExpectedCurrentSetID: after.Env.CurrentDeploymentSetID,
		Revision: configuration.Revision{ID: ids.New(), ApplicationKey: fx.Command.ApplicationKey, EnvironmentKey: fx.Command.EnvironmentKey, Version: 3, Entries: map[string]configuration.Entry{"NEW2": {Kind: configuration.Variable, Value: "v"}}}}
	if err := st.CommitDesiredRename(ctx, again); err != nil {
		t.Fatal(err)
	}
	last := TakeRenameSnapshot(t, st, fx.Options)
	if last.Scope.Version != 3 || last.Env.DraftVersion != after.Env.DraftVersion {
		t.Fatalf("draft-free rename: %#v", last)
	}
}

// SeedOptions are the seed options SeedRename applies.
func SeedOptions() seed.Options { return seed.Defaults() }
