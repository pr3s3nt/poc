package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// limitedBuffer keeps at most max bytes and drops the rest, so a noisy or
// hostile process cannot exhaust memory. It is safe for a copy goroutine that
// is still writing while the owner takes a snapshot.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
	cut bool
}

// snapshot copies the bytes written so far.
func (b *limitedBuffer) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() == 0 {
		return nil, b.cut
	}
	return append([]byte(nil), b.buf.Bytes()...), b.cut
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := b.max - b.buf.Len()
	if room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
			b.cut = true
		} else {
			b.buf.Write(p)
		}
	} else if len(p) > 0 {
		b.cut = true
	}
	return len(p), nil
}

// procResult is the raw result of one bounded process run.
type procResult struct {
	stdout, stderr []byte
	truncated      bool
	exitCode       int
	// signalled is true when this runner sent a stop signal (deadline or
	// cancellation), so the process did not end by itself.
	signalled bool
	timedOut  bool
	cancelled bool
	// exitedBySignal is true when the process was ended by a signal.
	exitedBySignal bool
	// stopped is true only when the leader exited, no process of the group
	// remains and no process outside it still holds the output pipes.
	stopped bool
	// pipesHeld means a process outside the group kept the output pipes open
	// after the leader exited; it may still run.
	pipesHeld bool
	// waitTimedOut means the leader could not be reaped within the bound.
	waitTimedOut bool
}

// runProcess runs cmd in its own process group, bounded by ctx. On
// cancellation or deadline it interrupts the group, waits grace, kills it and
// then waits up to confirm for the group to disappear.
func runProcess(ctx context.Context, cmd *exec.Cmd, maxOut int, grace, confirm time.Duration) (procResult, error) {
	stdout := &limitedBuffer{max: maxOut}
	stderr := &limitedBuffer{max: 1 << 20}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A process outside the group (it escaped with setsid) may keep the output
	// pipes open after the leader exited: Wait must not block on it.
	cmd.WaitDelay = confirm
	if err := cmd.Start(); err != nil {
		return procResult{stopped: true}, err
	}
	pgid := cmd.Process.Pid
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	res := procResult{}
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-ctx.Done():
		res.signalled = true
		res.timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		res.cancelled = !res.timedOut
		_ = syscall.Kill(-pgid, syscall.SIGINT)
		select {
		case waitErr = <-waited:
		case <-time.After(grace):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			select {
			case waitErr = <-waited:
			case <-time.After(confirm):
				// The leader cannot be reaped (uninterruptible, or pipes held
				// open past the bound). Report promptly; never block here.
				res.waitTimedOut = true
			}
		}
	}
	res.pipesHeld = errors.Is(waitErr, exec.ErrWaitDelay)
	// The leader is gone; children (provider plugins) may remain.
	deadline := time.Now().Add(confirm)
	for {
		if !groupAlive(pgid) {
			res.stopped = !res.waitTimedOut && !res.pipesHeld
			break
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Snapshots: copy goroutines may still be writing when the wait timed out.
	res.stdout, res.truncated = stdout.snapshot()
	res.stderr, _ = stderr.snapshot()
	if res.waitTimedOut {
		return res, errors.New("the process could not be reaped")
	}
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		res.exitCode = exit.ExitCode()
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			res.exitedBySignal = true
		}
		return res, waitErr
	}
	return res, waitErr
}

// groupAlive reports whether any non-zombie process still belongs to the
// process group. Without /proc it falls back to a signal-0 probe.
func groupAlive(pgid int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return syscall.Kill(-pgid, 0) == nil
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid pgrp ...; comm may contain spaces/parens.
		text := string(data)
		end := strings.LastIndexByte(text, ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(text[end+1:])
		if len(fields) < 3 {
			continue
		}
		pg, err := strconv.Atoi(fields[2])
		if err != nil || pg != pgid {
			continue
		}
		if fields[0] != "Z" {
			return true
		}
	}
	return false
}
