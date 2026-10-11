// Package runner is the single seam for executing external commands.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Result struct {
	Stdout string
	Stderr string
	Exit   int
}

type Runner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error)
}

// OS executes real processes. A non-zero exit is returned in Result.Exit with a nil error.
type OS struct{}

func (OS) Run(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	// A cancelled or timed-out command takes its whole process group with it,
	// and Wait never blocks forever on inherited pipes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 3 * time.Second
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	if cerr := ctx.Err(); cerr != nil {
		res.Exit = -1
		return res, fmt.Errorf("%s: %w", name, cerr) // timeout/cancel is never a plain exit status
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.Exit = ee.ExitCode()
		return res, nil
	}
	if err != nil {
		res.Exit = -1
		return res, err
	}
	return res, nil
}

// Call records one fake invocation.
type Call struct {
	Dir  string
	Name string
	Args []string
}

func (c Call) String() string { return c.Name + " " + strings.Join(c.Args, " ") }

// Handler answers a faked command. Return handled=false to fall through.
type Handler func(c Call) (res Result, err error, handled bool)

// Router sends commands with a registered handler to it and the rest to Next.
type Router struct {
	Next     Runner
	mu       sync.Mutex
	handlers map[string]Handler
	calls    []Call
}

func NewRouter(next Runner) *Router {
	return &Router{Next: next, handlers: map[string]Handler{}}
}

func (r *Router) Handle(name string, h Handler) {
	r.mu.Lock()
	r.handlers[name] = h
	r.mu.Unlock()
}

func (r *Router) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Call(nil), r.calls...)
}

func (r *Router) CallsTo(name string) []Call {
	var out []Call
	for _, c := range r.Calls() {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func (r *Router) Run(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error) {
	c := Call{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	r.mu.Lock()
	h := r.handlers[name]
	r.mu.Unlock()
	if h != nil {
		type out struct {
			res Result
			err error
			ok  bool
		}
		ch := make(chan out, 1)
		go func() {
			res, err, ok := h(c)
			ch <- out{res, err, ok}
		}()
		select {
		case o := <-ch:
			if o.ok {
				r.mu.Lock()
				r.calls = append(r.calls, c)
				r.mu.Unlock()
				return o.res, o.err
			}
		case <-ctx.Done():
			r.mu.Lock()
			r.calls = append(r.calls, c)
			r.mu.Unlock()
			return Result{Exit: -1}, fmt.Errorf("%s: %w", name, ctx.Err())
		}
	}
	if r.Next == nil {
		return Result{Exit: 127}, fmt.Errorf("no handler for %s", name)
	}
	return r.Next.Run(ctx, dir, env, name, args...)
}

// Timeout bounds every command: ByName wins over Default. Zero means no limit.
type Timeout struct {
	R       Runner
	Default time.Duration
	ByName  map[string]time.Duration
}

func (t Timeout) Run(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error) {
	d := t.Default
	if v, ok := t.ByName[name]; ok {
		d = v
	}
	if d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	return t.R.Run(ctx, dir, env, name, args...)
}
