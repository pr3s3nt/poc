package jobcache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobcache"
	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
)

func cfgFor(r *testfixture.Redis) jobcache.Config {
	return jobcache.Config{
		Host: r.Host, Port: r.Port, Username: r.Username, Password: r.Password, TLS: r.TLS,
		App: "acceptance", Env: "dev", Run: "run-01", TTL: 30 * time.Second,
	}
}

func doneJob(id int64) jobstore.Job {
	now := time.Now().UTC().Truncate(time.Second)
	return jobstore.Job{ID: id, Payload: "p", Status: jobstore.StatusDone, Result: "processed:P", ProcessedBy: "w", CreatedAt: now, ProcessedAt: &now}
}

func TestCachePutGetAuthenticatedWithDeterministicTTL(t *testing.T) {
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	cfg := cfgFor(r)
	cfg.TTL = 5 * time.Second
	c := jobcache.New(cfg)
	defer c.Close()
	ctx := context.Background()

	if _, found, err := c.Get(ctx, 7); err != nil || found {
		t.Fatalf("empty cache must be a clean miss, found=%v err=%v", found, err)
	}
	if err := c.Put(ctx, doneJob(7)); err != nil {
		t.Fatal(err)
	}
	got, found, err := c.Get(ctx, 7)
	if err != nil || !found || got.Result != "processed:P" || got.ID != 7 {
		t.Fatalf("hit expected, got %+v found=%v err=%v", got, found, err)
	}
	ttl := r.Client(t).TTL(ctx, cfg.KeyFor(7)).Val()
	if ttl <= 0 || ttl > 5*time.Second {
		t.Fatalf("ttl = %v, want within (0,5s]", ttl)
	}
	if err := c.Put(ctx, jobstore.Job{ID: 8, Status: jobstore.StatusPending}); err == nil {
		t.Fatal("a pending job must never be cached")
	}
	if n := r.Client(t).Exists(ctx, cfg.KeyFor(8)).Val(); n != 0 {
		t.Fatal("pending job key exists")
	}
}

func TestCacheEntryExpiresToMiss(t *testing.T) {
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	cfg := cfgFor(r)
	cfg.TTL = time.Second
	c := jobcache.New(cfg)
	defer c.Close()
	ctx := context.Background()
	if err := c.Put(ctx, doneJob(1)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1300 * time.Millisecond)
	if _, found, err := c.Get(ctx, 1); err != nil || found {
		t.Fatalf("expired entry must be a clean miss, found=%v err=%v", found, err)
	}
}

func TestCacheFailuresAreClassifiedAndNeverMisses(t *testing.T) {
	r := testfixture.StartRedis(t, testfixture.RedisOptions{Password: "p@ss:/?#[]%wo'rd"})
	ctx := context.Background()

	good := jobcache.New(cfgFor(r))
	defer good.Close()
	if err := good.Ping(ctx); err != nil {
		t.Fatalf("reserved characters in the password must work: %v", err)
	}

	cases := map[string]struct {
		mutate func(*jobcache.Config)
		want   jobcache.Reason
	}{
		"wrong password": {func(c *jobcache.Config) { c.Password = "wrong" }, jobcache.ReasonAuth},
		"wrong username": {func(c *jobcache.Config) { c.Username = "nobody" }, jobcache.ReasonAuth},
		"wrong port":     {func(c *jobcache.Config) { c.Port = c.Port + 1 }, jobcache.ReasonUnreachable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := cfgFor(r)
			tc.mutate(&cfg)
			c := jobcache.New(cfg)
			defer c.Close()
			for op, fn := range map[string]func() error{
				"ping": func() error { return c.Ping(ctx) },
				"get":  func() error { _, _, err := c.Get(ctx, 1); return err },
				"put":  func() error { return c.Put(ctx, doneJob(1)) },
			} {
				err := fn()
				if err == nil {
					t.Fatalf("%s must fail, not report a miss or success", op)
				}
				if got := jobcache.Classify(err); got != tc.want {
					t.Errorf("%s: reason = %q, want %q", op, got, tc.want)
				}
			}
		})
	}
}

func TestCacheTLSClientToPlainServerFails(t *testing.T) {
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	cfg := cfgFor(r)
	cfg.TLS = true
	c := jobcache.New(cfg)
	defer c.Close()
	err := c.Ping(context.Background())
	// A plain server never answers a ClientHello, so this surfaces as a handshake
	// timeout or a TLS record error; it must never succeed.
	if got := jobcache.Classify(err); err == nil || (got != jobcache.ReasonTLS && got != jobcache.ReasonTimeout) {
		t.Fatalf("err=%v reason=%q", err, got)
	}
}

func TestCacheOutageIsUnreachableNotMiss(t *testing.T) {
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	c := jobcache.New(cfgFor(r))
	defer c.Close()
	ctx := context.Background()
	if err := c.Put(ctx, doneJob(1)); err != nil {
		t.Fatal(err)
	}
	r.Stop()
	_, found, err := c.Get(ctx, 1)
	if err == nil || found {
		t.Fatalf("outage must be an error, found=%v err=%v", found, err)
	}
	if got := jobcache.Classify(err); got != jobcache.ReasonUnreachable {
		t.Fatalf("reason = %q", got)
	}
	if err := c.Ping(ctx); err == nil {
		t.Fatal("ping must fail during an outage")
	}
}

func TestCacheTLSVerifiesChainAndHostname(t *testing.T) {
	r := testfixture.StartRedis(t, testfixture.RedisOptions{TLS: true, CertNames: []string{"localhost"}})
	ctx := context.Background()

	t.Run("trusted CA and matching hostname", func(t *testing.T) {
		c := jobcache.New(cfgFor(r), jobcache.WithRootCAs(r.CA))
		defer c.Close()
		if err := c.Ping(ctx); err != nil {
			t.Fatalf("ping over verified TLS: %v", err)
		}
		if err := c.Put(ctx, doneJob(3)); err != nil {
			t.Fatal(err)
		}
		if _, found, err := c.Get(ctx, 3); err != nil || !found {
			t.Fatalf("get over TLS found=%v err=%v", found, err)
		}
	})
	t.Run("fixture CA is not a public root", func(t *testing.T) {
		c := jobcache.New(cfgFor(r)) // default roots: system/embedded public roots
		defer c.Close()
		assertTLSFailure(t, c)
	})
	t.Run("unrelated CA", func(t *testing.T) {
		c := jobcache.New(cfgFor(r), jobcache.WithRootCAs(testfixture.NewCA(t)))
		defer c.Close()
		assertTLSFailure(t, c)
	})
	t.Run("plain client to TLS server", func(t *testing.T) {
		cfg := cfgFor(r)
		cfg.TLS = false
		c := jobcache.New(cfg)
		defer c.Close()
		if err := c.Ping(ctx); err == nil {
			t.Fatal("plain text client must not talk to a TLS-only server")
		}
	})
}

func TestCacheTLSRejectsWrongHostname(t *testing.T) {
	// The certificate is valid for other.example only, but the client connects to
	// (and must verify) localhost.
	r := testfixture.StartRedis(t, testfixture.RedisOptions{TLS: true, CertNames: []string{"other.example"}})
	cfg := cfgFor(r)
	cfg.Host = "localhost"
	c := jobcache.New(cfg, jobcache.WithRootCAs(r.CA))
	defer c.Close()
	assertTLSFailure(t, c)
}

func assertTLSFailure(t *testing.T, c *jobcache.Cache) {
	t.Helper()
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("TLS verification must fail")
	}
	if got := jobcache.Classify(err); got != jobcache.ReasonTLS {
		t.Fatalf("reason = %q, want tls (%v)", got, err)
	}
}

func TestSourceNeverDisablesCertificateVerification(t *testing.T) {
	// A static guard next to the behavioral TLS tests above.
	for _, f := range []string{"cache.go", "config.go", "resolver.go"} {
		raw, err := readFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, "InsecureSkipVerify") {
			t.Fatalf("%s must not disable certificate verification", f)
		}
	}
}
