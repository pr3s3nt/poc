// Package proc tracks the processes of one supervised session without reading
// any process environment. At launch the supervisor records the tmux pane's
// leader PID and its kernel start time (/proc/<pid>/stat field 22). Every
// descendant that stays in that session (ordinary children, background jobs)
// has session id == leader PID, which is readable from /proc/<n>/stat alone.
//
// Linux never reuses a PID while it is still the session id of a live
// process, so survivors with SID == recorded PID are ours; if none exist,
// nothing of ours remains. Limitation (accepted: trusted agents, no hostile
// code): a process that calls setsid() leaves the session and is not tracked.
// Any unreadable or unparseable entry makes the answer "unknown", never "gone".
package proc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Owner struct {
	PID   int    `json:"pid"`
	Start uint64 `json:"start"` // clock ticks since boot at leader start
}

func (o Owner) Valid() bool { return o.PID > 1 && o.Start > 0 }

type Scanner interface {
	// Identify reads the start time of a live process.
	Identify(pid int) (Owner, error)
	// Members lists live processes of the owner's session. Any read problem
	// other than "process exited" is an error (unknown).
	Members(o Owner) (pids []int, err error)
	// Terminate signals only processes re-verified as members, TERM then KILL.
	Terminate(o Owner, pids []int) error
}

type statInfo struct {
	state           byte
	ppid, pgrp, sid int
	start           uint64
}

// parseStat parses /proc/<pid>/stat; comm may contain spaces and parentheses.
func parseStat(b []byte) (statInfo, error) {
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || i+2 >= len(s) {
		return statInfo{}, fmt.Errorf("malformed stat")
	}
	f := strings.Fields(s[i+2:])
	if len(f) < 20 {
		return statInfo{}, fmt.Errorf("malformed stat: %d fields", len(f))
	}
	var si statInfo
	si.state = f[0][0]
	var err error
	if si.ppid, err = strconv.Atoi(f[1]); err != nil {
		return si, err
	}
	if si.pgrp, err = strconv.Atoi(f[2]); err != nil {
		return si, err
	}
	if si.sid, err = strconv.Atoi(f[3]); err != nil {
		return si, err
	}
	if si.start, err = strconv.ParseUint(f[19], 10, 64); err != nil {
		return si, err
	}
	return si, nil
}

// Real reads only /proc/<pid>/stat.
type Real struct {
	Root string // default /proc
}

func (r Real) root() string {
	if r.Root == "" {
		return "/proc"
	}
	return r.Root
}

func (r Real) stat(pid int) (statInfo, error) {
	b, err := os.ReadFile(filepath.Join(r.root(), strconv.Itoa(pid), "stat"))
	if err != nil {
		return statInfo{}, err
	}
	return parseStat(b)
}

func (r Real) Identify(pid int) (Owner, error) {
	si, err := r.stat(pid)
	if err != nil {
		return Owner{}, err
	}
	return Owner{PID: pid, Start: si.start}, nil
}

func (r Real) Members(o Owner) ([]int, error) {
	if !o.Valid() {
		return nil, fmt.Errorf("no ownership record")
	}
	if err := r.checkLeader(o); err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(r.root())
	if err != nil {
		return nil, fmt.Errorf("cannot list processes: %w", err)
	}
	var out []int
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		si, err := r.stat(pid)
		if err != nil {
			if os.IsNotExist(err) {
				continue // exited between listing and reading
			}
			return nil, fmt.Errorf("cannot read stat of %d: %w", pid, err)
		}
		if si.state == 'Z' {
			continue // zombie: already dead
		}
		if si.sid == o.PID {
			out = append(out, pid)
		}
	}
	return out, nil
}

// checkLeader fails closed when the recorded pid is alive with a different
// start time: the pid was reused, so session membership no longer means ours.
func (r Real) checkLeader(o Owner) error {
	si, err := r.stat(o.PID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // leader gone; Linux keeps the pid reserved while members exist
		}
		return fmt.Errorf("cannot read leader %d: %w", o.PID, err)
	}
	if si.start != o.Start {
		return fmt.Errorf("pid %d was reused (start %d, recorded %d): refusing to treat its session as ours", o.PID, si.start, o.Start)
	}
	return nil
}

func (r Real) Terminate(o Owner, pids []int) error {
	if err := r.checkLeader(o); err != nil {
		return err
	}
	signal := func(sig syscall.Signal) {
		for _, p := range pids {
			si, err := r.stat(p)
			if err != nil || si.sid != o.PID || si.start < o.Start {
				continue // re-verify membership right before signalling
			}
			_ = syscall.Kill(p, sig)
		}
	}
	signal(syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		left, err := r.Members(o)
		if err == nil && len(left) == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	signal(syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
	return nil
}

// Fake is a controllable scanner for tests.
type Fake struct {
	mu         sync.Mutex
	Next       int
	Alive      map[int][]int // owner pid -> member pids
	Unkillable bool
	Err        error // makes Members unknown
}

func NewFake() *Fake { return &Fake{Next: 4000, Alive: map[int][]int{}} }

func (f *Fake) Identify(pid int) (Owner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Owner{PID: pid, Start: uint64(pid)*10 + 1}, nil
}

func (f *Fake) Members(o Owner) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]int(nil), f.Alive[o.PID]...), nil
}

func (f *Fake) Terminate(o Owner, pids []int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Unkillable {
		return nil
	}
	delete(f.Alive, o.PID)
	return nil
}

// Spawn simulates survivors of an owner.
func (f *Fake) Spawn(ownerPID int, pids ...int) {
	f.mu.Lock()
	f.Alive[ownerPID] = append(f.Alive[ownerPID], pids...)
	f.mu.Unlock()
}
