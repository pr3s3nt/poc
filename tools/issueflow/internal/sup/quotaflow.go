package sup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"issueflow/internal/quota"
	"issueflow/internal/state"
)

// handleFailure routes a classified failure. Quota sets a persisted wake;
// context needs a fresh session; auth needs a human; transient errors retry a
// bounded number of times. None of it needs a model to keep time.
func (s *Sup) handleFailure(ctx context.Context, t *state.Task, role string, res quota.Result, tail string) {
	if role == "coordinator" {
		// Events delivered to the failed turn must be seen again.
		t.CoordSeq = t.Coordinator.PrevSeq
		t.CoordEpoch = t.CoordGen + 1
	}
	cp := s.checkpoint(t, role+"-"+string(res.Class))
	switch res.Class {
	case quota.Quota:
		w := s.setWake(t, role, res)
		stopped := true
		if role == "coder" {
			stopped = s.stopCoder(ctx, t, "quota")
		}
		s.audit(t, "quota", fmt.Sprintf("%s wake %s (%s) match %q", role, w.At.Format(time.RFC3339), w.Computed, res.Match))
		if !stopped {
			return
		}
		s.enterWait(t, state.WaitQuota)
		_, _ = s.post(ctx, t, fmt.Sprintf("wake-%s-%d", role, w.Attempt), fmt.Sprintf("Paused: %s hit a usage limit. issueflow will resume it automatically at about %s UTC (%s). No action needed.", role, w.At.Format("2006-01-02 15:04"), w.Computed))
		if role == "coder" {
			s.maybeStartHandoff(ctx, t, cp)
		}
	case quota.Context:
		if role == "coder" {
			if !s.stopCoder(ctx, t, "context full") {
				return
			}
			t.CoderCtxResets++
			if t.CoderCtxResets > 2 {
				s.askUser(ctx, t, "other", "The coder ran out of context three times. Decide how to split or change the task.")
				return
			}
			s.event(t, "coder-context-full", fmt.Sprintf("The coder's context is full. Work stays in the worktree. Checkpoint: %s\nStart a new coder with a compact brief.", cp))
			return
		}
		t.Coordinator.CodexID = ""
		t.CoordCtxResets++
		if t.CoordCtxResets > 2 {
			s.askUser(ctx, t, "other", "The coordinator ran out of context three times. Decide how to split or change the task.")
			return
		}
		s.event(t, "context-reset", "Your previous session ran out of context; this is a fresh session. Checkpoint: "+cp)
	case quota.Auth:
		if role == "coder" && !s.stopCoder(ctx, t, "auth") {
			return
		}
		t.Attempts["auth"]++
		s.enterWait(t, state.WaitAuth)
		s.audit(t, "auth", role+": "+res.Match)
		_, _ = s.post(ctx, t, fmt.Sprintf("auth-%s-%d", role, t.Attempts["auth"]+1), fmt.Sprintf("Paused: %s needs a login/credential fix on the host (`%s`). Fix it, then run `issueflow resume --issue %d`. No timer is set.", role, res.Match, t.Issue))
	default:
		key := role + "-t"
		n := t.Attempts[key] + 1
		t.Attempts[key] = n
		s.audit(t, "transient", fmt.Sprintf("%s attempt %d: %s", role, n, firstLine(tail)))
		if n > s.D.Cfg.Quota.MaxTransientRetries {
			s.failTask(ctx, t, fmt.Sprintf("%s failed %d times without a recognised limit message. Last output:\n%s", role, n, truncate(tail, 1500)))
			return
		}
		if role == "coder" {
			if s.stopCoder(ctx, t, "transient error") {
				s.event(t, "coder-error", "The coder failed (attempt "+fmt.Sprint(n)+"):\n"+truncate(tail, 2000))
			}
			return
		}
		s.event(t, "turn-retry", "Your previous turn failed (attempt "+fmt.Sprint(n)+") with:\n"+truncate(tail, 2000))
	}
}

func (s *Sup) setWake(t *state.Task, role string, res quota.Result) *state.Wake {
	now := s.now()
	attempt := t.Attempts[role]
	t.Attempts[role] = attempt + 1
	at, how := s.D.Quota.WakeAt(res, attempt, now)
	w := &state.Wake{At: at, Role: role, Class: "quota", Attempt: attempt + 1, Computed: how, Reason: res.Match}
	if old := t.Wake; old != nil && old.At.After(now) && old.Role != role && old.Role != "" {
		w.Role = "both"
		if old.At.After(w.At) {
			w.At = old.At
		}
	}
	if old := t.Wake; old != nil && old.At.After(now) && old.Role == role && old.At.After(w.At) {
		w.At = old.At
	}
	t.Wake = w
	return w
}

// enterWait moves to a wait state without overriding a human stop.
func (s *Sup) enterWait(t *state.Task, to state.State) {
	if t.State == state.WaitUser {
		return // WAIT_USER takes precedence; resumeTarget re-applies the wake later
	}
	if !t.State.Waiting() && t.State != state.Failed {
		t.PrevState = t.State
	}
	if err := t.SetState(to, s.now()); err != nil {
		s.audit(t, "state-error", err.Error())
	}
}

func (s *Sup) failTask(ctx context.Context, t *state.Task, why string) {
	t.LastError = why
	if err := t.SetState(state.Failed, s.now()); err != nil {
		t.State = state.Failed
	}
	t.Attempts["failures"]++
	s.audit(t, "failed", firstLine(why))
	_, _ = s.post(ctx, t, fmt.Sprintf("failed-%d", t.Attempts["failures"]), "issueflow stopped this task and the queue:\n\n"+why)
}

func (s *Sup) stepWaitQuota(ctx context.Context, t *state.Task) error {
	s.pollGitHub(ctx, t)
	if t.State != state.WaitQuota {
		return nil
	}
	w := t.Wake
	if w == nil {
		s.resumeFromWait(t)
		return nil
	}
	if w.Role != "cloud" {
		s.cloudPoll(ctx, t)
	}
	if w.Role == "coder" || w.Role == "cloud" {
		// The coordinator still has quota: it may review and prepare a handoff.
		if err := s.coordStep(ctx, t); err != nil {
			return err
		}
	} else if err := s.coordWatch(ctx, t); err != nil {
		return err
	}
	if s.now().Before(w.At) {
		return nil
	}
	return s.resumeAfterQuota(ctx, t)
}

func (s *Sup) resumeAfterQuota(ctx context.Context, t *state.Task) error {
	w := t.Wake
	t.Wake = nil
	to := s.resumeTarget(t)
	if err := t.SetState(to, s.now()); err != nil {
		return err
	}
	s.audit(t, "quota-resume", fmt.Sprintf("%s -> %s", w.Role, to))
	if to == state.WaitUser {
		t.Wake = w
		t.Wake.At = s.now() // already elapsed; re-evaluated after the answer
		return nil
	}
	s.event(t, "quota-resume", fmt.Sprintf("The %s quota wait is over (%s). Continue from the checkpoint and the worktree.", w.Role, w.Computed))
	if w.Role == "cloud" {
		s.resumeCloudAfterQuota(ctx, t)
	}
	if (w.Role == "coder" || w.Role == "both") && t.Coder.Kind == "local" && !t.Coder.Done && t.Coder.Brief != "" && t.Coder.Session.Status == "stopped" && t.Lease.Holder == "" {
		extra := "RESUME: the previous coder run stopped on a usage limit. Partial work is already in the worktree. Inspect `git status` and continue; do not redo finished work."
		if err := s.startLocalCoder(ctx, t, t.Coder.Brief, extra); err != nil {
			s.event(t, "coder-resume-failed", err.Error())
		}
	}
	return nil
}

// checkpoint writes facts the supervisor knows, never model memory.
func (s *Sup) checkpoint(t *state.Task, why string) string {
	dir := s.D.Store.Sub(t.Issue, "checkpoints")
	name := fmt.Sprintf("%s-%s", s.now().Format("20060102T150405"), strings.NewReplacer(" ", "_", "/", "_").Replace(why))
	path := filepath.Join(dir, name+".md")
	var b strings.Builder
	fmt.Fprintf(&b, "# Checkpoint %s\n\nissue #%d run %s state %s prev %s\nbranch %s base %s\nworktree %s\n", why, t.Issue, t.RunID, t.State, t.PrevState, t.Branch, t.BaseSHA, t.Worktree)
	if h, err := s.D.Git.Head(context.Background(), t.Worktree); err == nil {
		fmt.Fprintf(&b, "head %s\n", h)
	}
	if h, err := s.D.Git.ContentHash(context.Background(), t.Worktree); err == nil {
		fmt.Fprintf(&b, "content hash %s\n", h)
	}
	fmt.Fprintf(&b, "\n## Coder\nkind=%s gen=%d status=%s done=%v lease=%s\nbrief=%s\npending feedback=%v\n", t.Coder.Kind, t.Coder.Gen, t.Coder.Session.Status, t.Coder.Done, t.Lease.Holder, t.Coder.Brief, t.Coder.Pending)
	if t.Wake != nil {
		fmt.Fprintf(&b, "\n## Wake\nrole=%s at=%s class=%s attempt=%d\n", t.Wake.Role, t.Wake.At.Format(time.RFC3339), t.Wake.Class, t.Wake.Attempt)
	}
	fmt.Fprintf(&b, "\n## Diff stat vs base\n%s\n", s.D.Git.DiffStat(context.Background(), t.Worktree, t.BaseSHA))
	fmt.Fprintf(&b, "\n## Checks\n")
	for _, c := range t.Checks {
		fmt.Fprintf(&b, "- %s exit=%d hash=%.12s\n", c.Name, c.Exit, c.Hash)
	}
	fmt.Fprintf(&b, "\n## Last events\n")
	from := len(t.Events) - 15
	if from < 0 {
		from = 0
	}
	for _, e := range t.Events[from:] {
		fmt.Fprintf(&b, "- #%d %s: %s\n", e.Seq, e.Kind, firstLine(e.Text))
	}
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = os.WriteFile(path, []byte(b.String()), 0o600)
	}
	return path
}
