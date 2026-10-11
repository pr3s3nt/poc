package sup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"issueflow/internal/agent"
	"issueflow/internal/evidence"
	"issueflow/internal/proc"
	"issueflow/internal/state"
)

// Command kinds submitted by the control CLI.
const (
	CmdReportDone     = "report-done"
	CmdReportBlocked  = "report-blocked"
	CmdReportProgress = "report-progress"
	CmdAgentStart     = "agent-start"
	CmdAgentSend      = "agent-send"
	CmdAgentStop      = "agent-stop"
	CmdAsk            = "ask"
	CmdAccept         = "accept"
	CmdEvidence       = "evidence"
	CmdHandoff        = "handoff"
	CmdPublished      = "published"
	CmdCheckpoint     = "checkpoint"
	CmdAnswer         = "answer"
	CmdResume         = "resume"
	CmdAbort          = "abort"
)

// validate rejects commands from a stale run or generation, so reports from a
// session that has since been replaced can never change the task.
var kindRole = map[string]string{
	CmdReportDone: "coder", CmdReportBlocked: "coder", CmdReportProgress: "coder",
	CmdAgentStart: "coordinator", CmdAgentSend: "coordinator", CmdAgentStop: "coordinator", CmdAsk: "coordinator",
	CmdAccept: "coordinator", CmdEvidence: "coordinator", CmdHandoff: "coordinator", CmdPublished: "coordinator", CmdCheckpoint: "coordinator",
	CmdAnswer: "user", CmdResume: "user", CmdAbort: "user",
}

func (s *Sup) validate(t *state.Task, c *state.Command) error {
	if want, ok := kindRole[c.Kind]; !ok {
		return fmt.Errorf("unknown command %q", c.Kind)
	} else if c.Role != want {
		return fmt.Errorf("command %s may only come from the %s, not %q", c.Kind, want, c.Role)
	}
	switch c.Role {
	case "user":
		return nil
	case "coordinator":
		if c.Run != t.RunID {
			return fmt.Errorf("stale: run %s is not %s", c.Run, t.RunID)
		}
		// Exactly the current turn: commands are applied before a new turn can
		// start, so a turn that already ended can never act after its successor began.
		if c.Gen != t.CoordGen || c.Gen < t.CoordEpoch || c.Gen == 0 {
			return fmt.Errorf("stale: coordinator turn %d is not the current turn %d (valid from %d)", c.Gen, t.CoordGen, t.CoordEpoch)
		}
	case "coder":
		if c.Run != t.RunID {
			return fmt.Errorf("stale: run %s is not %s", c.Run, t.RunID)
		}
		if t.Coder.Kind != "local" || c.Gen != t.Coder.Gen {
			return fmt.Errorf("stale: coder gen %d is not the current %d", c.Gen, t.Coder.Gen)
		}
		if st := t.Coder.Session.Status; st != "running" && st != "launching" {
			return fmt.Errorf("stale: coder session is %q", st)
		}
	default:
		return fmt.Errorf("unknown role %q", c.Role)
	}
	return nil
}

func (s *Sup) applyCommands(ctx context.Context, t *state.Task) {
	for i := range t.Commands {
		c := &t.Commands[i]
		if c.Status != "pending" {
			continue
		}
		var err error
		if err = s.validate(t, c); err == nil {
			err = s.apply(ctx, t, c)
		}
		if err != nil {
			c.Status, c.Result = "rejected", err.Error()
			s.audit(t, "command-rejected", c.Kind+": "+err.Error())
			if c.Role != "user" {
				s.event(t, "command-rejected", fmt.Sprintf("Command %s was rejected: %v", c.Kind, err))
			}
			continue
		}
		c.Status = "applied"
		if c.Role == "coordinator" && c.Gen == t.CoordGen {
			t.Coordinator.Applied++
		}
		s.audit(t, "command", c.Kind)
	}
	if len(t.Commands) > 100 {
		t.Commands = append([]state.Command(nil), t.Commands[len(t.Commands)-100:]...)
	}
}

func (s *Sup) activeState(t *state.Task) bool {
	return t.State == state.Running || t.State == state.Review
}

func (s *Sup) coderWakePending(t *state.Task) bool {
	return t.Wake != nil && (t.Wake.Role == "coder" || t.Wake.Role == "both") && s.now().Before(t.Wake.At)
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Sup) apply(ctx context.Context, t *state.Task, c *state.Command) error {
	a := c.Args
	switch c.Kind {
	case CmdReportDone, CmdReportBlocked, CmdReportProgress:
		kind := strings.TrimPrefix(c.Kind, "report-")
		s.event(t, "coder-"+kind, fmt.Sprintf("Coder gen %d reported %s:\n%s", c.Gen, kind, a["text"]))
		if kind == "done" {
			t.Coder.Done = true
			delete(t.Attempts, "coder")
			delete(t.Attempts, "coder-t")
			if t.State == state.Running {
				return t.SetState(state.Review, s.now())
			}
		}
		return nil
	case CmdAgentStart:
		if !s.activeState(t) && !(t.State == state.WaitQuota && !s.coderWakePending(t)) {
			return fmt.Errorf("cannot start a coder in state %s", t.State)
		}
		if s.coderWakePending(t) {
			return fmt.Errorf("the coder is waiting for a quota reset until %s", t.Wake.At.Format("15:04:05Z"))
		}
		if t.Coder.Kind == "local" && t.Coder.Session.Status != "" && t.Coder.Session.Status != "stopped" {
			return fmt.Errorf("a coder is already active (gen %d); stop it first", t.Coder.Gen)
		}
		if t.Coder.Kind == "cloud" && t.Lease.Holder == "cloud" {
			return fmt.Errorf("a cloud writer holds the lease")
		}
		if _, err := readText(a["file"]); err != nil {
			return fmt.Errorf("brief file: %w", err)
		}
		return s.startLocalCoder(ctx, t, a["file"], "")
	case CmdAgentSend:
		if t.Cloud != nil && t.Cloud.Mode == "auto" && t.Lease.Holder == "cloud" {
			return s.cloudFollowup(ctx, t, a["file"], "feedback from the Codex coordinator")
		}
		if t.Coder.Kind != "local" || t.Coder.Session.Status == "" || t.Coder.Session.Status == "stopped" {
			return fmt.Errorf("no active local coder to send to")
		}
		if _, err := readText(a["file"]); err != nil {
			return fmt.Errorf("feedback file: %w", err)
		}
		keep := s.D.Store.Sub(t.Issue, "feedback", fmt.Sprintf("%s.md", c.ID))
		b, _ := os.ReadFile(a["file"])
		if err := writeFile(keep, string(b)); err != nil {
			return err
		}
		t.Coder.Pending = append(t.Coder.Pending, keep)
		if t.State == state.Review {
			return t.SetState(state.Running, s.now())
		}
		return nil
	case CmdAgentStop:
		if t.Coder.Kind == "cloud" {
			return fmt.Errorf("a cloud session cannot be stopped from here")
		}
		if !s.stopCoder(ctx, t, "coordinator request") {
			return fmt.Errorf("stop could not be confirmed")
		}
		return nil
	case CmdAsk:
		txt, err := readText(a["file"])
		if err != nil {
			return err
		}
		s.askUser(ctx, t, "ask", txt)
		return nil
	case CmdCheckpoint:
		txt, err := readText(a["file"])
		if err != nil {
			return err
		}
		return writeFile(s.D.Store.Sub(t.Issue, "checkpoints", fmt.Sprintf("note-%s.md", c.ID)), txt)
	case CmdEvidence:
		return s.applyEvidence(t, a)
	case CmdAccept:
		return s.applyAccept(ctx, t, c)
	case CmdHandoff:
		return s.createCloud(ctx, t, a["sha"])
	case CmdPublished:
		return s.applyPublished(ctx, t, a)
	case CmdAnswer:
		if t.State != state.WaitUser || t.Question == nil || t.Question.Answered {
			return fmt.Errorf("no open question")
		}
		if a["id"] != t.Question.ID {
			return fmt.Errorf("question id %s does not match the open question %s", a["id"], t.Question.ID)
		}
		s.answer(ctx, t, a["text"], "local-operator")
		return nil
	case CmdResume:
		switch t.State {
		case state.WaitAuth, state.Failed:
			s.resumeFromWait(t)
			if t.State == state.Failed {
				if err := t.SetState(state.Running, s.now()); err != nil {
					return err
				}
			}
			s.event(t, "operator-resume", "The operator resumed this task. Check the state, the worktree and your checkpoint, then continue.")
			return nil
		case state.WaitQuota:
			if t.Question != nil && !t.Question.Answered {
				return fmt.Errorf("a question is open")
			}
			t.Wake = nil
			s.resumeFromWait(t)
			s.event(t, "operator-resume", "The operator ended the quota wait early. Continue.")
			return nil
		case state.WaitUser:
			return fmt.Errorf("WAIT_USER needs an authorized answer to question %s; resume cannot skip it", t.Question.ID)
		}
		return fmt.Errorf("nothing to resume in state %s", t.State)
	case CmdAbort:
		s.abortTask(ctx, t, "operator abort")
		return nil
	}
	return fmt.Errorf("unknown command %q", c.Kind)
}

// abortTask ends a task only after every writer is proven stopped. If a local
// writer cannot be proven stopped, or a cloud writer holds the lease, the task
// stays blocked in WAIT_USER (abort pending): the queue never advances over a
// live writer.
func (s *Sup) abortTask(ctx context.Context, t *state.Task, why string) {
	t.AbortWhy = why
	if t.Lease.Holder == "cloud" && !t.Lease.StopConfirmed {
		s.askUser(ctx, t, "abort-stop", fmt.Sprintf("Abort requested (%s), but a cloud session (%s) holds the write lease and cannot be stopped from here. Stop it yourself at claude.ai/code, then answer `stopped`. The queue will not advance until then.", why, orDash(t.Coder.CloudURL)))
		return
	}
	if t.Coder.Kind == "local" && t.Coder.Session.Status != "" && t.Coder.Session.Status != "stopped" {
		if ok, err := s.D.Coder.Stop(ctx, s.handle(t)); !ok {
			s.audit(t, "abort-stop-unconfirmed", fmt.Sprint(err))
			s.askUser(ctx, t, "abort-stop", fmt.Sprintf("Abort requested (%s), but the coder (%s) could not be proven stopped: %v. Stop it yourself, then answer `stopped` (or `retry`). The queue will not advance until then.", why, t.Coder.Session.Name, err))
			return
		}
		t.Coder.Session.Status = "stopped"
		t.Lease = state.Lease{Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "abort: session absent and no surviving process", Since: s.now()}
	}
	if t.Coordinator.Session != "" {
		if ok, err := agentStop(ctx, s, t); !ok {
			s.askUser(ctx, t, "abort-stop", fmt.Sprintf("Abort requested (%s), but the coordinator turn (%s) could not be proven stopped: %v. Stop it yourself, then answer `stopped` (or `retry`).", why, t.Coordinator.Session, err))
			return
		}
	}
	s.finalizeAbort(ctx, t)
}

func agentStop(ctx context.Context, s *Sup, t *state.Task) (bool, error) {
	c := &t.Coordinator
	return agent.StopSession(ctx, s.D.Tmux, s.D.Procs, c.Session, proc.Owner{PID: c.PID, Start: c.PIDStart})
}

func (s *Sup) finalizeAbort(ctx context.Context, t *state.Task) {
	why := t.AbortWhy
	t.Question = nil
	if err := t.SetState(state.Aborted, s.now()); err != nil {
		t.State = state.Aborted
	}
	_, _ = s.post(ctx, t, "aborted", "Task aborted: "+why+". The queue continues with the next ready Issue.")
	_ = s.D.GH.RemoveLabel(ctx, t.Issue, s.D.Cfg.ActiveLabel)
	s.audit(t, "aborted", why)
}

func (s *Sup) evidenceRoot(t *state.Task) string { return s.D.Store.Sub(t.Issue, "evidence-in") }

func (s *Sup) applyEvidence(t *state.Task, a map[string]string) error {
	if !s.activeState(t) && t.State != state.WaitQuota {
		return fmt.Errorf("cannot declare evidence in state %s", t.State)
	}
	mode := a["mode"]
	if mode == "cli" && t.Evidence.Required == "browser" {
		return fmt.Errorf("this Issue requires a browser scenario; a cli-only report cannot replace it")
	}
	if mode == "cli" && t.Evidence.Mode == "browser" {
		return fmt.Errorf("evidence was declared as browser; it cannot be downgraded to cli")
	}
	if err := os.MkdirAll(s.evidenceRoot(t), 0o700); err != nil {
		return err
	}
	dir := a["dir"]
	if mode == "browser" && dir == "" {
		dir = s.evidenceRoot(t)
	}
	if mode == "cli" && dir == "" {
		dir = ""
	}
	if _, err := evidence.Preflight(s.evidenceRoot(t), dir, mode, a["reason"]); err != nil {
		return err
	}
	if mode == "browser" && a["adapter"] != "real" && a["adapter"] != "fake" {
		return fmt.Errorf("browser evidence must declare adapter real|fake")
	}
	t.Evidence.Mode, t.Evidence.Reason, t.Evidence.Dir, t.Evidence.Adapter = mode, a["reason"], dir, a["adapter"]
	t.Evidence.Published, t.Evidence.Manifest, t.Evidence.ManifestHash, t.Evidence.Links = false, "", "", nil
	return nil
}

func (s *Sup) revisionOK(ctx context.Context, t *state.Task) error {
	br, err := s.D.Git.Branch(ctx, t.Worktree)
	if err != nil {
		return err
	}
	if br != t.Branch {
		return fmt.Errorf("worktree is on branch %q, not the task branch %q", br, t.Branch)
	}
	if t.Branch == s.D.Cfg.BaseBranch {
		return fmt.Errorf("task branch equals the base branch")
	}
	anc, err := s.D.Git.IsAncestor(ctx, t.Worktree, t.BaseSHA, "HEAD")
	if err != nil {
		return err
	}
	if !anc {
		return fmt.Errorf("HEAD no longer descends from the task base %s", t.BaseSHA)
	}
	return nil
}

// applyAccept binds acceptance to the content hash the coordinator saw when it
// SUBMITTED the report. Edits made between review and this tick are rejected,
// never silently accepted.
func (s *Sup) applyAccept(ctx context.Context, t *state.Task, c *state.Command) error {
	if !s.activeState(t) {
		return fmt.Errorf("cannot accept in state %s", t.State)
	}
	submitted := c.Args["hash"]
	if submitted == "" {
		return fmt.Errorf("accept must carry the content hash seen at submission (the CLI adds it; or pass --hash)")
	}
	if t.Evidence.Mode == "" {
		return fmt.Errorf("declare evidence first (report evidence --mode cli|browser)")
	}
	checks := c.Lists["check"]
	if len(checks)+len(s.D.Cfg.Checks) == 0 {
		return fmt.Errorf("no checks defined: pass --check 'CMD' or configure checks")
	}
	if err := s.revisionOK(ctx, t); err != nil {
		return err
	}
	// No writer may be active during validation.
	if cd := &t.Coder; cd.Kind == "local" && cd.Session.Status != "" && cd.Session.Status != "stopped" {
		if !cd.Done || len(cd.Pending) > 0 || cd.Session.Stage != "" {
			return fmt.Errorf("the coder is still working or has queued feedback; wait for its report or stop it with `agent stop`")
		}
		if !s.stopCoder(ctx, t, "acceptance") {
			return fmt.Errorf("the coder could not be proven stopped before validation")
		}
	}
	if err := s.noReportInTree(ctx, t); err != nil {
		return err
	}
	if t.Lease.Holder == "cloud" {
		auto := t.Cloud != nil && t.Cloud.Mode == "auto" && t.Cloud.Status == "reported"
		if !auto {
			return fmt.Errorf("a cloud writer holds the lease and has not reported idle")
		}
	}
	if err := s.cloudGate(ctx, t); err != nil {
		return err // unknown remote state or a late write: never accepted
	}
	h, err := s.D.Git.ContentHash(ctx, t.Worktree)
	if err != nil {
		return err
	}
	if h != submitted {
		return fmt.Errorf("content changed since the acceptance was submitted (submitted %.12s, now %.12s): review again", submitted, h)
	}
	t.Accept = &state.Acceptance{Hash: h, Note: c.Args["note"], Checks: checks, At: s.now(), Gen: c.Gen}
	t.Checks = nil
	t.Publish = state.Publish{}
	return t.SetState(state.Validating, s.now())
}

func (s *Sup) applyPublished(ctx context.Context, t *state.Task, a map[string]string) error {
	if t.State != state.Publishing || !t.Publish.AwaitCommit {
		return fmt.Errorf("not waiting for a commit (state %s)", t.State)
	}
	sha := a["sha"]
	if err := s.revisionOK(ctx, t); err != nil {
		return err
	}
	if err := s.noReportInTree(ctx, t); err != nil {
		return err
	}
	if err := s.cloudGate(ctx, t); err != nil {
		return err
	}
	head, err := s.D.Git.Head(ctx, t.Worktree)
	if err != nil || head != sha {
		return fmt.Errorf("sha %s is not the worktree HEAD %s", sha, head)
	}
	if ok, err := s.D.Git.IsClean(ctx, t.Worktree); err != nil || !ok {
		return fmt.Errorf("worktree has uncommitted or untracked files")
	}
	h, err := s.D.Git.ContentHash(ctx, t.Worktree)
	if err != nil {
		return err
	}
	if h != t.Accept.Hash {
		t.Accept = nil
		_ = t.SetState(state.Review, s.now())
		s.event(t, "stale-acceptance", "The content changed after acceptance; review and accept again.")
		return fmt.Errorf("content changed after acceptance")
	}
	remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, t.Branch)
	if err != nil {
		return err
	}
	if remote != sha {
		return fmt.Errorf("remote branch %s is at %q, not %s: push the task branch first", t.Branch, remote, sha)
	}
	t.Publish.Commit, t.Publish.PR, t.Publish.AwaitCommit = sha, a["pr"], false
	return nil
}

var _ = filepath.Join
