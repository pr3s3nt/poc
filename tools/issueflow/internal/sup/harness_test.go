package sup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/clock"
	"issueflow/internal/config"
	"issueflow/internal/evidence"
	"issueflow/internal/gh"
	"issueflow/internal/gitx"
	"issueflow/internal/proc"
	"issueflow/internal/quota"
	"issueflow/internal/runner"
	"issueflow/internal/state"
	"issueflow/internal/tmux"
)

type harness struct {
	t        *testing.T
	dir      string
	remote   string
	repo     string
	cfg      *config.Config
	clk      *clock.Fake
	gh       *gh.Fake
	tm       *tmux.Fake
	pr       *proc.Fake
	rt       *runner.Router
	st       *state.Store
	s        *Sup
	ctx      context.Context
	identity string
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newHarness(t *testing.T, mut func(*config.Config)) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, dir: dir, ctx: context.Background(), identity: "o/r"}
	h.remote = filepath.Join(dir, "remote.git")
	h.repo = filepath.Join(dir, "repo")
	git(t, dir, "init", "--bare", "-b", "main", h.remote)
	git(t, dir, "clone", h.remote, h.repo)
	git(t, h.repo, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(h.repo, "README.md"), []byte("hello\n"), 0o644)
	git(t, h.repo, "add", ".")
	git(t, h.repo, "commit", "-m", "init")
	git(t, h.repo, "push", "-u", "origin", "main")

	cfg := &config.Config{Repo: "o/r", RepoPath: h.repo, BaseBranch: "main", StateDir: filepath.Join(dir, "state"),
		WorktreeRoot: filepath.Join(dir, "wt"), OwnerLogins: []string{"owner"}, QuietSeconds: 20, EnterDelaySeconds: 2, SubmitCheckSeconds: 10}
	cfg.Defaults()
	cfg.Checks = []config.CheckCfg{{Name: "unit", Cmd: "true"}}
	if mut != nil {
		mut(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	h.cfg = cfg
	h.clk = clock.NewFake(time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC))
	h.gh = gh.NewFake(h.clk.Now)
	h.tm = tmux.NewFake()
	h.pr = proc.NewFake()
	h.rt = runner.NewRouter(runner.OS{})
	h.rt.Handle("claude", func(c runner.Call) (runner.Result, error, bool) {
		if len(c.Args) > 0 && c.Args[0] == "--version" {
			return runner.Result{Stdout: "2.1.296 (Claude Code)"}, nil, true
		}
		if len(c.Args) > 0 && c.Args[0] == "--help" {
			return runner.Result{Stdout: "--cloud [x]\n--teleport [s]\n"}, nil, true
		}
		return runner.Result{}, nil, false
	})
	h.rt.Handle("ffprobe", func(c runner.Call) (runner.Result, error, bool) {
		return runner.Result{Stdout: `{"format":{"duration":"12.5"},"streams":[{"codec_type":"video","width":1440,"height":900}]}`}, nil, true
	})
	h.rt.Handle("ffmpeg", func(c runner.Call) (runner.Result, error, bool) { return runner.Result{}, nil, true })
	st, err := state.Open(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	h.st = st
	h.build()
	return h
}

// build creates a fresh supervisor over the same durable state and fakes,
// which is exactly what a process restart does.
func (h *harness) build() {
	cfg := h.cfg
	ghc := gh.GitHub(h.gh)
	local := &agent.Local{NameStr: cfg.Coder.Adapter, Command: cfg.Coder.Command, T: h.tm, R: h.rt, Procs: h.pr}
	var cloud agent.Agent
	if cfg.Cloud.Mode != "off" {
		cloud = &agent.Cloud{Command: "claude", R: runner.Timeout{R: h.rt, ByName: cfg.TimeoutMap()}}
	}
	q := quota.New(cfg.Quota.QuotaPatterns, cfg.Quota.ContextPatterns, cfg.Quota.AuthPatterns, cfg.Quota.MarginSeconds, cfg.Quota.BackoffSeconds)
	q.Loc = time.UTC
	h.s = New(Deps{Cfg: cfg, CfgPath: "/cfg.json", Binary: "/bin/issueflow", Store: h.st, Clock: h.clk, GH: ghc, Tmux: h.tm, Run: h.rt,
		Git: gitx.Git{R: runner.OS{}}, RepoIdentity: func(context.Context, string) (string, error) { return h.identity, nil }, Coder: local, Cloud: cloud, Procs: h.pr, Quota: q,
		Val: evidence.Validator{R: h.rt}, Pub: evidence.Publisher{GH: ghc, Prefix: cfg.Evidence.TagPrefix}})
	h.s.ProbeCloud(h.ctx)
}

func (h *harness) restart() { h.build() }

func (h *harness) tick() {
	h.t.Helper()
	h.clk.Advance(5 * time.Second)
	if err := h.s.Tick(h.ctx); err != nil {
		h.t.Fatalf("tick: %v", err)
	}
}

func (h *harness) ticks(n int) {
	for i := 0; i < n; i++ {
		h.tick()
	}
}

// until ticks (advancing the clock) until cond holds.
func (h *harness) until(what string, max int, cond func() bool) {
	h.t.Helper()
	for i := 0; i < max; i++ {
		if cond() {
			return
		}
		h.tick()
	}
	if !cond() {
		h.t.Fatalf("condition not reached: %s (state=%s)", what, h.task(0).State)
	}
}

func (h *harness) issue(n int, author, labeler string, labels ...string) {
	h.gh.AddIssue(gh.Issue{Number: n, Title: fmt.Sprintf("Issue %d", n), Body: "do the thing", Author: author, Labels: labels}, labeler)
}

// task returns the (single or named) task; n==0 means the active one.
func (h *harness) task(n int) *state.Task {
	h.t.Helper()
	ts, err := h.st.ListTasks()
	if err != nil {
		h.t.Fatal(err)
	}
	for _, t := range ts {
		if n == 0 && !t.State.Terminal() || t.Issue == n {
			return t
		}
	}
	if n == 0 && len(ts) > 0 {
		return ts[len(ts)-1]
	}
	if n == 0 {
		return &state.Task{}
	}
	h.t.Fatalf("no task %d", n)
	return nil
}

func (h *harness) submit(t *state.Task, role, kind string, args map[string]string, lists map[string][]string) {
	h.t.Helper()
	c := state.Command{Kind: kind, Role: role, Run: t.RunID, Args: args, Lists: lists, CreatedAt: h.clk.Now()}
	switch role {
	case "coordinator":
		c.Gen = t.CoordGen
	case "coder":
		c.Gen = t.Coder.Gen
	}
	if _, err := h.st.Enqueue(t.Issue, c); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) writeTemp(name, content string) string {
	p := filepath.Join(h.dir, "tmp", name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(content), 0o644)
	return p
}

// startActive claims issue 1 and runs until the coordinator's first turn runs.
func (h *harness) startActive() *state.Task {
	h.t.Helper()
	h.issue(1, "owner", "owner", "agent:ready")
	h.until("running", 10, func() bool { return h.task(0).State == state.Running })
	h.until("coordinator turn started", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
	return h.task(0)
}

// endTurn simulates a coordinator turn ending with the given exit and log.
func (h *harness) endTurn(exit int, log string) {
	h.t.Helper()
	t := h.task(0)
	c := t.Coordinator
	os.MkdirAll(filepath.Dir(c.Log), 0o755)
	os.WriteFile(c.Log, []byte(log), 0o644)
	os.WriteFile(c.ExitFile, []byte(fmt.Sprint(exit)), 0o644)
	h.tm.End(c.Session)
}

// coderUp drives a started coder through ready, paste and submit to running.
func (h *harness) coderUp() {
	h.t.Helper()
	h.until("coder launching", 5, func() bool { return h.task(0).Coder.Session.Status == "launching" })
	t := h.task(0)
	tmux.AppendLog(t.Coder.Session.Log, "Welcome\n? for shortcuts\n")
	h.clk.Advance(30 * time.Second)
	h.until("brief pasted", 5, func() bool { return h.task(0).Coder.Session.Stage == "pasted" })
	h.clk.Advance(3 * time.Second)
	h.until("entered", 5, func() bool { return h.task(0).Coder.Session.Stage == "entered" })
	tmux.AppendLog(t.Coder.Session.Log, strings.Repeat("working on it...\n", 10))
	h.until("coder running", 5, func() bool { return h.task(0).Coder.Session.Status == "running" && h.task(0).Coder.Session.Stage == "" })
}

func (h *harness) startCoder() {
	h.t.Helper()
	t := h.task(0)
	h.submit(t, "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("brief.md", "implement X")}, nil)
	h.coderUp()
}

func (h *harness) events(kind string) []state.Event {
	var out []state.Event
	for _, e := range h.task(0).Events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func (h *harness) commands() []state.Command { return h.task(0).Commands }

func (h *harness) lastCommand() state.Command {
	cs := h.commands()
	return cs[len(cs)-1]
}

func (h *harness) comments(n int) []gh.Comment { return h.gh.Cmts[n] }

func (h *harness) commentsContaining(n int, s string) int {
	c := 0
	for _, cm := range h.gh.Cmts[n] {
		if strings.Contains(cm.Body, s) {
			c++
		}
	}
	return c
}

func (h *harness) wtWrite(t *state.Task, rel, content string) {
	h.t.Helper()
	p := filepath.Join(t.Worktree, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// accept submits an acceptance carrying the content hash seen right now,
// exactly like the CLI does.
func (h *harness) accept(tk *state.Task, checks ...string) {
	h.t.Helper()
	hash, err := h.s.D.Git.ContentHash(h.ctx, tk.Worktree)
	if err != nil {
		h.t.Fatal(err)
	}
	var lists map[string][]string
	if len(checks) > 0 {
		lists = map[string][]string{"check": checks}
	}
	h.submit(tk, "coordinator", CmdAccept, map[string]string{"note": "ok", "hash": hash}, lists)
}

// tickErr advances the clock and ticks, returning the tick error to the caller.
func (h *harness) tickErr() error {
	h.clk.Advance(5 * time.Second)
	return h.s.Tick(h.ctx)
}

func filepathStat(p string) (os.FileInfo, error) { return os.Stat(p) }

func osWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func execGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}
