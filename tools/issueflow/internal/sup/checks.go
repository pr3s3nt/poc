package sup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"issueflow/internal/evidence"
	"issueflow/internal/state"
)

// localOnlyEnv unsets opt-ins that would reach databases/clusters from checks.
var localOnlyEnv = []string{"ORCHESTRATOR_POSTGRES_TEST_URL=", "ORCH_KIND_VERIFY="}

func (s *Sup) backToReview(t *state.Task, why string) {
	t.Accept = nil
	if err := t.SetState(state.Review, s.now()); err != nil {
		s.audit(t, "state-error", err.Error())
	}
	s.event(t, "acceptance-void", why)
}

// stepValidating runs the supervisor's own checks against the accepted
// content. The model's report of passing tests is never evidence.
func (s *Sup) stepValidating(ctx context.Context, t *state.Task) error {
	if t.Accept == nil {
		s.backToReview(t, "no acceptance record")
		return nil
	}
	if err := s.revisionOK(ctx, t); err != nil {
		s.backToReview(t, "Acceptance voided: "+err.Error())
		return nil
	}
	if err := s.noReportInTree(ctx, t); err != nil {
		s.backToReview(t, "Acceptance voided: "+err.Error())
		return nil
	}
	if err := s.cloudGate(ctx, t); err != nil {
		s.audit(t, "cloud-gate", err.Error())
		return nil // unknown remote state or a late write: wait/halted, never validated
	}
	h0, err := s.D.Git.ContentHash(ctx, t.Worktree)
	if err != nil {
		return err
	}
	if h0 != t.Accept.Hash {
		s.backToReview(t, "Acceptance is stale: the content changed after you accepted it. Review again.")
		return nil
	}
	head, _ := s.D.Git.Head(ctx, t.Worktree)
	type chk struct{ name, cmd, dir string }
	var list []chk
	for _, c := range s.D.Cfg.Checks {
		list = append(list, chk{c.Name, c.Cmd, c.Dir})
	}
	for i, c := range t.Accept.Checks {
		list = append(list, chk{fmt.Sprintf("accept-%d", i+1), c, ""})
	}
	t.Checks = nil
	var failed []string
	for _, c := range list {
		dir := t.Worktree
		if c.dir != "" {
			dir = filepath.Join(t.Worktree, c.dir)
		}
		env := localOnlyEnv // default: opt-ins for databases/clusters stay off
		if t.AllowExternal {
			env = nil // the Issue explicitly authorises external verification
		}
		res, rerr := s.D.Run.Run(ctx, dir, env, "bash", "-c", c.cmd)
		exit := res.Exit
		if rerr != nil {
			exit = -1
		}
		logp := s.D.Store.Sub(t.Issue, "checks", fmt.Sprintf("%.12s-%s.log", h0, safe(c.name)))
		body := fmt.Sprintf("$ %s\n(dir %s, content %s, head %s)\nexit %d\n--- stdout ---\n%s\n--- stderr ---\n%s\n", c.cmd, dir, h0, head, exit, res.Stdout, res.Stderr)
		if rerr != nil {
			body += "\nrunner error: " + rerr.Error() + "\n"
		}
		if err := writeFile(logp, body); err != nil {
			return err
		}
		cr := state.CheckResult{Name: c.name, Cmd: c.cmd, Exit: exit, Log: logp, Hash: h0, Head: head, At: s.now(), Passed: exit == 0}
		t.Checks = append(t.Checks, cr)
		if exit != 0 {
			failed = append(failed, fmt.Sprintf("%s (exit %d, log %s)", c.name, exit, logp))
		}
	}
	h1, err := s.D.Git.ContentHash(ctx, t.Worktree)
	if err != nil {
		return err
	}
	if h1 != h0 {
		s.backToReview(t, "Acceptance voided: the content changed while the checks ran.")
		return nil
	}
	if len(failed) > 0 {
		s.backToReview(t, "Checks failed on the accepted content:\n- "+strings.Join(failed, "\n- ")+"\nFix and accept again.")
		return nil
	}
	if err := t.SetState(state.Publishing, s.now()); err != nil {
		return err
	}
	t.Publish = state.Publish{AwaitCommit: true}
	s.event(t, "checks-passed", fmt.Sprintf("All %d checks passed on content %.12s. Now commit the reviewed content on branch %s and push it (you only), then run:\n    %s report published --sha <commit> --pr <url or ->\nThe worktree must be clean and unchanged.", len(list), h0, t.Branch, s.cli(t)))
	return nil
}

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)
}

func (s *Sup) stepPublishing(ctx context.Context, t *state.Task) error {
	if t.Publish.AwaitCommit {
		return nil // applyPublished moves on; the coordinator was already told
	}
	if t.Accept == nil {
		s.backToReview(t, "no acceptance record")
		return nil
	}
	if s.now().Before(t.Publish.NextAttempt) {
		return nil
	}
	// Re-verify everything that could have moved since acceptance.
	if err := s.revisionOK(ctx, t); err != nil {
		s.backToReview(t, "Acceptance voided before publish: "+err.Error())
		return nil
	}
	h, err := s.D.Git.ContentHash(ctx, t.Worktree)
	if err != nil {
		return err
	}
	if h != t.Accept.Hash {
		s.backToReview(t, "Acceptance is stale: content changed before publish.")
		return nil
	}
	if head, _ := s.D.Git.Head(ctx, t.Worktree); head != t.Publish.Commit {
		s.backToReview(t, "HEAD moved after you reported the commit; commit state and report again.")
		return nil
	}
	if remote, err := s.D.Git.RemoteSHA(ctx, t.Worktree, t.Branch); err != nil || remote != t.Publish.Commit {
		s.backToReview(t, fmt.Sprintf("Remote branch %s is at %q, not the reported commit.", t.Branch, remote))
		return nil
	}
	if err := s.noReportInTree(ctx, t); err != nil {
		s.backToReview(t, "Acceptance voided before publish: "+err.Error())
		return nil
	}
	// The branch pinned from the cloud agent is read again at this gate.
	if err := s.cloudGate(ctx, t); err != nil {
		s.audit(t, "cloud-gate", err.Error())
		return nil
	}
	// No writer may remain when the task is published.
	if t.Coder.Kind == "local" && t.Coder.Session.Status != "" && t.Coder.Session.Status != "stopped" {
		if !s.stopCoder(ctx, t, "task publish") {
			return nil
		}
	}
	if t.Coordinator.Session != "" && !s.stopTurn(ctx, t, "task publish") {
		return nil
	}
	if !t.Evidence.Published {
		if err := s.publishEvidence(ctx, t); err != nil {
			t.Publish.Attempts++
			t.Publish.LastError = err.Error()
			delay := time.Duration(30<<uint(min(t.Publish.Attempts-1, 5))) * time.Second
			if delay > 10*time.Minute {
				delay = 10 * time.Minute
			}
			t.Publish.NextAttempt = s.now().Add(delay)
			s.audit(t, "publish-error", fmt.Sprintf("attempt %d: %v", t.Publish.Attempts, err))
			if t.Publish.Attempts >= 6 {
				s.failTask(ctx, t, "Publishing evidence failed 6 times: "+err.Error())
			}
			return nil
		}
	}
	if _, err := s.post(ctx, t, "final", s.finalSummary(t)); err != nil {
		return err
	}
	if err := s.D.GH.AddLabel(ctx, t.Issue, s.D.Cfg.DoneLabel); err != nil {
		return err
	}
	if err := s.D.GH.RemoveLabel(ctx, t.Issue, s.D.Cfg.ActiveLabel); err != nil {
		return err
	}
	if err := t.SetState(state.Done, s.now()); err != nil {
		return err
	}
	s.audit(t, "done", t.Publish.Commit)
	return nil
}

func (s *Sup) publishEvidence(ctx context.Context, t *state.Task) error {
	var logs []evidence.LogRef
	for _, c := range t.Checks {
		if c.Passed {
			logs = append(logs, evidence.LogRef{Name: c.Name, Path: c.Log})
		}
	}
	maxGen := t.Coder.Gen
	if t.CoordGen > maxGen {
		maxGen = t.CoordGen
	}
	m, err := s.D.Val.Build(ctx, evidence.Spec{
		Issue: t.Issue, RunID: t.RunID, Mode: t.Evidence.Mode, Reason: t.Evidence.Reason, Adapter: t.Evidence.Adapter,
		Root: s.evidenceRoot(t), InDir: t.Evidence.Dir, StageDir: s.D.Store.Sub(t.Issue, "evidence-staged"),
		ContentHash: t.Accept.Hash, MaxGen: maxGen, Commit: t.Publish.Commit, Checks: logs,
	}, s.now())
	if err != nil {
		return fmt.Errorf("evidence invalid: %w", err)
	}
	links, err := s.D.Pub.Publish(ctx, m, s.D.Store.Sub(t.Issue, "evidence-upload"))
	if err != nil {
		return err
	}
	if err := writeFile(s.D.Store.Sub(t.Issue, "manifest.json"), string(m.Bytes())); err != nil {
		return err
	}
	t.Evidence.Manifest, t.Evidence.ManifestHash, t.Evidence.Links, t.Evidence.Published = s.D.Store.Sub(t.Issue, "manifest.json"), m.Hash(), links, true
	return nil
}

func (s *Sup) finalSummary(t *state.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**issueflow: task complete**\n\n- Branch: `%s`\n- Commit: `%s`\n", t.Branch, t.Publish.Commit)
	if t.Publish.PR != "" && t.Publish.PR != "-" {
		fmt.Fprintf(&b, "- PR: %s\n", t.Publish.PR)
	}
	fmt.Fprintf(&b, "- Reviewed content hash: `%s`\n\n**Checks (run by the supervisor on that content)**\n", t.Accept.Hash)
	for _, c := range t.Checks {
		fmt.Fprintf(&b, "- `%s`: exit %d\n", c.Cmd, c.Exit)
	}
	fmt.Fprintf(&b, "\n**Evidence** (%s)\n", t.Evidence.Mode)
	if t.Evidence.Mode == "browser" {
		fmt.Fprintf(&b, "- Headed browser scenario, adapter: %s (a fake adapter does not prove a live deploy)\n", t.Evidence.Adapter)
	} else {
		fmt.Fprintf(&b, "- No browser video: %s\n", t.Evidence.Reason)
	}
	for _, l := range t.Evidence.Links {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	fmt.Fprintf(&b, "- Manifest sha256: `%s`\n", t.Evidence.ManifestHash)
	if c := t.Cloud; c != nil && c.Mode == "auto" {
		fmt.Fprintf(&b, "\nCloud exception used: a cloud session pushed only to `%s`; its completion was learned from the agent's idle report (generation %d, code commit `%s`, branch pinned at `%s`) - a report-acknowledged idle state, not an observed stop of the cloud process. Codex reviewed, imported the code without the report and reran the checks. Local coders never commit or push.\n", c.Branch, c.Gen, c.PinnedCode, c.PinnedHead)
	} else if t.Handoff != nil && t.Handoff.ConfirmedSHA != "" {
		fmt.Fprintf(&b, "\nCloud exception used: a cloud session pushed only to `%s` (user-attested finished at `%s`, legacy assisted mode); Codex reviewed and integrated it. Local coders never commit or push.\n", t.Handoff.CloudBranch, t.Handoff.ConfirmedSHA)
	}
	b.WriteString("\nThe Issue is left open; merging and closing are manual.\n")
	return b.String()
}

var _ = os.Remove
