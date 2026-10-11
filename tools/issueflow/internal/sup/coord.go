package sup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/proc"
	"issueflow/internal/quota"
	"issueflow/internal/state"
)

func (s *Sup) coordName(t *state.Task, gen int) string {
	return fmt.Sprintf("%s-%d-%s-co%d", s.D.Cfg.TmuxPrefix, t.Issue, t.RunID[len(t.RunID)-4:], gen)
}

func (s *Sup) pendingEvents(t *state.Task) []state.Event {
	var out []state.Event
	for _, e := range t.Events {
		if e.Seq > t.CoordSeq {
			out = append(out, e)
		}
	}
	return out
}

// coordStep drives the Codex coordinator as a series of deterministic
// `codex exec` turns inside tmux. A turn starts only when the supervisor has
// events to deliver; completion is the exit file, never pane scraping.
func (s *Sup) coordStep(ctx context.Context, t *state.Task) error {
	c := &t.Coordinator
	if c.Status == "launching" || c.Status == "running" || c.Status == "ambiguous" {
		return s.coordWatch(ctx, t)
	}
	ev := s.pendingEvents(t)
	if len(ev) == 0 && t.CoordGen > 0 {
		return nil
	}
	if s.coordinatorParked(t) {
		return nil
	}
	return s.startTurn(ctx, t, ev)
}

// coordWatch only supervises a turn that already exists (finish, reconcile,
// time out); it never starts one. Used where no new turn is wanted (checks).
func (s *Sup) coordWatch(ctx context.Context, t *state.Task) error {
	c := &t.Coordinator
	switch c.Status {
	case "launching", "running":
		return s.watchTurn(ctx, t)
	case "ambiguous":
		if c.PID != 0 {
			// A recorded owner exists: only a proven stop clears it.
			if s.stopTurn(ctx, t, "reconcile") {
				c.Status = "idle"
			}
			return nil
		}
		ok, err := s.D.Tmux.HasSession(ctx, c.Session)
		if err != nil {
			return err
		}
		if ok {
			owner, oerr := agent.RecordOwner(ctx, s.D.Tmux, s.D.Procs, c.Session)
			if oerr != nil {
				return fmt.Errorf("adopting %s: ownership not recorded: %w", c.Session, oerr)
			}
			c.PID, c.PIDStart, c.Status = owner.PID, owner.Start, "running"
			s.audit(t, "reconcile", "adopted existing coordinator session "+c.Session)
			return nil
		}
		// Not running and nothing ran: the turn can be started again.
		c.Status = "idle"
		t.CoordSeq = c.PrevSeq
		s.audit(t, "reconcile", "ambiguous coordinator launch left no session; will retry")
	}
	return nil
}

func (s *Sup) coordinatorParked(t *state.Task) bool {
	return t.Wake != nil && (t.Wake.Role == "coordinator" || t.Wake.Role == "both") && s.now().Before(t.Wake.At)
}

func (s *Sup) startTurn(ctx context.Context, t *state.Task, ev []state.Event) error {
	c := &t.Coordinator
	cfg := s.D.Cfg
	prevTurn, prevSeq, prevGen := *c, t.CoordSeq, t.CoordGen
	t.CoordGen++
	gen := t.CoordGen
	name := s.coordName(t, gen)
	dir := s.D.Store.Sub(t.Issue, "coord")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	prompt := filepath.Join(dir, fmt.Sprintf("prompt-%d.md", gen))
	logp := filepath.Join(dir, fmt.Sprintf("turn-%d.jsonl", gen))
	exit := filepath.Join(dir, fmt.Sprintf("turn-%d.exit", gen))
	last := filepath.Join(dir, fmt.Sprintf("turn-%d.last.md", gen))
	maxSeq := t.CoordSeq
	for _, e := range ev {
		if e.Seq > maxSeq {
			maxSeq = e.Seq
		}
	}
	if err := state.WriteFileAtomic(prompt, []byte(s.coordinatorPrompt(t, ev, gen)), 0o600); err != nil {
		return err
	}
	_ = os.Remove(exit)
	args := append([]string{"exec"}, cfg.Coordinator.Args...)
	var cmd []string
	if c.CodexID != "" {
		cmd = append([]string{shq(cfg.Coordinator.Command), "exec", "resume"}, quoteAll(cfg.Coordinator.Args)...)
		cmd = append(cmd, "-o", shq(last), shq(c.CodexID), "-")
	} else {
		cmd = append([]string{shq(cfg.Coordinator.Command)}, quoteAll(args)...)
		cmd = append(cmd, "-C", shq(t.Worktree), "-o", shq(last), "-")
	}
	shell := fmt.Sprintf("bash -c %s", shq(fmt.Sprintf("cd %s && %s < %s > %s 2>&1; echo $? > %s", shq(t.Worktree), strings.Join(cmd, " "), shq(prompt), shq(logp), shq(exit))))
	// Persist intent and the delivered cursor before the side effect.
	c.Status, c.Session, c.Log, c.ExitFile, c.LastMsg = "launching", name, logp, exit, last
	c.StartedAt, c.Delivered = s.now(), maxSeq
	c.PrevSeq = t.CoordSeq
	t.CoordSeq = maxSeq
	if err := s.setIntent(t, fmt.Sprintf("turn-%d", gen), "pending", name); err != nil {
		// Launch state must be durable before the side effect: do nothing.
		*c, t.CoordSeq, t.CoordGen = prevTurn, prevSeq, prevGen
		return fmt.Errorf("not launching coordinator, state not durable: %w", err)
	}
	env := s.roleEnv(t, "coordinator", gen)
	if err := s.D.Tmux.NewSession(ctx, name, t.Worktree, env, shell); err != nil {
		ok, herr := s.D.Tmux.HasSession(ctx, name)
		if herr != nil || ok {
			c.Status = "ambiguous"
			s.audit(t, "launch-ambiguous", "coordinator: "+err.Error())
			return nil
		}
		c.Status = "idle"
		t.CoordSeq = c.PrevSeq
		return fmt.Errorf("coordinator launch failed: %w", err)
	}
	owner, oerr := agent.RecordOwner(ctx, s.D.Tmux, s.D.Procs, name)
	if oerr != nil {
		// Untracked processes could not be proven stopped later: do not run.
		_ = s.D.Tmux.Kill(ctx, name)
		c.Status = "idle"
		t.CoordSeq = c.PrevSeq
		return fmt.Errorf("coordinator process ownership not recorded: %w", oerr)
	}
	c.PID, c.PIDStart = owner.PID, owner.Start
	c.Status = "running"
	_ = s.setIntent(t, fmt.Sprintf("turn-%d", gen), "done", name)
	s.audit(t, "turn-start", fmt.Sprintf("gen %d session %s events %d", gen, name, len(ev)))
	return nil
}

// stopTurn proves the coordinator's tmux session and its recorded process
// session are gone before any next writer. Failure parks the task.
func (s *Sup) stopTurn(ctx context.Context, t *state.Task, why string) bool {
	c := &t.Coordinator
	ok, err := agent.StopSession(ctx, s.D.Tmux, s.D.Procs, c.Session, proc.Owner{PID: c.PID, Start: c.PIDStart})
	if !ok {
		s.audit(t, "stop-unconfirmed", "coordinator "+why+": "+fmt.Sprint(err))
		if t.State != state.WaitUser {
			s.askUser(ctx, t, "stop-failed", fmt.Sprintf("The coordinator turn (%s) could not be proven stopped (%s): %v. No new writer will start. Fix it manually, then answer `retry` or `abort`.", c.Session, why, err))
		}
		return false
	}
	return true
}

func (s *Sup) roleEnv(t *state.Task, role string, gen int) map[string]string {
	return map[string]string{
		"ISSUEFLOW_CONFIG": s.D.CfgPath,
		"ISSUEFLOW_ISSUE":  fmt.Sprint(t.Issue),
		"ISSUEFLOW_RUN":    t.RunID,
		"ISSUEFLOW_ROLE":   role,
		"ISSUEFLOW_GEN":    fmt.Sprint(gen),
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func quoteAll(a []string) []string {
	out := make([]string, len(a))
	for i, s := range a {
		out[i] = shq(s)
	}
	return out
}

var idRe = regexp.MustCompile(`"(?:thread_id|session_id|conversation_id)"\s*:\s*"([0-9a-fA-F-]{16,})"`)

// watchTurn finishes a turn from its exit file, or handles a dead/overrun one.
func (s *Sup) watchTurn(ctx context.Context, t *state.Task) error {
	c := &t.Coordinator
	if b, err := os.ReadFile(c.ExitFile); err == nil {
		code, perr := strconv.Atoi(strings.TrimSpace(string(b)))
		if perr != nil {
			return fmt.Errorf("bad exit file %s", c.ExitFile)
		}
		return s.finishTurn(ctx, t, code)
	}
	alive, err := s.D.Tmux.HasSession(ctx, c.Session)
	if err != nil {
		return err
	}
	if !alive {
		// Session gone without an exit file: crash or kill.
		s.audit(t, "turn-lost", c.Session)
		return s.finishTurn(ctx, t, -1)
	}
	if limit := time.Duration(s.D.Cfg.Coordinator.TurnTimeoutSeconds) * time.Second; s.now().Sub(c.StartedAt) > limit {
		s.audit(t, "turn-timeout", c.Session)
		return s.finishTurn(ctx, t, -2)
	}
	return nil
}

func (s *Sup) finishTurn(ctx context.Context, t *state.Task, code int) error {
	c := &t.Coordinator
	tail := readTail(c.Log, 16<<10)
	if c.CodexID == "" {
		if m := idRe.FindStringSubmatch(readHead(c.Log, 64<<10)); m != nil {
			c.CodexID = m[1]
		}
	}
	gen := t.CoordGen
	if !s.stopTurn(ctx, t, fmt.Sprintf("turn %d end (exit %d)", gen, code)) {
		c.Status = "ambiguous" // keep parked; WAIT_USER owns the decision
		return nil
	}
	c.Status = "idle"
	if code == 0 {
		if t.Coordinator.Applied == 0 && t.State != state.WaitUser {
			t.Stalls++
		} else {
			t.Stalls = 0
		}
		t.Coordinator.Applied = 0
		s.audit(t, "turn-done", fmt.Sprintf("gen %d", gen))
		if t.Stalls >= 3 {
			t.Stalls = 0
			s.askUser(ctx, t, "stalled", "The coordinator finished three turns without issuing any control command. Tell it how to proceed, or abort.")
		} else if t.Stalls > 0 && len(s.pendingEvents(t)) == 0 && !t.Coder.Done && t.Coder.Session.Status == "" && t.Coder.Kind != "cloud" && t.State == state.Running {
			s.event(t, "stalled", "Your last turn issued no control command and no coder is running. Start the coder, ask the user, or accept.")
		}
		delete(t.Attempts, "coordinator-t")
		return nil
	}
	res := s.D.Quota.Classify(tail, s.now())
	s.audit(t, "turn-failed", fmt.Sprintf("gen %d exit %d class %q", gen, code, res.Class))
	if res.Class == quota.None {
		res.Class = quota.Transient
	}
	s.handleFailure(ctx, t, "coordinator", res, tail)
	return nil
}

func readTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	_, _ = f.ReadAt(buf, off)
	return string(buf)
}

func readHead(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewReaderSize(f, int(n))
	buf := make([]byte, n)
	k, _ := sc.Read(buf)
	return string(buf[:k])
}
