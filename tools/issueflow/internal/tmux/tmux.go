// Package tmux is the session port: real tmux through the runner, plus a fake.
package tmux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"issueflow/internal/runner"
)

type Tmux interface {
	HasSession(ctx context.Context, name string) (bool, error)
	// NewSession fails if the name already exists (no silent reuse).
	NewSession(ctx context.Context, name, dir string, env map[string]string, shellCmd string) error
	PipeLog(ctx context.Context, name, logPath string) error
	SendLine(ctx context.Context, name, text string) error
	// Enter presses Enter alone (separate from Paste: a bracketed paste may not be processed yet).
	Enter(ctx context.Context, name string) error
	// Keys sends named keys (for example Down, y, Enter) in one send-keys call.
	Keys(ctx context.Context, name string, keys ...string) error
	// Paste inserts text as a bracketed paste and presses Enter.
	Paste(ctx context.Context, name, text string) error
	Kill(ctx context.Context, name string) error
	// PanePID is the pid of the pane's leader process (a session leader).
	PanePID(ctx context.Context, name string) (int, error)
}

type Real struct {
	R      runner.Runner
	Tmp    string // scratch dir for paste buffers
	Socket string // optional tmux -L socket name (isolated server, used by tests)
}

func (t *Real) run(ctx context.Context, args ...string) (runner.Result, error) {
	if t.Socket != "" {
		args = append([]string{"-L", t.Socket}, args...)
	}
	return t.R.Run(ctx, "", nil, "tmux", args...)
}

// notFound matches only tmux's definite "no such session / no server" answers.
func notFound(stderr string) bool {
	return strings.Contains(stderr, "can't find session") || strings.Contains(stderr, "no server running") ||
		strings.Contains(stderr, "error connecting to") && strings.Contains(stderr, "No such file or directory")
}

// HasSession returns an error (unknown) for anything but a definite answer, so
// a permission or socket problem is never mistaken for "stopped".
func (t *Real) HasSession(ctx context.Context, name string) (bool, error) {
	res, err := t.run(ctx, "has-session", "-t", "="+name)
	if err != nil {
		return false, err
	}
	if res.Exit == 0 {
		return true, nil
	}
	if notFound(res.Stderr) {
		return false, nil
	}
	return false, fmt.Errorf("tmux has-session %s: indeterminate (exit %d): %s", name, res.Exit, strings.TrimSpace(res.Stderr))
}

func (t *Real) NewSession(ctx context.Context, name, dir string, env map[string]string, shellCmd string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+env[k])
	}
	args = append(args, shellCmd)
	res, err := t.run(ctx, args...)
	if err != nil {
		return err
	}
	if res.Exit != 0 {
		return fmt.Errorf("tmux new-session %s: %s", name, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (t *Real) PipeLog(ctx context.Context, name, logPath string) error {
	// `cat >> log` fails silently inside tmux when the directory is missing.
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	f.Close()
	res, err := t.run(ctx, "pipe-pane", "-t", name, "-o", "cat >> "+shQuote(logPath))
	if err != nil {
		return err
	}
	if res.Exit != 0 {
		return fmt.Errorf("tmux pipe-pane %s: %s", name, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (t *Real) SendLine(ctx context.Context, name, text string) error {
	if res, err := t.run(ctx, "send-keys", "-t", name, "-l", text); err != nil || res.Exit != 0 {
		return fmt.Errorf("tmux send-keys: %v %s", err, res.Stderr)
	}
	if res, err := t.run(ctx, "send-keys", "-t", name, "Enter"); err != nil || res.Exit != 0 {
		return fmt.Errorf("tmux send-keys Enter: %v %s", err, res.Stderr)
	}
	return nil
}

func (t *Real) Enter(ctx context.Context, name string) error {
	if res, err := t.run(ctx, "send-keys", "-t", name, "Enter"); err != nil || res.Exit != 0 {
		return fmt.Errorf("tmux send-keys Enter: %v %s", err, res.Stderr)
	}
	return nil
}

func (t *Real) Keys(ctx context.Context, name string, keys ...string) error {
	args := append([]string{"send-keys", "-t", name}, keys...)
	if res, err := t.run(ctx, args...); err != nil || res.Exit != 0 {
		return fmt.Errorf("tmux send-keys: %v %s", err, res.Stderr)
	}
	return nil
}

func (t *Real) Paste(ctx context.Context, name, text string) error {
	dir := t.Tmp
	if dir == "" {
		dir = os.TempDir()
	}
	f := filepath.Join(dir, ".paste-"+strings.ReplaceAll(name, "/", "_"))
	if err := os.WriteFile(f, []byte(text), 0o600); err != nil {
		return err
	}
	defer os.Remove(f)
	buf := "if-" + name
	if res, err := t.run(ctx, "load-buffer", "-b", buf, f); err != nil || res.Exit != 0 {
		return fmt.Errorf("tmux load-buffer: %v %s", err, res.Stderr)
	}
	if res, err := t.run(ctx, "paste-buffer", "-p", "-d", "-b", buf, "-t", name); err != nil || res.Exit != 0 {
		return fmt.Errorf("tmux paste-buffer: %v %s", err, res.Stderr)
	}
	return nil
}

func (t *Real) PanePID(ctx context.Context, name string) (int, error) {
	res, err := t.run(ctx, "display-message", "-p", "-t", name, "#{pane_pid}")
	if err != nil || res.Exit != 0 {
		return 0, fmt.Errorf("tmux display-message: %v %s", err, res.Stderr)
	}
	var pid int
	if _, err := fmt.Sscan(strings.TrimSpace(res.Stdout), &pid); err != nil || pid <= 1 {
		return 0, fmt.Errorf("tmux pane_pid unparseable: %q", res.Stdout)
	}
	return pid, nil
}

func (t *Real) Kill(ctx context.Context, name string) error {
	res, err := t.run(ctx, "kill-session", "-t", "="+name)
	if err != nil {
		return err
	}
	if res.Exit != 0 && !notFound(res.Stderr) {
		return fmt.Errorf("tmux kill-session %s: %s", name, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Fake is an in-memory tmux for tests.
type Fake struct {
	mu       sync.Mutex
	Sessions map[string]*FakeSession
	Created  []string
	// KillFails keeps the session alive after Kill (stop failure injection).
	KillFails map[string]bool
	// FailNew makes NewSession return an error after creating the session
	// (launch ambiguity: the caller cannot know whether it exists).
	FailNew map[string]bool
	HasErr  map[string]bool
	nextPid int
}

type FakeSession struct {
	Name, Dir, Cmd string
	Env            map[string]string
	Log            string
	Lines          []string
	Pastes         []string
	Enters         int
	Keys           []string
	Pid            int
}

func NewFake() *Fake {
	return &Fake{Sessions: map[string]*FakeSession{}, KillFails: map[string]bool{}, FailNew: map[string]bool{}}
}

func (f *Fake) HasSession(_ context.Context, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.HasErr[name] {
		return false, fmt.Errorf("injected has-session error")
	}
	_, ok := f.Sessions[name]
	return ok, nil
}

func (f *Fake) NewSession(_ context.Context, name, dir string, env map[string]string, cmd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Sessions[name]; ok {
		return fmt.Errorf("duplicate session: %s", name)
	}
	f.nextPid += 7
	f.Sessions[name] = &FakeSession{Name: name, Dir: dir, Cmd: cmd, Env: env, Pid: 1000 + f.nextPid}
	f.Created = append(f.Created, name)
	if f.FailNew[name] {
		return fmt.Errorf("injected new-session failure after creation: %s", name)
	}
	return nil
}

func (f *Fake) PipeLog(_ context.Context, name, logPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Sessions[name]
	if !ok {
		return fmt.Errorf("no session %s", name)
	}
	s.Log = logPath
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	return appendFile(logPath, "")
}

func (f *Fake) SendLine(_ context.Context, name, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Sessions[name]
	if !ok {
		return fmt.Errorf("no session %s", name)
	}
	s.Lines = append(s.Lines, text)
	return nil
}

func (f *Fake) Enter(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Sessions[name]
	if !ok {
		return fmt.Errorf("no session %s", name)
	}
	s.Enters++
	return nil
}

// HasErr makes HasSession fail for a name (indeterminate answer).
func (f *Fake) SetHasErr(name string, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.HasErr == nil {
		f.HasErr = map[string]bool{}
	}
	f.HasErr[name] = on
}

func (f *Fake) PanePID(_ context.Context, name string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Sessions[name]
	if !ok {
		return 0, fmt.Errorf("no session %s", name)
	}
	return s.Pid, nil
}

func (f *Fake) Keys(_ context.Context, name string, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Sessions[name]
	if !ok {
		return fmt.Errorf("no session %s", name)
	}
	s.Keys = append(s.Keys, keys...)
	return nil
}

func (f *Fake) Paste(_ context.Context, name, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Sessions[name]
	if !ok {
		return fmt.Errorf("no session %s", name)
	}
	s.Pastes = append(s.Pastes, text)
	return nil
}

func (f *Fake) Kill(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.KillFails[name] {
		return fmt.Errorf("injected kill failure: %s", name)
	}
	delete(f.Sessions, name)
	return nil
}

// End removes a session as if its command exited.
func (f *Fake) End(name string) {
	f.mu.Lock()
	delete(f.Sessions, name)
	f.mu.Unlock()
}

func (f *Fake) Session(name string) *FakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Sessions[name]
}

func appendFile(path, s string) error {
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.WriteString(s)
	return err
}

// AppendLog lets tests simulate session output.
func AppendLog(path, s string) error { return appendFile(path, s) }
