package sup

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"issueflow/internal/config"
	"issueflow/internal/runner"
	"issueflow/internal/state"
)

type cloudStub struct {
	create func(brief string) runner.Result
	send   func(msg string) runner.Result
	sends  []string
	mu     sync.Mutex
	n      int
	briefs []string
}

func (c *cloudStub) sent() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sends...)
}

func (c *cloudStub) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func cloudHarness(t *testing.T, stub *cloudStub, mut func(*config.Config)) *harness {
	h := newHarness(t, func(c *config.Config) {
		c.Cloud.Mode = "assisted"
		c.Coder.Fallback = []string{"claude-cloud"}
		if mut != nil {
			mut(c)
		}
	})
	h.rt.Handle("claude", func(c runner.Call) (runner.Result, error, bool) {
		switch {
		case len(c.Args) > 0 && c.Args[0] == "--version":
			return runner.Result{Stdout: "2.1.296 (Claude Code)"}, nil, true
		case len(c.Args) > 0 && c.Args[0] == "--help":
			return runner.Result{Stdout: "--cloud [x]\n--teleport [s]\n"}, nil, true
		case len(c.Args) > 4 && c.Args[0] == "-p" && c.Args[2] == "--cloud":
			stub.mu.Lock()
			stub.sends = append(stub.sends, c.Args[1])
			stub.mu.Unlock()
			if stub.send != nil {
				return stub.send(c.Args[1]), nil, true
			}
			return runner.Result{Stdout: `{"ok":true,"session_id":"session_abc123XYZ","url":"https://claude.ai/code/session_abc123XYZ"}`}, nil, true
		case len(c.Args) > 1 && c.Args[0] == "--cloud":
			stub.mu.Lock()
			stub.n++
			stub.briefs = append(stub.briefs, c.Args[1])
			stub.mu.Unlock()
			return stub.create(c.Args[1]), nil, true
		}
		return runner.Result{Exit: 2, Stderr: "unexpected claude call"}, nil, true
	})
	h.restart() // re-probe with the new handler
	return h
}

func okCloud(brief string) runner.Result {
	return runner.Result{Stdout: "Session created: session_abc123XYZ\nhttps://claude.ai/code/session_abc123XYZ\n"}
}

// toQuotaWithHandoff takes the task to: local coder out of quota, handoff awaited.
func (h *harness) toQuotaWithHandoff(t *testing.T) *state.Task {
	h.startActive()
	h.startCoder()
	tk := h.task(0)
	appendLog(t, tk.Coder.Session.Log, "Usage limit reached. Your limit resets in 3h 0m.\n")
	h.clk.Advance(30 * time.Second)
	h.until("wait quota with handoff", 10, func() bool {
		x := h.task(0)
		return x.State == state.WaitQuota && x.Handoff != nil && x.Handoff.Status == "await-push"
	})
	if len(h.events("handoff-needed")) != 1 {
		t.Fatal("coordinator not told to prepare the handoff")
	}
	return h.task(0)
}

func (h *harness) pushCheckpoint(t *testing.T, tk *state.Task) string {
	git(t, tk.Worktree, "push", "origin", tk.Branch)
	return git(t, tk.Worktree, "rev-parse", "HEAD")
}

func (h *harness) cloudPush(t *testing.T, tk *state.Task, base, content string) string {
	other := filepath.Join(h.dir, "cloud-clone")
	if _, err := filepathStat(other); err != nil {
		git(t, h.dir, "clone", h.remote, other)
	}
	git(t, other, "fetch", "origin")
	cb := tk.Branch + "-cloud"
	git(t, other, "checkout", "-B", cb, base)
	osWrite(t, filepath.Join(other, "cloud.txt"), content)
	git(t, other, "add", "-A")
	git(t, other, "commit", "-m", "cloud work")
	git(t, other, "push", "-f", "origin", cb)
	return git(t, other, "rev-parse", "HEAD")
}

func TestCloudAssistedHandoffAndAttestedCompletion(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := cloudHarness(t, stub, nil)
	tk := h.toQuotaWithHandoff(t)
	// the coordinator (coordinator not exhausted) is woken to prepare it
	h.until("coordinator woken for handoff", 8, func() bool { return h.task(0).CoordGen >= 1 })
	sha := h.pushCheckpoint(t, tk)
	h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
	h.ticks(2)
	if stub.count() != 1 {
		t.Fatalf("expected exactly one cloud create, got %d", stub.count())
	}
	brief := stub.briefs[0]
	cb := tk.Branch + "-cloud"
	for _, want := range []string{cb, "ONLY to " + cb, "Never push", sha, "implement X"} {
		if !strings.Contains(brief, want) {
			t.Errorf("cloud brief lacks %q:\n%s", want, brief)
		}
	}
	tk = h.task(0)
	if tk.State != state.WaitUser || tk.Question.Kind != "cloud-completion" || tk.Lease.Holder != "cloud" || tk.Coder.CloudID != "session_abc123XYZ" {
		t.Fatalf("state=%s lease=%+v coder=%+v", tk.State, tk.Lease, tk.Coder)
	}
	if tk.Wake != nil {
		t.Fatal("local wake must not outlive the cloud run")
	}
	// local quota resets while the cloud runs: no local writer may start
	h.clk.Advance(6 * time.Hour)
	h.ticks(10)
	if len(h.sessionsLike("-cd")) != 0 || h.task(0).State != state.WaitUser {
		t.Fatal("a local writer started while a cloud writer holds the lease")
	}
	if err := h.s.acquireLease(h.ctx, h.task(0), "local"); err == nil {
		t.Fatal("lease granted over an unconfirmed cloud writer")
	}
	// branch movement is NOT completion
	cloudSHA := h.cloudPush(t, tk, sha, "v1")
	h.clk.Advance(10 * time.Minute)
	h.ticks(5)
	if h.task(0).State != state.WaitUser || len(h.events("cloud-complete")) != 0 {
		t.Fatal("a moved branch was treated as completion")
	}
	// wrong sha re-opens a fresh question
	q1 := h.task(0).Question.ID
	h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": q1, "text": "done " + sha}, nil)
	h.ticks(2)
	if h.task(0).State != state.WaitUser || h.task(0).Question.ID == q1 || h.task(0).Lease.Holder != "cloud" {
		t.Fatalf("mismatching sha released the lease: %+v", h.task(0).Lease)
	}
	h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": h.task(0).Question.ID, "text": "done " + cloudSHA}, nil)
	h.ticks(3)
	tk = h.task(0)
	if tk.State != state.Review || tk.Lease.Holder != "" || !tk.Lease.StopConfirmed || !strings.Contains(tk.Lease.Evidence, "user-attested") {
		t.Fatalf("state=%s lease=%+v", tk.State, tk.Lease)
	}
	if len(h.events("cloud-complete")) != 1 {
		t.Fatal("coordinator not told to fetch and integrate")
	}
	// a late write after the attestation is noticed
	h.cloudPush(t, tk, cloudSHA, "v2 late")
	h.clk.Advance(5 * time.Minute)
	h.ticks(3)
	if h.task(0).State != state.WaitUser || h.task(0).Question.Kind != "cloud-late-write" {
		t.Fatalf("late cloud write not detected: %s", h.task(0).State)
	}
}

func TestCloudCreateQuotaRejectionResumesLocalWithoutSession(t *testing.T) {
	stub := &cloudStub{create: func(string) runner.Result {
		return runner.Result{Exit: 1, Stderr: "Usage limit reached. resets in 1h 0m"}
	}}
	h := cloudHarness(t, stub, nil)
	tk := h.toQuotaWithHandoff(t)
	h.until("coordinator", 8, func() bool { return h.task(0).CoordGen >= 1 })
	sha := h.pushCheckpoint(t, tk)
	h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
	h.ticks(2)
	tk = h.task(0)
	if tk.State != state.WaitQuota || tk.Lease.Holder != "" || tk.Coder.Kind != "local" || tk.Handoff.Status != "failed" {
		t.Fatalf("state=%s lease=%+v coder=%s handoff=%+v", tk.State, tk.Lease, tk.Coder.Kind, tk.Handoff)
	}
	if tk.Wake.Role != "coder" {
		t.Fatalf("wake role %q would park the task forever", tk.Wake.Role)
	}
	// no second attempt, ever, during this wake
	h.restart()
	h.ticks(10)
	if stub.count() != 1 {
		t.Fatalf("cloud create attempted %d times", stub.count())
	}
	// at the wake the LOCAL coder resumes
	h.clk.Advance(4 * time.Hour)
	h.ticks(4)
	tk = h.task(0)
	if tk.State != state.Running || tk.Coder.Kind != "local" || tk.Coder.Gen != 2 || len(h.sessionsLike("-cd")) != 1 {
		t.Fatalf("local coder did not resume: state=%s kind=%s gen=%d", tk.State, tk.Coder.Kind, tk.Coder.Gen)
	}
}

func TestCloudCreateUnknownOutcomeNeverCreatesSecondSession(t *testing.T) {
	for name, res := range map[string]runner.Result{
		"unclassified failure": {Exit: 1, Stderr: "network reset by peer"},
		"success without id":   {Stdout: "ok"},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &cloudStub{create: func(string) runner.Result { return res }}
			h := cloudHarness(t, stub, nil)
			tk := h.toQuotaWithHandoff(t)
			h.until("coordinator", 8, func() bool { return h.task(0).CoordGen >= 1 })
			sha := h.pushCheckpoint(t, tk)
			h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
			h.ticks(2)
			tk = h.task(0)
			if tk.State != state.WaitUser || tk.Question.Kind != "launch" || tk.Lease.Holder != "cloud" || tk.Lease.StopConfirmed {
				t.Fatalf("state=%s q=%+v lease=%+v", tk.State, tk.Question, tk.Lease)
			}
			h.restart()
			h.clk.Advance(8 * time.Hour)
			h.ticks(10)
			if stub.count() != 1 || len(h.sessionsLike("-cd")) != 0 {
				t.Fatalf("creates=%d local sessions=%v", stub.count(), h.sessionsLike("-cd"))
			}
			// the user says there is none: only then may the handoff be retried
			h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": h.task(0).Question.ID, "text": "none"}, nil)
			h.ticks(3)
			if h.task(0).Lease.Holder != "" || h.task(0).Handoff.Status != "await-push" {
				t.Fatalf("lease=%+v handoff=%+v", h.task(0).Lease, h.task(0).Handoff)
			}
		})
	}
}

func TestCloudIsOffByDefaultAndFailsClosed(t *testing.T) {
	t.Run("mode off", func(t *testing.T) {
		h := newHarness(t, nil)
		h.startActive()
		h.startCoder()
		appendLog(t, h.task(0).Coder.Session.Log, "Usage limit reached resets in 3h 0m\n")
		h.clk.Advance(30 * time.Second)
		h.until("quota", 10, func() bool { return h.task(0).State == state.WaitQuota })
		if h.task(0).Handoff != nil || len(h.events("handoff-needed")) != 0 {
			t.Fatal("cloud handoff offered while cloud mode is off")
		}
	})
	t.Run("cli without cloud flags", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := cloudHarness(t, stub, nil)
		h.rt.Handle("claude", func(c runner.Call) (runner.Result, error, bool) {
			return runner.Result{Stdout: "no such flags"}, nil, true
		})
		h.restart()
		h.startActive()
		h.startCoder()
		appendLog(t, h.task(0).Coder.Session.Log, "Usage limit reached resets in 3h 0m\n")
		h.clk.Advance(30 * time.Second)
		h.until("quota", 10, func() bool { return h.task(0).State == state.WaitQuota })
		if h.task(0).Handoff != nil {
			t.Fatal("handoff offered although the CLI has no --cloud/--teleport")
		}
	})
	t.Run("local-only label", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := cloudHarness(t, stub, nil)
		h.issue(1, "owner", "owner", "agent:ready", "agent:local-only")
		h.until("running", 10, func() bool { return h.task(0).State == state.Running })
		h.until("turn", 5, func() bool { return h.task(0).Coordinator.Status == "running" })
		h.startCoder()
		appendLog(t, h.task(0).Coder.Session.Log, "Usage limit reached resets in 3h 0m\n")
		h.clk.Advance(30 * time.Second)
		h.until("quota", 10, func() bool { return h.task(0).State == state.WaitQuota })
		if h.task(0).Handoff != nil {
			t.Fatal("local-only task offered a cloud handoff")
		}
	})
	t.Run("dirty worktree is not handed off", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := cloudHarness(t, stub, nil)
		tk := h.toQuotaWithHandoff(t)
		h.until("coordinator", 8, func() bool { return h.task(0).CoordGen >= 1 })
		sha := h.pushCheckpoint(t, tk)
		h.wtWrite(tk, "unpushed.txt", "local only work")
		h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
		h.ticks(2)
		if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "not clean") || stub.count() != 0 {
			t.Fatalf("%+v creates=%d", lc, stub.count())
		}
	})
	t.Run("remote does not have the sha", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := cloudHarness(t, stub, nil)
		tk := h.toQuotaWithHandoff(t)
		h.until("coordinator", 8, func() bool { return h.task(0).CoordGen >= 1 })
		git(t, tk.Worktree, "commit", "--allow-empty", "-m", "unpushed")
		sha := git(t, tk.Worktree, "rev-parse", "HEAD")
		h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
		h.ticks(2)
		if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "push the checkpoint") || stub.count() != 0 {
			t.Fatalf("%+v creates=%d", lc, stub.count())
		}
	})
}

// A create command that never returns must not freeze the supervisor, and a
// timeout is an UNKNOWN outcome (a session may exist), never a rejection.
func TestCloudCreateTimeoutIsUnknownOutcomeNotRejection(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	stub := &cloudStub{create: func(string) runner.Result { <-release; return runner.Result{} }}
	h := cloudHarness(t, stub, func(c *config.Config) { c.Timeouts.CloudSeconds = 1 })
	tk := h.toQuotaWithHandoff(t)
	h.until("coordinator", 8, func() bool { return h.task(0).CoordGen >= 1 })
	sha := h.pushCheckpoint(t, tk)
	h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
	start := time.Now()
	h.ticks(2)
	if time.Since(start) > 15*time.Second {
		t.Fatalf("supervisor blocked for %v on a hung create", time.Since(start))
	}
	tk = h.task(0)
	if tk.State != state.WaitUser || tk.Question.Kind != "launch" || tk.Lease.Holder != "cloud" || tk.Lease.StopConfirmed {
		t.Fatalf("state=%s q=%+v lease=%+v", tk.State, tk.Question, tk.Lease)
	}
	if tk.Wake != nil && tk.Wake.Computed == "backoff" && tk.State == state.WaitQuota {
		t.Fatal("timeout was treated as a rejection")
	}
	h.restart()
	h.clk.Advance(10 * time.Hour)
	h.ticks(8)
	if stub.count() != 1 {
		t.Fatalf("a second session was attempted after an unknown create: %d", stub.count())
	}
}
