package v03run

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These fault tests run the REAL v03-cleanup.sh and v03-supervisor.sh against a stub operator tool and real
// throw-away processes. Nothing external is touched: the stub only records what it was asked to do.

type cleanupFixture struct {
	dir, work, private, envFile, log string
}

func newCleanupFixture(t *testing.T) *cleanupFixture {
	t.Helper()
	dir := t.TempDir()
	f := &cleanupFixture{dir: dir, work: filepath.Join(dir, "work"), private: filepath.Join(dir, "work", "private")}
	f.log = filepath.Join(dir, "stub.log")
	for _, d := range []string{f.work, f.private, filepath.Join(f.private, "tmp")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stub := filepath.Join(dir, "v2ctl")
	script := `#!/usr/bin/env bash
# stub operator tool: records the command and, for reap, whether a recorded process group still lives.
echo "$1" >> "` + f.log + `"
if [[ "$1" == v03-reap && -s "` + filepath.Join(f.private, "server.pid") + `" ]]; then echo "reap-saw-live-server" >> "` + f.log + `"; fi
case "${STUB_FAIL:-}" in *"$1"*) echo "stub failure of $1" >&2; exit 1;; esac
exit 0
`
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	f.envFile = filepath.Join(f.private, "cleanup.env")
	env := fmt.Sprintf("WORK=%q\nPRIVATE=%q\nCTL=%q\nKUBECTL=kubectl\nHOST_KUBECONFIG=/dev/null\nHOST_CONTEXT=ctx\nRUN_ID=v03-test\nORG=acme\nVAULT_ADDR_LOCAL=http://127.0.0.1:1\nMOUNT=secret\nPG_ADMIN=%q\n", f.work, f.private, stub, filepath.Join(dir, "pg-admin"))
	if err := os.WriteFile(f.envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"baseline.json": "{}", "pg-manifest.json": "{}", "state.key": "KEY", "console.url": "url", "vault-token": "tok"} {
		if err := os.WriteFile(filepath.Join(f.private, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.private, "product-started"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *cleanupFixture) calls() []string {
	raw, _ := os.ReadFile(f.log)
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// spawnGroup starts a process in its own process group (like the runner's setsid), records the pgid and returns it.
func spawnGroup(t *testing.T, f *cleanupFixture, pidFile string) int {
	t.Helper()
	cmd := exec.Command("sleep", "300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	if err := os.WriteFile(filepath.Join(f.private, pidFile), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func groupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil
}

func runCleanup(t *testing.T, f *cleanupFixture, stubFail string) error {
	t.Helper()
	cmd := exec.Command("bash", "../v03-cleanup.sh", f.envFile, "run")
	cmd.Env = append(os.Environ(), "STUB_FAIL="+stubFail)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("cleanup output: %s", out)
	}
	return err
}

func TestCleanupStopsWritersBeforeDeletingAndDropsTheDatabaseLast(t *testing.T) {
	f := newCleanupFixture(t)
	server := spawnGroup(t, f, "server.pid")
	node := spawnGroup(t, f, "node.pid")
	if err := runCleanup(t, f, ""); err != nil {
		t.Fatalf("a fully proven cleanup must exit 0: %v", err)
	}
	if groupAlive(server) || groupAlive(node) {
		t.Fatal("the owned process groups must be stopped")
	}
	calls := f.calls()
	if strings.Join(calls, ",") != "v03-reap,v03-vault-clean,pg-drop" {
		t.Fatalf("order of operations = %v (the cluster, then Vault, then the database)", calls)
	}
	for _, call := range calls {
		if call == "reap-saw-live-server" {
			t.Fatal("the cluster was touched while the product server was still recorded as running")
		}
	}
	if state, _ := os.ReadFile(filepath.Join(f.work, "cleanup-state.txt")); strings.TrimSpace(string(state)) != "proven" {
		t.Fatalf("state = %q", state)
	}
	// Idempotent: a second run (the supervisor after the runner) is a no-op.
	before := len(f.calls())
	if err := runCleanup(t, f, ""); err != nil || len(f.calls()) != before {
		t.Fatalf("a proven cleanup must not run again: %v %v", err, f.calls())
	}
}

func TestCleanupKeepsTheDatabaseAndRecoveryMaterialWhenAnythingIsNotProven(t *testing.T) {
	for name, stubFail := range map[string]string{"cluster api error": "v03-reap", "vault not proven": "v03-vault-clean"} {
		t.Run(name, func(t *testing.T) {
			f := newCleanupFixture(t)
			if err := runCleanup(t, f, stubFail); err == nil {
				t.Fatal("an unproven cleanup must exit non-zero")
			}
			for _, call := range f.calls() {
				if call == "pg-drop" {
					t.Fatal("the database must not be dropped while the cleanup is unproven")
				}
			}
			for _, kept := range []string{"state.key", "console.url", "vault-token", "pg-manifest.json", "baseline.json"} {
				if _, err := os.Stat(filepath.Join(f.private, kept)); err != nil {
					t.Fatalf("%s must be kept for recovery: %v", kept, err)
				}
			}
			if raw, err := os.ReadFile(filepath.Join(f.private, "pg-manifest.json")); err != nil || string(raw) != "{}" {
				t.Fatalf("the database manifest must be intact: %q %v", raw, err)
			}
			if _, err := os.Stat(filepath.Join(f.work, "CLEANUP-NOT-PROVEN.txt")); err != nil {
				t.Fatal("CLEANUP-NOT-PROVEN.txt must say that recovery material is kept")
			}
			if state, _ := os.ReadFile(filepath.Join(f.work, "cleanup-state.txt")); strings.TrimSpace(string(state)) != "failed" {
				t.Fatalf("state = %q", state)
			}
		})
	}
}

func TestCleanupWithoutAProductOrBaselineClaimsNothingAndDoesNotTouchTheCluster(t *testing.T) {
	f := newCleanupFixture(t)
	_ = os.Remove(filepath.Join(f.private, "baseline.json"))
	_ = os.Remove(filepath.Join(f.private, "product-started"))
	if err := runCleanup(t, f, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls(), ","); got != "pg-drop" {
		t.Fatalf("with no baseline and no product only the database is dropped, got %v", got)
	}
}

func TestSupervisorCleansWhenTheRunnerIsKilledAndWhenItsDeadlinePasses(t *testing.T) {
	t.Run("runner SIGKILLed", func(t *testing.T) {
		f := newCleanupFixture(t)
		server := spawnGroup(t, f, "server.pid")
		runner := exec.Command("sleep", "300")
		if err := runner.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { _ = runner.Wait() }()
		sup := exec.Command("bash", "../v03-supervisor.sh", f.envFile, strconv.Itoa(runner.Process.Pid), strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
		sup.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		sup.Env = append(os.Environ(), "STUB_FAIL=")
		if err := sup.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- sup.Wait() }()
		time.Sleep(1500 * time.Millisecond)
		if len(f.calls()) != 0 {
			t.Fatal("the supervisor must not clean while the runner lives")
		}
		_ = runner.Process.Signal(syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Fatal("the supervisor did not finish after the runner was killed")
		}
		if groupAlive(server) {
			t.Fatal("the product process group must be stopped by the supervisor")
		}
		if got := strings.Join(f.calls(), ","); !strings.Contains(got, "v03-reap") || !strings.HasSuffix(got, "pg-drop") {
			t.Fatalf("calls = %v", got)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		f := newCleanupFixture(t)
		runner := exec.Command("sleep", "300")
		if err := runner.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() { _ = runner.Wait(); close(exited) }()
		sup := exec.Command("bash", "../v03-supervisor.sh", f.envFile, strconv.Itoa(runner.Process.Pid), strconv.FormatInt(time.Now().Add(2*time.Second).Unix(), 10))
		sup.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := sup.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- sup.Wait() }()
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Fatal("the supervisor did not act on the deadline")
		}
		select {
		case <-exited:
		default:
			t.Fatal("the runner must be stopped when the deadline passes")
		}
		if got := strings.Join(f.calls(), ","); !strings.HasSuffix(got, "pg-drop") {
			t.Fatalf("calls = %v", got)
		}
	})
}
