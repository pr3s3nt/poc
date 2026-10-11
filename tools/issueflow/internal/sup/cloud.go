package sup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/quota"
	"issueflow/internal/state"
)

// Cloud fallback is ASSISTED only. The documented CLI can create a cloud
// session and queue a follow-up, but has no verified status, result or stop
// command. So after creating the session the task parks in WAIT_USER and keeps
// the write lease; the user attests completion with a branch SHA. Branch
// movement is never treated as completion, and a cloud writer is never
// considered stopped without that attestation.

func (s *Sup) cloudEligible(t *state.Task) bool {
	if s.D.Cfg.Cloud.Mode == "off" || !s.cloudOK || s.D.Cloud == nil || t.LocalOnly {
		return false
	}
	ok := false
	for _, f := range s.D.Cfg.Coder.Fallback {
		if f == "claude-cloud" {
			ok = true
		}
	}
	return ok && t.Lease.Holder == "" && t.Handoff == nil
}

func (s *Sup) maybeStartHandoff(ctx context.Context, t *state.Task, checkpoint string) {
	if !s.cloudEligible(t) {
		return
	}
	t.Handoff = &state.Handoff{Status: "await-push", Branch: t.Branch, CloudBranch: t.Branch + "-cloud"}
	s.event(t, "handoff-needed", fmt.Sprintf(`The local coder is out of quota and cloud fallback (%s mode) is available.
A cloud session starts from the REMOTE branch, not from local files. To hand off:
1. Review the worktree. Commit the state you accept to branch %s and push that branch (you are the only one who commits/pushes; the worktree must be clean after).
2. Run: %s report handoff --sha <pushed sha>
The cloud agent may commit and push ONLY to %s. You fetch, review and integrate its result yourself.
It has no local kind/Vault/browser environment: required video evidence must be produced locally afterwards.
Checkpoint: %s`, s.D.Cfg.Cloud.Mode, t.Branch, s.cli(t), t.Handoff.CloudBranch, checkpoint))
}

func (s *Sup) cloudBrief(t *state.Task, sha string) string {
	h := t.Handoff
	var b strings.Builder
	fmt.Fprintf(&b, "You are the coding agent for GitHub issue #%d in this repository. You start from branch %s at commit %s.\n\n", t.Issue, h.Branch, sha)
	fmt.Fprintf(&b, "Rules: create branch %s from that commit and commit and push ONLY to %s. Never push %s, %s or any other branch. Do not open or update a pull request. Do not edit documentation (orchestrator_docs, README, AGENTS.md). Do not use or change credentials. You have no local cluster, Vault or browser; do not try to record browser video.\n\n", h.CloudBranch, h.CloudBranch, h.Branch, s.D.Cfg.BaseBranch)
	b.WriteString("Task brief (written by the Codex coordinator):\n")
	if body, err := readFileString(t.Coder.Brief); err == nil {
		b.WriteString(body)
	}
	if s.D.Cfg.Cloud.Mode == "auto" {
		fmt.Fprintf(&b, `

REPORT PROTOCOL (mandatory, read by a program, never executed): when a round is finished, first push your code, then in a FINAL separate commit that touches only .issueflow/ add the file %s with exactly this JSON schema and push it:
{"schema":1,"run_id":%q,"generation":1,"base_sha":%q,"code_sha":"<your last CODE commit, full 40-hex id>","status":"done|blocked|quota","writer_state":"idle","summary":"<short>","checks":[{"command":"<cmd>","exit":0}],"reset_at":"<RFC3339, only for status quota>"}
Use status "blocked" when you cannot continue without a decision and "quota" when you hit a usage limit. By pushing the report you promise to change nothing more until the next message arrives. The next message opens generation 2 with its own report path. Never put the report in a code commit.
`, reportPath(t.RunID, 1), t.RunID, sha)
		return b.String()
	}
	fmt.Fprintf(&b, "\n\nWhen finished, make sure your last commit is pushed to %s and stop.\n", h.CloudBranch)
	return b.String()
}

// createCloud runs after the coordinator pushed and confirmed the checkpoint.
func (s *Sup) createCloud(ctx context.Context, t *state.Task, sha string) error {
	h := t.Handoff
	if h == nil || h.Status != "await-push" {
		return fmt.Errorf("no handoff is waiting for a pushed checkpoint")
	}
	if !s.cloudEligible2(t) {
		return fmt.Errorf("cloud fallback is not available for this task")
	}
	if ok, err := s.D.Git.IsClean(ctx, t.Worktree); err != nil || !ok {
		return fmt.Errorf("worktree is not clean: uncommitted work is not available to a cloud session")
	}
	head, err := s.D.Git.Head(ctx, t.Worktree)
	if err != nil || head != sha {
		return fmt.Errorf("sha %s is not the worktree HEAD %s", sha, head)
	}
	remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, h.Branch)
	if err != nil {
		return err
	}
	if remote != sha {
		return fmt.Errorf("remote branch %s is at %q, not %s: push the checkpoint first", h.Branch, remote, sha)
	}
	if h.Branch == s.D.Cfg.BaseBranch {
		return fmt.Errorf("refusing to hand off from the base branch")
	}
	if !t.Coder.Session.LaunchedAt.IsZero() || t.Coder.Kind == "local" {
		if t.Coder.Session.Status != "stopped" && t.Coder.Session.Status != "" {
			return fmt.Errorf("local coder is not confirmed stopped")
		}
	}
	if err := s.acquireLease(ctx, t, "cloud"); err != nil {
		return err
	}
	h.SHA = sha
	gen := t.Coder.Gen + 1
	if err := s.setIntent(t, "cloud-create", "pending", sha); err != nil {
		return fmt.Errorf("not creating the cloud session, state not durable: %w", err)
	}
	hd, lerr := s.D.Cloud.Launch(ctx, agent.StartSpec{Issue: t.Issue, RunID: t.RunID, Gen: gen, Dir: t.Worktree, Text: s.cloudBrief(t, sha)})
	if lerr == nil {
		old := t.Coder
		t.Coder = state.Coder{Adapter: "claude-cloud", Kind: "cloud", Gen: gen, CloudID: hd.CloudID, CloudURL: hd.URL, Brief: old.Brief}
		t.Lease = state.Lease{Holder: "cloud", Gen: gen, Since: s.now()}
		t.Wake = nil
		h.Status = "created"
		t.Progress.CloudSince = s.now()
		_ = s.setIntent(t, "cloud-create", "done", hd.CloudID)
		if s.D.Cfg.Cloud.Mode == "auto" {
			s.startAutoCloud(t, hd.CloudID, hd.URL, sha)
			s.resumeFromWait(t)
			_, _ = s.post(ctx, t, "cloud-started", fmt.Sprintf("Cloud fallback started (%s). issueflow polls branch `%s` for the agent's idle report; completion is never inferred from branch movement.", orDash(hd.URL), t.Cloud.Branch))
			s.event(t, "cloud-started", fmt.Sprintf("Cloud session started on branch %s (round base %s). The supervisor reads the agent's report at %s without a model and will wake you with a review event. Do not start a local coder meanwhile (one writer).", t.Cloud.Branch, sha, reportPath(t.RunID, 1)))
			return nil
		}
		s.awaitCloud(ctx, t)
		return nil
	}
	// Only an explicit, classified rejection proves that no session exists.
	var le *agent.LimitError
	if errors.As(lerr, &le) {
		res := s.D.Quota.Classify(le.Text, s.now())
		switch res.Class {
		case quota.Quota:
			h.Status = "failed"
			_ = s.setIntent(t, "cloud-create", "done", "rejected (quota): no session")
			w := s.setWake(t, "coder", res) // the local coder resumes at this wake
			s.enterWait(t, state.WaitQuota)
			_, _ = s.post(ctx, t, fmt.Sprintf("wake-cloud-%d", w.Attempt), fmt.Sprintf("Cloud fallback was rejected (usage limit). The local coder resumes automatically at about %s UTC (%s); no cloud session exists.", w.At.Format("2006-01-02 15:04"), w.Computed))
			return nil
		case quota.Auth:
			h.Status = "failed"
			_ = s.setIntent(t, "cloud-create", "done", "rejected (auth): no session")
			t.Attempts["auth"]++
			s.enterWait(t, state.WaitAuth)
			_, _ = s.post(ctx, t, fmt.Sprintf("auth-cloud-%d", t.Attempts["auth"]), "Cloud fallback needs a login fix on the host (`"+res.Match+"`). Fix it, then run `issueflow resume --issue "+fmt.Sprint(t.Issue)+"`.")
			return nil
		}
	}
	// Unknown outcome: a session may exist. The lease is held and never
	// released without the user; no second session is created.
	h.Status = "ambiguous"
	old := t.Coder
	t.Coder = state.Coder{Adapter: "claude-cloud", Kind: "cloud", Gen: gen, Brief: old.Brief}
	t.Lease = state.Lease{Holder: "cloud", Gen: gen, Since: s.now(), Evidence: "launch outcome unknown"}
	s.askUser(ctx, t, "launch", fmt.Sprintf("The cloud session create command had an unknown outcome (%v). A session may exist: check https://claude.ai/code. Answer `none` if there is no session for this task, or `session=<id> [url]` to adopt it. No second session will be created.", lerr))
	return nil
}

func (s *Sup) cloudEligible2(t *state.Task) bool {
	return s.D.Cfg.Cloud.Mode != "off" && s.cloudOK && s.D.Cloud != nil && !t.LocalOnly
}

// awaitCloud parks the task: completion, status and stop are not observable.
func (s *Sup) awaitCloud(ctx context.Context, t *state.Task) {
	h := t.Handoff
	s.askUser(ctx, t, "cloud-completion", fmt.Sprintf(`A cloud coding session was started (%s). Its progress, completion and stop cannot be observed from here, so the whole queue waits and the write lease stays with it.
When the session has finished, reply:
    done <sha>      the last commit it pushed to %s (checked against the remote)
or, if you stopped it yourself:
    abandon         the session is stopped; its work is discarded`, orDash(t.Coder.CloudURL), h.CloudBranch))
}

func orDash(s string) string {
	if s == "" {
		return "no URL reported"
	}
	return s
}

// afterAnswer applies kind-specific meaning of an authorized answer.
func (s *Sup) afterAnswer(ctx context.Context, t *state.Task, q *state.Question) {
	text := strings.TrimSpace(q.Answer)
	low := strings.ToLower(text)
	switch q.Kind {
	case "cloud-send":
		c := t.Cloud
		switch {
		case low == "resend" && c != nil && c.SendMsg != "":
			// One explicit, user-approved resend of the same message and generation.
			c.SendAt = time.Time{}
			if err := s.cloudDeliver(ctx, t); err != nil {
				s.event(t, "cloud-resend-failed", err.Error())
			}
		case low == "abandon":
			t.Lease = state.Lease{Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "user-attested: cloud session stopped by the user", Since: s.now()}
			t.Coder.Done = false
			t.Cloud = nil
			s.event(t, "cloud-abandoned", "The user stopped the cloud session and discards its pending work. Continue locally when quota allows.")
		default:
			s.reopen(ctx, t, q, "Answer `resend` or `abandon`.")
		}
	case "cloud-completion", "cloud-late-write":
		if t.Cloud != nil && t.Cloud.Mode == "auto" && q.Kind == "cloud-late-write" {
			switch {
			case strings.HasPrefix(low, "repin "):
				sha := strings.TrimSpace(text[6:])
				remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, t.Cloud.Branch)
				if err != nil || remote != sha || !shaRe.MatchString(sha) {
					s.reopen(ctx, t, q, fmt.Sprintf("Branch %s is at %q (error %v), which does not match %q. Answer `repin <sha>` with its real head, or `abort`.", t.Cloud.Branch, remote, err, sha))
					return
				}
				// The agent wrote after promising idle: its report no longer counts.
				t.Cloud.Status, t.Cloud.PinnedHead, t.Cloud.NextPoll = "running", "", s.now()
				t.Coder.Done = false
				s.event(t, "cloud-late-write", fmt.Sprintf("The user reviewed branch %s at %s after a write that came after the idle report. The old report is void: wait for a new valid report for generation %d.", t.Cloud.Branch, sha, t.Cloud.Gen))
			case low == "abort":
				s.abortTask(ctx, t, "operator abort after a late cloud write")
			default:
				s.reopen(ctx, t, q, "Answer `repin <sha>` or `abort`.")
			}
			return
		}
		h := t.Handoff
		switch {
		case strings.HasPrefix(low, "done "):
			sha := strings.TrimSpace(text[5:])
			remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, h.CloudBranch)
			if err != nil || remote == "" || remote != sha {
				s.reopen(ctx, t, q, fmt.Sprintf("Remote branch %s is at %q, which does not match %q. Answer `done <sha>` with the real last pushed commit, or `abandon`.", h.CloudBranch, remote, sha))
				return
			}
			h.ConfirmedSHA = sha
			t.Progress.CloudSince = s.now()
			t.Lease = state.Lease{Holder: "", Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "user-attested completion at " + sha, Since: s.now()}
			t.Coder.Done = true
			t.PrevState = state.Review
			s.event(t, "cloud-complete", fmt.Sprintf("The user attests the cloud session finished at %s on branch %s. It may in principle still write there: verify the head again before integrating. As the only committer, run `git fetch origin %s`, review the diff against your checkpoint, integrate what you accept into %s, rerun the checks and any browser scenario LOCALLY, then continue.", sha, h.CloudBranch, h.CloudBranch, t.Branch))
		case low == "abort" && q.Kind == "cloud-late-write":
			s.abortTask(ctx, t, "operator abort after a late cloud write")
		case low == "abandon":
			t.Lease = state.Lease{Holder: "", Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "user-attested: session stopped by the user", Since: s.now()}
			t.Coder.Done = false
			s.event(t, "cloud-abandoned", "The user stopped the cloud session and discards its work. Continue locally when quota allows.")
		default:
			s.reopen(ctx, t, q, "Answer `done <sha>` or `abandon`.")
		}
	case "launch":
		switch {
		case low == "none":
			s.setIntent(t, "cloud-create", "done", "user: no session exists")
			t.Lease = state.Lease{Holder: "", Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "user: no cloud session exists", Since: s.now()}
			if t.Handoff != nil {
				t.Handoff.Status = "await-push"
			}
			s.event(t, "handoff-retry", "The user confirms no cloud session exists. Run `report handoff --sha <sha>` again to retry.")
		case strings.HasPrefix(low, "session="):
			id := strings.Fields(text[len("session="):])
			if len(id) == 0 {
				s.reopen(ctx, t, q, "Give `session=<id> [url]`.")
				return
			}
			t.Coder.Kind, t.Coder.Adapter, t.Coder.CloudID = "cloud", "claude-cloud", id[0]
			if len(id) > 1 {
				t.Coder.CloudURL = id[1]
			}
			t.Lease = state.Lease{Holder: "cloud", Gen: t.Lease.Gen, Since: s.now(), Evidence: "adopted by user"}
			_ = s.setIntent(t, "cloud-create", "done", id[0])
			t.Handoff.Status = "created"
			if s.D.Cfg.Cloud.Mode == "auto" {
				s.startAutoCloud(t, id[0], t.Coder.CloudURL, t.Handoff.SHA)
				s.event(t, "cloud-started", "The user adopted the cloud session; the supervisor now polls the branch for its report.")
				return
			}
			s.awaitCloud(ctx, t)
		default:
			s.reopen(ctx, t, q, "Answer `none` or `session=<id> [url]`.")
		}
	case "stop-failed":
		switch low {
		case "retry":
			ok := true
			if t.Coder.Kind == "local" && t.Coder.Session.Status != "stopped" && t.Coder.Session.Status != "" {
				ok = s.stopCoder(ctx, t, "operator retry")
			}
			if ok && t.Coordinator.Status == "ambiguous" && t.Coordinator.PID != 0 {
				if s.stopTurn(ctx, t, "operator retry") {
					t.Coordinator.Status = "idle"
				} else {
					ok = false
				}
			}
			if !ok {
				return // stop helpers asked a fresh question
			}
		case "abort":
			s.abortTask(ctx, t, "operator abort after stop failure")
		default:
			s.reopen(ctx, t, q, "Answer `retry` or `abort`.")
		}
	case "abort-stop":
		switch low {
		case "stopped", "confirmed":
			// Local writers are re-proven, never taken on trust. Only a cloud
			// session, which cannot be observed, rests on the user's word.
			if t.Coder.Kind == "local" && t.Coder.Session.Status != "" && t.Coder.Session.Status != "stopped" {
				if ok, err := s.D.Coder.Stop(ctx, s.handle(t)); !ok {
					s.reopen(ctx, t, q, fmt.Sprintf("The coder (%s) is still not proven stopped: %v. Stop it, then answer `stopped` again (or `retry`).", t.Coder.Session.Name, err))
					return
				}
				t.Coder.Session.Status = "stopped"
			}
			if t.Coordinator.Session != "" {
				if ok, err := agentStop(ctx, s, t); !ok {
					s.reopen(ctx, t, q, fmt.Sprintf("The coordinator turn (%s) is still not proven stopped: %v. Stop it, then answer `stopped` again (or `retry`).", t.Coordinator.Session, err))
					return
				}
				t.Coordinator.Status = "idle"
			}
			ev := "re-verified: session absent and no surviving process"
			if t.Lease.Holder == "cloud" {
				ev = "user-attested: cloud session stopped by the user"
			}
			t.Lease = state.Lease{Gen: t.Lease.Gen, StopConfirmed: true, Evidence: ev, Since: s.now()}
			s.finalizeAbort(ctx, t)
		case "retry":
			s.abortTask(ctx, t, t.AbortWhy)
		default:
			s.reopen(ctx, t, q, "Answer `stopped` (you stopped it) or `retry`.")
		}
	case "agent-not-ready":
		switch low {
		case "retry":
			s.event(t, "coder-restart", "The coder never became ready and was stopped. Start it again with `agent start` if you still want it.")
		case "abort":
			s.abortTask(ctx, t, "operator abort")
		default:
			s.reopen(ctx, t, q, "Answer `retry` or `abort`.")
		}
	}
}

func (s *Sup) reopen(ctx context.Context, t *state.Task, old *state.Question, msg string) {
	t.Question = nil
	s.askUser(ctx, t, old.Kind, msg)
}

// cloudWatch notices late cloud writes after completion was attested.
func (s *Sup) cloudWatch(ctx context.Context, t *state.Task) {
	h := t.Handoff
	if h == nil || h.ConfirmedSHA == "" || t.Progress.CloudSince.IsZero() || s.now().Before(t.Progress.CloudSince) {
		return
	}
	t.Progress.CloudSince = s.now().Add(time.Duration(s.D.Cfg.PollSeconds) * time.Second)
	remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, h.CloudBranch)
	if err != nil || remote == "" || remote == h.ConfirmedSHA {
		return
	}
	s.askUser(ctx, t, "cloud-late-write", fmt.Sprintf("Branch %s moved from the attested %s to %s after the cloud session was declared finished: a writer may still be running. Check the session, then answer `done <sha>` for the new head, or `abort`.", h.CloudBranch, h.ConfirmedSHA, remote))
}

var readFile = os.ReadFile

func readFileString(p string) (string, error) {
	b, err := readFile(p)
	return string(b), err
}
