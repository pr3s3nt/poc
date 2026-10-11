package sup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/state"
)

func (h *harness) browserEvidence(tk *state.Task, hash string, mut func(files map[string]any)) {
	h.t.Helper()
	dir := h.s.evidenceRoot(tk)
	os.MkdirAll(dir, 0o700)
	video := []byte("fake video bytes for the fake adapter")
	sum := sha256.Sum256(video)
	files := map[string]any{
		"assertions.json": map[string]any{"run_id": tk.RunID, "content_hash": hash, "gen": 1, "adapter": "fake", "headed": true, "overall": "pass",
			"assertions": []map[string]string{{"name": "console loads", "status": "pass"}}},
		"phases.json": map[string]any{"run_id": tk.RunID, "content_hash": hash, "duration_seconds": 12.0,
			"phases": []map[string]any{{"name": "open", "start": 0.5, "end": 6.0}, {"name": "save", "start": 6.0, "end": 11.5}}},
		"frame-review.json": map[string]any{"reviewer": "codex", "run_id": tk.RunID, "content_hash": hash, "video_sha256": hex.EncodeToString(sum[:]), "privacy_ok": true,
			"frames": []map[string]any{{"t": 2.0, "note": "console list", "ok": true}, {"t": 8.0, "note": "saved", "ok": true}}},
	}
	if mut != nil {
		mut(files)
	}
	for name, v := range files {
		b, _ := json.Marshal(v)
		os.WriteFile(filepath.Join(dir, name), b, 0o600)
	}
	os.WriteFile(filepath.Join(dir, "run.mp4"), video, 0o600)
	os.WriteFile(filepath.Join(dir, "captions.srt"), []byte("1\n00:00:01,000 --> 00:00:05,000\nBước 1: mở Web Console\n"), 0o600)
}

func TestBrowserTaskPublishesValidatedVideoEvidence(t *testing.T) {
	h := newHarness(t, nil)
	h.issue(1, "owner", "owner", "agent:ready", "agent:browser")
	h.until("running", 10, func() bool { return h.task(0).State == state.Running })
	h.until("turn", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
	tk := h.task(0)
	h.wtWrite(tk, "ui.tsx", "export const x = 1\n")
	// declare (structure only), accept, then write evidence bound to the accepted hash
	h.browserEvidence(tk, "0", nil)
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "browser", "adapter": "fake"}, nil)
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "applied" {
		t.Fatalf("declare: %s", lc.Result)
	}
	h.accept(h.task(0))
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
	accepted := h.task(0).Accept.Hash
	h.browserEvidence(h.task(0), accepted, nil)
	sha := h.commitPush(h.task(0))
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha, "pr": "https://example.invalid/pr/9"}, nil)
	h.until("done", 12, func() bool { return h.task(1).State == state.Done })
	final := h.gh.Cmts[1][len(h.gh.Cmts[1])-1].Body
	for _, want := range []string{"Headed browser scenario", "adapter: fake", "a fake adapter does not prove a live deploy", ".mp4", "captions"} {
		if !strings.Contains(final, want) {
			t.Errorf("final comment lacks %q:\n%s", want, final)
		}
	}
}

func TestBrowserEvidenceBoundToWrongHashBlocksPublishWithRetries(t *testing.T) {
	h := newHarness(t, nil)
	h.issue(1, "owner", "owner", "agent:ready", "agent:browser")
	h.until("running", 10, func() bool { return h.task(0).State == state.Running })
	h.until("turn", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
	tk := h.task(0)
	h.wtWrite(tk, "ui.tsx", "x")
	h.browserEvidence(tk, "0", nil)
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "browser", "adapter": "fake"}, nil)
	h.ticks(1)
	h.accept(h.task(0))
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
	h.browserEvidence(h.task(0), strings.Repeat("e", 64), nil) // recorded against other content
	sha := h.commitPush(h.task(0))
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.ticks(3)
	tk = h.task(0)
	if tk.State != state.Publishing || tk.Publish.Attempts == 0 || !strings.Contains(tk.Publish.LastError, "bound to content") {
		t.Fatalf("state=%s attempts=%d err=%q", tk.State, tk.Publish.Attempts, tk.Publish.LastError)
	}
	if len(h.gh.WritesOf("upload")) != 0 || h.commentsContaining(1, "task complete") != 0 {
		t.Fatal("invalid evidence was published")
	}
}

func TestPublishRetryIsIdempotentAndEventuallyFails(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "a.txt", "a")
	h.acceptCLI(tk)
	sha := h.commitPush(h.task(0))
	h.gh.FailOn["UploadAsset"] = 1
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.ticks(3)
	tk = h.task(0)
	if tk.State != state.Publishing || tk.Publish.Attempts != 1 || tk.Publish.NextAttempt.IsZero() {
		t.Fatalf("state=%s %+v", tk.State, tk.Publish)
	}
	h.ticks(2) // inside the backoff window: nothing is retried
	if h.task(0).Publish.Attempts != 1 {
		t.Fatal("retried inside the backoff window")
	}
	h.clk.Advance(2 * time.Minute)
	h.until("done", 8, func() bool { return h.task(1).State == state.Done })
	if h.commentsContaining(1, "task complete") != 1 {
		t.Fatal("final comment not exactly once")
	}
	tag := h.s.D.Pub.Tag(1, h.task(1).RunID)
	seen := map[string]int{}
	for _, a := range h.gh.Assets[tag] {
		seen[a.Name]++
	}
	for n, c := range seen {
		if c != 1 {
			t.Fatalf("%s uploaded %d times", n, c)
		}
	}
}

func TestPublishGivesUpAfterRepeatedFailureAndStopsQueue(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "a.txt", "a")
	h.acceptCLI(tk)
	sha := h.commitPush(h.task(0))
	h.gh.FailOn["EnsureRelease"] = 100
	h.issue(2, "owner", "owner", "agent:ready")
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	for i := 0; i < 60 && h.task(0).State == state.Publishing; i++ {
		h.clk.Advance(11 * time.Minute)
		h.ticks(1)
	}
	if h.task(1).State != state.Failed {
		t.Fatalf("state %s", h.task(1).State)
	}
	h.ticks(5)
	if ts, _ := h.st.ListTasks(); len(ts) != 1 {
		t.Fatal("queue advanced past a failed task")
	}
	q, _ := h.st.LoadQueue()
	if !q.Halted {
		t.Fatalf("queue not halted: %+v", q)
	}
}

func TestCrashAfterFinalCommentDoesNotDuplicate(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "a.txt", "a")
	h.acceptCLI(tk)
	sha := h.commitPush(h.task(0))
	crashed := false
	h.gh.AfterWrite = func(kind string) error {
		if kind == "comment" && !crashed {
			crashed = true
			return errors.New("crash after the comment was written")
		}
		return nil
	}
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	for i := 0; i < 4; i++ {
		_ = h.tickErr()
	}
	h.restart()
	h.until("done", 10, func() bool { return h.task(1).State == state.Done })
	if n := h.commentsContaining(1, "task complete"); n != 1 {
		t.Fatalf("final summary posted %d times", n)
	}
}

func TestAnswerNeverOverwritesTerminalOrReopenedState(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	// a cloud-completion answer with a wrong sha re-opens: no answer event, still waiting
	tk.Coder = state.Coder{Kind: "cloud", Adapter: "claude-cloud", Gen: 1, CloudID: "s"}
	tk.Lease = state.Lease{Holder: "cloud", Gen: 1}
	tk.Handoff = &state.Handoff{Status: "created", Branch: tk.Branch, CloudBranch: tk.Branch + "-cloud"}
	h.st.SaveTask(tk)
	h.s.askUser(h.ctx, h.task(0), "cloud-completion", "q")
	_ = h.s.save(h.task(0))
	cur := h.task(0)
	h.s.answer(h.ctx, cur, "done deadbeef", "owner")
	if cur.State != state.WaitUser || cur.Question == nil || cur.Question.Answered {
		t.Fatalf("a rejected answer must leave a fresh open question: %s %+v", cur.State, cur.Question)
	}
	for _, e := range cur.Events {
		if e.Kind == "answer" {
			t.Fatal("answer event delivered for a re-opened question")
		}
	}
	// abort: terminal state is never overwritten by the answer plumbing
	h.s.abortTask(h.ctx, cur, "test")
	cur2 := h.task(1)
	_ = cur2
	if cur.State != state.WaitUser {
		t.Fatalf("cloud abort must wait for attestation: %s", cur.State)
	}
	h.s.answer(h.ctx, cur, "stopped", "owner")
	if cur.State != state.Aborted {
		t.Fatalf("state %s", cur.State)
	}
	h.s.resumeFromWait(cur)
	if cur.State != state.Aborted {
		t.Fatal("ABORTED was overwritten")
	}
}

func TestBrowserTaskBriefsCarryTheEvidenceContract(t *testing.T) {
	h := newHarness(t, nil)
	h.s.D.Binary, h.s.D.CfgPath = "/opt/my tools/issueflow", "/home/u/my cfg.json"
	h.issue(1, "owner", "owner", "agent:ready", "agent:browser")
	h.until("running", 10, func() bool { return h.task(0).State == state.Running })
	h.until("turn", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
	tk := h.task(0)
	prompt, _ := os.ReadFile(h.st.Sub(1, "coord", "prompt-1.md"))
	h.submit(tk, "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("b.md", "record the scenario")}, nil)
	h.coderUp()
	brief, _ := os.ReadFile(h.task(0).Coder.BriefFile)
	area := h.s.evidenceRoot(tk)
	for who, text := range map[string]string{"coordinator prompt": string(prompt), "coder brief": string(brief)} {
		for _, want := range []string{"Browser evidence contract", area, tk.RunID, "assertions.json", "phases.json", "frame-review.json", "content_hash", "run_id",
			"video_sha256", "overall", "duration_seconds", "headed", "evidence-schema", "evidence-hash", "NO browser scenario and NO producer", "Playwright", "Do not fabricate",
			"'/opt/my tools/issueflow' --config '/home/u/my cfg.json'"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q", who, want)
			}
		}
	}
	if !strings.Contains(string(brief), "Do NOT write frame-review.json") || !strings.Contains(string(prompt), "As the coordinator") {
		t.Fatal("roles are not told which files are theirs")
	}
	// a CLI-only task does not get the full browser contract but is pointed to the schema command
	h2 := newHarness(t, nil)
	h2.startActive()
	p2, _ := os.ReadFile(h2.st.Sub(1, "coord", "prompt-1.md"))
	if strings.Contains(string(p2), "Browser evidence contract") || !strings.Contains(string(p2), "evidence-schema") {
		t.Fatalf("cli-only prompt:\n%s", p2)
	}
}

func TestEvidenceContractMatchesWhatTheValidatorEnforces(t *testing.T) {
	c := EvidenceContract(&state.Task{RunID: "r9"}, "/area", "issueflow")
	b, _ := json.Marshal(c)
	for _, want := range []string{"assertions.json", "phases.json", "frame-review.json", "overall", "duration_seconds", "video_sha256", "privacy_ok", "reviewer", "r9", "/area", "not_provided"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("contract lacks %q", want)
		}
	}
}
