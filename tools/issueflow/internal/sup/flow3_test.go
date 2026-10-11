package sup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/proc"
	"issueflow/internal/state"
)

func (h *harness) sessionsLike(sub string) []string {
	var out []string
	for n := range h.tm.Sessions {
		if strings.Contains(n, sub) {
			out = append(out, n)
		}
	}
	return out
}

func TestCoderQuotaWakeSurvivesRestartAndRelaunches(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	first := tk.Coder.Session.Name
	appendLog(t, tk.Coder.Session.Log, "Claude usage limit reached. Your limit resets in 1h 0m.\n")
	h.clk.Advance(30 * time.Second)
	h.until("wait quota", 10, func() bool { return h.task(0).State == state.WaitQuota })
	tk = h.task(0)
	wantWake := h.clk.Now().Add(time.Hour + 90*time.Second)
	if d := tk.Wake.At.Sub(wantWake); d < -45*time.Second || d > 45*time.Second {
		t.Fatalf("wake %v want about %v", tk.Wake.At, wantWake)
	}
	if h.tm.Session(first) != nil {
		t.Fatal("old coder session still alive during quota wait")
	}
	if tk.Lease.Holder != "" || !tk.Lease.StopConfirmed {
		t.Fatalf("lease not released after confirmed stop: %+v", tk.Lease)
	}
	if h.commentsContaining(1, "Paused: coder hit a usage limit") != 1 {
		t.Fatal("pause notice missing or duplicated")
	}
	h.restart() // supervisor restart: the wake is durable
	h.clk.Advance(20 * time.Minute)
	h.ticks(3)
	if h.task(0).State != state.WaitQuota || len(h.sessionsLike("-cd")) != 0 {
		t.Fatal("resumed before the reset time")
	}
	h.clk.Advance(50 * time.Minute)
	h.ticks(3)
	tk = h.task(0)
	if tk.State != state.Running || tk.Coder.Gen != 2 || tk.Coder.Session.Status != "launching" {
		t.Fatalf("not relaunched: state=%s gen=%d status=%s", tk.State, tk.Coder.Gen, tk.Coder.Session.Status)
	}
	if got := h.sessionsLike("-cd"); len(got) != 1 {
		t.Fatalf("expected exactly one coder session, got %v", got)
	}
	b, _ := os.ReadFile(tk.Coder.BriefFile)
	if !strings.Contains(string(b), "RESUME") || !strings.Contains(string(b), "implement X") {
		t.Fatalf("resume brief wrong:\n%s", b)
	}
	if tk.Wake != nil {
		t.Fatal("wake not cleared")
	}
}

func TestCoordinatorQuotaCheckpointRedeliversEventsAfterRestart(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	before := h.task(0)
	h.endTurn(1, `{"type":"error"}`+"\n"+"You've hit your usage limit. Try again at 2026-10-11T12:00:00Z.\n")
	h.until("wait quota", 6, func() bool { return h.task(0).State == state.WaitQuota })
	tk := h.task(0)
	if tk.Wake == nil || tk.Wake.Role != "coordinator" || tk.Wake.At.Before(time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("wake %+v", tk.Wake)
	}
	if tk.CoordSeq != before.CoordSeq-0 && tk.CoordSeq >= tk.Events[len(tk.Events)-1].Seq {
		t.Fatalf("events were not marked for redelivery: seq=%d", tk.CoordSeq)
	}
	cps, _ := filepath.Glob(h.st.Sub(1, "checkpoints", "*.md"))
	if len(cps) == 0 {
		t.Fatal("no checkpoint written")
	}
	h.restart()
	h.ticks(3)
	if h.task(0).Coordinator.Status == "running" {
		t.Fatal("coordinator restarted during quota wait")
	}
	h.clk.Advance(5 * time.Hour)
	h.until("new turn", 6, func() bool { return h.task(0).Coordinator.Status == "running" })
	tk = h.task(0)
	if tk.State != state.Running || tk.CoordGen != 2 {
		t.Fatalf("state=%s gen=%d", tk.State, tk.CoordGen)
	}
	p, _ := os.ReadFile(h.st.Sub(1, "coord", "prompt-2.md"))
	if !strings.Contains(string(p), "task-start") || !strings.Contains(string(p), "Latest checkpoint") {
		t.Fatalf("resumed turn lacks redelivered events or checkpoint:\n%s", p)
	}
	// a command from the dead turn (gen 1) is stale now
	h.st.Enqueue(1, state.Command{Kind: CmdCheckpoint, Role: "coordinator", Run: tk.RunID, Gen: 1, Args: map[string]string{"file": "/x"}})
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" {
		t.Fatalf("command from pre-quota turn accepted: %+v", lc)
	}
}

func TestUnknownResetUsesConfiguredBackoff(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.endTurn(1, "usage limit reached\n")
	h.until("wait", 6, func() bool { return h.task(0).State == state.WaitQuota })
	w1 := h.task(0).Wake
	if w1.Computed != "backoff" || w1.At.Sub(h.clk.Now()) > 901*time.Second || w1.At.Sub(h.clk.Now()) < 800*time.Second {
		t.Fatalf("first backoff %+v now %v", w1, h.clk.Now())
	}
	h.clk.Advance(20 * time.Minute)
	h.until("turn", 6, func() bool { return h.task(0).Coordinator.Status == "running" })
	h.endTurn(1, "usage limit reached\n")
	h.until("wait2", 6, func() bool { return h.task(0).State == state.WaitQuota })
	w2 := h.task(0).Wake
	if d := w2.At.Sub(h.clk.Now()); d < 1700*time.Second || d > 1801*time.Second {
		t.Fatalf("second backoff should be ~1800s, got %v", d)
	}
}

func TestContextFullAndAuthAreNotQuotaTimers(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	appendLog(t, tk.Coder.Session.Log, "Error: prompt is too long\n")
	h.clk.Advance(30 * time.Second)
	h.until("ctx event", 8, func() bool { return len(h.events("coder-context-full")) == 1 })
	if h.task(0).Wake != nil || h.task(0).State == state.WaitQuota {
		t.Fatal("context-full must not set a quota timer")
	}
	if h.task(0).Coder.Session.Status != "stopped" {
		t.Fatal("context-full coder must be stopped before a fresh one starts")
	}
	h.submit(h.task(0), "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("b3.md", "compact")}, nil)
	h.coderUp()
	tk = h.task(0)
	appendLog(t, tk.Coder.Session.Log, "Invalid API key · Please run /login\n")
	h.clk.Advance(30 * time.Second)
	h.until("wait auth", 8, func() bool { return h.task(0).State == state.WaitAuth })
	if h.task(0).Wake != nil {
		t.Fatal("auth must not set a timer")
	}
	h.clk.Advance(48 * time.Hour)
	h.ticks(5)
	if h.task(0).State != state.WaitAuth {
		t.Fatal("auth wait expired by itself")
	}
	h.submit(h.task(0), "user", CmdResume, nil, nil)
	h.ticks(2)
	if h.task(0).State != state.Running {
		t.Fatalf("resume after login failed: %s", h.task(0).State)
	}
}

func TestCrashAfterCommentBeforeSaveDoesNotDuplicate(t *testing.T) {
	h := newHarness(t, nil)
	crashed := false
	h.gh.AfterWrite = func(kind string) error {
		if kind == "comment" && !crashed {
			crashed = true
			return errors.New("simulated crash after the comment was written")
		}
		return nil
	}
	h.issue(1, "owner", "owner", "agent:ready")
	h.ticks(2) // claim -> comment written, then the "crash"
	h.restart()
	h.until("running", 10, func() bool { return h.task(0).State == state.Running })
	if n := h.commentsContaining(1, "Claimed by issueflow"); n != 1 {
		t.Fatalf("claim comment written %d times", n)
	}
}

func TestRestartAdoptsRunningCoordinatorAndAmbiguousLaunch(t *testing.T) {
	h := newHarness(t, nil)
	h.startActive()
	n := len(h.tm.Created)
	h.restart()
	h.ticks(4)
	if len(h.tm.Created) != n {
		t.Fatalf("restart created another session: %v", h.tm.Created)
	}
	// ambiguous launch: tmux created the session but reported failure
	h2 := newHarness(t, nil)
	h2.issue(1, "owner", "owner", "agent:ready")
	h2.until("claimed", 10, func() bool { return h2.task(0).State == state.Running })
	// the next created coordinator session is made to look ambiguous
	h2.tm.FailNew[h2.s.coordName(h2.task(0), 1)] = true
	h2.ticks(1)
	if h2.task(0).Coordinator.Status != "ambiguous" {
		t.Fatalf("status %s", h2.task(0).Coordinator.Status)
	}
	h2.restart()
	h2.ticks(2)
	tk := h2.task(0)
	if tk.Coordinator.Status != "running" || tk.Coordinator.PID == 0 {
		t.Fatalf("ambiguous launch not reconciled with ownership: %+v", tk.Coordinator)
	}
	if got := h2.sessionsLike("-co"); len(got) != 1 {
		t.Fatalf("duplicate coordinator sessions: %v", got)
	}
}

func TestStopMustBeProvenBeforeAnyWriter(t *testing.T) {
	ctx := (&harness{}).ctx
	_ = ctx
	mk := func(t *testing.T) (*harness, *state.Task) {
		h := newHarness(t, nil)
		h.startActive()
		h.startCoder()
		return h, h.task(0)
	}
	t.Run("kill fails", func(t *testing.T) {
		h, tk := mk(t)
		h.tm.KillFails[tk.Coder.Session.Name] = true
		h.submit(tk, "coordinator", CmdAgentStop, nil, nil)
		h.ticks(2)
		if h.task(0).State != state.WaitUser || h.task(0).Question.Kind != "stop-failed" {
			t.Fatalf("state=%s", h.task(0).State)
		}
		if h.task(0).Lease.Holder != "local" {
			t.Fatal("lease released without a proven stop")
		}
	})
	t.Run("detached child survives", func(t *testing.T) {
		h, tk := mk(t)
		h.pr.Spawn(tk.Coder.Session.PID, 777)
		h.pr.Unkillable = true
		h.submit(tk, "coordinator", CmdAgentStop, nil, nil)
		h.ticks(2)
		if h.task(0).State != state.WaitUser || !strings.Contains(h.task(0).Question.Text, "survive") {
			t.Fatalf("state=%s q=%+v", h.task(0).State, h.task(0).Question)
		}
		// a second writer must be refused while the first is not proven stopped
		if err := h.s.acquireLease(h.ctx, h.task(0), "local"); err == nil {
			t.Fatal("second writer was allowed")
		}
	})
	t.Run("scan unknown is not stopped", func(t *testing.T) {
		h, tk := mk(t)
		h.pr.Err = errors.New("permission denied reading /proc")
		h.submit(tk, "coordinator", CmdAgentStop, nil, nil)
		h.ticks(2)
		if h.task(0).State != state.WaitUser || h.task(0).Lease.Holder != "local" {
			t.Fatalf("unknown scan treated as stopped: %s", h.task(0).State)
		}
	})
	t.Run("has-session error after kill is not stopped", func(t *testing.T) {
		h, tk := mk(t)
		h.tm.SetHasErr(tk.Coder.Session.Name, true)
		h.submit(tk, "coordinator", CmdAgentStop, nil, nil)
		h.ticks(2)
		if h.task(0).State != state.WaitUser {
			t.Fatalf("has-session error treated as stopped: %s", h.task(0).State)
		}
	})
	t.Run("missing session but survivors remain", func(t *testing.T) {
		h, tk := mk(t)
		h.tm.End(tk.Coder.Session.Name) // tmux session vanished
		h.pr.Spawn(tk.Coder.Session.PID, 4242)
		h.pr.Unkillable = true
		h.ticks(3)
		if h.task(0).State != state.WaitUser || h.task(0).Lease.Holder != "local" {
			t.Fatalf("vanished session treated as death: %s lease=%q", h.task(0).State, h.task(0).Lease.Holder)
		}
	})
	t.Run("survivors terminated then confirmed", func(t *testing.T) {
		h, tk := mk(t)
		h.pr.Spawn(tk.Coder.Session.PID, 31337)
		h.submit(tk, "coordinator", CmdAgentStop, nil, nil)
		h.ticks(2)
		if h.task(0).Coder.Session.Status != "stopped" || h.task(0).Lease.Holder != "" {
			t.Fatalf("status=%s", h.task(0).Coder.Session.Status)
		}
	})
	t.Run("coordinator turn cannot be proven stopped", func(t *testing.T) {
		h := newHarness(t, nil)
		tk := h.startActive()
		h.pr.Spawn(tk.Coordinator.PID, 55)
		h.pr.Unkillable = true
		h.endTurn(0, "")
		h.ticks(3)
		if h.task(0).State != state.WaitUser || h.task(0).Question.Kind != "stop-failed" {
			t.Fatalf("state=%s", h.task(0).State)
		}
		if len(h.sessionsLike("-co")) != 0 && h.task(0).CoordGen != 1 {
			t.Fatal("a new coordinator started")
		}
	})
}

func TestStopSessionDirect(t *testing.T) {
	h := newHarness(t, nil)
	h.tm.NewSession(h.ctx, "s1", "/", nil, "x")
	owner := proc.Owner{PID: 1007, Start: 70071}
	ok, err := agent.StopSession(h.ctx, h.tm, h.pr, "s1", owner)
	if !ok || err != nil {
		t.Fatalf("clean stop: %v %v", ok, err)
	}
	ok, _ = agent.StopSession(h.ctx, h.tm, h.pr, "s1", proc.Owner{})
	if ok {
		t.Fatal("no ownership record must not confirm")
	}
}
