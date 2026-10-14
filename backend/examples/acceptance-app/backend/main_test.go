package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/examples/acceptance-app/internal/jobcache"
	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/examples/acceptance-app/internal/testfixture"
)

func TestAcceptanceChecks(t *testing.T) {
	hash := sha256.Sum256([]byte("do-not-print-me"))
	got := acceptanceChecks("acceptance-config-ok", "do-not-print-me", hex.EncodeToString(hash[:]), nil)
	for _, name := range []string{"environment", "secret", "database"} {
		if !got[name] {
			t.Errorf("%s should pass", name)
		}
	}
	got = acceptanceChecks("", "", "", errors.New("unavailable"))
	for _, name := range []string{"environment", "secret", "database"} {
		if got[name] {
			t.Errorf("%s should fail", name)
		}
	}
}

// app is the real backend handler on a real PostgreSQL and a real Redis.
type app struct {
	t        *testing.T
	store    *jobstore.Store
	redis    *testfixture.Redis
	cfg      jobcache.Config
	srv      *httptest.Server
	logs     *bytes.Buffer
	evidence *jobcache.Evidence
}

func newApp(t *testing.T, ttl time.Duration, mutate func(*jobcache.Config)) *app {
	t.Helper()
	return newAppPrepared(t, ttl, mutate, nil)
}

// newAppPrepared runs prepare against the started Redis (as its full-access
// user) before the backend's cache client is used, e.g. to create a limited ACL user.
func newAppPrepared(t *testing.T, ttl time.Duration, mutate func(*jobcache.Config), prepare func(*testfixture.Redis)) *app {
	t.Helper()
	ctx := context.Background()
	pg := testfixture.PostgresConfig(t)
	store, err := jobstore.Open(ctx, pg, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := testfixture.StartRedis(t, testfixture.RedisOptions{})
	if prepare != nil {
		prepare(r)
	}
	cfg := jobcache.Config{Host: r.Host, Port: r.Port, Username: r.Username, Password: r.Password, TLS: r.TLS,
		App: "acceptance", Env: "dev", Run: "run-01", TTL: ttl}
	if mutate != nil {
		mutate(&cfg)
	}
	cache := jobcache.New(cfg)
	t.Cleanup(func() { _ = cache.Close() })
	ev := jobcache.NewEvidence()
	logs := &bytes.Buffer{}
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	srv := httptest.NewServer(newMux(store, pg, cache, cfg, jobcache.NewResolver(cache, store, ev), ev))
	t.Cleanup(srv.Close)
	return &app{t: t, store: store, redis: r, cfg: cfg, srv: srv, logs: logs, evidence: ev}
}

// workerComplete mirrors the worker: commit to PostgreSQL, then write through.
func (a *app) workerComplete() jobstore.Job {
	a.t.Helper()
	job, claimed, err := a.store.ClaimAndProcess(context.Background(), "worker-1", func(p string) string { return "processed:" + strings.ToUpper(p) })
	if err != nil || !claimed {
		a.t.Fatalf("claim: %v %v", claimed, err)
	}
	cache := jobcache.New(a.cfg)
	defer cache.Close()
	if err := cache.Put(context.Background(), job); err != nil {
		a.t.Fatalf("write-through: %v", err)
	}
	if err := a.store.RecordCacheWrite(context.Background(), job.ID, true); err != nil {
		a.t.Fatal(err)
	}
	return job
}

func (a *app) do(method, path, body string) (int, map[string]any, string) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	return resp.StatusCode, decoded, string(raw)
}

func (a *app) submit(payload string) int64 {
	a.t.Helper()
	status, body, raw := a.do("POST", "/api/jobs", `{"payload":"`+payload+`"}`)
	if status != http.StatusCreated {
		a.t.Fatalf("submit: %d %s", status, raw)
	}
	return int64(body["id"].(float64))
}

func (a *app) jobs() int64 {
	n, err := a.store.CountJobs(context.Background())
	if err != nil {
		a.t.Fatal(err)
	}
	return n
}

func (a *app) assertSafe(bodies ...string) {
	a.t.Helper()
	all := a.logs.String() + strings.Join(bodies, "\n")
	for _, secret := range []string{a.cfg.Password, "WRONGPASS", "dial tcp", "connection refused", "127.0.0.1:" + itoa(a.cfg.Port)} {
		if secret != "" && strings.Contains(all, secret) {
			a.t.Fatalf("a response or log leaks %q", secret)
		}
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func cacheCheck(body map[string]any) bool { return body["checks"].(map[string]any)["cache"].(bool) }

func TestBackendCacheHitMissRefillThroughTheAPI(t *testing.T) {
	a := newApp(t, time.Second, nil)
	id := a.submit("hello")
	// Pending: served from PostgreSQL and never cached.
	status, body, _ := a.do("GET", "/api/jobs/"+itoa(int(id)), "")
	if status != 200 || body["status"] != "PENDING" || body["cache"] != "pending" {
		t.Fatalf("pending: %d %v", status, body)
	}
	if a.redis.Client(t).Exists(context.Background(), a.cfg.KeyFor(id)).Val() != 0 {
		t.Fatal("pending job cached")
	}
	job := a.workerComplete()

	_, body, _ = a.do("GET", "/api/jobs/"+itoa(int(id)), "")
	if body["cache"] != "hit" || body["result"] != job.Result || body["status"] != "DONE" {
		t.Fatalf("hit expected: %v", body)
	}
	// The list used by the existing UI resolves completed jobs through the cache too.
	_, list, _ := a.do("GET", "/api/jobs", "")
	first := list["jobs"].([]any)[0].(map[string]any)
	if first["cache"] != "hit" || first["result"] != job.Result || first["payload"] != "hello" {
		t.Fatalf("list should hit: %v", first)
	}

	time.Sleep(1300 * time.Millisecond) // TTL expiry
	_, list, _ = a.do("GET", "/api/jobs", "")
	first = list["jobs"].([]any)[0].(map[string]any)
	if first["cache"] != "miss-refilled" || first["result"] != job.Result || first["processedBy"] != "worker-1" {
		t.Fatalf("miss -> same PostgreSQL result -> refill expected: %v", first)
	}
	if a.redis.Client(t).Exists(context.Background(), a.cfg.KeyFor(id)).Val() != 1 {
		t.Fatal("refill missing")
	}

	_, checks, raw := a.do("GET", "/api/checks", "")
	evidence := checks["cache"].(map[string]any)
	lookups := evidence["lookups"].(map[string]any)
	if !cacheCheck(checks) || lookups["hit"].(float64) < 2 || lookups["miss-refilled"].(float64) != 1 || evidence["ttlSeconds"].(float64) != 1 || evidence["scope"] != "acceptance/dev/run-01" {
		t.Fatalf("evidence: %v", evidence)
	}
	a.assertSafe(raw)
	if a.jobs() != 1 {
		t.Fatalf("expected exactly one job, got %d", a.jobs())
	}
}

func TestBackendRedisOutageFailsDiagnosticsButKeepsDurableResult(t *testing.T) {
	a := newApp(t, 30*time.Second, nil)
	id := a.submit("outage")
	job := a.workerComplete()
	a.redis.Stop()

	status, body, raw := a.do("GET", "/api/jobs/"+itoa(int(id)), "")
	if status != 200 || body["result"] != job.Result || body["cache"] != "error-db-fallback" || body["cacheReason"] != "unreachable" {
		t.Fatalf("%d %v", status, body)
	}
	_, checks, raw2 := a.do("GET", "/api/checks", "")
	if cacheCheck(checks) || !checks["checks"].(map[string]any)["database"].(bool) {
		t.Fatalf("cache must fail while the database passes: %v", checks)
	}
	if checks["cache"].(map[string]any)["reachable"] != false || checks["cache"].(map[string]any)["reason"] != "unreachable" {
		t.Fatalf("%v", checks["cache"])
	}
	if status, _, _ := a.do("GET", "/readyz", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("readyz = %d, want 503", status)
	}
	if _, list, _ := a.do("GET", "/api/jobs", ""); list["jobs"].([]any)[0].(map[string]any)["result"] != job.Result {
		t.Fatal("list must still show the durable result")
	}
	a.assertSafe(raw, raw2)
	if a.jobs() != 1 {
		t.Fatal("no implicit resubmit")
	}
}

func TestBackendWrongRedisPasswordCannotPass(t *testing.T) {
	a := newApp(t, 30*time.Second, func(c *jobcache.Config) { c.Password = "definitely-wrong" })
	a.submit("auth")
	_, checks, raw := a.do("GET", "/api/checks", "")
	if cacheCheck(checks) || checks["cache"].(map[string]any)["reason"] != "auth" {
		t.Fatalf("%v", checks)
	}
	if status, body, _ := a.do("GET", "/readyz", ""); status != http.StatusServiceUnavailable || body["status"] != "cache unavailable" {
		t.Fatalf("readyz %d %v", status, body)
	}
	a.assertSafe(raw)
	if strings.Contains(raw, "definitely-wrong") || strings.Contains(a.logs.String(), "definitely-wrong") {
		t.Fatal("password leaked")
	}
}

func TestBackendWrongBindingCannotPass(t *testing.T) {
	ctx := context.Background()
	store, err := jobstore.Open(ctx, testfixture.PostgresConfig(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// An unparsable binding leaves the backend running without a cache client.
	_, cfgErr := jobcache.ConfigFromEnv(func(string) string { return "" })
	if cfgErr == nil {
		t.Fatal("empty binding must be rejected")
	}
	ev := jobcache.NewEvidence()
	srv := httptest.NewServer(newMux(store, jobstore.Config{}, nil, jobcache.Config{}, jobcache.NewResolver(nil, store, ev), ev))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/checks")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var checks map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&checks)
	if cacheCheck(checks) || checks["cache"].(map[string]any)["reason"] != "config" || checks["cache"].(map[string]any)["configured"] != false {
		t.Fatalf("%v", checks)
	}
	if !checks["checks"].(map[string]any)["database"].(bool) {
		t.Fatal("database check is independent and must pass")
	}
	if r, _ := http.Get(srv.URL + "/readyz"); r.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz %d", r.StatusCode)
	}
}

func TestBackendWorkerWriteFailureStaysVisibleAfterRedisRecovers(t *testing.T) {
	a := newApp(t, 30*time.Second, nil)
	id := a.submit("after-commit")
	a.redis.Stop()
	// The worker commits first; the write-through then fails.
	job, claimed, err := a.store.ClaimAndProcess(context.Background(), "worker-1", strings.ToUpper)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	cache := jobcache.New(a.cfg)
	if err := cache.Put(context.Background(), job); err == nil {
		t.Fatal("write-through must fail while Redis is down")
	}
	_ = cache.Close()
	if err := a.store.RecordCacheWrite(context.Background(), job.ID, false); err != nil {
		t.Fatal(err)
	}
	// The committed result is exactly one completed job and is not reprocessed.
	if _, again, _ := a.store.ClaimAndProcess(context.Background(), "worker-2", strings.ToLower); again {
		t.Fatal("completed job processed again")
	}
	a.redis.Restart()
	status, body, _ := a.do("GET", "/api/jobs/"+itoa(int(id)), "")
	if status != 200 || body["status"] != "DONE" || body["result"] != job.Result || body["processedBy"] != "worker-1" {
		t.Fatalf("committed result must survive: %d %v", status, body)
	}
	_, checks, _ := a.do("GET", "/api/checks", "")
	writes := checks["cache"].(map[string]any)["workerWrites"].(map[string]any)
	if cacheCheck(checks) || writes["failed"].(float64) != 1 {
		t.Fatalf("a failed write-through must keep the cache check failed even after Redis recovers: %v", checks)
	}
	// Redis answers again, yet readiness must still fail on the durable FAILED marker.
	if checks["cache"].(map[string]any)["reachable"] != true {
		t.Fatalf("redis should be reachable again: %v", checks["cache"])
	}
	status, ready, rawReady := a.do("GET", "/readyz", "")
	if status != http.StatusServiceUnavailable || ready["status"] != "cache write-through not verified" {
		t.Fatalf("readyz = %d %v", status, ready)
	}
	// Reads that refill the cache do not erase the recorded failure.
	a.do("GET", "/api/jobs/"+itoa(int(id)), "")
	if _, again, _ := a.do("GET", "/api/checks", ""); cacheCheck(again) {
		t.Fatal("a refill must not clear a FAILED marker")
	}
	a.assertSafe(rawReady)
	if a.jobs() != 1 {
		t.Fatal("exactly one job expected")
	}
}

func TestBackendBadRequestsDoNotTouchTheCache(t *testing.T) {
	a := newApp(t, 30*time.Second, nil)
	if status, _, _ := a.do("GET", "/api/jobs/abc", ""); status != 400 {
		t.Fatalf("status %d", status)
	}
	if status, _, _ := a.do("GET", "/api/jobs/999", ""); status != 404 {
		t.Fatalf("status %d", status)
	}
	if status, _, _ := a.do("POST", "/api/jobs", `{"payload":"  "}`); status != 400 {
		t.Fatalf("status %d", status)
	}
}

func TestBackendUnmarkedCompletedJobIsNotVerifiedUntilMarked(t *testing.T) {
	a := newApp(t, 30*time.Second, nil)
	a.submit("crash-before-marker")
	// Commit and write through, but the worker "crashes" before RecordCacheWrite.
	job, claimed, err := a.store.ClaimAndProcess(context.Background(), "worker-1", strings.ToUpper)
	if err != nil || !claimed {
		t.Fatal(err)
	}
	cache := jobcache.New(a.cfg)
	if err := cache.Put(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	_ = cache.Close()

	check := func(wantPass bool, wantUnverified, wantOK float64) {
		t.Helper()
		_, checks, raw := a.do("GET", "/api/checks", "")
		writes := checks["cache"].(map[string]any)["workerWrites"].(map[string]any)
		if cacheCheck(checks) != wantPass || writes["unverified"].(float64) != wantUnverified || writes["ok"].(float64) != wantOK {
			t.Fatalf("pass=%v writes=%v", cacheCheck(checks), writes)
		}
		status, _, _ := a.do("GET", "/readyz", "")
		if (status == http.StatusOK) != wantPass {
			t.Fatalf("readyz = %d, want pass=%v", status, wantPass)
		}
		a.assertSafe(raw)
	}
	check(false, 1, 0) // Redis is healthy and even holds the entry, but no write is proven.
	if err := a.store.RecordCacheWrite(context.Background(), job.ID, true); err != nil {
		t.Fatal(err)
	}
	check(true, 0, 1)
	// A new completed job without a marker fails the check again.
	a.submit("second")
	second, _, _ := a.store.ClaimAndProcess(context.Background(), "worker-1", strings.ToUpper)
	check(false, 1, 1)
	_ = a.store.RecordCacheWrite(context.Background(), second.ID, true)
	check(true, 0, 2)
	// Pending jobs are not completed work and do not affect the check.
	a.submit("pending-only")
	check(true, 0, 2)
}

// limitedUser is an authenticated account that may PING but is restricted on
// the keys/commands real lookups use, so only an actual operation reveals it.
const limitedPassword = "limited-secret-pw"

func limitedACL(t *testing.T, r *testfixture.Redis, rules ...string) {
	t.Helper()
	args := append([]any{"ACL", "SETUSER", "limited", "reset", "on", ">" + limitedPassword, "+ping", "+auth", "+hello", "+del", "+set", "+get"}, anyStrings(rules)...)
	if err := r.Client(t).Do(context.Background(), args...).Err(); err != nil {
		t.Fatalf("ACL SETUSER: %v", err)
	}
}

func anyStrings(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func useLimited(c *jobcache.Config) { c.Username, c.Password = "limited", limitedPassword }

const (
	probeKeys = "~acceptance:v1:acceptance:dev:run-01:probe:*"
	jobKeys   = "acceptance:v1:acceptance:dev:run-01:job:*"
)

// completeAsFullAccessWorker commits a job and writes it through with the
// full-access user, then removes the entry so the backend must use Redis itself.
func (a *app) completeAndEvict(t *testing.T, payload string) (int64, jobstore.Job) {
	id := a.submit(payload)
	full := a.cfg
	full.Username, full.Password = a.redis.Username, a.redis.Password
	job, claimed, err := a.store.ClaimAndProcess(context.Background(), "worker-1", func(p string) string { return "processed:" + strings.ToUpper(p) })
	if err != nil || !claimed {
		t.Fatal(err)
	}
	c := jobcache.New(full)
	defer c.Close()
	if err := c.Put(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := a.store.RecordCacheWrite(context.Background(), job.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := a.redis.Client(t).Del(context.Background(), full.KeyFor(job.ID)).Err(); err != nil {
		t.Fatal(err)
	}
	return id, job
}

func TestBackendDeniedCacheOperationFailsDiagnosticsWhilePingAndProbeWork(t *testing.T) {
	cases := map[string]struct {
		rules    []string
		wantKind string
	}{
		// GET allowed, SET on job keys denied: the refill after a miss fails.
		"refill denied": {[]string{probeKeys, "%R~" + jobKeys}, "refill"},
		// GET on job keys denied, SET allowed: the cache read fails.
		"read denied": {[]string{probeKeys, "%W~" + jobKeys}, "read"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAppPrepared(t, 30*time.Second, useLimited, func(r *testfixture.Redis) { limitedACL(t, r, tc.rules...) })
			id, job := a.completeAndEvict(t, "acl")
			path := "/api/jobs/" + itoa(int(id))

			// Before any lookup: PING, the probe and the worker markers are all fine.
			if _, checks, _ := a.do("GET", "/api/checks", ""); !cacheCheck(checks) {
				t.Fatalf("precondition: nothing has failed yet: %v", checks)
			}
			// The real lookup is denied but the durable result is still shown.
			status, body, raw := a.do("GET", path, "")
			if status != 200 || body["result"] != job.Result || body["status"] != "DONE" || body["cacheReason"] != "auth" {
				t.Fatalf("durable result with a reported auth failure expected: %d %v", status, body)
			}
			if tc.wantKind == "refill" && body["cache"] != "miss-refill-failed" {
				t.Fatalf("cache = %v", body["cache"])
			}
			if tc.wantKind == "read" && body["cache"] != "error-db-fallback" {
				t.Fatalf("cache = %v", body["cache"])
			}
			for i := 0; i < 2; i++ { // stays failed; diagnostics never turn green by themselves
				_, checks, rawChecks := a.do("GET", "/api/checks", "")
				c := checks["cache"].(map[string]any)
				fs, _ := c["unrecoveredFailures"].([]any)
				var f map[string]any
				if len(fs) == 1 {
					f, _ = fs[0].(map[string]any)
				}
				if cacheCheck(checks) || c["reason"] != "auth" || f == nil || f["kind"] != tc.wantKind || f["jobId"].(float64) != float64(id) {
					t.Fatalf("a denied real operation must fail the check: %v", c)
				}
				if !checks["checks"].(map[string]any)["database"].(bool) {
					t.Fatal("database check is independent")
				}
				if status, ready, _ := a.do("GET", "/readyz", ""); status != http.StatusServiceUnavailable || ready["status"] != "cache unavailable" {
					t.Fatalf("readyz = %d %v", status, ready)
				}
				a.assertSafe(rawChecks)
			}
			a.assertSafe(raw)
			if strings.Contains(raw+a.logs.String(), limitedPassword) || strings.Contains(raw, "NOPERM") {
				t.Fatal("secret or raw error leaked")
			}

			// Recovery: once the operation is permitted again the next diagnostics
			// re-attempt it and clear the failure, without user traffic and
			// without touching the job.
			limitedACL(t, a.redis, "~*", "&*", "+@all")
			_, checks, _ := a.do("GET", "/api/checks", "")
			if !cacheCheck(checks) {
				t.Fatalf("expected recovery after the permission is restored: %v", checks["cache"])
			}
			if status, _, _ := a.do("GET", "/readyz", ""); status != http.StatusOK {
				t.Fatalf("readyz = %d", status)
			}
			if a.jobs() != 1 {
				t.Fatal("recovery must not create or reprocess jobs")
			}
		})
	}
}

func TestBackendPingAllowedButProbeDeniedFails(t *testing.T) {
	a := newAppPrepared(t, 30*time.Second, useLimited, func(r *testfixture.Redis) {
		// +ping only: PING succeeds, SET/GET are not permitted at all.
		if err := r.Client(t).Do(context.Background(), "ACL", "SETUSER", "limited", "reset", "on", ">"+limitedPassword, "+ping", "+auth", "+hello", "~*", "&*").Err(); err != nil {
			t.Fatal(err)
		}
	})
	_, checks, raw := a.do("GET", "/api/checks", "")
	if cacheCheck(checks) || checks["cache"].(map[string]any)["reason"] != "auth" {
		t.Fatalf("%v", checks["cache"])
	}
	if status, _, _ := a.do("GET", "/readyz", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("readyz %d", status)
	}
	a.assertSafe(raw)
}

func TestBackendHealthyLookupOfOtherJobDoesNotHideDeniedJobs(t *testing.T) {
	const prefix = "acceptance:v1:acceptance:dev:run-01:job:"
	// Key-specific ACLs: job 2 is fully usable; jobs 1 and 3 are denied in the
	// operation under test (read: only SET allowed; refill: only GET allowed).
	for name, deny := range map[string]struct{ mode, kind string }{
		"read denied":   {"%W~", "read"},
		"refill denied": {"%R~", "refill"},
	} {
		t.Run(name, func(t *testing.T) {
			rules := func(allowFull ...int) []string {
				r := []string{probeKeys}
				for _, id := range []int{1, 2, 3} {
					full := id == 2
					for _, a := range allowFull {
						full = full || a == id
					}
					if full {
						r = append(r, "~"+prefix+itoa(id))
					} else {
						r = append(r, deny.mode+prefix+itoa(id))
					}
				}
				return r
			}
			a := newAppPrepared(t, 30*time.Second, useLimited, func(r *testfixture.Redis) { limitedACL(t, r, rules()...) })
			ids := make([]int64, 3)
			for i := range ids {
				ids[i], _ = a.completeAndEvict(t, "job")
			}
			get := func(id int64) map[string]any {
				_, body, _ := a.do("GET", "/api/jobs/"+itoa(int(id)), "")
				return body
			}
			failures := func() []int64 {
				_, checks, raw := a.do("GET", "/api/checks", "")
				a.assertSafe(raw)
				var out []int64
				fs, _ := checks["cache"].(map[string]any)["unrecoveredFailures"].([]any)
				for _, f := range fs {
					m := f.(map[string]any)
					if m["kind"] != deny.kind || m["reason"] != "auth" {
						t.Fatalf("unexpected failure %v", m)
					}
					out = append(out, int64(m["jobId"].(float64)))
				}
				if (len(out) == 0) != cacheCheck(checks) {
					t.Fatalf("check must pass exactly when no failure remains: %v", checks["cache"])
				}
				return out
			}
			ready := func() int { s, _, _ := a.do("GET", "/readyz", ""); return s }

			get(ids[0]) // A denied
			// B is healthy (miss -> refill, then hit) but must not erase A.
			if b := get(ids[1]); b["cache"] != "miss-refilled" {
				t.Fatalf("B = %v", b)
			}
			if b := get(ids[1]); b["cache"] != "hit" {
				t.Fatalf("B = %v", b)
			}
			if got := failures(); len(got) != 1 || got[0] != ids[0] || ready() != http.StatusServiceUnavailable {
				t.Fatalf("A must stay failed after B succeeded: %v ready=%d", got, ready())
			}
			// A second denied job must not overwrite or forget the first.
			get(ids[2])
			get(ids[1])
			if got := failures(); len(got) != 2 || got[0] != ids[0] || got[1] != ids[2] {
				t.Fatalf("both A and C must be remembered: %v", got)
			}
			if ready() != http.StatusServiceUnavailable {
				t.Fatal("readiness must fail")
			}
			// Diagnostics re-attempt the exact failing operations; they stay failed while denied.
			if got := failures(); len(got) != 2 {
				t.Fatalf("recheck must not clear while still denied: %v", got)
			}
			// Only the actual recovery of A clears A; C keeps the check failing.
			limitedACL(t, a.redis, rules(1)...)
			if got := failures(); len(got) != 1 || got[0] != ids[2] || ready() != http.StatusServiceUnavailable {
				t.Fatalf("only C should remain: %v", got)
			}
			limitedACL(t, a.redis, rules(1, 3)...)
			if got := failures(); len(got) != 0 || ready() != http.StatusOK {
				t.Fatalf("all recovered: %v", got)
			}
			if a.jobs() != 3 {
				t.Fatalf("jobs = %d, recovery must not create or reprocess jobs", a.jobs())
			}
			for _, id := range ids { // durable results were shown throughout
				if get(id)["status"] != "DONE" {
					t.Fatal("durable result must be shown")
				}
			}
		})
	}
}
