package tfexec_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/adapters/secrets"
	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/runcleanup"
	"orchestrator/internal/application/statebackend"
	deliveryhttp "orchestrator/internal/delivery/http"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/test/integration/v2run"
)

// PROCESS-LEVEL KILL CHAIN over REAL components (a local test fixture, not
// live-v2 proof): the real v2supervisor binary (own session and process group),
// the real runcleanup service over the real tfexec engine on a real disposable
// PostgreSQL with three roles, the real encrypted state backend and the ACTUAL
// Terraform CLI destroying built-in terraform_data fixtures (no provider, no
// cluster, no cloud). The supervisor runs the guard, the cleanup and the
// verification as separate processes (this test binary re-executed in helper
// modes; no shell stand-in). The "runner and browser" are real processes in
// their own process group that the test SIGKILLs. The provider binder, the
// readiness check and the namespace inspector are the same labelled fakes the
// other tfexec tests use; they do not change what is proven here: after the
// victim group is killed, an independent process reverse-destroys through the
// original engine under the Environment claim, holds on a foreign claim and
// never touches instances of another run.

const killEnv = "TFEXEC_KILLCHAIN_SPEC"

var killRole = flag.String("killchain.role", "", "helper role of the process-level kill-chain tests")

type killSpec struct {
	ConsoleFile  string   `json:"consoleFile"`
	StateFile    string   `json:"stateFile"`
	RecoveryDir  string   `json:"recoveryDir"`
	Org          string   `json:"org"`
	App          string   `json:"app"`
	EnvID        string   `json:"envId"`
	RunIDs       []string `json:"runIds"`
	Bundle       []byte   `json:"bundle"`
	HCL          string   `json:"hcl"`
	StateKeyByte byte     `json:"stateKeyByte"`
}

// attach rebuilds the harness wiring over an EXISTING database inside the
// helper process (the same constructors the product wiring uses).
func attach(t *testing.T, spec killSpec) *harness {
	t.Helper()
	console, _ := os.ReadFile(spec.ConsoleFile)
	stateURL, _ := os.ReadFile(spec.StateFile)
	ctx := context.Background()
	st, err := postgres.Open(ctx, string(bytes.TrimSpace(console)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	state, err := postgres.OpenState(ctx, string(bytes.TrimSpace(stateURL)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.Close)
	cipher, err := statecipher.New("key-1", bytes.Repeat([]byte{spec.StateKeyByte}, statecipher.KeySize))
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
	var bundle tfmodule.Bundle
	if err := json.Unmarshal(spec.Bundle, &bundle); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, store: st, state: state, cipher: cipher, backend: backend, base: l.Address, cli: runnertest.HostTerraform(t),
		secrets: secrets.NewMemory(), orgKey: spec.Org, appKey: spec.App, appID: spec.App, envID: spec.EnvID}
	h.runner = &countingRunner{real: runner.New(runner.Options{Grace: 500 * time.Millisecond, StopConfirm: 5 * time.Second})}
	h.binder = &fakeBinder{account: "123456789012", region: "us-east-1"}
	h.ready = &fakeReadiness{}
	h.recover = &recoveryDir{dir: spec.RecoveryDir}
	h.inspector = &fakeInspector{}
	h.stateRepo = state
	h.mod = module{bundle: bundle, hcl: spec.HCL}
	h.svc = h.newService(nil)
	return h
}

// TestKillChainHelper is the helper process of the kill-chain tests. Without
// the spec variable it does nothing (it is an ordinary passing test).
func TestKillChainHelper(t *testing.T) {
	path := os.Getenv(killEnv)
	if path == "" {
		return
	}
	var spec killSpec
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &spec) != nil {
		os.Exit(2)
	}
	h := attach(t, spec)
	// os.Exit skips the test cleanups: remove this process's temporary tree (a copy of the Terraform binary) itself.
	root := filepath.Dir(t.TempDir())
	exit := func(code int) { _ = os.RemoveAll(root); os.Exit(code) }
	target := runcleanup.Target{OrganizationKey: spec.Org, ApplicationKey: spec.App, EnvironmentKey: "staging", RunIDs: spec.RunIDs, Generation: 0}
	net := runcleanup.New(h.store, h.state, h.svc, h.store, envops.NewManager(h.store))
	ctx := context.Background()
	switch *killRole {
	case "guard":
		// Real ownership guard: the registry must be readable and hold at least one run-owned instance; unreadable is "unknown" (1).
		list, err := h.state.ListInstances(ctx, spec.EnvID)
		if err != nil {
			exit(1)
		}
		owned := map[string]bool{}
		for _, r := range spec.RunIDs {
			owned[r] = true
		}
		for _, inst := range list {
			if inst.Status != tfresource.InstanceDestroyed && owned[inst.CreatedByRunID] {
				exit(0)
			}
		}
		exit(v2run.ExitNotOwned)
	case "cleanup":
		rep, err := net.Run(ctx, target)
		switch {
		case rep.Outcome == runcleanup.OutcomeBusy:
			exit(4)
		case err != nil || rep.Outcome != runcleanup.OutcomeClean:
			exit(3)
		}
		exit(0)
	case "verify":
		// Independent proof, like v2cleanup status: NO instance of the Environment may remain (an unowned one is never "clean").
		list, err := h.state.ListInstances(ctx, spec.EnvID)
		if err != nil {
			exit(1)
		}
		for _, inst := range list {
			if inst.Status != tfresource.InstanceDestroyed {
				exit(1)
			}
		}
		exit(0)
	}
	exit(2)
}

type killRig struct {
	h          *harness
	dir        string
	supervisor string
	spec       killSpec
	specFile   string
	status     string
	victim     *exec.Cmd
	browser    *exec.Cmd
}

func newKillRig(t *testing.T) *killRig {
	t.Helper()
	h := newHarness(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	backendDir := filepath.Join(filepath.Dir(file), "..", "..", "..")
	sup := filepath.Join(dir, "v2supervisor")
	build := exec.Command("go", "build", "-o", sup, "orchestrator/test/integration/v2supervisor")
	build.Dir = backendDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the real supervisor: %v\n%s", err, out)
	}
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := v2run.WriteFilePrivate(p, []byte(content)); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bundle, _ := json.Marshal(h.mod.bundle)
	r := &killRig{h: h, dir: dir, supervisor: sup, status: filepath.Join(dir, "status.json")}
	r.spec = killSpec{ConsoleFile: write("console.url", h.db.Console), StateFile: write("state.url", h.db.State), RecoveryDir: h.recover.dir,
		Org: h.orgKey, App: h.appKey, EnvID: h.envID, RunIDs: []string{"run-1"}, Bundle: bundle, HCL: moduleHCL, StateKeyByte: 7}
	return r
}

// start launches the victim runner and browser (own process group), writes the
// supervisor spec whose steps are the real helper processes, and arms the real
// supervisor.
func (r *killRig) start(t *testing.T) (v2run.Armed, int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r.victim = exec.Command("sleep", "600")
	r.victim.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := r.victim.Start(); err != nil {
		t.Fatal(err)
	}
	r.browser = exec.Command("sleep", "600")
	r.browser.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: r.victim.Process.Pid}
	if err := r.browser.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-r.victim.Process.Pid, syscall.SIGKILL)
		_ = r.victim.Wait()
		_ = r.browser.Wait()
	})
	var watch []v2run.Watch
	for name, p := range map[string]*exec.Cmd{"runner": r.victim, "browser": r.browser} {
		ticks, err := v2run.ProcessIdentity(p.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		watch = append(watch, v2run.Watch{Name: name, PID: p.Process.Pid, StartTicks: ticks})
	}
	rawSpec, _ := json.Marshal(r.spec)
	specFile := filepath.Join(r.dir, "kill-spec.json")
	if err := v2run.WriteFilePrivate(specFile, rawSpec); err != nil {
		t.Fatal(err)
	}
	base := []string{exe, "-test.run=^TestKillChainHelper$", "-test.count=1"}
	with := func(role string) []string { return append(append([]string{}, base...), "-killchain.role="+role) }
	step := v2run.Step{Name: "app-resources", Layer: v2run.LayerAppResources, Argv: with("cleanup"), Guard: with("guard"), Verify: with("verify"),
		Env: map[string]string{killEnv: specFile, "REQUIRE_TERRAFORM": "1"}, Timeout: v2run.Duration{Duration: 4 * time.Minute}, MaxAttempts: 1}
	spec := v2run.Spec{RunID: "killchain", StatusPath: r.status, FinishPath: filepath.Join(r.dir, "finish"), LogDir: filepath.Join(r.dir, "logs"),
		ExpiresAt: time.Now().Add(30 * time.Minute), CleanupBudget: v2run.Duration{Duration: 10 * time.Minute}, Poll: v2run.Duration{Duration: 200 * time.Millisecond},
		Watch: watch, Steps: []v2run.Step{step}}
	raw, _ := json.Marshal(spec)
	specPath := filepath.Join(r.dir, "supervisor-spec.json")
	if err := v2run.WriteFilePrivate(specPath, raw); err != nil {
		t.Fatal(err)
	}
	armed, err := v2run.StartDetached(r.supervisor, []string{"-spec", specPath}, r.status, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return armed, r.victim.Process.Pid
}

// kill SIGKILLs the whole victim group and returns the supervisor's terminal status.
func (r *killRig) kill(t *testing.T, armed v2run.Armed, victimPID int) v2run.Status {
	t.Helper()
	own, _ := syscall.Getpgid(0)
	victimGroup, _ := syscall.Getpgid(victimPID)
	if armed.PGID == victimGroup || armed.PGID == own || armed.SID == victimGroup {
		t.Fatalf("the supervisor shares a process group with the runner or the test: %+v victim=%d own=%d", armed, victimGroup, own)
	}
	if err := syscall.Kill(-victimGroup, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(armed.PID, 0); err != nil {
		t.Fatalf("the supervisor did not survive the group kill: %v", err)
	}
	st, err := v2run.WaitTerminal(r.status, 6*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := filepath.Glob(filepath.Join(r.dir, "logs", "*"))
			for _, l := range logs {
				raw, _ := os.ReadFile(l)
				t.Logf("%s:\n%s", filepath.Base(l), raw)
			}
		}
	})
	return st
}

func TestKillChainDestroysAbandonedInstancesAfterTheRunnerGroupIsKilled(t *testing.T) {
	r := newKillRig(t)
	topo := r.h.chain() // run-1 created namespace, db and cache; the runner then dies
	armed, victim := r.start(t)
	st := r.kill(t, armed, victim)
	if st.State != v2run.StateClean || st.Reason == "" {
		t.Fatalf("supervisor status %+v", st)
	}
	for _, id := range []string{topo.db.InstanceID, topo.cache.InstanceID, topo.ns.InstanceID} {
		if r.h.status(id) != tfresource.InstanceDestroyed {
			t.Fatalf("%s is %s, not DESTROYED", id, r.h.status(id))
		}
	}
	if r.h.count(`SELECT count(*) FROM terraform_states`) != 3 {
		t.Fatal("state rows were purged")
	}
	if r.h.count(`SELECT count(*) FROM environment_operations WHERE status IN ('ACTIVE','RECOVERING','INTERRUPTED')`) != 0 {
		t.Fatal("a claim was left active")
	}
}

func TestKillChainHoldsWhenAnotherOperationOwnsTheEnvironment(t *testing.T) {
	r := newKillRig(t)
	topo := r.h.chain()
	_, op := r.h.claim("proc-other-operation") // the killed runner's own claim stays held: never unlocked
	armed, victim := r.start(t)
	st := r.kill(t, armed, victim)
	if st.State != v2run.StateAttention || !st.KeepEvidence {
		t.Fatalf("a held Environment must need attention and keep evidence: %+v", st)
	}
	for _, id := range []string{topo.db.InstanceID, topo.cache.InstanceID, topo.ns.InstanceID} {
		if r.h.status(id) == tfresource.InstanceDestroyed {
			t.Fatalf("%s was destroyed while another operation held the Environment", id)
		}
	}
	if r.h.count(`SELECT count(*) FROM environment_operations WHERE id=$1::uuid AND status='ACTIVE'`, op.ID) != 1 {
		t.Fatal("the foreign claim was unlocked or taken over")
	}
}

func TestKillChainNeverTouchesInstancesOfAnotherRun(t *testing.T) {
	r := newKillRig(t)
	own := r.h.createInstance("run-1", defaultDescriptor, defaultScope(), nil)
	foreign := r.h.createInstance("run-foreign", "postgres.default#modules.api.externals.retained", scopeOf("retained"), nil)
	armed, victim := r.start(t)
	st := r.kill(t, armed, victim)
	if st.State != v2run.StateAttention {
		t.Fatalf("an instance outside the run's ownership must leave the run needing attention: %+v", st)
	}
	if r.h.status(own.InstanceID) != tfresource.InstanceDestroyed {
		t.Fatal("the run's own instance must be destroyed")
	}
	if r.h.status(foreign.InstanceID) == tfresource.InstanceDestroyed {
		t.Fatal("an instance of another run was destroyed")
	}
}
