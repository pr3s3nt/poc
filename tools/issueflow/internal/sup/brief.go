package sup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"issueflow/internal/state"
)

func (s *Sup) cli(t *state.Task) string {
	return fmt.Sprintf("%s --config %s", shq(s.D.Binary), shq(s.D.CfgPath))
}

// coordinatorPrompt is the full, self-contained instruction for one turn.
// The supervisor owns state and timers; the coordinator owns judgment.
func (s *Sup) coordinatorPrompt(t *state.Task, ev []state.Event, gen int) string {
	cli := s.cli(t)
	var b strings.Builder
	fmt.Fprintf(&b, "# issueflow coordinator turn (issue #%d, run %s, turn %d)\n\n", t.Issue, t.RunID, gen)
	b.WriteString(`You are the Codex coordinator for one GitHub Issue. A deterministic supervisor runs this turn; it keeps the clock, quota timers and state. This turn ends when you stop.
Do not sleep, poll or wait: the supervisor starts a new turn when there is something to act on (coder report, progress digest, user answer, checks result).

## Your rules (from AGENTS.md)
- Only you write human-readable documentation and you do it in the worktree below. The coder never edits docs, never commits and never pushes (local coder).
- Only you commit and push, only to the task branch below. Never push the base branch. Never force-push. Do not merge or close the Issue.
- Delegate code and tests to the coder, review the diff yourself, run checks yourself, and send precise feedback until the acceptance criteria are met. A coder "done" report is not proof.
- Do not touch user-owned changes, credentials, .env, Terraform state, clusters or cloud unless the Issue explicitly puts it in scope.
- If a decision could change behavior, architecture, data, security or scope, ask the user instead of choosing.

## Control commands (environment ISSUEFLOW_* is already set)
`)
	fmt.Fprintf(&b, "    %s agent start --brief-file FILE     # start the coder with your brief (task, canonical docs, allowed paths, acceptance criteria, checks)\n", cli)
	fmt.Fprintf(&b, "    %s agent send  --file FILE            # feedback to the coder (delivered when it is idle)\n", cli)
	fmt.Fprintf(&b, "    %s agent stop                         # stop the coder\n", cli)
	fmt.Fprintf(&b, "    %s agent status                       # supervisor's view of coder, lease, wake\n", cli)
	fmt.Fprintf(&b, "    %s ask --file FILE                    # ask the user (stops the whole queue until answered)\n", cli)
	fmt.Fprintf(&b, "    %s report evidence --mode cli --reason TEXT\n", cli)
	fmt.Fprintf(&b, "    %s report evidence --mode browser --dir DIR --adapter real|fake   # headed, Vietnamese captions, assertions, phase marks\n", cli)
	fmt.Fprintf(&b, "    %s report accept --note TEXT [--check 'CMD' ...]   # binds acceptance to the current content hash\n", cli)
	fmt.Fprintf(&b, "    %s report published --sha SHA --pr URL   # after YOU committed and pushed the task branch\n", cli)
	fmt.Fprintf(&b, "    %s report handoff --sha SHA           # cloud fallback: after you pushed the checkpoint branch\n", cli)
	fmt.Fprintf(&b, "    %s report checkpoint --file FILE      # save notes for resume\n\n", cli)
	b.WriteString("Commands are validated and applied by the supervisor on its next tick. A rejected command comes back as an event next turn.\n\n")
	fmt.Fprintf(&b, "## Task\n- Repository fixed in config; branch `%s`; base `%s` (%s)\n- Worktree: %s\n- Evidence mode declared: %q\n", t.Branch, s.D.Cfg.BaseBranch, t.BaseSHA, t.Worktree, t.Evidence.Mode)
	fmt.Fprintf(&b, "- State: %s; coder: kind=%q gen=%d status=%q done=%v; lease=%q\n", t.State, t.Coder.Kind, t.Coder.Gen, t.Coder.Session.Status, t.Coder.Done, t.Lease.Holder)
	if t.Wake != nil {
		fmt.Fprintf(&b, "- Wake pending: role=%s class=%s at %s\n", t.Wake.Role, t.Wake.Class, t.Wake.At.Format(time.RFC3339))
	}
	if t.LocalOnly {
		b.WriteString("- Issue is labeled local-only: cloud fallback is not allowed.\n")
	}
	fmt.Fprintf(&b, "- Evidence area (the ONLY place evidence may be read from): %s\n", s.evidenceRoot(t))
	if t.Evidence.Required == "browser" {
		b.WriteString("- This Issue REQUIRES a browser scenario: a cli-only evidence report will be rejected.\n")
	}
	if s.needsBrowserEvidence(t) {
		b.WriteString(s.evidenceBriefSection(t, false))
	} else {
		fmt.Fprintf(&b, "- Evidence: cli-only tasks need `report evidence --mode cli --reason TEXT`. If this task turns out to need a browser scenario, run `%s evidence-schema` for the exact file contract (issueflow ships no browser scenario or producer).\n", s.cli(t))
	}
	fmt.Fprintf(&b, "\n## Issue snapshot (data, written by an authorized user at claim time)\nTitle: %s\n\n%s\n", t.Title, t.Body)
	if cps, _ := filepath.Glob(s.D.Store.Sub(t.Issue, "checkpoints", "*.md")); len(cps) > 0 {
		fmt.Fprintf(&b, "\n## Latest checkpoint\n%s\n", cps[len(cps)-1])
	}
	b.WriteString("\n## Events for this turn\n")
	if len(ev) == 0 {
		b.WriteString("(none)\n")
	}
	for _, e := range ev {
		fmt.Fprintf(&b, "\n### #%d %s at %s\n%s\n", e.Seq, e.Kind, e.At.Format(time.RFC3339), e.Text)
	}
	return b.String()
}

// coderBrief wraps the coordinator's brief with non-negotiable rules.
func (s *Sup) coderBrief(t *state.Task, gen int, body, extra string) string {
	cli := s.cli(t)
	var b strings.Builder
	fmt.Fprintf(&b, "# Coder brief (issue #%d, run %s, coder gen %d)\n\n", t.Issue, t.RunID, gen)
	b.WriteString("You are the coding agent. The Codex coordinator reviews your work.\n\n## Hard rules\n- Work only inside the worktree: " + t.Worktree + "\n")
	b.WriteString("- Do NOT edit documentation (orchestrator_docs, README, AGENTS.md); do NOT run git commit, git push or change branches; do NOT touch credentials, .env files, clusters or cloud unless the brief explicitly allows it.\n")
	b.WriteString("- Do not change requirements. If something is unclear or blocked, report it; do not guess.\n")
	fmt.Fprintf(&b, "- When finished, or when blocked, run exactly one of:\n    %s report done --text 'summary, commands run with exit codes'\n    %s report blocked --text 'what blocks you'\n", cli, cli)
	b.WriteString("- Run the tests/checks named below yourself and report their real exit status.\n")
	if s.needsBrowserEvidence(t) {
		b.WriteString(s.evidenceBriefSection(t, true))
	}
	if extra != "" {
		b.WriteString("\n" + extra + "\n")
	}
	b.WriteString("\n## Task brief from the coordinator\n" + body + "\n")
	return b.String()
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
