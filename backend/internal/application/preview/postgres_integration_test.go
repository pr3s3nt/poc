package preview_test

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/seed"
)

// TestPostgresPreview_RepeatableReadAndNoWrites runs Preview against a
// disposable PostgreSQL database: a commit made after the snapshot's first
// read stays invisible, and neither valid nor invalid previews write.
func TestPostgresPreview_RepeatableReadAndNoWrites(t *testing.T) {
	ctx := context.Background()
	st, err := postgres.Open(ctx, persistencetest.FreshPostgresDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	opts := seed.Defaults()
	opts.RunID = runID
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	state := fingerprint(t, st, opts)

	g := newGuard(st)
	svc := newPreview(g)
	if _, err := svc.PreviewDeployment(ctx, query(opts, "backend", preview.ActionDeploy, nil, scores(opts)["backend"])); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewDeployment(ctx, query(opts, "backend", preview.ActionRemove, scores(opts)["backend"], nil)); err == nil {
		t.Fatal("invalid remove accepted")
	}
	if g.c.writes.Load() != 0 || g.c.directReads.Load() != 0 {
		t.Fatalf("writes=%d directReads=%d", g.c.writes.Load(), g.c.directReads.Load())
	}
	if fingerprint(t, st, opts) != state {
		t.Fatal("preview changed PostgreSQL state")
	}

	g.midSnapshot = func() {
		moved := env
		moved.Version++
		if err := st.SaveEnvironment(context.Background(), moved); err != nil {
			t.Error(err)
		}
	}
	result, err := svc.PreviewDeployment(ctx, query(opts, "worker", preview.ActionDeploy, nil, scores(opts)["worker"]))
	if err != nil {
		t.Fatal(err)
	}
	committed, _ := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if committed.Version != env.Version+1 {
		t.Fatalf("concurrent commit did not happen: %d", committed.Version)
	}
	if result.BaseVersion != env.Version {
		t.Fatalf("snapshot saw a later commit: base version %d, want %d", result.BaseVersion, env.Version)
	}
}
