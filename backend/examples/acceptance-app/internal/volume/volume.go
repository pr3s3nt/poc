// Package volume stores public acceptance test data in one fixed file.
package volume

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const MaxValue = 4096

var ErrValue = errors.New("invalid test value")
var ErrUnavailable = errors.New("storage unavailable")

type Snapshot struct {
	Configured bool   `json:"configured"`
	Present    bool   `json:"present"`
	Value      string `json:"value,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Process    string `json:"process"`
}

type Store struct {
	dir     string
	process string
	mu      sync.Mutex
}

func New(dir, process string) *Store { return &Store{dir: dir, process: process} }

func (s *Store) Read() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}
func (s *Store) read() (Snapshot, error) {
	out := Snapshot{Configured: s.dir != "", Process: s.process}
	if !out.Configured {
		return out, nil
	}
	if !filepath.IsAbs(s.dir) {
		return out, ErrUnavailable
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return out, ErrUnavailable
	}
	defer root.Close()
	info, err := root.Lstat("acceptance-value")
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return out, ErrUnavailable
	}
	f, err := root.Open("acceptance-value")
	if err != nil {
		return out, ErrUnavailable
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return out, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxValue+1))
	if err != nil || !valid(string(b)) {
		return out, ErrUnavailable
	}
	h := sha256.Sum256(b)
	out.Present = true
	out.Value = string(b)
	out.SHA256 = hex.EncodeToString(h[:])
	return out, nil
}
func valid(value string) bool {
	return len(value) <= MaxValue && utf8.ValidString(value) && strings.TrimSpace(value) != ""
}

func (s *Store) Write(value string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(value) {
		return Snapshot{}, ErrValue
	}
	if s.dir == "" || !filepath.IsAbs(s.dir) {
		return Snapshot{}, ErrUnavailable
	}
	// CreateTemp uses an unpredictable name; rename never follows the target.
	f, err := os.CreateTemp(s.dir, ".acceptance-")
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.WriteString(value)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return Snapshot{}, ErrUnavailable
	}
	if os.Rename(name, filepath.Join(s.dir, "acceptance-value")) != nil {
		return Snapshot{}, ErrUnavailable
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	err = dir.Sync()
	_ = dir.Close()
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	return s.read()
}
