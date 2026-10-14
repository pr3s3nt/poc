package v2run_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

var supervisorBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "v2sup-")
	if err != nil {
		panic(err)
	}
	supervisorBinary = filepath.Join(dir, "v2supervisor")
	build := exec.Command("go", "build", "-o", supervisorBinary, "orchestrator/test/integration/v2supervisor")
	if out, err := build.CombinedOutput(); err != nil {
		os.Stderr.Write(out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fixture is a low-cost owned stand-in for the layered assets: directories
// carrying an ownership marker, removed by `rm` and verified absent by `test`.
type fixture struct {
	t       *testing.T
	root    string
	private string
	order   string
	spec    v2run.Spec
}

func sh(script string) []string { return []string{"/bin/sh", "-c", script} }

func newFixture(t *testing.T, layers ...string) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, root: root, private: filepath.Join(root, "private"), order: filepath.Join(root, "order.log")}
	if err := os.MkdirAll(f.private, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, layer := range layers {
		f.addAsset(layer, `{"run-id":"run-1"}`)
	}
	f.spec = v2run.Spec{
		RunID: "run-1", StatusPath: filepath.Join(f.private, "status.json"), FinishPath: filepath.Join(f.private, "finish"),
		LogDir: filepath.Join(f.private, "logs"), ExpiresAt: time.Now().Add(time.Hour), CleanupBudget: v2run.Duration{Duration: time.Minute},
		Poll: v2run.Duration{Duration: 50 * time.Millisecond},
	}
	for _, layer := range layers {
		f.spec.Steps = append(f.spec.Steps, f.step(layer))
	}
	return f
}

func (f *fixture) asset(name string) string { return filepath.Join(f.root, name) }

func (f *fixture) addAsset(name, marker string) {
	if err := os.MkdirAll(f.asset(name), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.asset(name), "owner.json"), []byte(marker), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) step(layer string) v2run.Step {
	dir := f.asset(layer)
	return v2run.Step{
		Name: layer, Layer: layer, Timeout: v2run.Duration{Duration: 10 * time.Second}, MaxAttempts: 2,
		Guard:  sh(`d="$1"; [ -e "$d" ] || exit 0; if grep -q '"run-id":"run-1"' "$d/owner.json"; then exit 0; elif grep -q preexisting "$d/owner.json"; then exit 10; else exit 1; fi`),
		Argv:   sh(`echo ` + layer + ` >> ` + f.order + `; rm -rf ` + dir),
		Verify: sh(`test ! -e ` + dir),
	}
}

func (f *fixture) arm(watch ...v2run.Watch) v2run.Armed {
	f.t.Helper()
	f.spec.Watch = watch
	// The guard script reads its asset directory from the first argument.
	for i := range f.spec.Steps {
		f.spec.Steps[i].Guard = append(f.spec.Steps[i].Guard, "guard", f.asset(f.spec.Steps[i].Layer))
	}
	if err := writeSpec(f.specFile(), f.spec); err != nil {
		f.t.Fatal(err)
	}
	armed, err := v2run.StartDetached(supervisorBinary, []string{"-spec", f.specFile()}, f.spec.StatusPath, 10*time.Second)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = syscall.Kill(armed.PID, syscall.SIGKILL) })
	return armed
}

func (f *fixture) specFile() string { return filepath.Join(f.private, "spec.json") }

func (f *fixture) cleanOrder() []string {
	raw, _ := os.ReadFile(f.order)
	return strings.Fields(string(raw))
}

func (f *fixture) terminal() v2run.Status {
	f.t.Helper()
	st, err := v2run.WaitTerminal(f.spec.StatusPath, 20*time.Second)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

// startRunner starts a stand-in runner (and a browser in the same group) in
// its own process group; the supervisor must survive killing that group.
func startRunner(t *testing.T) (runner, browser v2run.Watch, kill func()) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & echo $! > "+filepath.Join(t.TempDir(), "x")+"; sleep 300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	ticks, err := v2run.ProcessIdentity(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	browserCmd := exec.Command("sleep", "300")
	browserCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := browserCmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = browserCmd.Wait() }()
	bticks, _ := v2run.ProcessIdentity(browserCmd.Process.Pid)
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = syscall.Kill(-browserCmd.Process.Pid, syscall.SIGKILL)
	})
	return v2run.Watch{Name: "runner", PID: cmd.Process.Pid, StartTicks: ticks},
		v2run.Watch{Name: "browser", PID: browserCmd.Process.Pid, StartTicks: bticks},
		func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

func wantOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanup order = %v, want %v", got, want)
	}
}

var reverse = []string{v2run.LayerDelivery, v2run.LayerAppResources, v2run.LayerNamespace}

func TestSupervisorIsIsolatedAndSurvivesRunnerGroupKill(t *testing.T) {
	f := newFixture(t, reverse...)
	runner, browser, killRunner := startRunner(t)
	armed := f.arm(runner, browser)
	ownPGID, _ := syscall.Getpgid(0)
	if armed.PGID == ownPGID || armed.PGID == runner.PID || armed.SID == runner.PID || armed.PID != armed.PGID {
		t.Fatalf("supervisor not isolated: %+v runner=%d own=%d", armed, runner.PID, ownPGID)
	}
	killRunner() // kill -9 of the runner process group
	st := f.terminal()
	if st.State != v2run.StateClean || !strings.HasPrefix(st.Reason, "watched-process-exited:runner") {
		t.Fatalf("status %+v", st)
	}
	wantOrder(t, f.cleanOrder(), "delivery", "app-resources", "namespace")
	for _, layer := range reverse {
		if _, err := os.Stat(f.asset(layer)); !os.IsNotExist(err) {
			t.Fatalf("%s still present", layer)
		}
	}
}

func TestSupervisorTriggersOnBrowserDeath(t *testing.T) {
	f := newFixture(t, reverse...)
	runner, browser, _ := startRunner(t)
	f.arm(runner, browser)
	_ = syscall.Kill(-browser.PID, syscall.SIGKILL)
	st := f.terminal()
	if st.State != v2run.StateClean || !strings.HasSuffix(st.Reason, ":browser") {
		t.Fatalf("status %+v", st)
	}
}

func TestSupervisorTriggersOnFinishSignalAndExpiry(t *testing.T) {
	t.Run("finish", func(t *testing.T) {
		f := newFixture(t, reverse...)
		runner, _, _ := startRunner(t)
		f.arm(runner)
		if err := v2run.RequestFinish(f.spec.FinishPath); err != nil {
			t.Fatal(err)
		}
		if st := f.terminal(); st.State != v2run.StateClean || st.Reason != "runner-finished" {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("SIGTERM to the supervisor", func(t *testing.T) {
		f := newFixture(t, reverse...)
		runner, _, _ := startRunner(t)
		armed := f.arm(runner)
		_ = syscall.Kill(armed.PID, syscall.SIGTERM)
		if st := f.terminal(); st.State != v2run.StateClean || !strings.HasPrefix(st.Reason, "signal:") {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("expiry", func(t *testing.T) {
		f := newFixture(t, reverse...)
		f.spec.ExpiresAt = time.Now().Add(1500 * time.Millisecond)
		runner, _, _ := startRunner(t)
		f.arm(runner)
		if st := f.terminal(); st.State != v2run.StateClean || st.Reason != "expiry" {
			t.Fatalf("%+v", st)
		}
	})
}

func TestSupervisorNeverDeletesPreexistingOrUnknownOwnership(t *testing.T) {
	f := newFixture(t, reverse...)
	f.addAsset(v2run.LayerAppResources, `{"preexisting":true}`) // not run-owned
	runner, _, killRunner := startRunner(t)
	f.arm(runner)
	killRunner()
	st := f.terminal()
	if st.State != v2run.StateClean {
		t.Fatalf("a verified-not-owned asset is retained, not a failure: %+v", st)
	}
	if _, err := os.Stat(f.asset(v2run.LayerAppResources)); err != nil {
		t.Fatal("preexisting asset was deleted")
	}
	outcomes := map[string]string{}
	for _, s := range st.Steps {
		outcomes[s.Name] = s.Outcome
	}
	if outcomes["app-resources"] != v2run.StepNotOwned || outcomes["delivery"] != v2run.StepDone {
		t.Fatalf("outcomes %v", outcomes)
	}

	g := newFixture(t, reverse...)
	g.addAsset(v2run.LayerDelivery, `{"mystery":true}`) // unknown ownership
	runner2, _, kill2 := startRunner(t)
	g.arm(runner2)
	kill2()
	st = g.terminal()
	if st.State != v2run.StateAttention || !st.KeepEvidence {
		t.Fatalf("unknown ownership must need attention and keep evidence: %+v", st)
	}
	if _, err := os.Stat(g.asset(v2run.LayerDelivery)); err != nil {
		t.Fatal("unknown-ownership asset was touched")
	}
	// Dependent layers are blocked: the namespace is not deleted to skip the lifecycle.
	for _, layer := range []string{"app-resources", "namespace"} {
		if _, err := os.Stat(g.asset(layer)); err != nil {
			t.Fatalf("%s was deleted behind a layer that could not be cleaned", layer)
		}
	}
	if len(g.cleanOrder()) != 0 {
		t.Fatalf("effects ran: %v", g.cleanOrder())
	}
}

func TestSupervisorKeepsEvidenceWhenAbsenceIsNotVerified(t *testing.T) {
	f := newFixture(t, reverse...)
	// The effect "succeeds" but the object stays: absence is never assumed.
	f.spec.Steps[1].Argv = sh(`echo app-resources >> ` + f.order)
	runner, _, killRunner := startRunner(t)
	f.arm(runner)
	killRunner()
	st := f.terminal()
	if st.State != v2run.StateAttention || len(st.Remaining) == 0 || st.Remaining[0] != "app-resources" {
		t.Fatalf("%+v", st)
	}
	got := map[string]string{}
	for _, s := range st.Steps {
		got[s.Name] = s.Outcome
	}
	if got["app-resources"] != v2run.StepNotVerified || got["namespace"] != v2run.StepBlocked {
		t.Fatalf("outcomes %v", got)
	}
	if n := strings.Count(strings.Join(f.cleanOrder(), ","), "app-resources"); n != 2 {
		t.Fatalf("bounded attempts = %d, want 2", n)
	}
}

func TestSupervisorEnvironmentDoesNotLeakRunnerSecrets(t *testing.T) {
	t.Setenv("V2RUN_TEST_SECRET", "must-not-reach-steps")
	f := newFixture(t, reverse...)
	dump := filepath.Join(f.root, "env.txt")
	f.spec.Steps[0].Argv = sh(`env > ` + dump + `; rm -rf ` + f.asset("delivery"))
	f.spec.Steps[0].Env = map[string]string{"KUBECONFIG": "/private/handle"}
	runner, _, killRunner := startRunner(t)
	f.arm(runner)
	killRunner()
	f.terminal()
	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "V2RUN_TEST_SECRET") || !strings.Contains(string(raw), "KUBECONFIG=/private/handle") {
		t.Fatalf("step environment wrong:\n%s", raw)
	}
}

func TestSupervisorStepTimeoutAndBudgetAreFinite(t *testing.T) {
	f := newFixture(t, reverse...)
	f.spec.Steps[0].Argv = sh(`sleep 60`)
	f.spec.Steps[0].Timeout = v2run.Duration{Duration: 500 * time.Millisecond}
	f.spec.Steps[0].MaxAttempts = 1
	runner, _, killRunner := startRunner(t)
	f.arm(runner)
	start := time.Now()
	killRunner()
	st := f.terminal()
	if st.State != v2run.StateAttention || time.Since(start) > 15*time.Second {
		t.Fatalf("%+v after %s", st, time.Since(start))
	}
}

func TestSpecValidationRejectsUnboundedOrUnorderedSpecs(t *testing.T) {
	f := newFixture(t, reverse...)
	f.spec.Watch = []v2run.Watch{{Name: "runner", PID: 1}}
	base := f.spec
	now := time.Now()
	if err := base.Validate(now); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*v2run.Spec){
		"no expiry":         func(s *v2run.Spec) { s.ExpiresAt = time.Time{} },
		"past expiry":       func(s *v2run.Spec) { s.ExpiresAt = now.Add(-time.Second) },
		"over 180 minutes":  func(s *v2run.Spec) { s.ExpiresAt = now.Add(200 * time.Minute) },
		"no budget":         func(s *v2run.Spec) { s.CleanupBudget = v2run.Duration{} },
		"budget over 120m":  func(s *v2run.Spec) { s.CleanupBudget = v2run.Duration{Duration: 121 * time.Minute} },
		"no watch":          func(s *v2run.Spec) { s.Watch = nil },
		"namespace first":   func(s *v2run.Spec) { s.Steps = []v2run.Step{s.Steps[2], s.Steps[0]} },
		"no guard":          func(s *v2run.Spec) { s.Steps = append([]v2run.Step(nil), s.Steps...); s.Steps[0].Guard = nil },
		"no verify":         func(s *v2run.Spec) { s.Steps = append([]v2run.Step(nil), s.Steps...); s.Steps[0].Verify = nil },
		"unbounded retries": func(s *v2run.Spec) { s.Steps = append([]v2run.Step(nil), s.Steps...); s.Steps[0].MaxAttempts = 50 },
		"no step timeout": func(s *v2run.Spec) {
			s.Steps = append([]v2run.Step(nil), s.Steps...)
			s.Steps[0].Timeout = v2run.Duration{}
		},
		"relative path": func(s *v2run.Spec) { s.StatusPath = "status.json" },
	}
	for name, mutate := range mutations {
		s := base
		mutate(&s)
		if err := s.Validate(now); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestSpecFileMustBePrivate(t *testing.T) {
	f := newFixture(t, reverse...)
	f.spec.Watch = []v2run.Watch{{Name: "runner", PID: 1}}
	path := f.specFile()
	if err := writeSpec(path, f.spec); err != nil {
		t.Fatal(err)
	}
	if _, err := v2run.LoadSpec(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := v2run.LoadSpec(path); err == nil {
		t.Fatal("a group/world-readable spec was accepted")
	}
}
