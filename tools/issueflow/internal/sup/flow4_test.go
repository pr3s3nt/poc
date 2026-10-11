package sup

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"issueflow/internal/config"
	"issueflow/internal/proc"
	"issueflow/internal/state"
)

func TestPublishEventStartsCoordinatorTurn(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "a.txt", "a")
	h.endTurn(0, "") // turn 1 ends; a "stalled" nudge starts turn 2
	h.until("turn 2", 6, func() bool { return h.task(0).CoordGen == 2 && h.task(0).Coordinator.Status == "running" })
	h.submit(h.task(0), "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)
	h.accept(h.task(0))
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
	// the turn that issued accept ends; the checks-passed event must start another
	h.endTurn(0, "")
	h.until("a turn for the commit request", 8, func() bool { return h.task(0).CoordGen == 3 && h.task(0).Coordinator.Status == "running" })
	p, _ := os.ReadFile(h.st.Sub(1, "coord", "prompt-3.md"))
	if !strings.Contains(string(p), "checks-passed") || !strings.Contains(string(p), "report published") {
		t.Fatalf("turn prompt does not ask for the commit:\n%s", p)
	}
	tk = h.task(0)
	sha := h.commitPush(tk)
	h.submit(tk, "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.until("done", 12, func() bool { return h.task(1).State == state.Done })
}

func TestCoordWatchNeverStartsATurn(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.endTurn(0, "")
	h.until("idle", 4, func() bool { return h.task(0).Coordinator.Status == "idle" })
	tk = h.task(0)
	h.s.event(tk, "digest", "something happened")
	n := len(h.tm.Created)
	if err := h.s.coordWatch(h.ctx, tk); err != nil {
		t.Fatal(err)
	}
	if len(h.tm.Created) != n {
		t.Fatal("coordWatch started a turn while checks may be running")
	}
}

func TestAbortNeverAdvancesQueueOverLiveWriter(t *testing.T) {
	t.Run("unkillable local coder", func(t *testing.T) {
		h := newHarness(t, nil)
		h.startActive()
		h.startCoder()
		tk := h.task(0)
		h.pr.Spawn(tk.Coder.Session.PID, 9001)
		h.pr.Unkillable = true
		h.issue(2, "owner", "owner", "agent:ready")
		h.submit(tk, "user", CmdAbort, nil, nil)
		h.ticks(3)
		tk = h.task(0)
		if tk.State != state.WaitUser || tk.Question.Kind != "abort-stop" {
			t.Fatalf("state=%s", tk.State)
		}
		h.clk.Advance(2 * time.Hour)
		h.ticks(10)
		if ts, _ := h.st.ListTasks(); len(ts) != 1 {
			t.Fatal("next issue started while the old writer could not be stopped")
		}
		// attesting while the process is still alive does not work: the stop is re-proven
		q1 := h.task(0).Question.ID
		h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": q1, "text": "stopped"}, nil)
		h.ticks(3)
		if h.task(0).State != state.WaitUser || h.task(0).Question.ID == q1 || !strings.Contains(h.task(0).Question.Text, "still not proven") {
			t.Fatalf("attestation overrode a live process: state=%s", h.task(0).State)
		}
		if ts, _ := h.st.ListTasks(); len(ts) != 1 {
			t.Fatal("queue advanced over a live writer")
		}
		// the operator really stops it, then attests
		h.pr.Unkillable = false
		h.pr.Terminate(proc.Owner{PID: tk.Coder.Session.PID}, nil)
		h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": h.task(0).Question.ID, "text": "stopped"}, nil)
		h.until("next issue", 12, func() bool { return h.task(0).Issue == 2 })
		if h.task(1).State != state.Aborted {
			t.Fatalf("first task: %s", h.task(1).State)
		}
	})
	t.Run("cloud lease unconfirmed", func(t *testing.T) {
		h := newHarness(t, nil)
		h.startActive()
		tk := h.task(0)
		tk.Coder = state.Coder{Kind: "cloud", Adapter: "claude-cloud", Gen: 1, CloudID: "session_x", CloudURL: "https://claude.ai/code/session_x"}
		tk.Lease = state.Lease{Holder: "cloud", Gen: 1}
		h.st.SaveTask(tk)
		h.issue(2, "owner", "owner", "agent:ready")
		h.submit(h.task(0), "user", CmdAbort, nil, nil)
		h.ticks(3)
		if h.task(0).State != state.WaitUser || h.task(0).Question.Kind != "abort-stop" {
			t.Fatalf("state=%s", h.task(0).State)
		}
		h.ticks(15)
		if ts, _ := h.st.ListTasks(); len(ts) != 1 {
			t.Fatal("next issue started over a possibly live cloud writer")
		}
		h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": h.task(0).Question.ID, "text": "stopped"}, nil)
		h.until("next", 12, func() bool { return h.task(0).Issue == 2 })
	})
	t.Run("clean abort advances", func(t *testing.T) {
		h := newHarness(t, nil)
		h.startActive()
		h.startCoder()
		h.issue(2, "owner", "owner", "agent:ready")
		h.submit(h.task(0), "user", CmdAbort, nil, nil)
		h.until("next", 12, func() bool { return h.task(0).Issue == 2 })
		if h.task(1).State != state.Aborted {
			t.Fatal("not aborted")
		}
	})
}

func TestAcceptBoundToHashSeenAtSubmission(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.wtWrite(tk, "f.txt", "reviewed")
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)
	h.accept(h.task(0), "true")                   // queued with the reviewed hash...
	h.wtWrite(tk, "f.txt", "EDITED AFTER REVIEW") // ...then the content moves before the tick
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "content changed since") {
		t.Fatalf("edit between report and tick was accepted: %+v", lc)
	}
	if h.task(0).State == state.Validating || h.task(0).Accept != nil {
		t.Fatal("unreviewed content entered validation")
	}
	// a submission without a hash is refused
	h.submit(h.task(0), "coordinator", CmdAccept, map[string]string{}, map[string][]string{"check": {"true"}})
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "hash") {
		t.Fatalf("%+v", lc)
	}
}

func TestAcceptRejectedWhileCoderWritesAndStopsItFirst(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)
	h.accept(h.task(0), "true")
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "still working") {
		t.Fatalf("accept allowed during active coding: %+v", lc)
	}
	h.submit(h.task(0), "coder", CmdReportDone, map[string]string{"text": "done"}, nil)
	h.ticks(2)
	h.accept(h.task(0), "true")
	h.until("validation", 8, func() bool { s := h.task(0).State; return s == state.Publishing || s == state.Validating })
	if st := h.task(0).Coder.Session.Status; st != "stopped" {
		t.Fatalf("coder was not stopped before validation: %s", st)
	}
}

func TestDurableStateBeforeSideEffects(t *testing.T) {
	t.Run("coordinator launch", func(t *testing.T) {
		h := newHarness(t, nil)
		h.issue(1, "owner", "owner", "agent:ready")
		h.until("running", 10, func() bool { return h.task(0).State == state.Running })
		h.st.SaveHook = func(tk *state.Task) error {
			if in := tk.Intents["turn-1"]; in != nil && in.Status == "pending" {
				return errors.New("disk full")
			}
			return nil
		}
		for i := 0; i < 2; i++ {
			if err := h.tickErr(); err == nil {
				t.Fatal("a failing store must surface as a tick error")
			}
		}
		if len(h.sessionsLike("-co")) != 0 {
			t.Fatal("coordinator launched although its launch intent was not durable")
		}
		if tk := h.task(0); tk.CoordGen != 0 || tk.Coordinator.Status != "" {
			t.Fatalf("in-memory launch state leaked: %+v gen=%d", tk.Coordinator, tk.CoordGen)
		}
		h.st.SaveHook = nil
		h.until("launches once storage works", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
	})
	t.Run("coder launch", func(t *testing.T) {
		h := newHarness(t, nil)
		h.startActive()
		h.st.SaveHook = func(tk *state.Task) error {
			if in := tk.Intents["coder-launch-1"]; in != nil && in.Status == "pending" {
				return errors.New("disk full")
			}
			return nil
		}
		h.submit(h.task(0), "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("b.md", "x")}, nil)
		for i := 0; i < 2; i++ {
			_ = h.tickErr()
		}
		if len(h.sessionsLike("-cd")) != 0 {
			t.Fatal("coder launched without a durable intent")
		}
		if h.task(0).Lease.Holder != "" {
			t.Fatal("lease leaked from a failed launch")
		}
	})
	t.Run("question before comment", func(t *testing.T) {
		h := newHarness(t, nil)
		tk := h.startActive()
		h.st.SaveHook = func(x *state.Task) error {
			if x.State == state.WaitUser {
				return errors.New("disk full")
			}
			return nil
		}
		h.submit(tk, "coordinator", CmdAsk, map[string]string{"file": h.writeTemp("q.md", "A or B?")}, nil)
		h.clk.Advance(5 * time.Second)
		_ = h.s.Tick(h.ctx)
		if h.commentsContaining(1, "needs a decision") != 0 {
			t.Fatal("question posted although it was not durable")
		}
	})
	t.Run("comment intent", func(t *testing.T) {
		h := newHarness(t, nil)
		h.st.SaveHook = func(x *state.Task) error {
			if in := x.Intents["post-claim"]; in != nil && in.Status == "pending" {
				return errors.New("disk full")
			}
			return nil
		}
		h.issue(1, "owner", "owner", "agent:ready")
		for i := 0; i < 3; i++ {
			_ = h.tickErr()
		}
		if len(h.gh.Cmts[1]) != 0 {
			t.Fatal("comment written before its intent was durable")
		}
	})
}

func TestResumeStartsCoordinator(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	appendLog(t, tk.Coder.Session.Log, "Please run /login\n")
	h.clk.Advance(30 * time.Second)
	h.until("auth", 8, func() bool { return h.task(0).State == state.WaitAuth })
	h.endTurn(0, "")
	h.until("idle", 4, func() bool { return h.task(0).Coordinator.Status == "idle" })
	gen := h.task(0).CoordGen
	h.submit(h.task(0), "user", CmdResume, nil, nil)
	h.until("new turn after resume", 8, func() bool { return h.task(0).CoordGen > gen })
	if len(h.events("operator-resume")) != 1 {
		t.Fatal("no resume event")
	}
	// FAILED stays failed until resumed, and its notice has a stable key
	tk = h.task(0)
	h.s.failTask(h.ctx, tk, "boom")
	h.s.failTask(h.ctx, tk, "boom")
	if n := h.commentsContaining(1, "issueflow stopped this task"); n != 2 {
		t.Fatalf("two distinct failures should post 2 notices, got %d", n)
	}
	h.s.post(h.ctx, tk, "failed-1", "dup")
	if n := h.commentsContaining(1, "issueflow stopped this task"); n != 2 {
		t.Fatal("same key posted twice")
	}
}

func TestCommandKindsMustComeFromTheirRole(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	// a coder cannot accept, ask, publish or answer
	for _, k := range []string{CmdAccept, CmdAsk, CmdPublished, CmdAnswer, CmdAbort, CmdAgentStart} {
		h.st.Enqueue(1, state.Command{Kind: k, Role: "coder", Run: tk.RunID, Gen: tk.Coder.Gen, Args: map[string]string{"file": "/x", "sha": "x", "hash": "x", "id": "x"}})
	}
	// a coordinator cannot answer, abort or resume
	for _, k := range []string{CmdAnswer, CmdAbort, CmdResume, CmdReportDone} {
		h.st.Enqueue(1, state.Command{Kind: k, Role: "coordinator", Run: tk.RunID, Gen: tk.CoordGen, Args: map[string]string{"id": "x", "text": "x"}})
	}
	before := len(h.commands())
	h.ticks(1)
	for _, c := range h.commands()[before:] {
		if c.Status != "rejected" || !strings.Contains(c.Result, "may only come from") {
			t.Fatalf("%s from %s was not rejected by role: %+v", c.Kind, c.Role, c)
		}
	}
	if h.task(0).State == state.Aborted || h.task(0).State == state.WaitUser {
		t.Fatalf("a wrong-role command changed the task: %s", h.task(0).State)
	}
}

func TestControlCommandsAreShellQuoted(t *testing.T) {
	h := newHarness(t, nil)
	h.s.D.Binary = "/opt/my tools/issueflow"
	h.s.D.CfgPath = "/home/u/my cfg/it's.json"
	h.startActive()
	p, _ := os.ReadFile(h.st.Sub(1, "coord", "prompt-1.md"))
	for _, want := range []string{`'/opt/my tools/issueflow' --config '/home/u/my cfg/it'\''s.json' agent start`} {
		if !strings.Contains(string(p), want) {
			t.Fatalf("control command not quoted:\n%s", p)
		}
	}
	h.submit(h.task(0), "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("dir with space/brief.md", "x")}, nil)
	h.coderUp()
	b, _ := os.ReadFile(h.task(0).Coder.BriefFile)
	if !strings.Contains(string(b), `'/opt/my tools/issueflow' --config`) {
		t.Fatalf("coder brief not quoted:\n%s", b)
	}
	if got := h.tm.Session(h.task(0).Coder.Session.Name).Pastes[0]; !strings.Contains(got, "'") {
		t.Fatalf("brief pointer path not quoted: %s", got)
	}
}

func TestMismatchedOriginStartsNothing(t *testing.T) {
	h := newHarness(t, nil)
	h.identity = "other/repo"
	h.issue(1, "owner", "owner", "agent:ready")
	for i := 0; i < 3; i++ {
		err := h.tickErr()
		if err == nil || !strings.Contains(err.Error(), "refusing to run") {
			t.Fatalf("mismatch not refused: %v", err)
		}
	}
	if ts, _ := h.st.ListTasks(); len(ts) != 0 {
		t.Fatal("a task was claimed")
	}
	if len(h.gh.Writes) != 0 || len(h.tm.Created) != 0 {
		t.Fatalf("side effects before preflight: %v %v", h.gh.Writes, h.tm.Created)
	}
	if _, err := os.Stat(h.cfg.WorktreeRoot); err == nil {
		t.Fatal("worktree area created")
	}
	// case-insensitive match is fine
	h.identity = "O/R"
	h.ticks(3)
	if h.task(0).State == "" {
		t.Fatal("matching identity should claim")
	}
}

func (h *harness) launchingCoder(screen string) *state.Task {
	h.t.Helper()
	h.startActive()
	h.submit(h.task(0), "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("b.md", "x")}, nil)
	h.until("launching", 5, func() bool { return h.task(0).Coder.Session.Status == "launching" })
	tk := h.task(0)
	appendLog(h.t, tk.Coder.Session.Log, screen)
	h.ticks(1)                      // the supervisor sees the new output
	h.clk.Advance(30 * time.Second) // and it stays quiet long enough
	return tk
}

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestTrustDialogNumberedUsesTheNumberNextToYes(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.launchingCoder("Do you trust the files in this folder?\n  1. No, exit\n  2. Yes, I trust this folder\n")
	h.ticks(2)
	s := h.tm.Session(tk.Coder.Session.Name)
	if !containsStr(s.Lines, "2") || containsStr(s.Lines, "1") {
		t.Fatalf("typed %v; the number next to Yes is 2", s.Lines)
	}
	if s.Enters != 0 {
		t.Fatal("a bare Enter was pressed on the trust dialog")
	}
	if h.task(0).Coder.Session.Stage != "" {
		t.Fatal("brief pasted before the agent was ready")
	}
	appendLog(t, tk.Coder.Session.Log, "\n? for shortcuts\n")
	h.ticks(1)
	h.clk.Advance(30 * time.Second)
	h.until("pasted", 5, func() bool { return h.task(0).Coder.Session.Stage == "pasted" })
}

func TestTrustDialogUnnumberedFailsClosedWithoutKeys(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.launchingCoder("Do you trust the files in this folder?\n> No, exit\n  Yes, I trust this folder\n")
	h.ticks(3)
	if s := h.tm.Session(tk.Coder.Session.Name); s != nil {
		t.Fatalf("coder session should have been stopped (lines %v keys %v enters %d)", s.Lines, s.Keys, s.Enters)
	}
	got := h.task(0)
	if got.State != state.WaitUser || got.Question.Kind != "agent-not-ready" || !strings.Contains(got.Question.Text, "trust") {
		t.Fatalf("state=%s q=%+v", got.State, got.Question)
	}
}

func TestTrustDialogUnnumberedUsesConfiguredKeysOnly(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Coder.TrustUnnumberedKeys = []string{"Up", "Enter"} })
	tk := h.launchingCoder("Do you trust the files in this folder?\n> No, exit\n  Yes, I trust this folder\n")
	h.ticks(2)
	s := h.tm.Session(tk.Coder.Session.Name)
	if strings.Join(s.Keys, " ") != "Up Enter" || s.Enters != 0 {
		t.Fatalf("keys %v enters %d", s.Keys, s.Enters)
	}
}

func TestAskingTheUserParksCoderAndCoordinator(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	// work is in flight: queued feedback and a running coordinator turn
	h.submit(h.task(0), "coder", CmdReportDone, map[string]string{"text": "done"}, nil)
	h.ticks(2)
	fb := h.writeTemp("fb.md", "more please")
	h.submit(h.task(0), "coordinator", CmdAgentSend, map[string]string{"file": fb}, nil)
	h.submit(h.task(0), "coordinator", CmdAsk, map[string]string{"file": h.writeTemp("q.md", "Which way?")}, nil)
	h.ticks(2)
	tk = h.task(0)
	if tk.State != state.WaitUser {
		t.Fatalf("state %s", tk.State)
	}
	if h.tm.Session(tk.Coder.Session.Name) != nil || tk.Coder.Session.Status != "stopped" || tk.Lease.Holder != "" {
		t.Fatalf("coder still running while the user decides: %+v lease=%+v", tk.Coder.Session, tk.Lease)
	}
	if len(h.sessionsLike("-co")) != 0 || tk.Coordinator.Status != "idle" {
		t.Fatalf("coordinator turn still running: %v %s", h.sessionsLike("-co"), tk.Coordinator.Status)
	}
	if len(tk.Coder.Pending) != 1 {
		t.Fatal("queued feedback must survive the park")
	}
	if !strings.Contains(h.gh.Cmts[1][len(h.gh.Cmts[1])-1].Body, "was stopped") {
		t.Fatal("the question must tell the user that writers were stopped")
	}
	// events pile up but no turn starts and no session appears while waiting
	h.s.event(h.task(0), "digest", "x")
	_ = h.s.save(h.task(0))
	h.clk.Advance(10 * time.Hour)
	h.ticks(12)
	if len(h.sessionsLike("-co")) != 0 || len(h.sessionsLike("-cd")) != 0 {
		t.Fatalf("a process started while WAIT_USER: %v", h.tm.Created)
	}
	// a command from the stopped turn is void
	h.st.Enqueue(1, state.Command{Kind: CmdCheckpoint, Role: "coordinator", Run: tk.RunID, Gen: tk.CoordGen, Args: map[string]string{"file": "/x"}})
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" {
		t.Fatalf("command from a parked turn accepted: %+v", lc)
	}
	// after the answer the coordinator learns what happened and may continue
	h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": h.task(0).Question.ID, "text": "A"}, nil)
	h.until("turn after answer", 8, func() bool { return h.task(0).Coordinator.Status == "running" })
	if len(h.events("parked")) != 1 {
		t.Fatal("no parked notice")
	}
}

func TestParkFailureIsReportedNotHidden(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	h.pr.Spawn(tk.Coder.Session.PID, 8080)
	h.pr.Unkillable = true
	h.submit(h.task(0), "coordinator", CmdAsk, map[string]string{"file": h.writeTemp("q.md", "Which way?")}, nil)
	h.ticks(2)
	tk = h.task(0)
	if tk.State != state.WaitUser || tk.Lease.Holder != "local" {
		t.Fatalf("state=%s lease=%+v", tk.State, tk.Lease)
	}
	if !strings.Contains(h.gh.Cmts[1][len(h.gh.Cmts[1])-1].Body, "could not be proven stopped") {
		t.Fatal("park failure not shown to the user")
	}
}

func TestExternalVerificationOptInsAreClearedUnlessTheIssueAuthorises(t *testing.T) {
	t.Setenv("ORCH_KIND_VERIFY", "1")
	t.Setenv("ORCHESTRATOR_POSTGRES_TEST_URL", "postgres://placeholder")
	run := func(t *testing.T, labels ...string) string {
		h := newHarness(t, func(c *config.Config) {
			c.Checks = []config.CheckCfg{{Name: "env", Cmd: `echo "kind=[$ORCH_KIND_VERIFY] pg=[$ORCHESTRATOR_POSTGRES_TEST_URL]"`}}
		})
		h.issue(1, "owner", "owner", append([]string{"agent:ready"}, labels...)...)
		h.until("running", 10, func() bool { return h.task(0).State == state.Running })
		h.until("turn", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
		tk := h.task(0)
		h.wtWrite(tk, "a.txt", "a")
		h.submit(tk, "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
		h.ticks(1)
		h.accept(h.task(0))
		h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
		b, err := os.ReadFile(h.task(0).Checks[0].Log)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if out := run(t); !strings.Contains(out, "kind=[] pg=[]") {
		t.Fatalf("external opt-ins leaked into checks by default:\n%s", out)
	}
	if out := run(t, "agent:external-verify"); !strings.Contains(out, "kind=[1] pg=[postgres://placeholder]") {
		t.Fatalf("an Issue that authorises external verification must keep the opt-ins:\n%s", out)
	}
}
