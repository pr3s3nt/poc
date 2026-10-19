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
if [[ "$1" == v04-output-refs ]]; then printf '[]\n' > "` + filepath.Join(f.private, "output-refs.json") + `"; fi
if [[ "$1" == v03-reap && -s "` + filepath.Join(f.private, "server.pid") + `" ]]; then echo "reap-saw-live-server" >> "` + f.log + `"; fi
case "${STUB_FAIL:-}" in *"$1"*) echo "stub failure of $1" >&2; exit 1;; esac
exit 0
`
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	f.envFile = filepath.Join(f.private, "cleanup.env")
	env := fmt.Sprintf("WORK=%q\nPRIVATE=%q\nCTL=%q\nKUBECTL=kubectl\nHOST_KUBECONFIG=/dev/null\nHOST_CONTEXT=ctx\nRUN_ID=v03-test\nORG=acme\nVAULT_ADDR_LOCAL=http://127.0.0.1:1\nMOUNT=secret\nOUTPUT_VAULT_ADDR=http://127.0.0.1:1\nOUTPUT_MOUNT=secret\nOUTPUT_REFS_FILE=%q\nOUTPUT_REFS_OWNER_URL_FILE=%q\nOUTPUT_CLEANUP_TOKEN=%q\nPG_ADMIN=%q\n", f.work, f.private, stub, filepath.Join(f.private, "output-refs.json"), filepath.Join(f.private, "owner.url"), filepath.Join(f.private, "output-cleanup-token"), filepath.Join(dir, "pg-admin"))
	if err := os.WriteFile(f.envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"baseline.json": "{}", "pg-manifest.json": "{}", "state.key": "KEY", "console.url": "url", "owner.url": "owner-url", "vault-token": "tok", "output-cleanup-token": "cleanup", "output-refs.json": `["secret://vault/secret/orchestrator/outputs/0123456789abcdef/11111111-1111-4111-8111-111111111111"]`} {
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
	if strings.Join(calls, ",") != "v03-reap,v04-output-refs,v03-vault-clean,v04-output-clean,pg-drop" {
		t.Fatalf("order of operations = %v (the cluster and both exact Vault inventories, then the database)", calls)
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

func TestCleanupPreservesLegacyV03WithoutV04OutputInventory(t *testing.T) {
	f := newCleanupFixture(t)
	env, err := os.ReadFile(f.envFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(env), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "OUTPUT_REFS_FILE=") || strings.HasPrefix(line, "OUTPUT_REFS_OWNER_URL_FILE=") || strings.HasPrefix(line, "OUTPUT_CLEANUP_TOKEN=") {
			continue
		}
		kept = append(kept, line)
	}
	if err := os.WriteFile(f.envFile, []byte(strings.Join(kept, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runCleanup(t, f, ""); err != nil {
		t.Fatalf("legacy V03 cleanup failed: %v", err)
	}
	if got := strings.Join(f.calls(), ","); got != "v03-reap,v03-vault-clean,pg-drop" {
		t.Fatalf("legacy cleanup called V04 inventory without opt-in: %s", got)
	}
	log, _ := os.ReadFile(filepath.Join(f.work, "cleanup.txt"))
	if !strings.Contains(string(log), "V04 output inventory is not configured") {
		t.Fatal("legacy no-inventory path was not identified")
	}
}

func TestCleanupKeepsTheDatabaseAndRecoveryMaterialWhenAnythingIsNotProven(t *testing.T) {
	for name, stubFail := range map[string]string{"cluster api error": "v03-reap", "output reference inventory not proven": "v04-output-refs", "account vault not proven": "v03-vault-clean", "output vault not proven": "v04-output-clean"} {
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
			for _, kept := range []string{"state.key", "console.url", "vault-token", "output-cleanup-token", "output-refs.json", "pg-manifest.json", "baseline.json"} {
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

// A browser leader may die before its recorder. Run the actual runner's
// post-wait fragment, then the real cleanup against a live orphan child.
func TestBrowserLeaderExitKeepsReceiptUntilOrphanGroupIsReaped(t *testing.T) {
	f := newCleanupFixture(t)
	raw, err := os.ReadFile("../v04-internal-full.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	start := strings.Index(script, `wait "${NODE_PID}" || STATUS=$?`)
	if start < 0 {
		t.Fatal("browser wait boundary missing")
	}
	end := strings.Index(script[start:], "# ---- Post-run checks")
	if end < 0 {
		t.Fatal("post-run check boundary missing")
	}
	fragment := script[start : start+end]
	body := `source "$1"
STATUS=0
setsid bash -c 'sleep 300 </dev/null >/dev/null 2>&1 & echo "$!" > "$1"; exit 1' _ "$PRIVATE/orphan-child.pid" &
NODE_PID=$!
printf '%s\n' "$NODE_PID" > "$PRIVATE/node.pid"
` + fragment
	cmd := exec.Command("bash", "-c", body, "_", f.envFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser exit fragment: %v: %s", err, out)
	}
	receipt, err := os.ReadFile(filepath.Join(f.private, "node.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pgid, err := strconv.Atoi(strings.TrimSpace(string(receipt)))
	if err != nil || pgid <= 1 {
		t.Fatalf("leader exit lost group receipt: %q", receipt)
	}
	t.Cleanup(func() {
		if groupAlive(pgid) {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	})
	if !groupAlive(pgid) {
		t.Fatal("regression fixture did not retain its recorder-like child")
	}
	result, err := os.ReadFile(filepath.Join(f.work, "result.txt"))
	if err != nil || strings.TrimSpace(string(result)) != "FAIL" {
		t.Fatalf("browser crash must be explicit FAIL: %q %v", result, err)
	}
	if err := runCleanup(t, f, ""); err != nil {
		t.Fatalf("orphan group cleanup: %v", err)
	}
	if groupAlive(pgid) {
		t.Fatal("recorder-like child survived verified cleanup")
	}
}

func TestCleanupMetadataGuardBlocksDeletionAndRetainsRecovery(t *testing.T) {
	for _, failure := range []string{"", "v04-output-inventory", "v04-output-refs"} {
		t.Run("failure-"+failure, func(t *testing.T) {
			f := newCleanupFixture(t)
			env, err := os.OpenFile(f.envFile, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(env, "OUTPUT_BASELINE_FILE=%q\n", filepath.Join(f.private, "output-baseline.json"))
			env.Close()
			if err := os.WriteFile(filepath.Join(f.private, "output-baseline.json"), []byte(`{"version":1}`), 0600); err != nil {
				t.Fatal(err)
			}
			err = runCleanup(t, f, failure)
			calls := strings.Join(f.calls(), ",")
			if failure == "" {
				if err != nil {
					t.Fatal(err)
				}
				if calls != "v03-reap,v04-output-refs,v03-vault-clean,v04-output-inventory,v04-output-clean,v04-output-inventory,pg-drop" {
					t.Fatalf("order=%s", calls)
				}
			} else {
				if err == nil {
					t.Fatal("unproven inventory must fail")
				}
				if strings.Contains(calls, "v04-output-clean") || strings.Contains(calls, "pg-drop") {
					t.Fatalf("destructive cleanup followed unproven inventory: %s", calls)
				}
				for _, name := range []string{"pg-manifest.json", "state.key", "output-baseline.json", "owner.url"} {
					if _, err := os.Stat(filepath.Join(f.private, name)); err != nil {
						t.Fatalf("recovery handle %s lost: %v", name, err)
					}
				}
			}
		})
	}
}
