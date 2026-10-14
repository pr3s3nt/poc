package jobstore_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
)

func TestDSNEscapesReservedCharacters(t *testing.T) {
	cfg := jobstore.Config{Host: "db.internal", Port: 5433, Database: "app/db", User: "us@er:x", Password: `p@ss:/?#[]%wo'rd " \`}
	parsed, err := pgx.ParseConfig(cfg.DSN())
	if err != nil {
		t.Fatalf("DSN must parse: %v", err)
	}
	if parsed.Host != "db.internal" || parsed.Port != 5433 || parsed.Database != "app/db" || parsed.User != "us@er:x" || parsed.Password != cfg.Password {
		t.Fatalf("DSN changed the meaning of a component: %+v", parsed)
	}
}

func TestConfigFromEnvBounds(t *testing.T) {
	good := map[string]string{"PGHOST": "h", "PGPORT": "5432", "PGDATABASE": "d", "PGUSER": "u", "PGPASSWORD": "pw"}
	set := func(m map[string]string) {
		for k, v := range m {
			t.Setenv(k, v)
		}
	}
	set(good)
	if _, err := jobstore.ConfigFromEnv(); err != nil {
		t.Fatal(err)
	}
	for name, override := range map[string]map[string]string{
		"port zero":      {"PGPORT": "0"},
		"port too large": {"PGPORT": "70000"},
		"port garbage":   {"PGPORT": "5x"},
		"host missing":   {"PGHOST": ""},
		"user newline":   {"PGUSER": "a\nb"},
		"db too long":    {"PGDATABASE": strings.Repeat("d", 300)},
		"password long":  {"PGPASSWORD": strings.Repeat("p", 2000)},
	} {
		t.Run(name, func(t *testing.T) {
			set(good)
			set(override)
			_, err := jobstore.ConfigFromEnv()
			if err == nil {
				t.Fatal("must fail closed")
			}
			if strings.Contains(err.Error(), "pw") && name == "password long" {
				t.Fatalf("error leaks the password: %v", err)
			}
		})
	}
}

func TestStoreWithReservedCharacterPassword(t *testing.T) {
	cfg := testfixture.PostgresConfigWithPassword(t, `p@ss:/?#[]%wo'rd`)
	ctx := context.Background()
	store, err := jobstore.Open(ctx, cfg, 1)
	if err != nil {
		t.Fatalf("login with a reserved-character password: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	bad := cfg
	bad.Password = "wrong"
	if s, err := jobstore.Open(ctx, bad, 1); err == nil {
		s.Close()
		t.Fatal("a wrong password must be rejected (the fixture server must enforce passwords)")
	}
}

func TestMigrateIsConcurrentAndIdempotent(t *testing.T) {
	cfg := testfixture.PostgresConfig(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := jobstore.Open(ctx, cfg, 1)
			if err != nil {
				errs <- err
				return
			}
			defer s.Close()
			errs <- s.Migrate(ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestClaimProcessesEachJobExactlyOnce(t *testing.T) {
	cfg := testfixture.PostgresConfig(t)
	ctx := context.Background()
	store, err := jobstore.Open(ctx, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := store.Submit(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	done, claimed, err := store.ClaimAndProcess(ctx, "w1", func(p string) string { return "processed:" + strings.ToUpper(p) })
	if err != nil || !claimed || done.ID != job.ID || done.Status != jobstore.StatusDone || done.Result != "processed:ABC" {
		t.Fatalf("claim: %+v claimed=%v err=%v", done, claimed, err)
	}
	if _, claimed, _ := store.ClaimAndProcess(ctx, "w2", strings.ToUpper); claimed {
		t.Fatal("a completed job must not be claimed again")
	}
	if err := store.RecordCacheWrite(ctx, job.ID, true); err != nil {
		t.Fatal(err)
	}
	ok, failed, unverified, err := store.CacheWriteCounts(ctx)
	if err != nil || ok != 1 || failed != 0 || unverified != 0 {
		t.Fatalf("counts ok=%d failed=%d err=%v", ok, failed, err)
	}
	summaries, err := store.ListSummaries(ctx, 10)
	if err != nil || len(summaries) != 1 || summaries[0].Result != "" || summaries[0].Status != jobstore.StatusDone {
		t.Fatalf("summaries must omit results: %+v err=%v", summaries, err)
	}
}
