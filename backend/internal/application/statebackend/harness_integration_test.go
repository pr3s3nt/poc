package statebackend_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/application/statebackend"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/ports/terraformstate"
)

// fixture is one disposable option-A PostgreSQL database with seeded
// Organizations, Environments and one verified module bundle row. Claims and
// product data go through the Console login (store); the state backend and
// the Terraform repository run on the separate State login (state). The
// administrator URL (url) only arranges and inspects fixtures.
type fixture struct {
	t      *testing.T
	db     persistencetest.PostgresDatabase
	url    string
	store  *postgres.Store
	state  *postgres.StateStore
	cipher *statecipher.AESGCM
	svc    *statebackend.Service
	base   string // loopback state backend base URL (as given to Terraform)
	bundle string
	appKey string // Organization "acme": Environments "staging" and "prod"
	other  string // Organization "globex": Environment "staging"
}

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, statecipher.KeySize) }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := postgrestest.Fresh(t)
	ctx := context.Background()
	st, err := postgres.Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	state, err := postgres.OpenState(ctx, db.State)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.Close)
	if st.Identity() != state.Identity() {
		t.Fatal("console and state connections serve different installations")
	}
	f := &fixture{t: t, db: db, url: db.Admin, store: st, state: state}
	f.appKey = f.seed("acme", "staging", "prod")
	f.other = f.seed("globex", "staging")
	f.bundle = ids.New()
	f.exec(`INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES($1::uuid,'platform','redis','0123456789abcdef0123456789abcdef01234567','.',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`, f.bundle)
	f.cipher, err = statecipher.New("key-1", testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	f.svc = f.service(state, f.cipher)
	f.base = f.serve(f.svc)
	return f
}

// serve starts the production loopback listener for svc.
func (f *fixture) serve(svc *statebackend.Service) string {
	f.t.Helper()
	l, err := deliveryhttp.ServeTerraformStateLoopback(deliveryhttp.NewTerraformStateHandler(svc))
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.Close(ctx)
	})
	return l.Address
}

func (f *fixture) service(repo terraformstate.Repository, c terraformstate.Cipher) *statebackend.Service {
	svc, err := statebackend.New(repo, c)
	if err != nil {
		f.t.Fatal(err)
	}
	return svc
}

func (f *fixture) seed(org string, envs ...string) string {
	f.t.Helper()
	ctx := context.Background()
	if _, err := f.store.GetOrganization(ctx, org); err != nil {
		if err := f.store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: org, Name: org}); err != nil {
			f.t.Fatal(err)
		}
	}
	appID := ids.New()
	if err := f.store.SaveApplication(ctx, application.Application{ID: appID, Key: appID, OrganizationKey: org, Name: "App " + appID[:8], Subdomain: "s-" + appID[:8], Version: 1}); err != nil {
		f.t.Fatal(err)
	}
	for _, env := range envs {
		if err := f.store.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), Key: env, ApplicationID: appID, ApplicationKey: appID, Name: env, Type: env, NamespaceIdentity: "ns-" + appID[:8] + "-" + env, Version: 1}); err != nil {
			f.t.Fatal(err)
		}
	}
	return appID
}

func (f *fixture) conn() *pgx.Conn {
	f.t.Helper()
	c, err := pgx.Connect(context.Background(), f.url)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { c.Close(context.Background()) })
	return c
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.conn().Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("exec: %v", err)
	}
}

func (f *fixture) execErr(sql string, args ...any) error {
	_, err := f.conn().Exec(context.Background(), sql, args...)
	return err
}

func (f *fixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.conn().QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		f.t.Fatalf("count: %v", err)
	}
	return n
}

// claim takes the Environment claim and returns the owner context.
func (f *fixture) claim(st persistence.Store, app, env, owner string) (context.Context, environment.Operation) {
	f.t.Helper()
	op, err := st.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: app, EnvironmentKey: env, Owner: owner, Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		f.t.Fatalf("claim: %v", err)
	}
	return ownerCtx(op), op
}

func ownerCtx(op environment.Operation) context.Context {
	return persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
}

func (f *fixture) instance(ctx context.Context, descriptor, run string) tfresource.Instance {
	f.t.Helper()
	inst, err := f.state.RegisterInstance(ctx, terraformstate.NewInstance{Descriptor: descriptor, ScopeType: "SHARED", ScopeID: descriptor, ModuleBundleID: f.bundle, TargetRef: map[string]any{"connection": "k8s-4f", "namespace": "orch-v2-test"}, PhysicalIntent: map[string]any{"name": "redis-stable"}, CreatedByRunID: run})
	if err != nil {
		f.t.Fatalf("register: %v", err)
	}
	return inst
}

func (f *fixture) attempt(ctx context.Context, inst tfresource.Instance, run string, action tfresource.Action, ownership tfresource.Ownership) (tfresource.Attempt, error) {
	return f.state.BeginAttempt(ctx, tfresource.Attempt{InstanceID: inst.ID, RunID: run, Action: action, Ownership: ownership, Stage: "PLANNED", ModuleSnapshot: map[string]any{"bundle": f.bundle, "revision": "0123456789abcdef"}, TargetSnapshot: map[string]any{"connection": "k8s-4f"}, Deadline: time.Now().Add(time.Hour)})
}

func (f *fixture) token(ctx context.Context, inst tfresource.Instance, action tfresource.CapabilityAction) string {
	f.t.Helper()
	tok, _, err := f.svc.IssueCapability(ctx, inst.ID, action, time.Hour)
	if err != nil {
		f.t.Fatalf("issue capability: %v", err)
	}
	return tok
}

// call performs one Terraform HTTP backend request against base.
func call(t *testing.T, base, method, stateKey, token, lockQuery string, body []byte) (int, []byte) {
	t.Helper()
	url := base + stateKey
	if lockQuery != "" {
		url += "?ID=" + lockQuery
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.SetBasicAuth("executor", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func lockBody(id string) []byte {
	return []byte(`{"ID":"` + id + `","Operation":"OperationTypeApply","Info":"","Who":"runner@host-secret","Version":"1.9.8","Created":"2026-10-13T00:00:00Z","Path":"/home/secret/path"}`)
}

func stateDoc(lineage string, serial int, marker string) []byte {
	return []byte(`{"version":4,"terraform_version":"1.9.8","serial":` + strconv.Itoa(serial) + `,"lineage":"` + lineage + `","outputs":{},"resources":[{"mode":"managed","type":"kubernetes_secret","name":"p","instances":[{"attributes":{"data":{"password":"` + marker + `"}}}]}]}`)
}
