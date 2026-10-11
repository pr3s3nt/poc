package sup

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"issueflow/internal/state"
)

var answerRe = regexp.MustCompile(`(?s)^\s*/answer\s+(Q-[0-9a-f]+)\b[ \t]*(.*)$`)

// pollGitHub runs at the configured cadence (default 240s). It reads comments
// and detects Issue edits; it never consults a model.
func (s *Sup) pollGitHub(ctx context.Context, t *state.Task) {
	now := s.now()
	if now.Before(t.Progress.NextPollAt) {
		return
	}
	t.Progress.NextPollAt = now.Add(time.Duration(s.D.Cfg.PollSeconds) * time.Second)
	if is, err := s.D.GH.GetIssue(ctx, t.Issue); err == nil {
		if h := sum(is.Title, is.Body); h != t.Progress.BodyHashSeen {
			t.Progress.BodyHashSeen = h
			s.event(t, "issue-changed", "The Issue text changed after the snapshot. Evaluate it; do not silently change acceptance criteria.\n--- new title ---\n"+is.Title+"\n--- new body ---\n"+is.Body)
		}
	} else {
		s.audit(t, "poll-error", err.Error())
	}
	cs, err := s.D.GH.Comments(ctx, t.Issue, t.Progress.CommentCursor)
	if err != nil {
		s.audit(t, "poll-error", err.Error())
		return
	}
	for _, c := range cs {
		if c.ID > t.Progress.CommentCursor {
			t.Progress.CommentCursor = c.ID
		}
		if strings.Contains(c.Body, "<!-- issueflow:") {
			continue // our own comments
		}
		if !s.D.Cfg.IsOwner(c.Login) {
			s.audit(t, "ignored-comment", fmt.Sprintf("id %d by non-owner %s", c.ID, c.Login))
			continue
		}
		if m := answerRe.FindStringSubmatch(c.Body); m != nil {
			if t.State != state.WaitUser || t.Question == nil || t.Question.Answered {
				s.audit(t, "ignored-answer", fmt.Sprintf("comment %d: no open question", c.ID))
				continue
			}
			if m[1] != t.Question.ID {
				s.audit(t, "ignored-answer", fmt.Sprintf("comment %d: id %s does not match open %s", c.ID, m[1], t.Question.ID))
				continue
			}
			s.answer(ctx, t, strings.TrimSpace(m[2]), c.Login)
			continue
		}
		if t.State == state.Running || t.State == state.Review || t.State == state.WaitQuota {
			s.event(t, "steering", fmt.Sprintf("Owner comment by %s:\n%s", c.Login, c.Body))
		}
	}
}

// askUser parks the task and halts the queue until an authorized answer with
// the question ID arrives. Quota timers cannot clear this state.
func (s *Sup) askUser(ctx context.Context, t *state.Task, kind, text string) {
	if t.State == state.WaitUser && t.Question != nil && !t.Question.Answered {
		if t.Question.Kind == kind {
			s.audit(t, "ask-ignored", "question already open: "+t.Question.ID)
			return
		}
		// A different decision supersedes the open one (e.g. an abort).
		s.audit(t, "ask-superseded", t.Question.ID+" by "+kind)
	}
	q := &state.Question{ID: "Q-" + state.NewID()[:8], Kind: kind, Text: text, AskedAt: s.now()}
	t.Question = q
	if !t.State.Waiting() && t.State != state.Failed {
		t.PrevState = t.State
	}
	if err := t.SetState(state.WaitUser, s.now()); err != nil {
		t.State = state.WaitUser
	}
	t.Progress.NextPollAt = s.now()
	s.audit(t, "ask", q.ID+" "+kind+": "+firstLine(text))
	if err := s.save(t); err != nil {
		s.audit(t, "save-error", "question not durable, nothing posted: "+err.Error())
		return
	}
	if parkKinds[kind] {
		s.parkWriters(ctx, t)
	}
	s.ensureQuestionPosted(ctx, t)
}

// Questions that do not concern a specific stuck writer stop the task for
// real: the coder and the coordinator turn are stopped (proven where possible)
// so nothing keeps writing while the user decides.
var parkKinds = map[string]bool{"ask": true, "stalled": true, "other": true, "agent-not-ready": true}

func (s *Sup) parkWriters(ctx context.Context, t *state.Task) {
	var notes []string
	if c := &t.Coder; c.Kind == "local" && c.Session.Status != "" && c.Session.Status != "stopped" {
		if ok, err := s.D.Coder.Stop(ctx, s.handle(t)); ok {
			c.Session.Status, c.Session.Stage = "stopped", ""
			t.Lease = state.Lease{Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "parked for a user decision: session absent and no surviving process", Since: s.now()}
			t.ParkedWriters = true
			notes = append(notes, "the coder was stopped (partial work stays in the worktree)")
		} else {
			s.audit(t, "park-unconfirmed", fmt.Sprint(err))
			notes = append(notes, fmt.Sprintf("WARNING: the coder could not be proven stopped (%v)", err))
		}
	}
	if c := &t.Coordinator; c.Session != "" && (c.Status == "running" || c.Status == "launching" || c.Status == "ambiguous") {
		if ok, err := agentStop(ctx, s, t); ok {
			c.Status = "idle"
			t.CoordEpoch = t.CoordGen + 1 // commands from the stopped turn are void
			t.ParkedWriters = true
			notes = append(notes, "the coordinator turn was stopped")
		} else {
			c.Status = "ambiguous"
			s.audit(t, "park-unconfirmed", "coordinator: "+fmt.Sprint(err))
			notes = append(notes, fmt.Sprintf("WARNING: the coordinator turn could not be proven stopped (%v)", err))
		}
	}
	if len(notes) > 0 && t.Question != nil {
		t.Question.Text += "\n\n(issueflow: " + strings.Join(notes, "; ") + ".)"
	}
	_ = s.save(t)
}

func (s *Sup) ensureQuestionPosted(ctx context.Context, t *state.Task) {
	q := t.Question
	if q == nil || q.Answered || q.CommentID != 0 {
		return
	}
	body := fmt.Sprintf("**issueflow needs a decision** (queue stopped)\n\n%s\n\nReply on this Issue as an authorized user with:\n\n    /answer %s <your answer>\n\nOnly this question ID and an authorized login resume the task.", q.Text, q.ID)
	id, err := s.post(ctx, t, "question-"+q.ID, body)
	if err != nil {
		s.audit(t, "post-error", err.Error())
		return
	}
	q.CommentID = id
}

// answer records an answer and resumes the same task (same run, branch, worktree).
func (s *Sup) answer(ctx context.Context, t *state.Task, text, by string) {
	q := t.Question
	if q == nil || q.Answered {
		return
	}
	q.Answered, q.Answer, q.By = true, text, by
	s.audit(t, "answer", fmt.Sprintf("%s by %s: %s", q.ID, by, firstLine(text)))
	s.afterAnswer(ctx, t, q)
	// afterAnswer may have ended the task or opened a fresh question: then the
	// answer did not resume anything and must not overwrite that outcome.
	if t.State.Terminal() || (t.Question != nil && t.Question != q && !t.Question.Answered) || t.State == state.WaitUser && t.Question != nil && !t.Question.Answered {
		return
	}
	s.event(t, "answer", fmt.Sprintf("User answer to %s (%s) by %s:\n%s", q.ID, q.Kind, by, text))
	if t.ParkedWriters {
		t.ParkedWriters = false
		s.event(t, "parked", "While the question was open the coder and your turn were stopped. Partial work is in the worktree; restart the coder with `agent start` if it is still needed.")
	}
	s.resumeFromWait(t)
}

// resumeTarget returns the state to resume into. It never skips an open
// question and never ignores a still-running quota timer.
func (s *Sup) resumeTarget(t *state.Task) state.State {
	if t.Question != nil && !t.Question.Answered {
		return state.WaitUser
	}
	if t.Wake != nil && s.now().Before(t.Wake.At) && t.Wake.Role != "" && t.State != state.WaitQuota {
		return state.WaitQuota
	}
	if t.PrevState != "" && !t.PrevState.Waiting() && t.PrevState != state.Failed {
		return t.PrevState
	}
	return state.Running
}

func (s *Sup) resumeFromWait(t *state.Task) {
	if t.State.Terminal() {
		return
	}
	to := s.resumeTarget(t)
	if to == t.State {
		return
	}
	if err := t.SetState(to, s.now()); err != nil {
		s.audit(t, "resume-error", err.Error())
		return
	}
	t.LastError = ""
	s.audit(t, "resume", string(to))
}
