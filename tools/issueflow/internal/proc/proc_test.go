package proc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stat builds a /proc/<pid>/stat line (comm may contain spaces and parens).
func stat(pid int, comm string, state byte, ppid, pgrp, sid int, start uint64) string {
	f := []string{string(state), fmt.Sprint(ppid), fmt.Sprint(pgrp), fmt.Sprint(sid)}
	for i := 4; i < 19; i++ {
		f = append(f, "0")
	}
	f = append(f, fmt.Sprint(start))
	f = append(f, "0", "0")
	return fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(f, " "))
}

func fakeProc(t *testing.T, entries map[int]string) Real {
	root := t.TempDir()
	for pid, s := range entries {
		d := filepath.Join(root, fmt.Sprint(pid))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "stat"), []byte(s), 0o644)
		// an environ file full of secrets that must never be opened
		os.WriteFile(filepath.Join(d, "environ"), []byte("SECRET=1\x00"), 0o000)
	}
	os.MkdirAll(filepath.Join(root, "self"), 0o755) // non-numeric entries are ignored
	return Real{Root: root}
}

func TestParseStatHandlesOddComm(t *testing.T) {
	si, err := parseStat([]byte(stat(10, "we ird) (name", 'S', 1, 10, 10, 777)))
	if err != nil || si.sid != 10 || si.start != 777 || si.state != 'S' {
		t.Fatalf("%+v %v", si, err)
	}
	if _, err := parseStat([]byte("garbage")); err == nil {
		t.Fatal("garbage parsed")
	}
}

func TestMembersAreExactlyTheRecordedSession(t *testing.T) {
	r := fakeProc(t, map[int]string{
		100: stat(100, "bash", 'S', 1, 100, 100, 500),    // leader
		101: stat(101, "sleep", 'S', 100, 100, 100, 510), // child
		102: stat(102, "bg", 'S', 1, 102, 100, 520),      // background job, same session
		103: stat(103, "zombie", 'Z', 100, 100, 100, 530),
		200: stat(200, "unrelated", 'S', 1, 200, 200, 400),
	})
	got, err := r.Members(Owner{PID: 100, Start: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 { // leader, child, bg; zombie and unrelated excluded
		t.Fatalf("members %v", got)
	}
	for _, p := range got {
		if p == 200 || p == 103 {
			t.Fatalf("wrong member %d", p)
		}
	}
}

func TestPIDReuseFailsClosed(t *testing.T) {
	// pid 100 was recorded with start 500, but now belongs to a new process
	// (start 900) that leads its own, unrelated session.
	r := fakeProc(t, map[int]string{
		100: stat(100, "other-user-app", 'S', 1, 100, 100, 900),
		101: stat(101, "its-child", 'S', 100, 100, 100, 910),
	})
	o := Owner{PID: 100, Start: 500}
	if _, err := r.Members(o); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("reused pid treated as ours: %v", err)
	}
	if err := r.Terminate(o, []int{100, 101}); err == nil {
		t.Fatal("Terminate must refuse a reused pid")
	}
}

func TestLeaderGoneNoMembersMeansNothingRemains(t *testing.T) {
	r := fakeProc(t, map[int]string{200: stat(200, "unrelated", 'S', 1, 200, 200, 400)})
	got, err := r.Members(Owner{PID: 100, Start: 500})
	if err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestUnreadableStatIsUnknownNotGone(t *testing.T) {
	r := fakeProc(t, map[int]string{
		100: stat(100, "bash", 'S', 1, 100, 100, 500),
		300: stat(300, "hidden", 'S', 1, 300, 300, 450),
	})
	os.Chmod(filepath.Join(r.Root, "300", "stat"), 0o000)
	if os.Getuid() == 0 {
		t.Skip("root ignores file modes")
	}
	if _, err := r.Members(Owner{PID: 100, Start: 500}); err == nil {
		t.Fatal("an unreadable process entry was skipped as if it had exited")
	}
	// unreadable leader as well
	r2 := fakeProc(t, map[int]string{100: stat(100, "bash", 'S', 1, 100, 100, 500)})
	os.Chmod(filepath.Join(r2.Root, "100", "stat"), 0o000)
	if _, err := r2.Members(Owner{PID: 100, Start: 500}); err == nil {
		t.Fatal("unreadable leader treated as gone")
	}
}

func TestNoOwnershipRecordIsAnError(t *testing.T) {
	r := Real{}
	if _, err := r.Members(Owner{}); err == nil {
		t.Fatal("empty owner accepted")
	}
}

func TestNeverOpensEnviron(t *testing.T) {
	// environ files are mode 000 in the fake tree; a read would fail with an
	// error and Members would not succeed.
	r := fakeProc(t, map[int]string{100: stat(100, "bash", 'S', 1, 100, 100, 500)})
	if _, err := r.Members(Owner{PID: 100, Start: 500}); err != nil {
		t.Fatalf("environ was touched: %v", err)
	}
}

// Real processes: a detached (same session) subprocess is found and ended; an
// unrelated process in another session survives.
func TestRealDetachedSubprocessTerminatedUnrelatedPreserved(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	// session leader with a background child that outlives the leader's shell
	leader := exec.Command("bash", "-c", "sleep 60 & echo $! ; wait")
	leader.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, _ := leader.StdoutPipe()
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	var childPID int
	fmt.Fscan(out, &childPID)
	// exactly one Wait per process, owned by one goroutine; cleanup only waits on its channel
	leaderDone := reap(leader)
	defer func() { leader.Process.Kill(); waitDone(t, leaderDone) }()

	unrelated := exec.Command("sleep", "60")
	unrelated.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	unrelatedDone := reap(unrelated)
	defer func() { unrelated.Process.Kill(); waitDone(t, unrelatedDone) }()

	r := Real{}
	o, err := r.Identify(leader.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	mem, err := r.Members(o)
	if err != nil {
		t.Fatal(err)
	}
	found := map[int]bool{}
	for _, p := range mem {
		found[p] = true
	}
	if !found[leader.Process.Pid] || !found[childPID] || found[unrelated.Process.Pid] {
		t.Fatalf("members %v (leader %d child %d unrelated %d)", mem, leader.Process.Pid, childPID, unrelated.Process.Pid)
	}
	if err := r.Terminate(o, mem); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		left, err := r.Members(o)
		if err == nil && len(left) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("survivors %v %v", left, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if syscall.Kill(unrelated.Process.Pid, 0) != nil {
		t.Fatal("unrelated process was killed")
	}
}

func TestFakeScanner(t *testing.T) {
	f := NewFake()
	o, _ := f.Identify(5)
	f.Spawn(5, 9)
	if m, _ := f.Members(o); len(m) != 1 {
		t.Fatal("spawn")
	}
	f.Terminate(o, []int{9})
	if m, _ := f.Members(o); len(m) != 0 {
		t.Fatal("terminate")
	}
}

// reap owns the single cmd.Wait of a started process and signals completion.
func reap(c *exec.Cmd) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()
	return done
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Error("process was not reaped within 10s")
	}
}
