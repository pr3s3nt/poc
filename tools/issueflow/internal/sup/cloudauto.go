package sup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/quota"
	"issueflow/internal/state"
)

// Automatic cloud protocol. After each round the cloud agent pushes, to its own
// branch, a report at .issueflow/result-<run>-<generation>.json AFTER its code
// and promises to stay idle until the next message. The supervisor polls the
// branch without a model, pins the head, reads the report blob at that fixed
// sha (bounded, parsed as data, nothing executed) and only then tells Codex.
// A report is the trusted agent's idle acknowledgment. It is NOT an observed
// termination of the cloud process; that is why every gate checks the branch
// head again and why the cloud lease is never released on its strength.

const reportDir = ".issueflow"

func reportPath(run string, gen int) string {
	return fmt.Sprintf("%s/result-%s-%d.json", reportDir, run, gen)
}

// cloudReport uses pointers so that a MISSING field is told apart from a zero
// value: a missing exit code must never read as a pass.
type cloudReport struct {
	Schema      *int    `json:"schema"`
	RunID       *string `json:"run_id"`
	Generation  *int    `json:"generation"`
	BaseSHA     *string `json:"base_sha"`
	CodeSHA     *string `json:"code_sha"`
	Status      *string `json:"status"`
	WriterState *string `json:"writer_state"`
	Summary     *string `json:"summary"`
	ResetAt     *string `json:"reset_at"`
	Checks      []struct {
		Command *string `json:"command"`
		Exit    *int    `json:"exit"`
	} `json:"checks"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (s *Sup) cloudAuto(t *state.Task) bool {
	return t.Cloud != nil && t.Cloud.Mode == "auto"
}

func (s *Sup) pinRef(t *state.Task) string {
	return fmt.Sprintf("refs/issueflow/cloud/%d-%s", t.Issue, t.RunID)
}

// validateReport fetches the branch once and judges the report for the
// CURRENT generation. unknown=true means the remote could not be read: that is
// never "unchanged" and never "complete".
func (s *Sup) validateReport(ctx context.Context, t *state.Task) (rep *parsedReport, head string, reject string, unknown error) {
	c := t.Cloud
	head, err := s.D.Git.FetchPinned(ctx, t.Worktree, c.Branch, s.pinRef(t))
	if err != nil {
		return nil, "", "", fmt.Errorf("cannot fetch %s: %w", c.Branch, err)
	}
	blob, exists, err := s.D.Git.ShowBlob(ctx, t.Worktree, head, reportPath(t.RunID, c.Gen), s.D.Cfg.Cloud.ReportMaxBytes)
	if err != nil {
		if strings.Contains(err.Error(), "over the") {
			return nil, head, err.Error(), nil
		}
		return nil, head, "", err
	}
	if !exists {
		return nil, head, "", nil // not reported yet
	}
	dec := json.NewDecoder(bytes.NewReader(blob))
	dec.DisallowUnknownFields()
	var raw cloudReport
	if err := dec.Decode(&raw); err != nil {
		return nil, head, "report is not valid JSON of the expected schema: " + err.Error(), nil
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, head, "trailing data after the report", nil
	}
	for name, p := range map[string]bool{"schema": raw.Schema != nil, "run_id": raw.RunID != nil, "generation": raw.Generation != nil,
		"base_sha": raw.BaseSHA != nil, "code_sha": raw.CodeSHA != nil, "status": raw.Status != nil, "writer_state": raw.WriterState != nil, "summary": raw.Summary != nil} {
		if !p {
			return nil, head, "required field missing: " + name, nil
		}
	}
	for i, ck := range raw.Checks {
		if ck.Command == nil || ck.Exit == nil {
			return nil, head, fmt.Sprintf("checks[%d] needs both command and exit", i), nil
		}
	}
	r := parsedReport{Schema: *raw.Schema, RunID: *raw.RunID, Generation: *raw.Generation, BaseSHA: *raw.BaseSHA, CodeSHA: *raw.CodeSHA,
		Status: *raw.Status, WriterState: *raw.WriterState, Summary: *raw.Summary, ResetAt: str(raw.ResetAt)}
	for _, ck := range raw.Checks {
		r.Checks = append(r.Checks, reportCheck{*ck.Command, *ck.Exit})
	}
	switch {
	case r.Schema != 1:
		return nil, head, fmt.Sprintf("unsupported schema %d", r.Schema), nil
	case r.RunID != t.RunID:
		return nil, head, fmt.Sprintf("report is for run %q, not %q", r.RunID, t.RunID), nil
	case r.Generation != c.Gen:
		return nil, head, fmt.Sprintf("report is for generation %d, awaiting %d", r.Generation, c.Gen), nil
	case r.BaseSHA != c.RoundBase:
		return nil, head, fmt.Sprintf("report base %q is not this round's base %q", r.BaseSHA, c.RoundBase), nil
	case !shaRe.MatchString(r.CodeSHA):
		return nil, head, "code_sha is not a full commit id", nil
	case r.WriterState != "idle":
		return nil, head, fmt.Sprintf("writer_state is %q, not idle", r.WriterState), nil
	case r.Status != "done" && r.Status != "blocked" && r.Status != "quota":
		return nil, head, fmt.Sprintf("unknown status %q", r.Status), nil
	}
	if ok, err := s.D.Git.IsAncestor(ctx, t.Worktree, r.BaseSHA, r.CodeSHA); err != nil || !ok {
		return nil, head, "code_sha does not descend from the round base", nil
	}
	if ok, err := s.D.Git.IsAncestor(ctx, t.Worktree, r.CodeSHA, head); err != nil || !ok {
		return nil, head, "code_sha is not on the reported branch head", nil
	}
	// done with code == base is allowed ("already satisfied"): Codex's independent
	// review and checks decide, not the report.
	names, err := s.D.Git.ChangedPaths(ctx, t.Worktree, r.CodeSHA, head)
	if err != nil {
		return nil, head, "", err
	}
	for _, p := range names {
		if !strings.HasPrefix(p, reportDir+"/") {
			return nil, head, fmt.Sprintf("code changed after the report's code commit (%s): not idle", p), nil
		}
	}
	return &r, head, "", nil
}

type reportCheck struct {
	Command string
	Exit    int
}

type parsedReport struct {
	Schema      int
	RunID       string
	Generation  int
	BaseSHA     string
	CodeSHA     string
	Status      string
	WriterState string
	Summary     string
	ResetAt     string
	Checks      []reportCheck
}

// cloudPoll runs without a model at the poll cadence.
func (s *Sup) cloudPoll(ctx context.Context, t *state.Task) {
	c := t.Cloud
	if c == nil || c.Mode != "auto" {
		return
	}
	now := s.now()
	if now.Before(c.NextPoll) {
		return
	}
	c.NextPoll = now.Add(time.Duration(s.D.Cfg.PollSeconds) * time.Second)
	// A send whose outcome was lost in a crash is ambiguous: the next round's
	// identity was persisted before the send, so its report is still recognised,
	// and the message is never re-sent blindly.
	if in := t.Intents[fmt.Sprintf("cloud-send-%d", c.SendGen)]; c.SendGen > 0 && in != nil && in.Status == "pending" && c.Status != "send-ambiguous" {
		c.Gen, c.RoundBase, c.Status, c.PinnedHead = c.SendGen, c.SendBase, "send-ambiguous", ""
		s.audit(t, "cloud-send-ambiguous", "a follow-up was started but its outcome was not recorded")
	}
	switch c.Status {
	case "reported", "quota-wait":
		if err := s.cloudGate(ctx, t); err != nil {
			s.audit(t, "cloud-gate", err.Error())
		}
		return
	case "running", "send-ambiguous":
	default:
		return
	}
	rep, head, reject, unknown := s.validateReport(ctx, t)
	if unknown != nil {
		s.audit(t, "cloud-poll-unknown", unknown.Error())
		return
	}
	if rep == nil {
		if reject != "" && reject != c.LastReject {
			c.LastReject = reject
			s.audit(t, "cloud-report-rejected", reject) // audit only: never completion, no model turn
		}
		s.escalateAmbiguousSend(ctx, t)
		return
	}
	c.LastReject = ""
	c.Reports++
	c.Summary = firstLine(rep.Summary)
	switch rep.Status {
	case "quota":
		res := quota.Result{Class: quota.Quota, Match: "cloud report: quota"}
		if rep.ResetAt != "" {
			if at, err := time.Parse(time.RFC3339, rep.ResetAt); err == nil {
				res.ResetAt = at
			}
		}
		c.Status, c.PinnedHead, c.PinnedCode = "quota-wait", head, rep.CodeSHA
		w := s.setWake(t, "cloud", res)
		s.enterWait(t, state.WaitQuota)
		_, _ = s.post(ctx, t, fmt.Sprintf("wake-cloud-report-%d", c.Gen), fmt.Sprintf("Paused: the cloud agent reported a usage limit (generation %d). issueflow will continue the same session automatically at about %s UTC (%s).", c.Gen, w.At.Format("2006-01-02 15:04"), w.Computed))
	case "blocked":
		c.Status, c.Outcome, c.PinnedHead, c.PinnedCode = "reported", "blocked", head, rep.CodeSHA
		s.event(t, "cloud-blocked", fmt.Sprintf("The cloud agent reports it is BLOCKED (generation %d, idle acknowledgment, not an observed stop).\nSummary: %s\nPinned head %s, code %s on %s.\nDecide: send guidance with `agent send --file F` (opens generation %d on the same session), ask the user only if this needs a decision, or take over locally.", c.Gen, rep.Summary, head, rep.CodeSHA, c.Branch, c.Gen+1))
	default:
		c.Status, c.Outcome, c.PinnedHead, c.PinnedCode = "reported", "done", head, rep.CodeSHA
		t.Coder.Done = true
		var cs []string
		for _, ck := range rep.Checks {
			cs = append(cs, fmt.Sprintf("`%s` exit %d", ck.Command, ck.Exit))
		}
		s.event(t, "cloud-review", fmt.Sprintf(`The cloud agent reports DONE (generation %d). This is the agent's idle acknowledgment, NOT an observed process stop.
Branch %s pinned at head %s; final code commit %s (round base %s).
Summary (untrusted): %s
Checks it claims (untrusted, rerun them yourself): %s

Review and import code at the pinned commit ONLY, without the report:
    git fetch origin refs/heads/%s
    git diff %s %s -- . ':(exclude).issueflow' | git apply   # code only; every .issueflow/ report is excluded explicitly
Never bring any .issueflow/ path into the product: accept/published reject it. Rerun the checks independently on your tree, and any browser scenario locally.
If more work is needed use "agent send --file F": that opens generation %d on the same session. Before accepting or publishing the supervisor re-checks that the branch is still at %s; a late write stops the task and the queue.`,
			c.Gen, c.Branch, head, rep.CodeSHA, c.RoundBase, rep.Summary, strings.Join(cs, "; "), c.Branch, c.RoundBase, rep.CodeSHA, c.Gen+1, head))
		if t.State == state.Running {
			_ = t.SetState(state.Review, s.now())
		}
	}
	s.audit(t, "cloud-report", fmt.Sprintf("gen %d status %s head %.12s code %.12s", c.Gen, rep.Status, head, rep.CodeSHA))
}

// cloudGate re-reads the branch head before a gate (feedback, integration,
// accept, publish). Unknown remote state blocks; movement halts the queue.
func (s *Sup) cloudGate(ctx context.Context, t *state.Task) error {
	var expected, branch string
	switch {
	case t.Cloud != nil && (t.Cloud.Status == "reported" || t.Cloud.Status == "quota-wait") && t.Cloud.PinnedHead != "":
		expected, branch = t.Cloud.PinnedHead, t.Cloud.Branch
	case t.Handoff != nil && t.Handoff.ConfirmedSHA != "":
		expected, branch = t.Handoff.ConfirmedSHA, t.Handoff.CloudBranch
	default:
		return nil
	}
	remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, branch)
	if err != nil {
		return fmt.Errorf("the remote state of %s is unknown (%v): treated as not safe", branch, err)
	}
	if remote == expected {
		return nil
	}
	s.askUser(ctx, t, "cloud-late-write", fmt.Sprintf("Branch %s moved from the pinned %s to %q after the cloud agent reported idle: a writer may still be running. Nothing is integrated or published until you decide. Check the session, then answer `repin <sha>` (you reviewed that head; the agent must report again) or `abort`.", branch, expected, remote))
	return fmt.Errorf("late write on %s: pinned %s, remote %q", branch, expected, remote)
}

// ---- follow-up on the same session (feedback, or continue after quota)

func (s *Sup) msgFile(t *state.Task, gen int) string {
	return s.D.Store.Sub(t.Issue, "cloud", fmt.Sprintf("send-%d.md", gen))
}

// cloudFollowup composes the message for the next generation, persists it
// durably, and only then delivers it.
func (s *Sup) cloudFollowup(ctx context.Context, t *state.Task, feedbackFile string, reason string) error {
	c := t.Cloud
	if c == nil || c.Mode != "auto" {
		return fmt.Errorf("no automatic cloud session")
	}
	if c.Status != "reported" && c.Status != "quota-wait" {
		return fmt.Errorf("cloud round %d has not reported yet (status %s): feedback is only sent to an idle agent", c.Gen, c.Status)
	}
	if c.Status == "quota-wait" && c.SendGen == c.Gen+1 && c.SendMsg != "" {
		// A rejected send is retried with EXACTLY the message that was rejected.
		if err := s.cloudGate(ctx, t); err != nil {
			return err
		}
		return s.cloudDeliver(ctx, t)
	}
	if err := s.cloudGate(ctx, t); err != nil {
		return err
	}
	newGen := c.Gen + 1
	var body string
	if feedbackFile != "" {
		b, err := os.ReadFile(feedbackFile)
		if err != nil {
			return fmt.Errorf("feedback file: %w", err)
		}
		body = string(b)
	}
	msg := fmt.Sprintf(`issueflow follow-up for run %s, generation %d (%s).
Same rules as before: commit and push ONLY to %s; no docs; no other branch. Start from the current head of that branch (last code commit %s).
%s
When finished: push your code, then in a final separate commit add %s with the same schema as before (schema 1, run_id %q, generation %d, base_sha %q, code_sha = your last code commit, status done|blocked|quota, writer_state "idle", summary, checks) and push it. After that stay idle until the next message.`,
		t.RunID, newGen, reason, c.Branch, c.PinnedCode, body, reportPath(t.RunID, newGen), t.RunID, newGen, c.PinnedCode)
	path := s.msgFile(t, newGen)
	if err := state.WriteFileAtomic(path, []byte(msg), 0o600); err != nil {
		return fmt.Errorf("cannot persist the follow-up: %w", err)
	}
	c.SendGen, c.SendMsg, c.SendBase, c.SendAt = newGen, path, c.PinnedCode, s.now()
	return s.cloudDeliver(ctx, t)
}

// cloudDeliver sends the persisted message for c.SendGen. The next round's
// generation, base and exact text are durable BEFORE the external call.
func (s *Sup) cloudDeliver(ctx context.Context, t *state.Task) error {
	c := t.Cloud
	msgBytes, err := os.ReadFile(c.SendMsg)
	if err != nil {
		return fmt.Errorf("persisted follow-up unreadable: %w", err)
	}
	gen := c.SendGen
	sum := sha256.Sum256(msgBytes)
	if err := s.setIntent(t, fmt.Sprintf("cloud-send-%d", gen), "pending", hex.EncodeToString(sum[:8])); err != nil {
		return fmt.Errorf("not sending, state not durable: %w", err)
	}
	res, serr := s.D.Cloud.Send(ctx, agent.Handle{Kind: "cloud", CloudID: c.SessionID, URL: c.URL}, string(msgBytes))
	if serr == nil && res.Queued && (res.SessionID == "" || res.SessionID == c.SessionID) {
		c.Gen, c.RoundBase, c.Status, c.PinnedHead = gen, c.SendBase, "running", ""
		c.Outcome, c.NextPoll = "", s.now()
		t.Coder.Done = false
		_ = s.setIntent(t, fmt.Sprintf("cloud-send-%d", gen), "done", "queued (not completion)")
		s.audit(t, "cloud-followup", fmt.Sprintf("generation %d queued", gen))
		return nil
	}
	var le *agent.LimitError
	if errors.As(serr, &le) {
		res := s.D.Quota.Classify(le.Text, s.now())
		if res.Class == quota.Quota {
			// Explicit rejection: nothing was queued. The SAME message is kept
			// and re-delivered at the wake.
			_ = s.setIntent(t, fmt.Sprintf("cloud-send-%d", gen), "done", "rejected (quota): not queued")
			c.Status = "quota-wait"
			w := s.setWake(t, "cloud", res)
			s.enterWait(t, state.WaitQuota)
			_, _ = s.post(ctx, t, fmt.Sprintf("wake-cloud-send-%d-%d", gen, w.Attempt), fmt.Sprintf("Paused: the cloud follow-up was rejected (usage limit). issueflow re-sends the same message automatically at about %s UTC.", w.At.Format("2006-01-02 15:04")))
			return nil
		}
	}
	// Unknown (error, timeout, unqueued or foreign-session answer): it may have
	// been queued. The generation advances so its report is recognised; no resend.
	c.Gen, c.RoundBase, c.Status, c.PinnedHead = gen, c.SendBase, "send-ambiguous", ""
	c.NextPoll = s.now().Add(time.Duration(s.D.Cfg.PollSeconds) * time.Second)
	c.SendAt = s.now()
	t.Coder.Done = false
	s.audit(t, "cloud-send-ambiguous", fmt.Sprintf("err=%v queued=%v session=%q", serr, res.Queued, res.SessionID))
	return nil
}

// escalateAmbiguousSend asks the user once an ambiguous send has had no report
// for the configured time. The user may allow one explicit resend or abandon.
func (s *Sup) escalateAmbiguousSend(ctx context.Context, t *state.Task) {
	c := t.Cloud
	if c.Status != "send-ambiguous" || c.SendAt.IsZero() {
		return
	}
	if s.now().Sub(c.SendAt) < time.Duration(s.D.Cfg.Cloud.AmbiguousSendSeconds)*time.Second {
		return
	}
	s.askUser(ctx, t, "cloud-send", fmt.Sprintf("A follow-up to the cloud session (%s) had an unknown outcome and no report for generation %d arrived. It will not be re-sent blindly. Answer `resend` to send it once more as the same generation, or `abandon` if you stopped the session yourself.", orDash(c.URL), c.Gen))
	c.SendAt = time.Time{}
}

// resumeCloudAfterQuota continues the same session at the quota wake.
func (s *Sup) resumeCloudAfterQuota(ctx context.Context, t *state.Task) {
	if t.Cloud == nil || t.Cloud.Status != "quota-wait" {
		return
	}
	reason := "continue after the usage limit reset; carry on where you stopped"
	if err := s.cloudFollowup(ctx, t, "", reason); err != nil {
		s.audit(t, "cloud-resume-error", err.Error())
		s.event(t, "cloud-resume-failed", "Continuing the cloud session after the quota wake failed: "+err.Error())
	}
}

// startAutoCloud records a freshly created automatic session.
func (s *Sup) startAutoCloud(t *state.Task, id, url, sha string) {
	t.Cloud = &state.CloudRun{Mode: "auto", SessionID: id, URL: url, Branch: t.Handoff.CloudBranch, Gen: 1, RoundBase: sha, Status: "running"}
	t.Cloud.NextPoll = s.now()
}

// noReportInTree refuses any .issueflow path in the product tree.
func (s *Sup) noReportInTree(ctx context.Context, t *state.Task) error {
	paths, err := s.D.Git.PathsUnder(ctx, t.Worktree, reportDir)
	if err != nil {
		return fmt.Errorf("cannot check for report files: %w", err)
	}
	if len(paths) > 0 {
		return fmt.Errorf("the cloud report path %s is part of the task tree (%s): the report must not be carried into the product", reportDir, paths[0])
	}
	return nil
}
