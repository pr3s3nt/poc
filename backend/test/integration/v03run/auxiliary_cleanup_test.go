package v03run

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func startOwnedVault(t *testing.T, f *cleanupFixture, expectedName string) (int, string, <-chan error) {
	t.Helper()
	portFile := filepath.Join(f.private, "vault-port")
	server := `const fs=require('node:fs'),net=require('node:net');const s=net.createServer(c=>c.end('ok'));s.listen(0,'127.0.0.1',()=>fs.writeFileSync(process.argv[1],String(s.address().port)));process.on('SIGTERM',()=>s.close(()=>process.exit(0)));`
	cmd := exec.Command("node", "-e", server, portFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	var port string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if raw, err := os.ReadFile(portFile); err == nil {
			port = strings.TrimSpace(string(raw))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if port == "" {
		t.Fatal("owned Vault fixture did not bind a port")
	}
	address := "http://127.0.0.1:" + port
	commBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	comm := strings.TrimSpace(string(commBytes))
	if expectedName != "" {
		comm = expectedName
	}
	statBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	stat := string(statBytes)
	closeParen := strings.LastIndex(stat, ") ")
	if closeParen < 0 {
		t.Fatalf("unexpected process stat format: %q", stat)
	}
	fields := strings.Fields(stat[closeParen+2:])
	if len(fields) <= 19 {
		t.Fatalf("process stat is missing start time: %q", stat)
	}
	startTime := fields[19]
	data := fmt.Sprintf("%d\n%d\n%s\n%s\n%s\n", cmd.Process.Pid, cmd.Process.Pid, comm, address, startTime)
	if err := os.WriteFile(filepath.Join(f.private, "aux-secret-vault.pid"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid, address, done
}

func portReachable(address string) bool {
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func minimalAuxFixture(t *testing.T) *cleanupFixture {
	t.Helper()
	f := newCleanupFixture(t)
	for _, name := range []string{"baseline.json", "product-started", "pg-manifest.json"} {
		_ = os.Remove(filepath.Join(f.private, name))
	}
	return f
}

func TestCleanupWaitsForRunOwnedVaultProcessAndPortAbsence(t *testing.T) {
	f := minimalAuxFixture(t)
	pid, address, exited := startOwnedVault(t, f, "")
	if !portReachable(address) {
		t.Fatal("Vault fixture is not listening before cleanup")
	}
	if err := runCleanup(t, f, ""); err != nil {
		log, _ := os.ReadFile(filepath.Join(f.work, "cleanup.txt"))
		state, _ := os.ReadFile(filepath.Join(f.work, "cleanup-state.txt"))
		t.Logf("cleanup log=%s state=%s", log, state)
		t.Fatalf("cleanup failed: %v", err)
	}
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("owned Vault process did not exit")
	}
	if portReachable(address) {
		t.Fatal("owned Vault port still accepts connections after cleanup")
	}
	if err := syscall.Kill(-pid, 0); err == nil {
		t.Fatal("owned Vault process group still exists")
	}
	if state, _ := os.ReadFile(filepath.Join(f.work, "cleanup-state.txt")); strings.TrimSpace(string(state)) != "proven" {
		t.Fatalf("cleanup state = %q", state)
	}
}

func TestCleanupRefusesToSignalForeignAuxiliaryProcessIdentity(t *testing.T) {
	f := minimalAuxFixture(t)
	pid, address, exited := startOwnedVault(t, f, "foreign-process")
	if err := runCleanup(t, f, ""); err == nil {
		t.Fatal("uncertain process identity must fail cleanup")
	}
	if !portReachable(address) {
		t.Fatal("cleanup signalled a process whose identity did not match the owner record")
	}
	if err := syscall.Kill(-pid, 0); err != nil {
		t.Fatalf("foreign process was unexpectedly stopped: %v", err)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("fixture cleanup failed")
	}
}

func TestSupervisorStopsActualOwnedServiceAfterRunnerDeathAndDeadline(t *testing.T) {
	for _, mode := range []string{"runner-death", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			f := minimalAuxFixture(t)
			_, address, exited := startOwnedVault(t, f, "")
			runner := exec.Command("sleep", "300")
			if err := runner.Start(); err != nil {
				t.Fatal(err)
			}
			runnerDone := make(chan struct{})
			go func() { _ = runner.Wait(); close(runnerDone) }()
			deadline := time.Now().Add(time.Hour).Unix()
			if mode == "deadline" {
				deadline = time.Now().Add(2 * time.Second).Unix()
			}
			cmd := exec.Command("bash", "../v03-supervisor.sh", f.envFile, strconv.Itoa(runner.Process.Pid), strconv.FormatInt(deadline, 10))
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			if mode == "runner-death" {
				time.Sleep(250 * time.Millisecond)
				_ = runner.Process.Signal(syscall.SIGKILL)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("supervisor: %v", err)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("supervisor did not complete")
			}
			select {
			case <-runnerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("runner was not stopped")
			}
			select {
			case <-exited:
			case <-time.After(3 * time.Second):
				log, _ := os.ReadFile(filepath.Join(f.work, "cleanup.txt"))
				t.Fatalf("owned Vault did not exit; cleanup log=%s", log)
			}
			if portReachable(address) {
				t.Fatal("owned service port remains reachable")
			}
			if state, _ := os.ReadFile(filepath.Join(f.work, "cleanup-state.txt")); strings.TrimSpace(string(state)) != "proven" {
				t.Fatalf("cleanup state = %q", state)
			}
		})
	}
}
