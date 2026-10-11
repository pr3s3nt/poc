package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Store keeps all durable state under Dir with private permissions. The
// supervisor is the only writer of task.json; other processes only add spool
// files, so there is no lock contention between supervisor and control CLI.
type Store struct {
	Dir  string
	lock *os.File
	// SaveHook, when set, runs before every task write and may fail it
	// (fault injection for durability tests).
	SaveHook func(*Task) error
}

func Open(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "tasks")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{Dir: dir}, nil
}

var ErrLocked = errors.New("another supervisor holds the lock")

// Lock takes the exclusive supervisor lock (flock, released by the kernel on exit).
func (s *Store) Lock() error {
	f, err := os.OpenFile(filepath.Join(s.Dir, "supervisor.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return ErrLocked
	}
	_ = f.Truncate(0)
	fmt.Fprintf(f, "%d\n", os.Getpid())
	s.lock = f
	return nil
}

func (s *Store) Unlock() {
	if s.lock != nil {
		_ = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		s.lock.Close()
		s.lock = nil
	}
}

func (s *Store) TaskDir(issue int) string { return filepath.Join(s.Dir, "tasks", fmt.Sprint(issue)) }

func (s *Store) Sub(issue int, parts ...string) string {
	return filepath.Join(append([]string{s.TaskDir(issue)}, parts...)...)
}

// WriteFileAtomic writes via temp file, fsync, rename and directory fsync.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func (s *Store) SaveTask(t *Task) error {
	t.Schema = 1
	if s.SaveHook != nil {
		if err := s.SaveHook(t); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.Sub(t.Issue, "task.json"), b, 0o600)
}

func (s *Store) LoadTask(issue int) (*Task, error) {
	b, err := os.ReadFile(s.Sub(issue, "task.json"))
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("task %d: %w", issue, err)
	}
	if t.Applied == nil {
		t.Applied = map[string]bool{}
	}
	if t.Attempts == nil {
		t.Attempts = map[string]int{}
	}
	if t.Intents == nil {
		t.Intents = map[string]*Intent{}
	}
	return &t, nil
}

// ListTasks returns all tasks ordered by issue number.
func (s *Store) ListTasks() ([]*Task, error) {
	ents, err := os.ReadDir(filepath.Join(s.Dir, "tasks"))
	if err != nil {
		return nil, err
	}
	var out []*Task
	for _, e := range ents {
		var n int
		if _, err := fmt.Sscan(e.Name(), &n); err != nil {
			continue
		}
		t, err := s.LoadTask(n)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Issue < out[j].Issue })
	return out, nil
}

func (s *Store) LoadQueue() (Queue, error) {
	var q Queue
	b, err := os.ReadFile(filepath.Join(s.Dir, "queue.json"))
	if os.IsNotExist(err) {
		return q, nil
	}
	if err != nil {
		return q, err
	}
	return q, json.Unmarshal(b, &q)
}

func (s *Store) SaveQueue(q Queue) error {
	b, _ := json.MarshalIndent(q, "", "  ")
	return WriteFileAtomic(filepath.Join(s.Dir, "queue.json"), b, 0o600)
}

// AppendEvent adds one JSON line to the append-only audit log.
func (s *Store) AppendEvent(issue int, at time.Time, kind, msg string) {
	_ = os.MkdirAll(s.TaskDir(issue), 0o700)
	f, err := os.OpenFile(s.Sub(issue, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(map[string]string{"at": at.Format(time.RFC3339), "kind": kind, "msg": msg})
	f.Write(append(b, '\n'))
	_ = f.Sync()
}

func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Enqueue adds a command to the spool; safe from any process.
func (s *Store) Enqueue(issue int, c Command) (string, error) {
	if c.ID == "" {
		c.ID = NewID()
	}
	c.Status = "pending"
	b, _ := json.Marshal(c)
	name := fmt.Sprintf("%020d-%s.json", time.Now().UnixNano(), c.ID)
	return c.ID, WriteFileAtomic(s.Sub(issue, "spool", name), b, 0o600)
}

// Spooled lists spool commands in order with their file paths (not deleted).
func (s *Store) Spooled(issue int) ([]Command, []string, error) {
	dir := s.Sub(issue, "spool")
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var cmds []Command
	var paths []string
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".tmp-") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var c Command
		if json.Unmarshal(b, &c) != nil {
			_ = os.Rename(p, p+".bad")
			continue
		}
		cmds = append(cmds, c)
		paths = append(paths, p)
	}
	return cmds, paths, nil
}

// Cursor files let the (read-only) inbox consumer remember delivered events.
func (s *Store) ReadCursor(issue int, role string) int {
	b, err := os.ReadFile(s.Sub(issue, "cursor-"+role))
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscan(string(b), &n)
	return n
}

func (s *Store) WriteCursor(issue int, role string, n int) error {
	return WriteFileAtomic(s.Sub(issue, "cursor-"+role), []byte(fmt.Sprint(n)), 0o600)
}
