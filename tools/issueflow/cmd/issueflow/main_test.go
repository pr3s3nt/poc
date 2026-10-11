package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"issueflow/internal/gitx"
	"issueflow/internal/runner"
	"issueflow/internal/state"
)

func setup(t *testing.T) (cfg string, st *state.Store, wt string) {
	dir := t.TempDir()
	wt = filepath.Join(dir, "wt")
	os.MkdirAll(wt, 0o755)
	for _, a := range [][]string{{"init", "-b", "main", "."}, {"commit", "--allow-empty", "-m", "i"}} {
		c := exec.Command("git", a...)
		c.Dir = wt
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	cfg = filepath.Join(dir, "cfg.json")
	body := fmt.Sprintf(`{"repo":"o/r","repo_path":%q,"state_dir":%q,"worktree_root":%q,"owner_logins":["me"]}`, wt, filepath.Join(dir, "state"), filepath.Join(dir, "w"))
	os.WriteFile(cfg, []byte(body), 0o600)
	st, err := state.Open(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveTask(&state.Task{Issue: 4, RunID: "r1", State: state.Running, Worktree: wt, Branch: "b", Question: &state.Question{ID: "Q-1"}}); err != nil {
		t.Fatal(err)
	}
	return cfg, st, wt
}

func spooled(t *testing.T, st *state.Store) []state.Command {
	cmds, _, err := st.Spooled(4)
	if err != nil {
		t.Fatal(err)
	}
	return cmds
}

func TestAnswerAndRoleGating(t *testing.T) {
	cfg, st, _ := setup(t)
	if err := run([]string{"--config", cfg, "answer", "--issue", "4", "--id", "Q-1", "--text", "yes"}); err != nil {
		t.Fatal(err)
	}
	c := spooled(t, st)
	if len(c) != 1 || c[0].Kind != "answer" || c[0].Role != "user" || c[0].Args["id"] != "Q-1" {
		t.Fatalf("%+v", c)
	}
	// coordinator-only commands refuse without the coordinator role
	t.Setenv("ISSUEFLOW_ROLE", "coder")
	t.Setenv("ISSUEFLOW_ISSUE", "4")
	t.Setenv("ISSUEFLOW_RUN", "r1")
	t.Setenv("ISSUEFLOW_GEN", "1")
	for _, args := range [][]string{
		{"--config", cfg, "agent", "start", "--brief-file", "/x"},
		{"--config", cfg, "ask", "--file", "/x"},
		{"--config", cfg, "report", "accept"},
		{"--config", cfg, "report", "published", "--sha", "abc"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("coder was allowed to run %v", args)
		}
	}
	t.Setenv("ISSUEFLOW_ROLE", "coordinator")
	if err := run([]string{"--config", cfg, "report", "done", "--text", "x"}); err == nil {
		t.Fatal("coordinator allowed to file a coder report")
	}
	if err := run([]string{"--config", cfg, "answer", "--issue", "4", "--id", "", "--text", ""}); err == nil {
		t.Fatal("empty answer accepted")
	}
}

func TestReportAcceptCapturesHashAtSubmission(t *testing.T) {
	cfg, st, wt := setup(t)
	t.Setenv("ISSUEFLOW_ROLE", "coordinator")
	t.Setenv("ISSUEFLOW_ISSUE", "4")
	t.Setenv("ISSUEFLOW_RUN", "r1")
	t.Setenv("ISSUEFLOW_GEN", "2")
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("reviewed"), 0o644)
	want, _ := (gitx.Git{R: runner.OS{}}).ContentHash(context.Background(), wt)
	if err := run([]string{"--config", cfg, "report", "accept", "--note", "n", "--check", "go test ./...", "--check", "go vet ./..."}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("edited later"), 0o644)
	c := spooled(t, st)
	if len(c) != 1 || c[0].Args["hash"] != want || c[0].Gen != 2 || c[0].Run != "r1" || len(c[0].Lists["check"]) != 2 {
		t.Fatalf("%+v want hash %s", c, want)
	}
	// explicit --hash from a review wins
	if err := run([]string{"--config", cfg, "report", "accept", "--hash", "abc123"}); err != nil {
		t.Fatal(err)
	}
	if c := spooled(t, st); c[len(c)-1].Args["hash"] != "abc123" {
		t.Fatal("explicit hash ignored")
	}
}

func TestStatusAndAuditAndUnknownCommands(t *testing.T) {
	cfg, st, _ := setup(t)
	if err := run([]string{"--config", cfg, "status", "--audit"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(st.Sub(4, "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil || m["task"] == nil {
		t.Fatalf("audit not generated: %s", b)
	}
	if err := run([]string{"--config", cfg, "bogus"}); err == nil {
		t.Fatal("unknown command accepted")
	}
	if err := run(nil); err == nil {
		t.Fatal("no command accepted")
	}
	os.Unsetenv("ISSUEFLOW_CONFIG")
	if err := run([]string{"status"}); err == nil || !strings.Contains(err.Error(), "config") {
		t.Fatalf("missing config: %v", err)
	}
}

func TestOnceRefusesWhenAnotherSupervisorHoldsTheLock(t *testing.T) {
	cfg, st, _ := setup(t)
	if err := st.Lock(); err != nil {
		t.Fatal(err)
	}
	defer st.Unlock()
	if err := run([]string{"--config", cfg, "once"}); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("second supervisor allowed: %v", err)
	}
}

func TestEvidenceSchemaAndHashCommands(t *testing.T) {
	cfg, st, wt := setup(t)
	tk, _ := st.LoadTask(4)
	tk.RunID = "r-schema"
	st.SaveTask(tk)
	capture := func(args ...string) string {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		err := run(args)
		w.Close()
		os.Stdout = old
		b, _ := io.ReadAll(r)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return string(b)
	}
	out := capture("--config", cfg, "evidence-schema", "--issue", "4")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	files, _ := m["files"].([]any)
	if len(files) < 5 || !strings.Contains(out, "r-schema") || !strings.Contains(out, filepath.Join(st.Dir, "tasks", "4", "evidence-in")) || m["not_provided"] == nil {
		t.Fatalf("schema incomplete:\n%s", out)
	}
	os.WriteFile(filepath.Join(wt, "x.txt"), []byte("x"), 0o644)
	want, _ := (gitx.Git{R: runner.OS{}}).ContentHash(context.Background(), wt)
	if got := strings.TrimSpace(capture("--config", cfg, "evidence-hash", "--issue", "4")); got != want {
		t.Fatalf("hash %s want %s", got, want)
	}
	if err := run([]string{"--config", cfg, "evidence-hash"}); err == nil {
		t.Fatal("evidence-hash needs an issue")
	}
}
