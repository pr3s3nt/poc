package state

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSupervisorLockIsExclusiveAndReleasable(t *testing.T) {
	dir := t.TempDir()
	a, _ := Open(dir)
	b, _ := Open(dir)
	if err := a.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := b.Lock(); err != ErrLocked {
		t.Fatalf("second supervisor allowed: %v", err)
	}
	a.Unlock()
	if err := b.Lock(); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	b.Unlock()
}

func TestAtomicWriteLeavesNoTempAndIsPrivate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "f.json")
	for i := 0; i < 5; i++ {
		if err := WriteFileAtomic(p, []byte(`{"i":`+string(rune('0'+i))+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Fatalf("leftover files: %v", ents)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	if b, _ := os.ReadFile(p); string(b) != `{"i":4}` {
		t.Fatalf("%s", b)
	}
}

func TestTaskRoundTripNormalisesMaps(t *testing.T) {
	s, _ := Open(t.TempDir())
	tk := &Task{Issue: 3, RunID: "r", State: Running, CreatedAt: time.Now()}
	if err := s.SaveTask(tk); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadTask(3)
	if err != nil || got.Applied == nil || got.Attempts == nil || got.Intents == nil {
		t.Fatalf("%+v %v", got, err)
	}
	st, _ := os.Stat(s.Sub(3, "task.json"))
	if st.Mode().Perm() != 0o600 {
		t.Fatal("task file not private")
	}
}

func TestSaveHookCanFailAWrite(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.SaveHook = func(*Task) error { return os.ErrClosed }
	if err := s.SaveTask(&Task{Issue: 1}); err == nil {
		t.Fatal("hook ignored")
	}
	if _, err := os.Stat(s.Sub(1, "task.json")); err == nil {
		t.Fatal("a failed save left a file")
	}
}

func TestSpoolConcurrentEnqueueOrderedAndDedupable(t *testing.T) {
	s, _ := Open(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Enqueue(1, Command{Kind: "x", Role: "user"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	cmds, paths, err := s.Spooled(1)
	if err != nil || len(cmds) != 20 || len(paths) != 20 {
		t.Fatalf("%d %v", len(cmds), err)
	}
	ids := map[string]bool{}
	for _, c := range cmds {
		if ids[c.ID] || c.Status != "pending" {
			t.Fatal("duplicate id or status")
		}
		ids[c.ID] = true
	}
}

func TestStateMachineTransitions(t *testing.T) {
	ok := [][2]State{{Ready, Running}, {Running, Review}, {Review, Validating}, {Validating, Publishing}, {Publishing, Done}, {Running, WaitQuota}, {WaitQuota, Running}, {WaitUser, Review}, {Failed, Running}}
	for _, p := range ok {
		if !CanTransition(p[0], p[1]) {
			t.Errorf("%s -> %s should be allowed", p[0], p[1])
		}
	}
	bad := [][2]State{{Done, Running}, {Aborted, Running}, {Ready, Done}, {Running, Done}, {Publishing, Validating}, {Failed, Done}}
	for _, p := range bad {
		if CanTransition(p[0], p[1]) {
			t.Errorf("%s -> %s must be illegal", p[0], p[1])
		}
	}
	tk := &Task{State: Done}
	if err := tk.SetState(Running, time.Now()); err == nil {
		t.Fatal("terminal state left")
	}
}
