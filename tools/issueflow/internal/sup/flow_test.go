package sup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/state"
)

func TestClaimsOnlyOwnerIssuesSequentially(t *testing.T) {
	h := newHarness(t, nil)
	h.issue(3, "stranger", "stranger", "agent:ready") // wrong author
	h.issue(2, "owner", "stranger", "agent:ready")    // owner wrote it, stranger labeled
	h.issue(5, "owner", "owner", "agent:ready")
	h.issue(4, "owner", "owner", "agent:ready")
	h.ticks(3)
	tk := h.task(0)
	if tk.Issue != 4 {
		t.Fatalf("expected lowest eligible issue 4, got %d", tk.Issue)
	}
	ts, _ := h.st.ListTasks()
	if len(ts) != 1 {
		t.Fatalf("one task at a time, got %d", len(ts))
	}
	if h.commentsContaining(4, "Claimed by issueflow") != 1 {
		t.Fatal("claim comment missing or duplicated")
	}
	h.ticks(20)
	ts, _ = h.st.ListTasks()
	if len(ts) != 1 {
		t.Fatalf("second task started while first is active: %d", len(ts))
	}
}

func TestWorktreeIsolationAndUserChangesPreserved(t *testing.T) {
	h := newHarness(t, nil)
	os.WriteFile(filepath.Join(h.repo, "mine.txt"), []byte("user work"), 0o644)
	os.WriteFile(filepath.Join(h.repo, "README.md"), []byte("user edit\n"), 0o644)
	tk := h.startActive()
	if tk.Worktree == h.repo || !strings.HasPrefix(tk.Worktree, h.cfg.WorktreeRoot) {
		t.Fatalf("worktree not isolated: %s", tk.Worktree)
	}
	h.wtWrite(tk, "new.go", "package x\n")
	if b, _ := os.ReadFile(filepath.Join(h.repo, "README.md")); string(b) != "user edit\n" {
		t.Fatal("user's edit to the main checkout was touched")
	}
	if _, err := os.Stat(filepath.Join(h.repo, "mine.txt")); err != nil {
		t.Fatal("user's untracked file vanished")
	}
	if _, err := os.Stat(filepath.Join(h.repo, "new.go")); err == nil {
		t.Fatal("task file leaked into the main checkout")
	}
	if got := git(t, h.repo, "branch", "--show-current"); got != "main" {
		t.Fatalf("main checkout branch changed to %s", got)
	}
}

func TestWaitUserQuestionIDActorAndQueueStop(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.issue(9, "owner", "owner", "agent:ready")
	h.submit(tk, "coordinator", CmdAsk, map[string]string{"file": h.writeTemp("q.md", "Use A or B?")}, nil)
	h.until("wait user", 10, func() bool { return h.task(0).State == state.WaitUser })
	q := h.task(0).Question
	if q == nil || !strings.HasPrefix(q.ID, "Q-") {
		t.Fatalf("question not recorded: %+v", q)
	}
	h.until("question posted", 5, func() bool { return h.task(0).Question.CommentID != 0 })
	if h.commentsContaining(1, q.ID) != 1 {
		t.Fatal("question comment missing or duplicated")
	}
	// wrong actor, wrong id, right id: only the last resumes.
	h.gh.AddComment(1, "stranger", "/answer "+q.ID+" A")
	h.gh.AddComment(1, "owner", "/answer Q-deadbeef A")
	h.clk.Advance(241 * time.Second)
	h.ticks(2)
	if h.task(0).State != state.WaitUser {
		t.Fatal("answer from wrong actor or with wrong id resumed the task")
	}
	runID := h.task(0).RunID
	h.gh.AddComment(1, "owner", "/answer "+q.ID+" choose B")
	h.clk.Advance(241 * time.Second)
	h.ticks(2)
	tk = h.task(0)
	if tk.State != state.Running || tk.RunID != runID || tk.Question == nil || !tk.Question.Answered || tk.Question.Answer != "choose B" {
		t.Fatalf("not resumed on same task: state=%s run=%s q=%+v", tk.State, tk.RunID, tk.Question)
	}
	if len(h.events("answer")) != 1 {
		t.Fatal("coordinator did not receive the answer")
	}
	// the queue stayed stopped: issue 9 was never claimed.
	if ts, _ := h.st.ListTasks(); len(ts) != 1 {
		t.Fatalf("queue was not stopped, %d tasks", len(ts))
	}
	// local answer with a wrong id is rejected too.
	h.submit(tk, "user", CmdAnswer, map[string]string{"id": "Q-1", "text": "x"}, nil)
	h.ticks(1)
}

func TestQuotaTimerCannotBypassWaitUser(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.startCoder()
	tk = h.task(0)
	tmux := h.tm
	_ = tmux
	// coder hits a limit with a known reset 2h away
	appendLog(t, tk.Coder.Session.Log, "You've hit your usage limit. resets in 2h 0m\n")
	h.clk.Advance(30 * time.Second)
	h.until("wait quota", 10, func() bool { return h.task(0).State == state.WaitQuota })
	w := h.task(0).Wake
	if w == nil || w.Role != "coder" || w.Computed != "reset" {
		t.Fatalf("wake: %+v", w)
	}
	// the coordinator asks the user meanwhile -> WAIT_USER takes precedence
	h.submit(h.task(0), "coordinator", CmdAsk, map[string]string{"file": h.writeTemp("q.md", "decide?")}, nil)
	h.until("wait user", 5, func() bool { return h.task(0).State == state.WaitUser })
	// the quota timer elapses while the question is open
	h.clk.Advance(3 * time.Hour)
	h.ticks(5)
	if h.task(0).State != state.WaitUser {
		t.Fatalf("quota timer bypassed the human stop: %s", h.task(0).State)
	}
	// resume-now cannot skip it either
	h.submit(h.task(0), "user", CmdResume, nil, nil)
	h.ticks(2)
	if h.task(0).State != state.WaitUser {
		t.Fatal("resume skipped WAIT_USER")
	}
	// after the answer the elapsed wake lets the work continue
	q := h.task(0).Question
	h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": q.ID, "text": "go"}, nil)
	h.until("resumed", 10, func() bool {
		s := h.task(0).State
		return s == state.Running || s == state.WaitQuota || s == state.Review
	})
}

func appendLog(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(s)
}
