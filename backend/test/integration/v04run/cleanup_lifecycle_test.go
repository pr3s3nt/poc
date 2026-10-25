package v04run

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func cleanupEnv(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	work, private := filepath.Join(root, "work"), filepath.Join(root, "private")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(private, "cleanup.env")
	content := fmt.Sprintf("WORK=%q\nPRIVATE=%q\nRUN_ID=%q\nORG=%q\nCTL=%q\nKUBECTL=%q\nHOST_KUBECONFIG=%q\nHOST_CONTEXT=%q\nVAULT_ADDR_LOCAL=%q\nMOUNT=%q\nPG_ADMIN=%q\nOUTPUT_VAULT_ADDR=%q\nOUTPUT_MOUNT=%q\nOUTPUT_REFS_FILE=%q\nOUTPUT_REFS_OWNER_URL_FILE=%q\nOUTPUT_CLEANUP_TOKEN=%q\n",
		work, private, "test-run-lifecycle", "test-org", "/bin/false", "/bin/false", "/dev/null", "test-context", "http://127.0.0.1:1", "secret", filepath.Join(private, "pg-admin"), "http://127.0.0.1:1", "secret", filepath.Join(private, "output-refs.json"), filepath.Join(private, "owner.url"), filepath.Join(private, "output-cleanup-token"))
	if err := os.WriteFile(env, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return env, work, private
}

func startOwnedHTTPChild(t *testing.T) (*exec.Cmd, string, string, string) {
	t.Helper()
	cmd := exec.Command("setsid", "node", "-e", `const s=require('node:net').createServer(c=>{c.on('error',()=>{});c.end('owned')});s.listen(0,'127.0.0.1',()=>console.log(s.address().port));process.on('SIGTERM',()=>s.close(()=>process.exit(0)))`)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	port := strings.TrimSpace(line)
	if _, err := strconv.Atoi(port); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("child port %q: %v", port, err)
	}
	pid := cmd.Process.Pid
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	start := procStartTime(t, pid)
	return cmd, strconv.Itoa(pid), strings.TrimSpace(string(comm)), "http://127.0.0.1:" + port + "|" + start
}

func procStartTime(t *testing.T, pid int) string {
	t.Helper()
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(stat[strings.LastIndex(string(stat), ")")+2:])) // remainder begins at proc stat field 3
	if len(fields) < 20 {
		t.Fatalf("short proc stat for pid %d", pid)
	}
	return fields[19] // proc stat field 22 (starttime)
}

func writeAuxRecord(t *testing.T, private, pid, comm, target, start string) {
	t.Helper()
	content := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n", pid, pid, comm, target, start)
	if err := os.WriteFile(filepath.Join(private, "aux-secret-vault.pid"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func cleanupRun(t *testing.T, env string) error {
	t.Helper()
	cmd := exec.Command("bash", "./v03-cleanup.sh", env)
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cleanup: %w: %s", err, out)
	}
	return nil
}

func TestCleanupStopsExactOwnedAuxiliaryAndVerifiesServiceAbsence(t *testing.T) {
	env, work, private := cleanupEnv(t)
	child, pid, comm, target := startOwnedHTTPChild(t)
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	parts := strings.SplitN(target, "|", 2)
	writeAuxRecord(t, private, pid, comm, parts[0], parts[1])
	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(parts[0], "http://"), time.Second); err != nil {
		t.Fatalf("owned service did not start: %v", err)
	} else {
		_ = conn.Close()
	}
	if err := cleanupRun(t, env); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(work, "cleanup-state.txt")); strings.TrimSpace(string(got)) != "proven" {
		t.Fatalf("cleanup state = %q", got)
	}
	if err := child.Wait(); err != nil && !strings.Contains(err.Error(), "signal: terminated") {
		t.Fatalf("owned child wait: %v", err)
	}
	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(parts[0], "http://"), 250*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("cleanup returned proven while the owned service port was still open")
	}
}

func TestCleanupRefusesUncertainAuxiliaryOwnershipAndRetainsRecovery(t *testing.T) {
	env, work, private := cleanupEnv(t)
	child, pid, comm, target := startOwnedHTTPChild(t)
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	parts := strings.SplitN(target, "|", 2)
	writeAuxRecord(t, private, pid, "foreign-process", parts[0], parts[1])
	if err := cleanupRun(t, env); err == nil {
		t.Fatal("cleanup accepted a mismatched process owner")
	}
	if _, err := os.Stat(filepath.Join(work, "CLEANUP-NOT-PROVEN.txt")); err != nil {
		t.Fatalf("recovery handoff missing: %v", err)
	}
	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(parts[0], "http://"), time.Second); err != nil {
		t.Fatalf("uncertain ownership cleanup stopped a process it could not prove: %v", err)
	} else {
		_ = conn.Close()
	}
	writeAuxRecord(t, private, pid, comm, parts[0], parts[1])
	if err := cleanupRun(t, env); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil && !strings.Contains(err.Error(), "signal: terminated") {
		t.Fatalf("owned child wait: %v", err)
	}
}

func TestSupervisorCleansAfterRunnerDeathAndExpiredDeadline(t *testing.T) {
	for _, mode := range []string{"runner-death", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			env, work, _ := cleanupEnv(t)
			runner := exec.Command("bash", "-c", "trap 'exit 0' TERM; while :; do sleep 0.1; done")
			if err := runner.Start(); err != nil {
				t.Fatal(err)
			}
			waitRunner := make(chan error, 1)
			go func() { waitRunner <- runner.Wait() }()
			deadline := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
			if mode == "deadline" {
				deadline = strconv.FormatInt(time.Now().Add(-time.Second).Unix(), 10)
			}
			supervisor := exec.Command("bash", "./v03-supervisor.sh", env, strconv.Itoa(runner.Process.Pid), deadline)
			supervisor.Dir = ".."
			if err := supervisor.Start(); err != nil {
				_ = runner.Process.Kill()
				_ = runner.Wait()
				t.Fatal(err)
			}
			if mode == "runner-death" {
				_ = runner.Process.Kill()
			}
			if err := supervisor.Wait(); err != nil {
				t.Fatalf("supervisor: %v", err)
			}
			<-waitRunner
			state, _ := os.ReadFile(filepath.Join(work, "cleanup-state.txt"))
			if strings.TrimSpace(string(state)) != "proven" {
				t.Fatalf("supervisor cleanup state = %q", state)
			}
			log, _ := os.ReadFile(filepath.Join(work, "cleanup.txt"))
			if !strings.Contains(string(log), "supervisor: cleanup proven") {
				t.Fatalf("supervisor did not record cleanup proof: %s", log)
			}
		})
	}
}
