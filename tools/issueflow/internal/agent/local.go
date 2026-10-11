package agent

import (
	"context"
	"fmt"
	"strings"

	"issueflow/internal/proc"
	"issueflow/internal/runner"
	"issueflow/internal/tmux"
)

// Local runs an interactive CLI agent in tmux. The command is typed into an
// interactive bash so shell aliases (clauded) resolve. Output is mirrored to a
// log file with pipe-pane; the supervisor reads the log, never the pane.
//
// Only the claude-local configuration is exercised by tests and real use.
// "tmux-generic" just runs a configured command with the same plumbing; it is
// not a claim that OpenCode/Kimi or any other agent works.
type Local struct {
	NameStr string
	Command string
	T       tmux.Tmux
	R       runner.Runner
	Procs   proc.Scanner
}

func (l *Local) Name() string { return l.NameStr }

func (l *Local) Caps() Caps {
	return Caps{Execution: "local", Interactive: true, Status: Verified, Result: Verified, ConfirmedStop: true, SeesLocalFS: true, HasLocalEnv: true, Reports: true}
}

func (l *Local) Probe(ctx context.Context) ProbeResult {
	p := ProbeResult{Name: l.NameStr, Caps: l.Caps()}
	if res, err := l.R.Run(ctx, "", nil, "tmux", "-V"); err == nil && res.Exit == 0 {
		p.Notes = append(p.Notes, "tmux: "+strings.TrimSpace(res.Stdout))
		p.Available = true
	} else {
		p.Notes = append(p.Notes, "tmux missing")
	}
	// The alias exists only in an interactive shell.
	first := strings.Fields(l.Command)
	if len(first) > 0 {
		res, err := l.R.Run(ctx, "", nil, "bash", "-ic", "type "+first[0])
		if err != nil || res.Exit != 0 {
			p.Available = false
			p.Notes = append(p.Notes, fmt.Sprintf("%q not found in bash -i", first[0]))
		} else {
			p.Notes = append(p.Notes, strings.TrimSpace(res.Stdout))
		}
	}
	return p
}

func (l *Local) Launch(ctx context.Context, s StartSpec) (Handle, error) {
	h := Handle{Kind: "local", Session: s.Session, Log: s.Log}
	if err := l.T.NewSession(ctx, s.Session, s.Dir, s.Env, "bash -i"); err != nil {
		// Unknown whether it exists: caller reconciles via HasSession.
		return h, fmt.Errorf("%w: %v", ErrAmbiguous, err)
	}
	owner, oerr := RecordOwner(ctx, l.T, l.Procs, s.Session)
	if oerr != nil {
		return h, fmt.Errorf("%w: process ownership not recorded: %v", ErrAmbiguous, oerr)
	}
	h.Owner = owner
	if err := l.T.PipeLog(ctx, s.Session, s.Log); err != nil {
		return h, fmt.Errorf("%w: pipe-pane: %v", ErrAmbiguous, err)
	}
	if err := l.T.SendLine(ctx, s.Session, l.Command); err != nil {
		return h, fmt.Errorf("%w: start command: %v", ErrAmbiguous, err)
	}
	return h, nil
}

func (l *Local) Alive(ctx context.Context, h Handle) (Liveness, error) {
	ok, err := l.T.HasSession(ctx, h.Session)
	if err != nil {
		return Indeterminate, err
	}
	if ok {
		return Alive, nil
	}
	return Dead, nil
}

// Send pastes text (bracketed) WITHOUT pressing Enter: a paste may not be
// processed yet, so the supervisor submits with Enter in a later step.
func (l *Local) Send(ctx context.Context, h Handle, text string) (SendResult, error) {
	if err := l.T.Paste(ctx, h.Session, text); err != nil {
		return SendResult{}, err
	}
	return SendResult{Pasted: true}, nil
}

// Stop proves the writer is gone; see StopSession.
func (l *Local) Stop(ctx context.Context, h Handle) (bool, error) {
	return StopSession(ctx, l.T, l.Procs, h.Session, h.Owner)
}

// RecordOwner reads the pane leader pid and its kernel start time.
func RecordOwner(ctx context.Context, t tmux.Tmux, procs proc.Scanner, session string) (proc.Owner, error) {
	if procs == nil {
		return proc.Owner{}, fmt.Errorf("no process scanner")
	}
	pid, err := t.PanePID(ctx, session)
	if err != nil {
		return proc.Owner{}, err
	}
	return procs.Identify(pid)
}

// StopSession kills an owned tmux session and proves the work ended: the
// session must be definitely absent AND no live process may remain in the
// recorded session (a missing tmux session alone proves nothing, background
// jobs outlive it). Survivors are signalled and re-checked; an unknown scan,
// a missing ownership record or a survivor means NOT confirmed.
func StopSession(ctx context.Context, t tmux.Tmux, procs proc.Scanner, name string, owner proc.Owner) (bool, error) {
	if err := t.Kill(ctx, name); err != nil {
		return false, err
	}
	ok, err := t.HasSession(ctx, name)
	if err != nil {
		return false, fmt.Errorf("cannot confirm stop: %w", err)
	}
	if ok {
		return false, fmt.Errorf("session %s still exists after kill", name)
	}
	if procs == nil || !owner.Valid() {
		return false, fmt.Errorf("no ownership record: cannot rule out surviving processes")
	}
	pids, err := procs.Members(owner)
	if err != nil {
		return false, fmt.Errorf("process scan unknown: %w", err)
	}
	if len(pids) == 0 {
		return true, nil
	}
	if err := procs.Terminate(owner, pids); err != nil {
		return false, err
	}
	pids, err = procs.Members(owner)
	if err != nil {
		return false, fmt.Errorf("process re-scan unknown: %w", err)
	}
	if len(pids) > 0 {
		return false, fmt.Errorf("%d process(es) of the session survive: %v", len(pids), pids)
	}
	return true, nil
}
