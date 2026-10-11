package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/state"
)

func goBuild(t *testing.T, out, pkg string) {
	t.Helper()
	c := exec.Command("go", "build", "-o", out, pkg)
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, b)
	}
}

func sh(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// A complete local run of the real issueflow binary against fake gh, tmux,
// codex, claude and git-origin processes: claim, worktree, coordinator turns,
// coder in a real session with a trust dialog, report, checks, commit/push by
// the "coordinator", evidence release and the final comment. No network, no
// model, no browser: this is a CLI-only task, so no video is produced.
func TestCLISmokeWithFakeCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	for _, tool := range []string{"git", "bash", "tail", "sleep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " missing")
		}
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0o755)
	realGit, _ := exec.LookPath("git")
	goBuild(t, filepath.Join(bin, "issueflow"), ".")
	for _, f := range []string{"gh", "tmux", "codex", "clauded", "git"} {
		goBuild(t, filepath.Join(bin, f), "./testdata/fakes/"+f)
	}
	fakeDir := filepath.Join(root, "fake")
	os.MkdirAll(fakeDir, 0o755)
	remote := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "repo")
	sh(t, root, realGit, "init", "--bare", "-b", "main", remote)
	sh(t, root, realGit, "clone", remote, repo)
	sh(t, repo, realGit, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644)
	sh(t, repo, realGit, "add", ".")
	sh(t, repo, realGit, "commit", "-m", "init")
	sh(t, repo, realGit, "push", "-u", "origin", "main")
	baseSHA := sh(t, repo, realGit, "rev-parse", "HEAD")
	// a dirty user checkout must stay untouched
	os.WriteFile(filepath.Join(repo, "mine.txt"), []byte("user work"), 0o644)

	gh := map[string]any{"issues": []map[string]any{
		{"number": 1, "title": "Add feature", "body": "Create feature.txt", "author": "owner", "labels": []string{"agent:ready"}},
		{"number": 2, "title": "Second", "body": "later", "author": "owner", "labels": []string{"agent:ready"}},
	}}
	b, _ := json.Marshal(gh)
	os.WriteFile(filepath.Join(fakeDir, "gh.json"), b, 0o644)

	cfgPath := filepath.Join(root, "my config.json") // a space on purpose
	cfg := fmt.Sprintf(`{"repo":"o/r","repo_path":%q,"state_dir":%q,"worktree_root":%q,"owner_logins":["owner"],
	 "quiet_seconds":1,"enter_delay_seconds":1,"submit_check_seconds":4,"tick_seconds":1,"ready_timeout_seconds":60,
	 "coordinator":{"command":"codex","args":["--yolo","--json"]},
	 "coder":{"adapter":"claude-local","command":"clauded"},
	 "checks":[{"name":"unit","cmd":"true"}]}`, repo, filepath.Join(root, "state"), filepath.Join(root, "wt"))
	os.WriteFile(cfgPath, []byte(cfg), 0o600)

	// A private HOME keeps the developer's ~/.bashrc (and its real `clauded`
	// alias) out of the session shells, and a guard `claude` makes any attempt
	// to start a real Claude fail loudly instead of running a model.
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0o755)
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho 'REAL claude must not run in this test' >&2\nexit 99\n"), 0o755)
	env := append(os.Environ(),
		"HOME="+home,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_DIR="+fakeDir, "FAKE_REAL_GIT="+realGit, "FAKE_ISSUEFLOW_BIN="+filepath.Join(bin, "issueflow"))
	for _, e := range []string{"ORCH_KIND_VERIFY", "ORCHESTRATOR_POSTGRES_TEST_URL"} {
		env = append(env, e+"=")
	}
	once := func() string {
		c := exec.Command(filepath.Join(bin, "issueflow"), "--config", cfgPath, "once")
		c.Env = env
		c.Dir = root
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("once: %v\n%s", err, out)
		}
		return string(out)
	}
	stateDir := filepath.Join(root, "state")
	loadTask := func() *state.Task {
		st, _ := state.Open(stateDir)
		ts, _ := st.ListTasks()
		if len(ts) == 0 {
			return nil
		}
		return ts[0]
	}
	defer func() { // never leave fake sessions behind
		ents, _ := os.ReadDir(filepath.Join(fakeDir, "tmux"))
		for _, e := range ents {
			exec.Command(filepath.Join(bin, "tmux"), "kill-session", "-t", e.Name()).Run()
		}
		exec.Command("pkill", "-f", filepath.Join(fakeDir, "tmux")).Run()
	}()

	deadline := time.Now().Add(120 * time.Second)
	var last state.State
	for time.Now().Before(deadline) {
		once()
		tk := loadTask()
		if tk != nil {
			if tk.State != last {
				t.Logf("state -> %s", tk.State)
				last = tk.State
			}
			if tk.State == state.Done {
				break
			}
			if tk.State == state.Failed || tk.State == state.WaitUser {
				lg, _ := os.ReadFile(tk.Coder.Session.Log)
				ev, _ := os.ReadFile(filepath.Join(stateDir, "tasks", "1", "events.jsonl"))
				t.Fatalf("task stopped in %s: %+v %s\ncoder log:\n%q\nevents:\n%s", tk.State, tk.Question, tk.LastError, lg, ev)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	tk := loadTask()
	if tk == nil || tk.State != state.Done {
		ev, _ := os.ReadFile(filepath.Join(stateDir, "tasks", "1", "events.jsonl"))
		t.Fatalf("task did not finish: %+v\n%s", tk, ev)
	}

	// results
	if got := sh(t, repo, realGit, "rev-parse", "main"); got != baseSHA {
		t.Fatal("the base branch moved")
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "mine.txt")); string(b) != "user work" {
		t.Fatal("user's checkout was modified")
	}
	if got := sh(t, remote, realGit, "show", tk.Branch+":feature.txt"); got != "done" {
		t.Fatalf("task branch lacks the work: %q", got)
	}
	var db struct {
		Issues []struct {
			Number int
			Labels []string
		}
		Comments map[string][]struct{ Body string }
		Writes   []string
	}
	raw, _ := os.ReadFile(filepath.Join(fakeDir, "gh.json"))
	json.Unmarshal(raw, &db)
	finals := 0
	for _, c := range db.Comments["1"] {
		if strings.Contains(c.Body, "task complete") {
			finals++
			for _, want := range []string{"No browser video: CLI-only fake task", "test -f feature.txt", "pr/1", "Manifest sha256"} {
				if !strings.Contains(c.Body, want) {
					t.Errorf("final comment lacks %q:\n%s", want, c.Body)
				}
			}
			if strings.Contains(c.Body, "not-a-real-token") {
				t.Error("credentials leaked into a comment")
			}
		}
	}
	if finals != 1 {
		t.Fatalf("final comment count %d", finals)
	}
	hasLabel := func(n int, l string) bool {
		for _, i := range db.Issues {
			if i.Number == n {
				for _, x := range i.Labels {
					if x == l {
						return true
					}
				}
			}
		}
		return false
	}
	if !hasLabel(1, "agent:done") || hasLabel(1, "agent:active") || hasLabel(2, "agent:active") {
		t.Fatalf("labels wrong: %+v", db.Issues)
	}
	assets, _ := os.ReadDir(filepath.Join(fakeDir, "assets"))
	if len(assets) != 1 {
		t.Fatalf("expected one evidence release, got %v", assets)
	}
	files, _ := os.ReadDir(filepath.Join(fakeDir, "assets", assets[0].Name()))
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	joined := strings.Join(names, " ")
	if !strings.Contains(joined, "manifest-") || !strings.Contains(joined, "check-unit.log") || strings.Contains(joined, ".mp4") {
		t.Fatalf("evidence assets: %v", names)
	}
	// the supervisor released its lock and a status report works
	c := exec.Command(filepath.Join(bin, "issueflow"), "--config", cfgPath, "status", "--audit")
	c.Env = env
	out, err := c.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "DONE") {
		t.Fatalf("status: %v\n%s", err, out)
	}
	// all fake sessions are gone and no stray process of the session remains
	for _, e := range sessionsAlive(fakeDir) {
		t.Errorf("session still alive: %s", e)
	}
}

func sessionsAlive(fakeDir string) []string {
	var out []string
	ents, _ := os.ReadDir(filepath.Join(fakeDir, "tmux"))
	for _, e := range ents {
		if b, err := os.ReadFile(filepath.Join(fakeDir, "tmux", e.Name(), "pid")); err == nil {
			out = append(out, e.Name()+" pid "+string(b))
		}
	}
	return out
}
