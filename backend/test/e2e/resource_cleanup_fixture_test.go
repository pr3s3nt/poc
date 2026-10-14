package e2e

// Fixtures of the I04-08 resource detail / confirmed deletion tests.
//
// REAL: the HTTP server, sessions and roles, the Console and State PostgreSQL
// logins (a disposable three-role database), the admission transaction and the
// persisted claim.
//
// FAKE (and labelled so): the cleanup engine below. It stands in for
// tfexec.RunCleanup so the admission, the HTTP contract and the persisted
// projections can be exercised without Terraform. It is NOT evidence of a live
// deletion: the real engine over a real Terraform CLI is exercised in
// internal/application/tfexec (TestResourceCleanupEndToEnd...) and live
// deletion proof belongs to A01.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/ports/terraformstate"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// Markers that must never reach a response: they are planted in the private
// columns (target_ref, physical_intent, attempt snapshots) of every instance.
const (
	privateTarget = "PRIVATE-TARGET-MARKER-7f3a"
	privateIntent = "PRIVATE-INTENT-MARKER-91c2"
	privateRecov  = "PRIVATE-RECOVERY-MARKER-4d8e"
)

// scriptedEngine is the FAKE cleanup engine. Per descriptor it can destroy
// (default), fail with remaining objects, fail with unknown remaining, or hold.
type scriptedEngine struct {
	t     *testing.T
	state *postgres.StateStore

	mu     sync.Mutex
	calls  int
	script map[string]string // descriptor -> "fail" | "unknown" | "hold"
	// gate, when set, is closed by the test to let a run proceed.
	gate chan struct{}
	// holdStart, when set, blocks the engine before it reports anything, so
	// the test sees exactly what the admission itself committed.
	holdStart chan struct{}
	// started is signalled when a run entered the engine.
	started chan struct{}
}

func (e *scriptedEngine) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func (e *scriptedEngine) RunCleanup(ctx context.Context, _ string, source tfexec.CleanupSource, report tfexec.StageReporter) (tfexec.CleanupResult, error) {
	e.mu.Lock()
	e.calls++
	gate, started, script, holdStart := e.gate, e.started, e.script, e.holdStart
	e.mu.Unlock()
	if holdStart != nil {
		<-holdStart
	}
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	owner, _ := persistence.OwnerFrom(ctx)
	_ = report(tfexec.CleanupRequested, nil)
	req, err := source.CleanupRequest(ctx, owner.OperationID)
	if err != nil {
		return tfexec.CleanupResult{}, err
	}
	_ = report(tfexec.CleanupChecking, nil)
	if gate != nil {
		<-gate
	}
	out := tfexec.CleanupResult{Terminal: tfexec.CleanupSucceeded, Stage: tfexec.CleanupVerifying}
	held, failed := false, false
	// The root consumes its providers, so it goes first.
	order := []string{req.RootInstanceID}
	for _, id := range req.Scope.SelectedResourceIDs {
		if id != req.RootInstanceID {
			order = append(order, id)
		}
	}
	for _, id := range order {
		inst, err := e.state.GetInstance(ctx, id)
		if err != nil {
			return tfexec.CleanupResult{}, err
		}
		res := tfexec.InstanceResult{InstanceID: id}
		mode := script[inst.Identity.Descriptor]
		switch {
		case held:
			res.Reason, res.Code = "not attempted", tfexec.CodeNotAttempted
		case failed && inst.Identity.Descriptor != "" && mode == "":
			// A provider of a failed consumer is retained.
			res.Reason, res.Code = "retained", tfexec.CodeRetainedConsumer
			res.Remaining = []string{"fake_resource.kept=" + id[:8]}
		case mode == "fail":
			failed = true
			res.Code = tfexec.CodeDestroyFailed
			res.Remaining = []string{"fake_resource.main=obj-" + id[:8], "fake_resource.<script>alert(1)</script>"}
		case mode == "unknown":
			failed = true
			res.Code, res.RemainingUnknown = tfexec.CodeDestroyFailed, true
		case mode == "hold":
			held = true
			res.Code, res.RemainingUnknown = tfexec.CodeNeedsAttention, true
			persistence.MarkHold(ctx)
		default:
			_ = report(tfexec.CleanupDestroying, map[string]any{"resource": id})
			cur, err := e.state.SetInstanceStatus(ctx, id, inst.Version, tfresource.InstanceDestroying)
			if err != nil {
				return tfexec.CleanupResult{}, err
			}
			_ = report(tfexec.CleanupVerifying, map[string]any{"resource": id})
			// The persisted proof of THIS operation: a DESTROY attempt bound to it.
			attempt, err := e.state.BeginAttempt(ctx, tfresource.Attempt{InstanceID: id, RunID: "cleanup-" + owner.OperationID, Action: tfresource.ActionDestroy, Ownership: tfresource.Preexisting, Stage: tfexec.StageVerifying,
				ModuleSnapshot: map[string]any{"action": "DESTROY"}, TargetSnapshot: map[string]any{"fake": true}, Deadline: time.Now().Add(time.Hour)})
			if err != nil {
				return tfexec.CleanupResult{}, err
			}
			if _, err := e.state.UpdateAttempt(ctx, attempt.ID, tfresource.AttemptProgress{Stage: tfexec.StageDestroyed, Status: tfresource.AttemptSucceeded}); err != nil {
				return tfexec.CleanupResult{}, err
			}
			if _, err := e.state.SetInstanceStatus(ctx, id, cur.Version, tfresource.InstanceDestroyed); err != nil {
				return tfexec.CleanupResult{}, err
			}
			res.Destroyed = true
		}
		out.Instances = append(out.Instances, res)
	}
	switch {
	case held:
		out.Terminal = tfexec.CleanupNeedsAttention
	case failed:
		out.Terminal = tfexec.CleanupFailed
	}
	for _, r := range out.Instances {
		if !r.Destroyed && out.Terminal == tfexec.CleanupSucceeded {
			out.Terminal = tfexec.CleanupFailed
		}
	}
	return out, nil
}

// rcFixture is one PostgreSQL-backed server with the deletion topology:
//
//	ROOT  postgres.default#shared.db          UNREFERENCED, consumes A, B, C and PLATFORM
//	A     vpc.default#environments.<app>.vpc    UNREFERENCED, consumed only by ROOT       (valid dependency)
//	B     k8s-namespace.default#environments.<app>.ns   UNREFERENCED, also consumed by X  (locked: retained consumer)
//	C     k8s-secret.default#environments.<app>.cfg     UNREFERENCED, consumed only by ROOT (valid, left unticked)
//	X     redis.default#shared.cache            READY consumer of B (outside the set)
//	PLAT  k8s-cluster.internal#connections.k8s-4f  platform provider of ROOT (never listed)
type rcFixture struct {
	t        *testing.T
	db       persistencetest.PostgresDatabase
	server   *httptest.Server
	app      *bootstrap.App
	state    *postgres.StateStore
	engine   *scriptedEngine
	seedOpts seed.Options
	appKey   string
	orgKey   string
	envID    string
	bundle   string

	dev, pe, foreign *http.Client

	root, a, b, c, x, plat tfresource.Instance
}

// fixtureOption adjusts the fixture before the application is built.
type fixtureOption func(*bootstrap.Options)

// withoutEngine builds the application as if the Terraform state, module,
// credential and engine services were not configured.
func withoutEngine(o *bootstrap.Options) { o.CleanupEngineOverride = nil }

func newRCFixture(t *testing.T, options ...fixtureOption) *rcFixture {
	t.Helper()
	db := postgrestest.Fresh(t)
	keyFile := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("ab", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := postgres.OpenState(context.Background(), db.State)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.Close)
	engine := &scriptedEngine{t: t, state: state, script: map[string]string{}}
	seedOpts := seedtest.Defaults()
	seedOpts.Region, seedOpts.AccountID, seedOpts.RunID = "us-east-1", "000000000000", "run-e2e"
	creds := credentialmemory.New()
	buildOptions := bootstrap.Options{
		Seed: seedOpts, Adapters: bootstrap.AdapterFake, ConnectionCredentialsOverride: creds,
		DatabaseURL: db.Console, TerraformStateDatabaseURL: db.State, TerraformStateKeyFile: keyFile, TerraformStateKeyID: "key-1",
		CleanupEngineOverride: engine,
	}
	for _, option := range options {
		option(&buildOptions)
	}
	app, err := bootstrap.Build(context.Background(), buildOptions)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(app.Close)
	prepareSeedCluster(t, app, creds, seedOpts)
	server := httptest.NewServer(app.Server)
	t.Cleanup(func() { app.ResourceCleanup.Wait(); server.Close() })

	f := &rcFixture{t: t, db: db, server: server, app: app, state: state, engine: engine, seedOpts: seedOpts, orgKey: seedOpts.OrganizationKey}
	f.dev = f.signIn("developer", seed.FixedTestPassword)
	f.pe = f.signIn("platform-engineer", seed.FixedTestPassword)
	// The Application (with its staging and production Environments) is made
	// through the product API, exactly as a user creates one (UC-01).
	if status, raw, _ := f.do(f.pe, http.MethodPost, server.URL+"/api/v1/applications", `{"name":"Payments","subdomain":"payments"}`); status != http.StatusCreated {
		t.Fatalf("create application = %d %s", status, raw)
	}
	apps, err := app.Store.ListApplications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.Subdomain == "payments" {
			f.appKey = a.Key
		}
	}
	env, err := app.Store.GetEnvironment(context.Background(), f.appKey, "staging")
	if err != nil {
		t.Fatalf("staging environment of the created application: %v", err)
	}
	f.envID = env.ID

	// A second Organization with its own Platform Engineer: a foreign caller.
	hash, err := password.Hash("foreign-password")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := app.Store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "globex", Name: "Globex"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Store.SaveUserAccount(ctx, identity.UserAccount{ID: ids.New(), OrganizationKey: "globex", Username: "globex-admin", PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive}); err != nil {
		t.Fatal(err)
	}
	f.foreign = f.signIn("globex-admin", "foreign-password")

	f.bundle = ids.New()
	f.exec(`INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES($1::uuid,'platform','fixture-module','0123456789abcdef0123456789abcdef01234567','.',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`, f.bundle)
	f.seedTopology()
	return f
}

func (f *rcFixture) signIn(username, pw string) *http.Client {
	f.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		f.t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"username": username, "password": pw})
	resp, err := client.Post(f.server.URL+"/api/v1/auth/sign-in", "application/json", bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.t.Fatalf("sign-in %s = %d", username, resp.StatusCode)
	}
	return client
}

func (f *rcFixture) conn() *pgx.Conn {
	f.t.Helper()
	c, err := pgx.Connect(context.Background(), f.db.Admin)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { c.Close(context.Background()) })
	return c
}

func (f *rcFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.conn().Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("exec: %v", err)
	}
}

func (f *rcFixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.conn().QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		f.t.Fatalf("count: %v", err)
	}
	return n
}

// claim takes a DEPLOY-kind Environment claim, the way a deployment would, so
// the State login may write instances and edges.
func (f *rcFixture) claim(envKey string) (context.Context, environment.Operation) {
	f.t.Helper()
	op, err := f.app.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: f.appKey, EnvironmentKey: envKey, Owner: "fixture", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		f.t.Fatalf("claim: %v", err)
	}
	return persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}), op
}

func (f *rcFixture) release(op environment.Operation) {
	f.t.Helper()
	if err := f.app.Store.ReleaseOperation(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpSucceeded, ""); err != nil {
		f.t.Fatalf("release: %v", err)
	}
}

// register adds one instance with planted private data and one finished attempt.
func (f *rcFixture) register(ctx context.Context, descriptor string, scope resource.ScopeType, status tfresource.InstanceStatus) tfresource.Instance {
	f.t.Helper()
	in, err := f.state.RegisterInstance(ctx, terraformstate.NewInstance{
		Descriptor: descriptor, ScopeType: string(scope), ScopeID: f.appKey + ".staging.fixture",
		ModuleBundleID: f.bundle, TargetRef: map[string]any{"connection": privateTarget, "namespace": privateTarget},
		PhysicalIntent: map[string]any{"name": privateIntent}, CreatedByRunID: "run-1",
	})
	if err != nil {
		f.t.Fatalf("register %s: %v", descriptor, err)
	}
	att, err := f.state.BeginAttempt(ctx, tfresource.Attempt{InstanceID: in.ID, RunID: "run-1", Action: tfresource.ActionCreate, Ownership: tfresource.CreatedByRun, Stage: "PLANNED",
		ModuleSnapshot: map[string]any{"bundle": f.bundle}, TargetSnapshot: map[string]any{"connection": privateTarget}, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		f.t.Fatalf("attempt: %v", err)
	}
	if _, err := f.state.UpdateAttempt(ctx, att.ID, tfresource.AttemptProgress{Stage: "READY", Status: tfresource.AttemptSucceeded, ObservedPhysicalIDs: []string{"obj-" + privateIntent}, RecoveryHandle: privateRecov}); err != nil {
		f.t.Fatal(err)
	}
	in, err = f.state.SetInstanceStatus(ctx, in.ID, in.Version, status)
	if err != nil {
		f.t.Fatal(err)
	}
	return in
}

func (f *rcFixture) consume(ctx context.Context, consumer tfresource.Instance, providers ...tfresource.Instance) tfresource.Instance {
	f.t.Helper()
	cur, err := f.state.GetInstance(ctx, consumer.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	var idsList []string
	for _, p := range providers {
		idsList = append(idsList, p.ID)
	}
	out, err := f.state.RecordDependencies(ctx, cur.ID, cur.Version, idsList)
	if err != nil {
		f.t.Fatalf("record dependencies: %v", err)
	}
	return out
}

func (f *rcFixture) seedTopology() {
	f.t.Helper()
	ctx, op := f.claim("staging")
	app := f.appKey
	f.plat = f.register(ctx, "k8s-cluster.internal#connections.k8s-4f", resource.ScopeApplication, tfresource.InstanceReady)
	f.a = f.register(ctx, "vpc.default#environments."+app+".vpc", resource.ScopeEnvironment, tfresource.InstanceUnreferenced)
	f.b = f.register(ctx, "k8s-namespace.default#environments."+app+".ns", resource.ScopeEnvironment, tfresource.InstanceUnreferenced)
	f.c = f.register(ctx, "k8s-secret.default#environments."+app+".cfg", resource.ScopeEnvironment, tfresource.InstanceUnreferenced)
	f.root = f.register(ctx, "postgres.default#shared.db", resource.ScopeShared, tfresource.InstanceUnreferenced)
	f.x = f.register(ctx, "redis.default#shared.cache", resource.ScopeShared, tfresource.InstanceReady)
	f.consume(ctx, f.root, f.a, f.b, f.c, f.plat)
	f.consume(ctx, f.x, f.b)
	f.release(op)
	// Recording edges advanced the consumers' versions: keep the current rows.
	for _, in := range []*tfresource.Instance{&f.plat, &f.a, &f.b, &f.c, &f.root, &f.x} {
		*in = f.refresh(*in)
	}
}

// refresh re-reads an instance as the State login sees it.
func (f *rcFixture) refresh(in tfresource.Instance) tfresource.Instance {
	f.t.Helper()
	out, err := f.state.GetInstance(context.Background(), in.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *rcFixture) base() string {
	return f.server.URL + "/api/v1/applications/" + f.appKey + "/environments/staging"
}

// do performs a request and returns status, raw body and the decoded object.
func (f *rcFixture) do(client *http.Client, method, url string, body any) (int, string, map[string]any) {
	f.t.Helper()
	var reader io.Reader
	switch v := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			f.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		f.t.Fatal(err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, string(raw), out
}

func (f *rcFixture) resourceURL(id string) string { return f.base() + "/resources/" + id }

func (f *rcFixture) preview(client *http.Client, id string) (int, string, map[string]any) {
	return f.do(client, http.MethodPost, f.resourceURL(id)+"/deletion-preview", map[string]any{})
}

func (f *rcFixture) previewToken(client *http.Client) string {
	f.t.Helper()
	status, raw, body := f.preview(client, f.root.ID)
	if status != http.StatusOK {
		f.t.Fatalf("preview = %d %s", status, raw)
	}
	return body["preview"].(map[string]any)["token"].(string)
}

func (f *rcFixture) deletion(client *http.Client, token, name string, selected ...string) (int, string, map[string]any) {
	return f.do(client, http.MethodPost, f.resourceURL(f.root.ID)+"/deletions", map[string]any{"token": token, "confirmationName": name, "selectedResourceIds": selected})
}

// rootName is the exact persisted confirmation name.
func (f *rcFixture) rootName() string { return f.root.Identity.Descriptor }

// assertSafe fails when a response carries any planted private marker or a
// field that only exists in private records.
func assertSafe(t *testing.T, label, body string, extra ...string) {
	t.Helper()
	forbidden := append([]string{privateTarget, privateIntent, privateRecov, "targetRef", "physicalIntent", "stateKey", "state_key", "recoveryHandle", "moduleSnapshot", "targetSnapshot", "observedPhysicalIds", "resolvedInputs", "secretRef", "kubeconfig"}, extra...)
	for _, bad := range forbidden {
		if strings.Contains(body, bad) {
			t.Errorf("%s leaks %q: %.400s", label, bad, body)
		}
	}
}

// waitOperation polls the cleanup operation until it leaves the active states.
func (f *rcFixture) waitOperation(client *http.Client, opID string) map[string]any {
	f.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		status, raw, body := f.do(client, http.MethodGet, f.base()+"/resource-cleanups/"+opID, nil)
		if status != http.StatusOK {
			f.t.Fatalf("cleanup status = %d %s", status, raw)
		}
		view := body["cleanup"].(map[string]any)
		if st := view["status"].(string); st != "ACTIVE" && st != "RECOVERING" {
			return view
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("operation did not finish: %v", view)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func idsOf(list ...tfresource.Instance) []string {
	out := make([]string, 0, len(list))
	for _, in := range list {
		out = append(out, in.ID)
	}
	return out
}

var _ = fmt.Sprintf

func rcJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// signInAdmin signs in a second Admin of the same Organization (another actor).
func (f *rcFixture) signInAdmin() *http.Client {
	f.t.Helper()
	hash, err := password.Hash("admin-password")
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.app.Store.GetUserAccountByUsername(context.Background(), "acme-admin"); err != nil {
		if err := f.app.Store.SaveUserAccount(context.Background(), identity.UserAccount{ID: ids.New(), OrganizationKey: f.orgKey, Username: "acme-admin", PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive}); err != nil {
			f.t.Fatal(err)
		}
	}
	return f.signIn("acme-admin", "admin-password")
}

// linkActive attaches an Active Resource with the given status to an instance
// (fixture arrangement through the administrator connection).
func (f *rcFixture) linkActive(in tfresource.Instance, status string) string {
	f.t.Helper()
	typeID, defID, activeID := ids.New(), ids.New(), ids.New()
	f.exec(`INSERT INTO resource_types(id,organization_id,resource_type_key,input_schema,output_schema) SELECT $1::uuid,o.id,'fixture-type','[]'::jsonb,'[{"name":"host","type":"string"},{"name":"password","type":"string","secret":true}]'::jsonb FROM organizations o WHERE o.organization_key=$2 ON CONFLICT DO NOTHING`, typeID, f.orgKey)
	f.exec(`INSERT INTO resource_definitions(id,organization_id,definition_key,resource_type_id,execution_profile,driver_type,driver_inputs,provision_rules) SELECT $1::uuid,o.id,'fixture-def',t.id,'internal-k8s','terraform','{}'::jsonb,'{}'::jsonb FROM organizations o JOIN resource_types t ON t.organization_id=o.id AND t.resource_type_key='fixture-type' WHERE o.organization_key=$2 ON CONFLICT DO NOTHING`, defID, f.orgKey)
	f.exec(`INSERT INTO active_resources(id,organization_id,descriptor,scope_type,scope_id,resource_definition_id,status,executor_state_ref,outputs,version,created_at,updated_at) SELECT $1::uuid,o.id,$3,$4,$5,d.id,$6,'{}'::jsonb,'{"host":"h","password":"hunter2"}'::jsonb,1,now(),now() FROM organizations o JOIN resource_definitions d ON d.organization_id=o.id AND d.definition_key='fixture-def' WHERE o.organization_key=$2`,
		activeID, f.orgKey, in.Identity.Descriptor, string(in.Identity.Scope.Type), in.Identity.Scope.ID, status)
	f.exec(`UPDATE terraform_resource_instances SET active_resource_id=$2::uuid WHERE id=$1::uuid`, in.ID, activeID)
	return activeID
}

func (f *rcFixture) activeStatus(activeID string) string {
	f.t.Helper()
	var status string
	if err := f.conn().QueryRow(context.Background(), `SELECT status FROM active_resources WHERE id=$1::uuid`, activeID).Scan(&status); err != nil {
		f.t.Fatal(err)
	}
	return status
}

// linkDeployment records a past run of the resource's Active Resource with the
// given Connection and target generation (fixture arrangement). The Connection
// row is created when missing.
func (f *rcFixture) linkDeployment(activeID, connectionKey, connectionName string, generation int64) string {
	f.t.Helper()
	f.exec(`INSERT INTO connections(id,organization_id,connection_key,name,kind,authentication_type,config,secret_ref,status,verification) SELECT gen_random_uuid(),o.id,$2,$3,'AWS','AWS_ACCESS_KEY',$4::jsonb,'vault:private/ref','READY','{}'::jsonb FROM organizations o WHERE o.organization_key=$1 ON CONFLICT DO NOTHING`,
		f.orgKey, connectionKey, connectionName, `{"accountId":"123456789012","region":"us-east-1","allowedRegions":["us-east-1","eu-west-1","ap-southeast-1"],"privateNote":"`+privateTarget+`"}`)
	depID := ids.New()
	f.exec(`INSERT INTO deployments(id,environment_id,organization_key,application_key,environment_key,execution_profile,action,workload_id,actor_ref,status,base_environment_version,started_at,connection_key,target_generation) VALUES($1::uuid,$2::uuid,$3,$4,'staging','aws-eks','DEPLOY','api','developer','FAILED',1,now(),$5,$6)`,
		depID, f.envID, f.orgKey, f.appKey, connectionKey, generation)
	f.exec(`UPDATE active_resources SET last_deployment_id=$2::uuid WHERE id=$1::uuid`, activeID, depID)
	return depID
}

// savePlanRaw stores a plan document whose target is the given raw JSON, to
// plant malformed stored metadata.
func (f *rcFixture) savePlanRaw(deploymentID, targetJSON string) {
	f.t.Helper()
	f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,$2::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, deploymentID, `{"target":`+targetJSON+`}`)
}

// savePlan stores the immutable plan document of a Deployment with the given
// pinned target, the way the planner persists it.
func (f *rcFixture) savePlan(deploymentID, connectionKey, region string, generation int64) {
	f.t.Helper()
	target := map[string]any{"connectionKey": connectionKey, "executionProfile": "aws-eks", "region": region, "infrastructureScope": "AWS_EKS"}
	if generation != 0 {
		target["generation"] = generation
	}
	doc, err := json.Marshal(map[string]any{"target": target})
	if err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,$2::jsonb,'plan-hash') ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document`, deploymentID, string(doc))
}
