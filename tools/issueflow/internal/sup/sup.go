// Package sup is the deterministic supervisor: queue, state machine, timers,
// recovery and all side effects. It never asks a model to keep time or state.
package sup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/clock"
	"issueflow/internal/config"
	"issueflow/internal/evidence"
	"issueflow/internal/gh"
	"issueflow/internal/gitx"
	"issueflow/internal/proc"
	"issueflow/internal/quota"
	"issueflow/internal/runner"
	"issueflow/internal/state"
	"issueflow/internal/tmux"
)

type Deps struct {
	Cfg     *config.Config
	CfgPath string
	Binary  string // path of the issueflow executable, used in briefs
	Store   *state.Store
	Clock   clock.Clock
	GH      gh.GitHub
	Tmux    tmux.Tmux
	Run     runner.Runner
	Git     gitx.Git
	// RepoIdentity returns owner/name of the checkout's origin. Production uses
	// the origin URL; tests inject an identity for local temporary remotes.
	RepoIdentity func(ctx context.Context, repoPath string) (string, error)
	Coder        agent.Agent // local interactive coder
	Cloud        agent.Agent // nil when not configured
	Procs        proc.Scanner
	Quota        *quota.Classifier
	Val          evidence.Validator
	Pub          evidence.Publisher
	Out          func(string) // optional progress output
}

type Sup struct {
	D       Deps
	cloudOK bool
}

func New(d Deps) *Sup { return &Sup{D: d} }

func (s *Sup) now() time.Time { return s.D.Clock.Now() }

func (s *Sup) say(format string, a ...any) {
	if s.D.Out != nil {
		s.D.Out(fmt.Sprintf(format, a...))
	}
}

func (s *Sup) save(t *state.Task) error {
	t.UpdatedAt = s.now()
	return s.D.Store.SaveTask(t)
}

func (s *Sup) audit(t *state.Task, kind, msg string) {
	s.D.Store.AppendEvent(t.Issue, s.now(), kind, msg)
}

// event queues a message for the coordinator's next turn.
func (s *Sup) event(t *state.Task, kind, text string) {
	t.AddEvent(kind, text, "", 0, s.now())
	s.audit(t, "event:"+kind, firstLine(text))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func sum(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Run holds the single-supervisor lock and ticks until ctx ends.
func (s *Sup) Run(ctx context.Context) error {
	if err := s.D.Store.Lock(); err != nil {
		return err
	}
	defer s.D.Store.Unlock()
	s.ProbeCloud(ctx)
	for {
		if err := s.Tick(ctx); err != nil {
			return err
		}
		d := time.Duration(s.D.Cfg.TickSeconds) * time.Second
		if _, real := s.D.Clock.(clock.Real); real {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(d):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		s.D.Clock.Sleep(d)
	}
}

// ProbeCloud decides once whether cloud fallback may be used. It creates no
// session. Cloud stays off unless config enables it and the CLI has the flags.
func (s *Sup) ProbeCloud(ctx context.Context) {
	s.cloudOK = false
	if s.D.Cfg.Cloud.Mode == "off" || s.D.Cloud == nil {
		return
	}
	p := s.D.Cloud.Probe(ctx)
	s.cloudOK = p.Available && p.CloudFlags
}

// Tick performs one supervision pass over at most one active task.
func (s *Sup) Tick(ctx context.Context) error {
	tasks, err := s.D.Store.ListTasks()
	if err != nil {
		return err
	}
	var active *state.Task
	for _, t := range tasks {
		if t.State.Terminal() {
			continue
		}
		if active != nil {
			return fmt.Errorf("invariant violated: tasks %d and %d are both active", active.Issue, t.Issue)
		}
		active = t
	}
	if active == nil {
		return s.claimNext(ctx, tasks)
	}
	if err := s.step(ctx, active); err != nil {
		return err
	}
	return s.syncQueue(active)
}

func (s *Sup) syncQueue(t *state.Task) error {
	q := state.Queue{}
	if t.State.Waiting() || t.State == state.Failed {
		q = state.Queue{Halted: true, Reason: string(t.State), Issue: t.Issue}
	}
	old, _ := s.D.Store.LoadQueue()
	if old == q {
		return nil
	}
	return s.D.Store.SaveQueue(q)
}

// preflightRepo refuses to work when the configured GitHub repository and the
// checkout's origin are different repositories. Nothing (claim, worktree,
// model) starts before it passes.
func (s *Sup) preflightRepo(ctx context.Context) error {
	id := s.D.RepoIdentity
	if id == nil {
		id = func(ctx context.Context, p string) (string, error) { return s.D.Git.OriginRepo(ctx, p) }
	}
	got, err := id(ctx, s.D.Cfg.RepoPath)
	if err != nil {
		return fmt.Errorf("repo preflight: %w", err)
	}
	if !strings.EqualFold(got, s.D.Cfg.Repo) {
		return fmt.Errorf("repo preflight: config repo %q but the checkout's origin is %q; refusing to run", s.D.Cfg.Repo, got)
	}
	return nil
}

func (s *Sup) claimNext(ctx context.Context, existing []*state.Task) error {
	if err := s.preflightRepo(ctx); err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, t := range existing {
		seen[t.Issue] = true
	}
	issues, err := s.D.GH.ListIssues(ctx, s.D.Cfg.ReadyLabel)
	if err != nil {
		s.say("list issues: %v", err)
		return nil
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Number < issues[j].Number })
	for _, is := range issues {
		if seen[is.Number] {
			continue
		}
		if !s.D.Cfg.IsOwner(is.Author) {
			continue
		}
		actor, err := s.D.GH.LabelActor(ctx, is.Number, s.D.Cfg.ReadyLabel)
		if err != nil || !s.D.Cfg.IsOwner(actor) {
			continue
		}
		now := s.now()
		t := &state.Task{
			Issue: is.Number, Title: is.Title, Body: is.Body, BodyHash: sum(is.Title, is.Body), Author: is.Author,
			RunID: "r" + now.Format("20060102T150405") + state.NewID()[:4], State: state.Ready,
			Intents: map[string]*state.Intent{}, Attempts: map[string]int{}, Applied: map[string]bool{},
			CreatedAt: now, UpdatedAt: now,
		}
		for _, l := range is.Labels {
			if l == s.D.Cfg.LocalOnlyLabel {
				t.LocalOnly = true
			}
			if l == s.D.Cfg.BrowserLabel {
				t.Evidence.Required = "browser"
			}
			if l == s.D.Cfg.ExternalLabel {
				t.AllowExternal = true
			}
		}
		if strings.Contains(is.Body, "issueflow-external: verify") {
			t.AllowExternal = true // the owner's Issue explicitly puts external verification in scope
		}
		if strings.Contains(is.Body, "issueflow-e2e: browser") {
			t.Evidence.Required = "browser"
		}
		t.Progress.BodyHashSeen = t.BodyHash
		t.Progress.CommentCursor = 0
		if err := s.save(t); err != nil {
			return err
		}
		s.audit(t, "claimed", fmt.Sprintf("run %s author %s labeled-by %s", t.RunID, is.Author, actor))
		return nil
	}
	return nil
}

func (s *Sup) step(ctx context.Context, t *state.Task) error {
	if err := s.drain(t); err != nil {
		return err
	}
	s.applyCommands(ctx, t)
	var err error
	switch t.State {
	case state.Ready:
		err = s.stepReady(ctx, t)
	case state.Running, state.Review:
		err = s.stepActive(ctx, t)
	case state.WaitUser:
		s.cloudPoll(ctx, t)
		s.pollGitHub(ctx, t)
		s.ensureQuestionPosted(ctx, t)
		_ = s.coordWatch(ctx, t) // finish or time out a running turn; never start one
	case state.WaitAuth, state.Failed:
		_ = s.coordWatch(ctx, t)
	case state.WaitQuota:
		err = s.stepWaitQuota(ctx, t)
	case state.Validating:
		// No new turns while checks run, but a running turn is still supervised.
		if err = s.coordWatch(ctx, t); err == nil && t.State == state.Validating {
			err = s.stepValidating(ctx, t)
		}
	case state.Publishing:
		if t.Publish.AwaitCommit {
			s.pollGitHub(ctx, t)
			if t.State == state.Publishing {
				err = s.coordStep(ctx, t) // the checks-passed event must start a turn
			}
		} else {
			err = s.coordWatch(ctx, t)
			if err == nil && t.State == state.Publishing {
				err = s.stepPublishing(ctx, t)
			}
		}
	}
	if err != nil {
		t.LastError = err.Error()
		s.audit(t, "step-error", err.Error())
	}
	return s.save(t)
}

func (s *Sup) drain(t *state.Task) error {
	cmds, paths, err := s.D.Store.Spooled(t.Issue)
	if err != nil || len(cmds) == 0 {
		return err
	}
	for _, c := range cmds {
		if t.Applied[c.ID] {
			continue
		}
		t.Applied[c.ID] = true
		c.Status = "pending"
		t.Commands = append(t.Commands, c)
	}
	if err := s.save(t); err != nil {
		return err
	}
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return nil
}

// setIntent durably records an intent. Callers must not perform the side
// effect when it returns an error.
func (s *Sup) setIntent(t *state.Task, key, status, detail string) error {
	if t.Intents == nil {
		t.Intents = map[string]*state.Intent{}
	}
	t.Intents[key] = &state.Intent{Status: status, Detail: detail}
	return s.save(t)
}

func (s *Sup) intentDone(t *state.Task, key string) bool {
	in := t.Intents[key]
	return in != nil && in.Status == "done"
}

// post writes one issue comment at most once per key, even across crashes:
// the intent is saved before the call and the marker is searched before retry.
func (s *Sup) post(ctx context.Context, t *state.Task, key, body string) (int64, error) {
	if in := t.Intents["post-"+key]; in != nil && in.Status == "done" {
		var id int64
		fmt.Sscan(in.Detail, &id)
		return id, nil
	}
	marker := fmt.Sprintf("<!-- issueflow:%s:run=%s -->", key, t.RunID)
	if err := s.setIntent(t, "post-"+key, "pending", ""); err != nil {
		return 0, err
	}
	id, found, err := s.D.GH.FindMarker(ctx, t.Issue, marker)
	if err != nil {
		return 0, err
	}
	if !found {
		id, err = s.D.GH.Comment(ctx, t.Issue, body+"\n\n"+marker)
		if err != nil {
			return 0, err
		}
	}
	if err := s.setIntent(t, "post-"+key, "done", fmt.Sprint(id)); err != nil {
		return id, err
	}
	return id, nil
}

func (s *Sup) stepReady(ctx context.Context, t *state.Task) error {
	cfg := s.D.Cfg
	if err := s.preflightRepo(ctx); err != nil {
		return err
	}
	if !s.intentDone(t, "claim") {
		if _, err := s.post(ctx, t, "claim", fmt.Sprintf("Claimed by issueflow. Run `%s`, branch `issueflow/%d-%s`. One task runs at a time.", t.RunID, t.Issue, t.RunID)); err != nil {
			return err
		}
		if err := s.D.GH.AddLabel(ctx, t.Issue, cfg.ActiveLabel); err != nil {
			return err
		}
		if err := s.D.GH.RemoveLabel(ctx, t.Issue, cfg.ReadyLabel); err != nil {
			return err
		}
		if err := s.setIntent(t, "claim", "done", ""); err != nil {
			return err
		}
	}
	if !s.intentDone(t, "worktree") {
		t.Branch = fmt.Sprintf("issueflow/%d-%s", t.Issue, t.RunID)
		t.Worktree = filepath.Join(cfg.WorktreeRoot, fmt.Sprintf("%d-%s", t.Issue, t.RunID))
		if err := s.setIntent(t, "worktree", "pending", ""); err != nil {
			return err
		}
		if !s.D.Git.WorktreeExists(t.Worktree) {
			if err := s.D.Git.Fetch(ctx, cfg.RepoPath, cfg.BaseBranch); err != nil {
				return err
			}
			base, err := s.D.Git.RevParse(ctx, cfg.RepoPath, "origin/"+cfg.BaseBranch)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(cfg.WorktreeRoot, 0o700); err != nil {
				return err
			}
			if err := s.D.Git.AddWorktree(ctx, cfg.RepoPath, t.Worktree, t.Branch, base); err != nil {
				return err
			}
			t.BaseSHA = base
		}
		if t.BaseSHA == "" {
			// Recovered after a crash: the worktree exists; base is its merge base.
			out, err := s.D.Git.RevParse(ctx, t.Worktree, "origin/"+cfg.BaseBranch)
			if err != nil {
				return err
			}
			t.BaseSHA = out
		}
		if err := s.setIntent(t, "worktree", "done", t.Worktree); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(s.evidenceRoot(t), 0o700); err != nil {
		return err
	}
	if err := t.SetState(state.Running, s.now()); err != nil {
		return err
	}
	t.Progress.NextPollAt = s.now().Add(time.Duration(cfg.PollSeconds) * time.Second)
	s.event(t, "task-start", "Task claimed. Read the issue snapshot, update docs if needed, delegate code/test to the coder, review independently.")
	return nil
}

func (s *Sup) stepActive(ctx context.Context, t *state.Task) error {
	s.pollGitHub(ctx, t)
	if t.State != state.Running && t.State != state.Review {
		return nil
	}
	s.coderStep(ctx, t)
	if t.State != state.Running && t.State != state.Review {
		return nil
	}
	s.cloudWatch(ctx, t)
	s.cloudPoll(ctx, t)
	if t.State != state.Running && t.State != state.Review {
		return nil
	}
	return s.coordStep(ctx, t)
}

var errStale = errors.New("stale")
