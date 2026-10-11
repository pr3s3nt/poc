package sup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/config"
	"issueflow/internal/state"
)

// finishTask drives an active task with a running coder to DONE using cli evidence.
func (h *harness) acceptCLI(tk *state.Task) {
	h.t.Helper()
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "documentation-only change, no browser flow"}, nil)
	h.ticks(1)
	if c := h.lastCommand(); c.Status != "applied" {
		h.t.Fatalf("evidence rejected: %s", c.Result)
	}
	h.accept(h.task(0))
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
}

func (h *harness) commitPush(tk *state.Task) string {
	h.t.Helper()
	git(h.t, tk.Worktree, "add", "-A")
	git(h.t, tk.Worktree, "commit", "-m", "task work")
	git(h.t, tk.Worktree, "push", "origin", tk.Branch)
	return git(h.t, tk.Worktree, "rev-parse", "HEAD")
}

func TestFullLoopFeedbackAcceptPublishThenNextTask(t *testing.T) {
	h := newHarness(t, nil)
	h.issue(2, "owner", "owner", "agent:ready")
	tk := h.startActive()
	h.startCoder()
	tk = h.task(0)
	h.wtWrite(tk, "feature.go", "package feature\n")
	h.submit(tk, "coder", CmdReportDone, map[string]string{"text": "done, tests pass"}, nil)
	h.until("review", 5, func() bool { return h.task(0).State == state.Review })
	// first review: ask for a change
	fb := h.writeTemp("fb.md", "please add a doc comment")
	h.submit(h.task(0), "coordinator", CmdAgentSend, map[string]string{"file": fb}, nil)
	h.until("feedback pasted", 8, func() bool { return h.task(0).Coder.Session.Stage == "pasted" })
	sess := h.tm.Session(h.task(0).Coder.Session.Name)
	if len(sess.Pastes) < 2 || !strings.Contains(sess.Pastes[len(sess.Pastes)-1], "doc comment") {
		t.Fatalf("feedback not pasted: %v", sess.Pastes)
	}
	enters := sess.Enters
	h.clk.Advance(3 * time.Second)
	h.until("submitted", 5, func() bool { return h.tm.Session(h.task(0).Coder.Session.Name).Enters > enters })
	appendLog(t, h.task(0).Coder.Session.Log, strings.Repeat("editing...\n", 20))
	h.until("running again", 5, func() bool { return h.task(0).Coder.Session.Stage == "" })
	h.wtWrite(tk, "feature.go", "// Package feature does x.\npackage feature\n")
	h.submit(h.task(0), "coder", CmdReportDone, map[string]string{"text": "added comment"}, nil)
	h.until("review again", 5, func() bool { return h.task(0).State == state.Review })

	h.acceptCLI(h.task(0))
	if got := h.task(0); len(got.Checks) != 1 || got.Checks[0].Exit != 0 || !got.Publish.AwaitCommit {
		t.Fatalf("checks/publish state wrong: %+v %+v", got.Checks, got.Publish)
	}
	sha := h.commitPush(h.task(0))
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha, "pr": "https://example.invalid/pr/1"}, nil)
	h.until("done", 10, func() bool { return h.task(1).State == state.Done })

	if h.commentsContaining(1, "task complete") != 1 {
		t.Fatal("final summary must be posted exactly once")
	}
	final := h.gh.Cmts[1][len(h.gh.Cmts[1])-1].Body
	for _, want := range []string{sha, "exit 0", "No browser video: documentation-only", "Manifest sha256", "pr/1"} {
		if !strings.Contains(final, want) {
			t.Errorf("final comment lacks %q:\n%s", want, final)
		}
	}
	if strings.Contains(final, ".mp4") {
		t.Error("CLI-only task must not link a video")
	}
	tag := h.s.D.Pub.Tag(1, h.task(1).RunID)
	var haveManifest, haveLog bool
	for _, a := range h.gh.Assets[tag] {
		if strings.HasPrefix(a.Name, "manifest-") {
			haveManifest = true
		}
		if strings.Contains(a.Name, "check-unit.log") {
			haveLog = true
		}
	}
	if !haveManifest || !haveLog {
		t.Fatalf("assets: %+v", h.gh.Assets[tag])
	}
	if h.tm.Session(h.task(1).Coder.Session.Name) != nil {
		t.Fatal("coder session still alive after DONE")
	}
	labels := h.gh.Issues[1].Labels
	if !contains(labels, "agent:done") || contains(labels, "agent:active") {
		t.Fatalf("labels: %v", labels)
	}
	// only now the next Issue is claimed
	h.until("second task", 10, func() bool { return h.task(0).Issue == 2 })
	if h.task(0).RunID == h.task(1).RunID {
		t.Fatal("second task reused the run id")
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestStaleReportsAreRejected(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	// coordinator command from a future/old turn and from another run
	c := state.Command{Kind: CmdCheckpoint, Role: "coordinator", Run: "r-other", Gen: tk.CoordGen, Args: map[string]string{"file": "/x"}}
	h.st.Enqueue(1, c)
	c2 := c
	c2.Run, c2.Gen = tk.RunID, tk.CoordGen+5
	h.st.Enqueue(1, c2)
	h.ticks(1)
	for _, cm := range h.commands() {
		if cm.Status != "rejected" || !strings.Contains(cm.Result, "stale") {
			t.Fatalf("expected stale rejection, got %+v", cm)
		}
	}
	// coder report with an old generation after a restart of the coder
	h.startCoder()
	tk = h.task(0)
	old := tk.Coder.Gen
	h.submit(tk, "coordinator", CmdAgentStop, nil, nil)
	h.ticks(2)
	h.submit(h.task(0), "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("b2.md", "again")}, nil)
	h.coderUp()
	if h.task(0).Coder.Gen != old+1 {
		t.Fatalf("gen %d", h.task(0).Coder.Gen)
	}
	stale := state.Command{Kind: CmdReportDone, Role: "coder", Run: tk.RunID, Gen: old, Args: map[string]string{"text": "late"}}
	h.st.Enqueue(1, stale)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" {
		t.Fatalf("stale coder report applied: %+v", lc)
	}
	if h.task(0).Coder.Done {
		t.Fatal("stale report marked the new coder done")
	}
}

func TestAcceptanceStaleOnUntrackedEditButStableAcrossCommit(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "new_file.txt", "v1\n") // untracked
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.accept(tk, "true")
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
	accepted := h.task(0).Accept.Hash
	// staging and committing the same bytes must not change the hash
	sha := h.commitPush(h.task(0))
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.ticks(1)
	if h.lastCommand().Status != "applied" {
		t.Fatalf("published rejected after commit: %s", h.lastCommand().Result)
	}
	if got, _ := h.s.D.Git.ContentHash(h.ctx, h.task(0).Worktree); got != accepted {
		t.Fatal("content hash changed by git add/commit of identical bytes")
	}
}

func TestAcceptanceVoidWhenUntrackedFileEditedBeforeChecks(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Checks = nil })
	tk := h.startActive()
	h.wtWrite(tk, "u.txt", "v1\n")
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)
	// the check itself edits an untracked file: content changed while checks ran
	h.accept(h.task(0), "echo changed >> u.txt")
	h.until("review", 8, func() bool { return h.task(0).State == state.Review })
	if h.task(0).Accept != nil {
		t.Fatal("acceptance must be void")
	}
	if len(h.events("acceptance-void")) == 0 {
		t.Fatal("coordinator not told the acceptance is void")
	}
}

func TestAcceptRejectedWithoutEvidenceOrChecks(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Checks = nil })
	tk := h.startActive()
	h.accept(tk)
	h.ticks(1)
	if h.lastCommand().Status != "rejected" || !strings.Contains(h.lastCommand().Result, "evidence") {
		t.Fatalf("%+v", h.lastCommand())
	}
	h.submit(h.task(0), "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "r"}, nil)
	h.accept(h.task(0))
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "no checks") {
		t.Fatalf("%+v", lc)
	}
}

func TestPublishedRejectsDirtyOrUnpushed(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "a.txt", "a")
	h.acceptCLI(tk)
	tk = h.task(0)
	git(t, tk.Worktree, "add", "-A")
	git(t, tk.Worktree, "commit", "-m", "x")
	sha := git(t, tk.Worktree, "rev-parse", "HEAD")
	h.submit(tk, "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "push") {
		t.Fatalf("unpushed commit accepted: %+v", lc)
	}
	// pushing to the base branch is never what the supervisor verifies
	h.wtWrite(h.task(0), "late.txt", "late")
	git(t, tk.Worktree, "push", "origin", tk.Branch)
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" {
		t.Fatalf("dirty worktree accepted: %+v", lc)
	}
}

func TestEvidenceBrowserRequiredCannotDowngradeToCLI(t *testing.T) {
	h := newHarness(t, nil)
	h.issue(1, "owner", "owner", "agent:ready", "agent:browser")
	h.until("running", 10, func() bool { return h.task(0).State == state.Running })
	h.until("turn", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
	tk := h.task(0)
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "just trust me"}, nil)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "requires a browser") {
		t.Fatalf("%+v", lc)
	}
}

func TestEvidenceDirOutsideTaskAreaRejected(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	outside := filepath.Join(h.dir, "dotaws")
	os.MkdirAll(outside, 0o755)
	os.WriteFile(filepath.Join(outside, "credentials"), []byte("x"), 0o600)
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "browser", "dir": outside, "adapter": "real"}, nil)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "outside") {
		t.Fatalf("%+v", lc)
	}
	// a symlink inside the area pointing outside is rejected too
	in := h.task(0)
	root := h.s.evidenceRoot(in)
	os.MkdirAll(root, 0o700)
	os.Symlink(outside, filepath.Join(root, "link"))
	h.submit(h.task(0), "coordinator", CmdEvidence, map[string]string{"mode": "browser", "dir": filepath.Join(root, "link"), "adapter": "real"}, nil)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" {
		t.Fatalf("symlinked dir accepted: %+v", lc)
	}
}
