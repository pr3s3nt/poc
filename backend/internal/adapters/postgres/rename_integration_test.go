package postgres

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// These T30 tests need a disposable database (ORCHESTRATOR_POSTGRES_TEST_URL,
// created and dropped per test by FreshPostgresDatabase); they skip otherwise,
// or fail under REQUIRE_POSTGRES=1.
// The only schema object they add is a test-owned trigger inside that database.

func twoRenameBackends(t *testing.T) (*Store, *Store, string) {
	t.Helper()
	url := freshDatabase(t).Console
	a, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b, url
}

func TestPostgresDesiredRenameRollsBackAfterWritesBegin(t *testing.T) {
	st, url := openFresh(t)
	fx := persistencetest.SeedRename(t, st)
	ctx := context.Background()
	app, env := fx.Command.ApplicationKey, fx.Command.EnvironmentKey
	// A second affected workload whose draft write raises after the revision,
	// pointer, draft-version bump and first draft were already written.
	cmd := fx.Command
	cmd.Drafts = append(append([]environment.WorkloadDraft(nil), cmd.Drafts...),
		environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: "zz-fail", State: environment.DraftUpsert, Score: map[string]any{"apiVersion": "score.dev/v1b1"}})
	before := persistencetest.TakeRenameSnapshot(t, st, fx.Options)
	// Test-only fault injection runs as the administrator: no runtime login has DDL.
	adminExec(t, url,
		`CREATE FUNCTION t30_fail_draft() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workload_id = 'zz-fail' THEN RAISE EXCEPTION 't30 test failure'; END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER t30_fail_draft BEFORE INSERT OR UPDATE ON workload_drafts FOR EACH ROW EXECUTE FUNCTION t30_fail_draft()`)
	if err := st.CommitDesiredRename(ctx, cmd); err == nil {
		t.Fatal("mid-write failure was ignored")
	}
	if after := persistencetest.TakeRenameSnapshot(t, st, fx.Options); !reflect.DeepEqual(before, after) {
		t.Fatalf("partial write survived rollback:\nbefore %#v\nafter  %#v", before, after)
	}
	if _, err := st.GetConfigurationRevision(ctx, cmd.Revision.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("revision survived rollback: %v", err)
	}
	// Without the failure the same command commits completely.
	adminExec(t, url, `DROP TRIGGER t30_fail_draft ON workload_drafts`)
	cmd.Drafts = fx.Command.Drafts
	if err := st.CommitDesiredRename(ctx, cmd); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDesiredRenameBusyWritesNothing(t *testing.T) {
	st, _ := openFresh(t)
	fx := persistencetest.SeedRename(t, st)
	ctx := context.Background()
	if _, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: fx.Command.ApplicationKey, EnvironmentKey: fx.Command.EnvironmentKey, Owner: "other", Kind: environment.OpDeploy}); err != nil {
		t.Fatal(err)
	}
	before := persistencetest.TakeRenameSnapshot(t, st, fx.Options)
	if err := st.CommitDesiredRename(ctx, fx.Command); !errors.Is(err, persistence.ErrEnvironmentBusy) {
		t.Fatalf("err = %v, want ErrEnvironmentBusy", err)
	}
	if after := persistencetest.TakeRenameSnapshot(t, st, fx.Options); !reflect.DeepEqual(before, after) {
		t.Fatal("busy rename changed state")
	}
}

func TestPostgresDesiredRenameTwoBackendsHaveOneWinner(t *testing.T) {
	a, b, _ := twoRenameBackends(t)
	fx := persistencetest.SeedRename(t, a)
	before := persistencetest.TakeRenameSnapshot(t, a, fx.Options)
	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := a
			if i%2 == 1 {
				store = b
			}
			cmd := fx.Command
			cmd.Revision.ID = ids.New()
			results[i] = store.CommitDesiredRename(context.Background(), cmd)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, persistence.ErrVersionConflict):
			t.Fatalf("loser err = %v, want ErrVersionConflict", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d (%v)", wins, results)
	}
	after := persistencetest.TakeRenameSnapshot(t, b, fx.Options)
	if after.Scope.Version != 2 || after.Env.DraftVersion != before.Env.DraftVersion+1 || after.Env.Version != before.Env.Version {
		t.Fatalf("after race: %#v (before %#v)", after, before)
	}
}

func TestPostgresDesiredRenameSurvivesReopen(t *testing.T) {
	first, url := openFresh(t)
	persistencetest.DesiredRename(t, first)
	want := persistencetest.TakeRenameSnapshot(t, first, persistencetest.SeedOptions())
	first.Close()
	reopened, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := persistencetest.TakeRenameSnapshot(t, reopened, persistencetest.SeedOptions()); !reflect.DeepEqual(got, want) {
		t.Fatalf("reopened %#v, want %#v", got, want)
	}
}
