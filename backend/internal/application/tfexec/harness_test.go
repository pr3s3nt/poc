package tfexec_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/postgres/postgrestest"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/application/tfexec"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

// The harness uses a REAL disposable PostgreSQL (three roles), the REAL
// encrypted state repository, the REAL internal HTTP state backend on
// loopback and the ACTUAL Terraform CLI through the real runner. Modules are
// hand-built fixtures using the built-in terraform_data resource (no provider
// download, no cloud or cluster); the provider binder and the readiness check
// are FAKES and say so in their names.

const moduleHCL = `
variable "note" { type = string }
variable "stable" { type = string }
variable "region" { type = string }
variable "tags" { type = map(string) }
variable "master_password" {
  type      = string
  sensitive = true
}
resource "terraform_data" "db" {
  input            = var.note
  triggers_replace = var.stable
}
resource "terraform_data" "pw" {
  input = var.master_password
}
output "host" { value = "db-${terraform_data.db.id}" }
output "port" { value = 5432 }
output "database" { value = var.note }
output "username" { value = "app" }
output "tls" { value = false }
output "password" {
  value     = var.master_password
  sensitive = true
}
`

func postgresType() resource.Type {
	return resource.Type{Key: "postgres", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true},
		{Name: "database", Type: "string"}, {Name: "username", Type: "string"},
		{Name: "password", Type: "string", Secret: true, Required: true},
	}}
}

func identityMapping() map[string]string {
	return map[string]string{"host": "host", "port": "port", "database": "database", "username": "username", "password": "password"}
}

// module is one fixture module registered in the bundle table.
type module struct {
	bundle tfmodule.Bundle
	hcl    string
}

type harness struct {
	t       *testing.T
	db      persistencetest.PostgresDatabase
	store   *postgres.Store
	state   *postgres.StateStore
	cipher  *statecipher.AESGCM
	backend *statebackend.Service
	base    string
	secrets *secrets.Memory
	svc     *tfexec.Service
	runner  *countingRunner
	binder  *fakeBinder
	ready   *fakeReadiness
	recover *recoveryDir
	// inspector is the FAKE namespace content inspector.
	inspector *fakeInspector
	// stateRepo is the repository the service uses (real by default; tests
	// wrap it to inject storage faults).
	stateRepo terraformstate.Repository

	orgID, appID, envID string
	appKey, orgKey      string
	cli                 string
	hashes              int
	mod                 module
}

func (h *harness) conn() *pgx.Conn {
	h.t.Helper()
	c, err := pgx.Connect(context.Background(), h.db.Admin)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { c.Close(context.Background()) })
	return c
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	if _, err := h.conn().Exec(context.Background(), sql, args...); err != nil {
		h.t.Fatalf("exec: %v", err)
	}
}

func (h *harness) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.conn().QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		h.t.Fatalf("count: %v", err)
	}
	return n
}

// fakeBinder is a FAKE provider binder: it returns a fixed private
// environment and records what it was asked for. The real binder is tested in
// its own package.
type fakeBinder struct {
	mu      sync.Mutex
	calls   []terraformexec.BindRequest
	account string
	region  string
	err     error
	// closed counts released provider environments; retained records handles.
	closed   int
	retained []string
}

func (b *fakeBinder) Bind(_ context.Context, req terraformexec.BindRequest) (terraformexec.ProviderEnv, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, req)
	if b.err != nil {
		return terraformexec.ProviderEnv{}, b.err
	}
	return terraformexec.ProviderEnv{
		Env: []string{"FAKE_PROVIDER_CREDENTIAL=scoped"}, AccountID: b.account, Region: b.region,
		Close: func() error { b.mu.Lock(); b.closed++; b.mu.Unlock(); return nil },
		Retain: func(handle string) error {
			b.mu.Lock()
			b.retained = append(b.retained, handle)
			b.mu.Unlock()
			return nil
		},
	}, nil
}

// fakeReadiness is a FAKE readiness check with a scripted result.
type fakeReadiness struct {
	mu    sync.Mutex
	err   error
	calls []terraformexec.ReadinessRequest
}

func (r *fakeReadiness) Check(_ context.Context, req terraformexec.ReadinessRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req)
	return r.err
}

// fakeInspector is a FAKE namespace content inspector with scripted contents.
type fakeInspector struct {
	mu      sync.Mutex
	uid     string
	objects []terraformexec.NamespaceObject
	err     error
	calls   []string
}

func (f *fakeInspector) Inspect(_ context.Context, env []string, namespace string) (terraformexec.NamespaceInspection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, namespace)
	uid := f.uid
	if uid == "" {
		uid = "uid-ns"
	}
	return terraformexec.NamespaceInspection{UID: uid, Objects: f.objects}, f.err
}

// recoveryDir stores recovery artifacts as plain files in a private
// directory for assertions (the real adapter encrypts; see its tests).
type recoveryDir struct {
	dir  string
	fail bool
	// failVerify refuses to store a verification artifact; unreadable makes
	// every load fail.
	failVerify bool
	unreadable bool
	mu         sync.Mutex
	n          int
}

// SaveVerification / LoadVerification keep the pre-destroy artifact as a plain
// file in the FAKE directory (the real store encrypts; see its own tests).
func (r *recoveryDir) SaveVerification(_ context.Context, handle, instanceID string, state []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failVerify {
		return os.ErrPermission
	}
	path := filepath.Join(r.dir, handle)
	if _, err := os.Stat(path); err == nil {
		return nil // never overwritten
	}
	return os.WriteFile(path, state, 0o600)
}

func (r *recoveryDir) LoadVerification(_ context.Context, handle, instanceID string) ([]byte, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unreadable {
		return nil, false, os.ErrPermission
	}
	data, err := os.ReadFile(filepath.Join(r.dir, handle))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (r *recoveryDir) Save(_ context.Context, instanceID, attemptID string, state []byte) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return "", os.ErrPermission
	}
	r.n++
	handle := "recovery-" + attemptID
	return handle, os.WriteFile(filepath.Join(r.dir, handle), state, 0o600)
}

// countingRunner wraps the REAL runner and lets a test script failures on
// individual steps (FAKE injection over real sessions).
type countingRunner struct {
	real  terraformexec.Runner
	mu    sync.Mutex
	opens int
	// applies counts Apply calls across sessions.
	applies int
	// onApply, if set, replaces the real apply of call n (1-based).
	onApply func(n int, real terraformexec.Session) error
	// recoveryState, if set, is returned by RecoveryState.
	recoveryState []byte
	// lastArtifact is the artifact of the most recently opened session.
	lastArtifact *tfmodule.VerifiedArtifact
	// onResources, if set, rewrites the state resource list of every session
	// (FAKE provider objects for absence tests).
	onResources func([]terraformexec.StateResource) []terraformexec.StateResource
	// onPlan, if set, post-processes a real plan.
	onPlan func(plan terraformexec.Plan, mode terraformexec.PlanMode) (terraformexec.Plan, error)
	// onDrift is an explicit FAKE private-plan observation used only to prove
	// repair admission refuses changed values/actions before any Apply call.
	onDrift func(drift.Report) (drift.Report, error)
}

func (c *countingRunner) Open(ctx context.Context, req terraformexec.SessionRequest) (terraformexec.Session, error) {
	c.mu.Lock()
	c.opens++
	c.lastArtifact = req.Artifact
	c.mu.Unlock()
	s, err := c.real.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	return &hookSession{Session: s, runner: c}, nil
}

type hookSession struct {
	terraformexec.Session
	runner *countingRunner
}

func (s *hookSession) InspectDrift(ctx context.Context) (drift.Report, error) {
	report, err := s.Session.(terraformexec.DriftPlanSession).InspectDrift(ctx)
	if err != nil || s.runner.onDrift == nil {
		return report, err
	}
	return s.runner.onDrift(report)
}
func (s *hookSession) ReadSavedPlan(ctx context.Context) ([]byte, error) {
	return s.Session.(terraformexec.DriftPlanSession).ReadSavedPlan(ctx)
}

func (s *hookSession) Apply(ctx context.Context) error {
	s.runner.mu.Lock()
	s.runner.applies++
	n := s.runner.applies
	hook := s.runner.onApply
	s.runner.mu.Unlock()
	if hook != nil {
		return hook(n, s.Session)
	}
	return s.Session.Apply(ctx)
}

func (s *hookSession) Plan(ctx context.Context, mode terraformexec.PlanMode) (terraformexec.Plan, error) {
	plan, err := s.Session.Plan(ctx, mode)
	if err != nil || s.runner.onPlan == nil {
		return plan, err
	}
	return s.runner.onPlan(plan, mode)
}

func (s *hookSession) StateResources(ctx context.Context) ([]terraformexec.StateResource, error) {
	list, err := s.Session.StateResources(ctx)
	if err != nil || s.runner.onResources == nil {
		return list, err
	}
	return s.runner.onResources(list), nil
}

func (s *hookSession) RecoveryState() ([]byte, bool) {
	if s.runner.recoveryState != nil {
		return s.runner.recoveryState, true
	}
	return s.Session.RecoveryState()
}

// fixtureModules is the FAKE module source: it serves hand-built artifacts
// built from the fixture HCL and the bundle rows inserted in the database.
type fixtureModules struct {
	h       *harness
	mu      sync.Mutex
	bundles map[string]module // by bundle ID
	opens   int
}

func (m *fixtureModules) VerifyForExecution(_ context.Context, ref tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error) {
	m.mu.Lock()
	m.opens++
	var found *module
	for _, mod := range m.bundles {
		if mod.bundle.Ref.ModuleID == ref.ModuleID {
			mod := mod
			found = &mod
		}
	}
	m.mu.Unlock()
	if found == nil {
		return tfmodule.VerifiedArtifact{}, os.ErrNotExist
	}
	art := runnertest.Artifact(m.h.t, runnertest.Module{Files: map[string]string{"main.tf": found.hcl}}, m.h.cli)
	bundle := found.bundle
	bundle.TerraformBinarySHA256 = art.Bundle.TerraformBinarySHA256
	bundle.Ref.ContentSHA256 = art.Bundle.Ref.ContentSHA256
	art.Bundle = bundle
	return *art, nil
}

func (m *fixtureModules) VerifyHistoricalForExecution(ctx context.Context, bundle tfmodule.Bundle) (tfmodule.VerifiedArtifact, error) {
	return m.VerifyForExecution(ctx, bundle.Ref)
}

func (m *fixtureModules) BundleByID(_ context.Context, id string) (tfmodule.Bundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mod, ok := m.bundles[id]
	if !ok {
		return tfmodule.Bundle{}, os.ErrNotExist
	}
	// The content digest is only known once built; build one to read it.
	art := runnertest.Artifact(m.h.t, runnertest.Module{Files: map[string]string{"main.tf": mod.hcl}}, m.h.cli)
	b := mod.bundle
	b.TerraformBinarySHA256, b.Ref.ContentSHA256 = art.Bundle.TerraformBinarySHA256, art.Bundle.Ref.ContentSHA256
	return b, nil
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	cli := runnertest.HostTerraform(t)
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
	cipher, err := statecipher.New("key-1", bytes.Repeat([]byte{7}, statecipher.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	backend, err := statebackend.New(state, cipher)
	if err != nil {
		t.Fatal(err)
	}
	l, err := deliveryhttp.ServeTerraformStateLoopback(deliveryhttp.NewTerraformStateHandler(backend))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.Close(c)
	})
	h := &harness{t: t, db: db, store: st, state: state, cipher: cipher, backend: backend, base: l.Address, cli: cli,
		secrets: secrets.NewMemory(), orgKey: "acme"}
	h.runner = &countingRunner{real: runner.New(runner.Options{Grace: 500 * time.Millisecond, StopConfirm: 5 * time.Second})}
	h.binder = &fakeBinder{account: "123456789012", region: "us-east-1"}
	h.ready = &fakeReadiness{}
	h.recover = &recoveryDir{dir: t.TempDir()}
	h.inspector = &fakeInspector{}
	h.stateRepo = state
	h.seed()
	h.mod = h.registerModule("pg", moduleHCL)
	h.svc = h.newService(nil)
	return h
}

func (h *harness) newService(mods *fixtureModules) *tfexec.Service {
	if mods == nil {
		mods = &fixtureModules{h: h, bundles: map[string]module{h.mod.bundle.ID: h.mod}}
	}
	svc, err := tfexec.New(tfexec.Config{BackendBase: h.base, Tags: map[string]string{"platform": "poc"}, InvocationTimeout: 2 * time.Minute, ReadinessTimeout: time.Minute, RetryBackoff: time.Millisecond, AbsencePolls: 2, AbsenceInterval: time.Millisecond},
		tfexec.Deps{StateWriter: h.backend, Baselines: h.state, Cipher: h.cipher, State: h.stateRepo, Tokens: h.backend, Modules: mods, Runner: h.runner, Binder: h.binder, Readiness: h.ready, Namespaces: h.inspector, Recovery: h.recover, Verification: h.recover, Secrets: h.secrets})
	if err != nil {
		h.t.Fatal(err)
	}
	return svc
}

func (h *harness) seed() {
	h.t.Helper()
	ctx := context.Background()
	h.orgID = ids.New()
	if err := h.store.SaveOrganization(ctx, application.Organization{ID: h.orgID, Key: h.orgKey, Name: h.orgKey}); err != nil {
		h.t.Fatal(err)
	}
	h.appID = ids.New()
	h.appKey = h.appID
	if err := h.store.SaveApplication(ctx, application.Application{ID: h.appID, Key: h.appKey, OrganizationKey: h.orgKey, Name: "App", Subdomain: "s-" + h.appID[:8], Version: 1}); err != nil {
		h.t.Fatal(err)
	}
	h.envID = ids.New()
	if err := h.store.SaveEnvironment(ctx, environment.Environment{ID: h.envID, Key: "staging", ApplicationID: h.appID, ApplicationKey: h.appKey, Name: "staging", Type: "staging", NamespaceIdentity: "ns-" + h.appID[:8], Version: 1}); err != nil {
		h.t.Fatal(err)
	}
}

// registerModule inserts a verified bundle row and returns the fixture module.
func (h *harness) registerModule(moduleID, hcl string) module {
	h.t.Helper()
	id := ids.New()
	contract := tfmodule.Contract{
		SchemaVersion: tfmodule.ContractSchemaVersion, SourceKind: tfmodule.SourceKindPackaged, ResourceTypes: []string{"postgres"},
		Inputs: map[string]tfmodule.Input{
			"note": {Type: "string"}, "stable": {Type: "string"}, "region": {Type: "string"},
			"tags": {Type: "map(string)"}, "master_password": {Type: "string", Sensitive: true},
		},
		Outputs: map[string]tfmodule.Output{
			"host": {Type: "string"}, "port": {Type: "number"}, "database": {Type: "string"},
			"username": {Type: "string"}, "tls": {Type: "bool"}, "password": {Type: "string", Sensitive: true},
		},
		ExecutorOwned: []string{"master_password", "region", "tags"},
	}
	bundle := tfmodule.Bundle{
		ID: id, Ref: tfmodule.SourceRef{SourceID: "test", ModuleID: moduleID, Revision: "rev1", Subdirectory: "root", ContentSHA256: strings.Repeat("a", 64)},
		TerraformVersion: "1.9.8", TerraformBinarySHA256: strings.Repeat("b", 64), ProviderLockDigest: strings.Repeat("c", 64),
		Contract: contract, Providers: map[string]tfmodule.ProviderRequirement{}, Status: tfmodule.StatusVerified,
	}
	h.exec(`INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES($1::uuid,'test',$2,'rev1','root',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`, id, moduleID)
	return module{bundle: bundle, hcl: hcl}
}

// claim takes the Environment claim under a new operation and returns the owner context.
func (h *harness) claim(owner string) (context.Context, environment.Operation) {
	h.t.Helper()
	op, err := h.store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ApplicationKey: h.appKey, EnvironmentKey: "staging", Owner: owner, Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Hour)})
	if err != nil {
		h.t.Fatalf("claim: %v", err)
	}
	return ownerCtx(op), op
}

// claimHeld is claim with the hold flag the Environment lease carries.
func (h *harness) claimHeld(owner string) (context.Context, environment.Operation) {
	h.t.Helper()
	ctx, op := h.claim(owner)
	return persistence.WithHold(ctx), op
}

func ownerCtx(op environment.Operation) context.Context {
	return persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
}

func (h *harness) release(op environment.Operation, status environment.OperationStatus) {
	h.t.Helper()
	if err := h.store.ReleaseOperation(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, status, ""); err != nil {
		h.t.Fatalf("release: %v", err)
	}
}

func (h *harness) identity(descriptor string, scope resource.Scope) tfresource.Identity {
	return tfresource.Identity{OrganizationID: h.orgID, ApplicationID: h.appID, EnvironmentID: h.envID, TargetGeneration: 0, Descriptor: descriptor, Scope: scope}
}

const defaultDescriptor = "postgres.default#modules.api.externals.db"

func defaultScope() resource.Scope {
	return resource.Scope{Type: resource.ScopeWorkload, ID: "app.staging.api"}
}

// request builds an ExecuteRequest for the default instance.
func (h *harness) request(run string, mod module, inputs map[string]any) tfexec.ExecuteRequest {
	return tfexec.ExecuteRequest{
		RunID: run, Descriptor: defaultDescriptor, Scope: defaultScope(), Generation: 0, Identity: h.identity(defaultDescriptor, defaultScope()),
		Source: mod.bundle.Ref, Type: postgresType(), Inputs: inputs, OutputMapping: identityMapping(), OrganizationKey: h.orgKey,
	}
}

func inputs(note, stable string) map[string]any {
	return map[string]any{"note": note, "stable": stable}
}

func (h *harness) instance(_ context.Context, descriptor string, scope resource.Scope) (tfresource.Instance, bool) {
	h.t.Helper()
	list, err := h.state.ListInstances(context.Background(), h.envID)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, inst := range list {
		if inst.Identity.Descriptor == descriptor && inst.Identity.Scope == scope && inst.Identity.TargetGeneration == 0 {
			return inst, true
		}
	}
	return tfresource.Instance{}, false
}

func (h *harness) attempts(instanceID string) []tfresource.Attempt {
	h.t.Helper()
	list, err := h.state.ListAttempts(context.Background(), instanceID)
	if err != nil {
		h.t.Fatal(err)
	}
	return list
}

func readFile(dir, name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, name)) }

func farFuture() time.Time { return time.Now().Add(time.Hour) }
