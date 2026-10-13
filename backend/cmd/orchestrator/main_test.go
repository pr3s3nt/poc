package main

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"orchestrator/internal/ports/persistence/persistencetest"
)

// TestMain runs main() when the test binary is re-executed as the
// orchestrator command by the CLI tests below.
func TestMain(m *testing.M) {
	if os.Getenv("ORCHESTRATOR_MAIN_UNDER_TEST") == "1" {
		os.Args = append([]string{"orchestrator"}, os.Args[1:]...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// command runs the orchestrator command with args and a clean environment
// (no inherited ORCHESTRATOR_* settings), capturing its output.
func command(args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], args...)
	env := []string{"ORCHESTRATOR_MAIN_UNDER_TEST=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ORCHESTRATOR_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	return cmd
}

func privateFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func password(url string) string {
	at := strings.LastIndex(url, "@")
	colon := strings.LastIndex(url[:at], ":")
	return url[colon+1 : at]
}

// The CLI wiring of option A: -setup-database is the explicit owner step
// (owner URL only from an owner-only file), the runtime starts with separate
// Console and State URL files plus an independent key file, and an owner URL
// given as the runtime login or a world-readable credential file is refused.
// No output contains a password.
func TestDatabaseSetupAndRuntimeLoginsThroughTheCLI(t *testing.T) {
	db := persistencetest.FreshPostgresRoles(t)
	dir := t.TempDir()
	owner := privateFile(t, dir, "owner-url", db.Owner, 0o600)
	console := privateFile(t, dir, "console-url", db.Console, 0o600)
	state := privateFile(t, dir, "state-url", db.State, 0o400)
	key := privateFile(t, dir, "state-key", strings.Repeat("ef", 32), 0o600)
	secrets := []string{password(db.Owner), password(db.Console), password(db.State), password(db.Admin)}
	run := func(label string, wantOK bool, args ...string) string {
		t.Helper()
		out, err := command(args...).CombinedOutput()
		if (err == nil) != wantOK {
			t.Fatalf("%s: exit error %v\n%s", label, err, out)
		}
		for _, s := range secrets {
			if s != "" && bytes.Contains(out, []byte(s)) {
				t.Fatalf("%s: output contains a password", label)
			}
		}
		return string(out)
	}
	roles := []string{"-console-database-role", db.ConsoleRole, "-state-database-role", db.StateRole}

	run("runtime before setup", false, "-database-url-file", console, "-addr", "127.0.0.1:0")
	exposed := privateFile(t, dir, "owner-url-readable", db.Owner, 0o644)
	if out := run("world-readable owner file", false, append([]string{"-setup-database", "-database-owner-url-file", exposed}, roles...)...); !strings.Contains(out, "owner-only") {
		t.Fatalf("readable owner file: %s", out)
	}
	if out := run("setup", true, append([]string{"-setup-database", "-database-owner-url-file", owner}, roles...)...); !strings.Contains(out, "baseline created") {
		t.Fatalf("setup output: %s", out)
	}
	if out := run("repeated setup", true, append([]string{"-setup-database", "-database-owner-url-file", owner}, roles...)...); !strings.Contains(out, "nothing changed") {
		t.Fatalf("repeated setup output: %s", out)
	}
	if out := run("owner as runtime", false, "-database-url-file", owner, "-addr", "127.0.0.1:0"); !strings.Contains(out, "least-privilege") {
		t.Fatalf("owner runtime output: %s", out)
	}
	run("key without State login", false, "-database-url-file", console, "-terraform-state-key-file", key, "-terraform-state-key-id", "key-1", "-addr", "127.0.0.1:0")
	run("key file reused as credential", false, "-database-url-file", console, "-terraform-state-database-url-file", key, "-terraform-state-key-file", key, "-terraform-state-key-id", "key-1", "-addr", "127.0.0.1:0")

	addrFile := filepath.Join(dir, "addr")
	cmd := command("-database-url-file", console, "-terraform-state-database-url-file", state, "-terraform-state-key-file", key, "-terraform-state-key-id", "key-1", "-addr", "127.0.0.1:0", "-addr-file", addrFile, "-ui-dir", "")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	deadline := time.Now().Add(60 * time.Second)
	var addr []byte
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(addrFile); err == nil && len(b) > 0 {
			addr = b
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if addr == nil {
		t.Fatalf("server did not start:\n%s", out.String())
	}
	resp, err := http.Get("http://" + string(addr) + "/api/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
	// SIGTERM shuts the server down and closes both pools.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown: %v\n%s", err, out.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("server did not stop on SIGTERM")
	}
	for _, s := range secrets {
		if bytes.Contains(out.Bytes(), []byte(s)) {
			t.Fatal("server output contains a password")
		}
	}
}
