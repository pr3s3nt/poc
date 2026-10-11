package sup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/proc"
	"issueflow/internal/quota"
	"issueflow/internal/state"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\r`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func (s *Sup) coderName(t *state.Task, gen int) string {
	return fmt.Sprintf("%s-%d-%s-cd%d", s.D.Cfg.TmuxPrefix, t.Issue, t.RunID[len(t.RunID)-4:], gen)
}

func (s *Sup) handle(t *state.Task) agent.Handle {
	c := &t.Coder
	return agent.Handle{Kind: c.Kind, Session: c.Session.Name, Log: c.Session.Log, CloudID: c.CloudID, URL: c.CloudURL,
		Owner: proc.Owner{PID: c.Session.PID, Start: c.Session.PIDStart}}
}

// readLog returns bytes appended since the last read (bounded) and tracks growth.
func (s *Sup) readLog(sess *state.Session) string {
	st, err := os.Stat(sess.Log)
	if err != nil {
		return ""
	}
	if st.Size() < sess.LogOffset {
		sess.LogOffset = 0
	}
	if st.Size() != sess.LastSize {
		sess.LastSize, sess.LastGrowth = st.Size(), s.now()
	}
	if st.Size() == sess.LogOffset {
		return ""
	}
	f, err := os.Open(sess.Log)
	if err != nil {
		return ""
	}
	defer f.Close()
	n := st.Size() - sess.LogOffset
	if n > 256<<10 {
		sess.LogOffset = st.Size() - 256<<10
		n = 256 << 10
	}
	buf := make([]byte, n)
	k, _ := f.ReadAt(buf, sess.LogOffset)
	sess.LogOffset += int64(k)
	return stripANSI(string(buf[:k]))
}

func (s *Sup) quiet(sess *state.Session) time.Duration {
	if sess.LastGrowth.IsZero() {
		return 0
	}
	return s.now().Sub(sess.LastGrowth)
}

func (s *Sup) quietEnough(sess *state.Session) bool {
	return s.quiet(sess) >= time.Duration(s.D.Cfg.QuietSeconds)*time.Second
}

func (s *Sup) coderStep(ctx context.Context, t *state.Task) {
	c := &t.Coder
	if c.Kind == "local" && c.Session.Status != "" && c.Session.Status != "stopped" {
		s.localCoder(ctx, t)
	}
	if t.State != state.Running && t.State != state.Review {
		return
	}
	s.deliverPending(ctx, t)
	s.digest(ctx, t)
}

func (s *Sup) localCoder(ctx context.Context, t *state.Task) {
	c := &t.Coder
	sess := &c.Session
	cfg := s.D.Cfg
	live, err := s.D.Coder.Alive(ctx, s.handle(t))
	if err != nil || live == agent.Indeterminate {
		s.audit(t, "alive-unknown", fmt.Sprint(err))
		return // never treat an unknown answer as dead or alive
	}
	if sess.Status == "ambiguous" {
		if live == agent.Alive {
			sess.Status, sess.Stage = "launching", ""
			sess.LaunchedAt, sess.LastGrowth = s.now(), s.now()
			s.audit(t, "reconcile", "adopted existing coder session "+sess.Name)
			return
		}
		if s.stopCoder(ctx, t, "ambiguous launch left no session") {
			s.event(t, "coder-launch-failed", "The coder launch outcome was ambiguous and no session exists; it is confirmed stopped. Start it again if needed.")
		}
		return
	}
	text := s.readLog(sess)
	if live == agent.Dead {
		s.onCoderDead(ctx, t)
		return
	}
	switch sess.Status {
	case "launching":
		s.advanceLaunch(ctx, t)
	case "running":
		if sess.Stage != "" {
			s.advanceSubmit(ctx, t)
		}
		if res := s.D.Quota.Classify(text, s.now()); res.Class != quota.None && c.PendingFail == "" {
			c.PendingFail, c.PendingFailAt = text, s.now()
		}
		if c.PendingFail != "" && s.quietEnough(sess) {
			fail := c.PendingFail
			c.PendingFail = ""
			if res := s.D.Quota.Classify(fail, s.now()); res.Class != quota.None {
				s.handleFailure(ctx, t, "coder", res, fail)
				return
			}
		}
	}
	_ = cfg
}

func (s *Sup) onCoderDead(ctx context.Context, t *state.Task) {
	c := &t.Coder
	tail := stripANSI(readTail(c.Session.Log, 16<<10))
	res := s.D.Quota.Classify(c.PendingFail+"\n"+tail, s.now())
	c.PendingFail = ""
	if res.Class != quota.None && !c.Done {
		s.handleFailure(ctx, t, "coder", res, tail)
		return
	}
	if !s.stopCoder(ctx, t, "session ended") {
		return
	}
	if c.Done {
		s.audit(t, "coder-ended", "after done report")
		return
	}
	t.Attempts["coder-t"]++
	s.event(t, "coder-exited", "The coder session ended without a done/blocked report and without a recognised limit message. Last output:\n"+truncate(tail, 3000))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// advanceLaunch moves a freshly started interactive agent to a delivered
// brief: answer the folder-trust dialog explicitly (never a bare Enter), wait
// for a real input prompt, paste, then submit with a separate Enter.
func (s *Sup) advanceLaunch(ctx context.Context, t *state.Task) {
	cfg := s.D.Cfg
	c := &t.Coder
	sess := &c.Session
	if sess.Stage != "" {
		s.advanceSubmit(ctx, t)
		return
	}
	view := stripANSI(readTail(sess.Log, 64<<10))
	if sess.StageSize > 0 && int64(len(view)) > 0 {
		// Only output after the trust answer can make the agent "ready".
		if full, err := os.ReadFile(sess.Log); err == nil && int64(len(full)) > sess.StageSize {
			view = stripANSI(string(full[sess.StageSize:]))
		} else {
			view = ""
		}
	}
	trustRe, yesRe, readyRe := regexp.MustCompile(cfg.Coder.TrustRegex), regexp.MustCompile(cfg.Coder.TrustYesRegex), regexp.MustCompile(cfg.Coder.ReadyRegex)
	unnumRe := regexp.MustCompile(cfg.Coder.TrustUnnumberedRegex)
	if !sess.TrustAnswered && trustRe.MatchString(view) && s.quietEnough(sess) {
		switch m := yesRe.FindStringSubmatch(view); {
		case m != nil:
			// Numbered dialog: type the number printed next to "Yes".
			if err := s.D.Tmux.SendLine(ctx, sess.Name, m[1]); err != nil {
				s.audit(t, "trust-error", err.Error())
				return
			}
			sess.TrustAnswered, sess.StageSize = true, sess.LastSize
			s.audit(t, "trust", "folder-trust dialog answered with the number shown next to Yes: "+m[1])
		case unnumRe.MatchString(view):
			if len(cfg.Coder.TrustUnnumberedKeys) == 0 {
				// Fail closed: a blind Enter may accept the preselected "No, exit".
				if s.stopCoder(ctx, t, "folder-trust dialog needs a one-time decision") {
					s.askUser(ctx, t, "agent-not-ready", fmt.Sprintf("The coder shows the folder-trust dialog with unnumbered options for %s. issueflow will not press Enter blindly (the highlighted option may be 'No, exit'). Trust that folder (or its parent) once in a normal Claude session, or set coder.trust_unnumbered_keys to a verified key sequence, then answer `retry` and start the coder again.", t.Worktree))
				}
				return
			}
			if err := s.D.Tmux.Keys(ctx, sess.Name, cfg.Coder.TrustUnnumberedKeys...); err != nil {
				s.audit(t, "trust-error", err.Error())
				return
			}
			sess.TrustAnswered, sess.StageSize = true, sess.LastSize
			s.audit(t, "trust", "folder-trust dialog answered with configured keys")
		}
		return
	}
	if readyRe.MatchString(view) && !(trustRe.MatchString(view) && !sess.TrustAnswered) && s.quietEnough(sess) {
		briefPtr := fmt.Sprintf("Read %s completely, then do exactly what it says. Report only with the issueflow command it names.", shq(c.BriefFile))
		if _, err := s.D.Coder.Send(ctx, s.handle(t), briefPtr); err != nil {
			s.audit(t, "paste-error", err.Error())
			return
		}
		sess.Stage, sess.StageAt, sess.StageSize = "pasted", s.now(), sess.LastSize
		s.audit(t, "brief-pasted", c.BriefFile)
		return
	}
	if s.now().Sub(sess.LaunchedAt) > time.Duration(cfg.ReadyTimeoutSeconds)*time.Second {
		if s.stopCoder(ctx, t, "agent never showed an input prompt") {
			s.askUser(ctx, t, "agent-not-ready", fmt.Sprintf("The coder session %s did not show a usable input prompt within %ds (trust dialog or unexpected screen). Check %s, then answer `retry` or `abort`.", sess.Name, cfg.ReadyTimeoutSeconds, sess.Log))
		}
	}
}

// advanceSubmit presses Enter separately after a delay and re-presses once if
// the pasted text had no visible effect.
func (s *Sup) advanceSubmit(ctx context.Context, t *state.Task) {
	cfg := s.D.Cfg
	sess := &t.Coder.Session
	switch sess.Stage {
	case "pasted":
		if s.now().Sub(sess.StageAt) < time.Duration(cfg.EnterDelaySeconds)*time.Second {
			return
		}
		if err := s.D.Tmux.Enter(ctx, sess.Name); err != nil {
			s.audit(t, "enter-error", err.Error())
			return
		}
		sess.Stage, sess.StageAt = "entered", s.now()
	case "entered":
		if sess.LastSize > sess.StageSize+8 && s.quiet(sess) < time.Duration(cfg.QuietSeconds)*time.Second || sess.LastSize > sess.StageSize+64 {
			sess.Stage, sess.EnterRetried = "", false
			sess.Status = "running"
			return
		}
		if s.now().Sub(sess.StageAt) < time.Duration(cfg.SubmitCheckSeconds)*time.Second {
			return
		}
		if !sess.EnterRetried {
			_ = s.D.Tmux.Enter(ctx, sess.Name)
			sess.EnterRetried, sess.StageAt = true, s.now()
			s.audit(t, "enter-retry", "no output after submit")
			return
		}
		if s.stopCoder(ctx, t, "submit had no effect") {
			s.askUser(ctx, t, "agent-not-ready", fmt.Sprintf("The coder did not react to the pasted brief in %s. Answer `retry` or `abort`.", sess.Name))
		}
	}
}

func (s *Sup) deliverPending(ctx context.Context, t *state.Task) {
	c := &t.Coder
	sess := &c.Session
	if c.Kind != "local" || sess.Status != "running" || sess.Stage != "" || len(c.Pending) == 0 || !s.quietEnough(sess) {
		return
	}
	b, err := os.ReadFile(c.Pending[0])
	if err != nil {
		s.audit(t, "feedback-error", err.Error())
		c.Pending = c.Pending[1:]
		return
	}
	text := string(b)
	if len(text) > s.D.Cfg.BriefInlineMax {
		text = fmt.Sprintf("Feedback from the coordinator is in %s. Read it completely and act on it.", shq(c.Pending[0]))
	}
	if _, err := s.D.Coder.Send(ctx, s.handle(t), text); err != nil {
		s.audit(t, "feedback-error", err.Error())
		return
	}
	c.Pending = c.Pending[1:]
	c.Done = false
	sess.Stage, sess.StageAt, sess.StageSize = "pasted", s.now(), sess.LastSize
	s.audit(t, "feedback-pasted", "")
}

func (s *Sup) digest(ctx context.Context, t *state.Task) {
	c := &t.Coder
	now := s.now()
	if c.Kind != "local" || c.Session.Status != "running" || now.Before(t.Progress.NextDigestAt) {
		return
	}
	t.Progress.NextDigestAt = now.Add(time.Duration(s.D.Cfg.PollSeconds) * time.Second)
	hash, _ := s.D.Git.ContentHash(ctx, t.Worktree)
	sig := sum(fmt.Sprint(c.Session.LastSize), hash, fmt.Sprint(c.Done))
	if sig == t.Progress.LastDigestSig {
		s.audit(t, "digest-nochange", "no model turn needed")
		return
	}
	t.Progress.LastDigestSig = sig
	tail := truncate(strings.TrimSpace(stripANSI(readTail(c.Session.Log, 2000))), 1800)
	s.event(t, "digest", fmt.Sprintf("Coder gen %d progress (log %d bytes, quiet %s, done=%v).\nDiff vs base:\n%s\nLast output:\n%s",
		c.Gen, c.Session.LastSize, s.quiet(&c.Session).Round(time.Second), c.Done, s.D.Git.DiffStat(ctx, t.Worktree, t.BaseSHA), tail))
}

// acquireLease grants the single write lease. A previous writer must be
// proven stopped; a cloud writer cannot be proven stopped without a user's
// attestation, so it never is.
func (s *Sup) acquireLease(ctx context.Context, t *state.Task, kind string) error {
	l := &t.Lease
	switch l.Holder {
	case "":
	case "local":
		if !s.stopCoder(ctx, t, "lease handover to "+kind) {
			return fmt.Errorf("previous local writer is not confirmed stopped")
		}
	case "cloud":
		if !l.StopConfirmed {
			return fmt.Errorf("a cloud writer holds the lease and its stop is not confirmed")
		}
	}
	return nil
}

// stopCoder stops the owned local session and proves it. Failure parks the task.
func (s *Sup) stopCoder(ctx context.Context, t *state.Task, why string) bool {
	c := &t.Coder
	switch c.Kind {
	case "":
		return true
	case "cloud":
		return false
	}
	ok, err := s.D.Coder.Stop(ctx, s.handle(t))
	if !ok {
		msg := fmt.Sprint(err)
		s.audit(t, "stop-unconfirmed", why+": "+msg)
		if t.State != state.WaitUser {
			s.askUser(ctx, t, "stop-failed", fmt.Sprintf("The coder (%s) could not be proven stopped (%s): %s. No second writer will start. Fix it manually, then answer `retry` or `abort`.", c.Session.Name, why, msg))
		}
		return false
	}
	c.Session.Status, c.Session.Stage = "stopped", ""
	t.Lease = state.Lease{Holder: "", Gen: t.Lease.Gen, StopConfirmed: true, Evidence: "tmux session absent and no marked process (" + why + ")", Since: s.now()}
	s.audit(t, "coder-stopped", why)
	return true
}

func (s *Sup) startLocalCoder(ctx context.Context, t *state.Task, bodyFile, extra string) error {
	c := &t.Coder
	if err := s.acquireLease(ctx, t, "local"); err != nil {
		return err
	}
	body, err := os.ReadFile(bodyFile)
	if err != nil {
		return fmt.Errorf("brief file: %w", err)
	}
	prevCoder, prevLease, prevHandoff := *c, t.Lease, t.Handoff
	gen := c.Gen + 1
	name := s.coderName(t, gen)
	briefFile := s.D.Store.Sub(t.Issue, "briefs", fmt.Sprintf("coder-%d.md", gen))
	if err := writeFile(briefFile, s.coderBrief(t, gen, string(body), extra)); err != nil {
		return err
	}
	// Keep a stable copy of the coordinator's body so relaunches can reuse it.
	stable := s.D.Store.Sub(t.Issue, "briefs", "body.md")
	if bodyFile != stable {
		if err := writeFile(stable, string(body)); err != nil {
			return err
		}
	}
	logp := s.D.Store.Sub(t.Issue, "coder", fmt.Sprintf("coder-%d.log", gen))
	*c = state.Coder{Adapter: s.D.Cfg.Coder.Adapter, Kind: "local", Gen: gen, Brief: stable, BriefFile: briefFile,
		Session: state.Session{Name: name, Log: logp, Status: "launching", LaunchedAt: s.now(), LastGrowth: s.now()}}
	t.Lease = state.Lease{Holder: "local", Gen: gen, Since: s.now()}
	t.Handoff = nil
	if err := s.setIntent(t, fmt.Sprintf("coder-launch-%d", gen), "pending", name); err != nil {
		*c, t.Lease, t.Handoff = prevCoder, prevLease, prevHandoff
		return fmt.Errorf("not launching coder, state not durable: %w", err)
	}
	h, lerr := s.D.Coder.Launch(ctx, agent.StartSpec{Issue: t.Issue, RunID: t.RunID, Gen: gen, Dir: t.Worktree, Session: name, Log: logp, Env: s.roleEnv(t, "coder", gen)})
	c.Session.PID, c.Session.PIDStart = h.Owner.PID, h.Owner.Start
	if lerr != nil {
		c.Session.Status = "ambiguous"
		s.audit(t, "launch-ambiguous", lerr.Error())
		return nil
	}
	_ = s.setIntent(t, fmt.Sprintf("coder-launch-%d", gen), "done", name)
	return nil
}

func hashFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

var _ = filepath.Join
