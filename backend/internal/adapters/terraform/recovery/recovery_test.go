package recovery

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/statecipher"
	"orchestrator/internal/platform/ids"
)

func store(t *testing.T) (*Store, string) {
	t.Helper()
	cipher, err := statecipher.New("key-1", bytes.Repeat([]byte{3}, statecipher.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "recovery")
	s, err := New(dir, cipher)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func TestSaveEncryptsPrivatelyAndNeverOverwrites(t *testing.T) {
	s, dir := store(t)
	instance, attempt := ids.New(), ids.New()
	secret := []byte(`{"version":4,"serial":9,"lineage":"l","resources":[{"password":"PLAINTEXT-MARKER"}]}`)
	handle, err := s.Save(context.Background(), instance, attempt, secret)
	if err != nil || handle != "recovery-"+attempt {
		t.Fatalf("save: %q %v", handle, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, handle))
	if err != nil || strings.Contains(string(raw), "PLAINTEXT-MARKER") {
		t.Fatalf("artifact is not encrypted: %v", err)
	}
	if info, _ := os.Stat(filepath.Join(dir, handle)); info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact mode %v", info.Mode())
	}
	if opened, err := s.Open(instance, attempt); err != nil || !bytes.Equal(opened, secret) {
		t.Fatalf("open: %v", err)
	}
	// Bound to its instance and attempt: another identity cannot open it.
	if _, err := s.Open(ids.New(), attempt); err == nil {
		t.Fatal("opened under another instance")
	}
	// The same attempt is acknowledged, never replaced by other bytes.
	if _, err := s.Save(context.Background(), instance, attempt, []byte(`{"other":1}`)); err != nil {
		t.Fatal(err)
	}
	if opened, _ := s.Open(instance, attempt); !bytes.Equal(opened, secret) {
		t.Fatal("an existing recovery artifact was overwritten")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".tmp-*")); len(leftovers) != 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}
}

func TestDirectoryMustBePrivateAbsoluteAndOutsideGit(t *testing.T) {
	cipher, _ := statecipher.New("key-1", bytes.Repeat([]byte{3}, statecipher.KeySize))
	if _, err := New("relative", cipher); err == nil {
		t.Fatal("relative directory accepted")
	}
	open := t.TempDir()
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(open, cipher); err == nil {
		t.Fatal("a world-readable directory was accepted")
	}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(repo, "rec"), cipher); err == nil {
		t.Fatal("a directory inside a Git work tree was accepted")
	}
	if _, err := New(filepath.Join(t.TempDir(), "r"), nil); err == nil {
		t.Fatal("no cipher accepted")
	}
}

func TestVerificationArtifactIsEncryptedBoundAndNeverOverwritten(t *testing.T) {
	s, dir := store(t)
	instance := ids.New()
	handle := "verify-" + ids.New()
	original := []byte(`{"version":4,"serial":3,"lineage":"l","resources":[{"password":"PLAINTEXT-MARKER"}]}`)
	if err := s.SaveVerification(context.Background(), handle, instance, original); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, handle))
	if err != nil || strings.Contains(string(raw), "PLAINTEXT-MARKER") {
		t.Fatalf("artifact not encrypted: %v", err)
	}
	if info, _ := os.Stat(filepath.Join(dir, handle)); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	// A second save (an empty terminal state, say) never replaces the original.
	if err := s.SaveVerification(context.Background(), handle, instance, []byte(`{"version":4,"serial":9,"lineage":"l","resources":[]}`)); err != nil {
		t.Fatal(err)
	}
	got, found, err := s.LoadVerification(context.Background(), handle, instance)
	if err != nil || !found || !bytes.Equal(got, original) {
		t.Fatalf("load: %v %v", found, err)
	}
	// Bound to instance and handle; tampering and absence are distinguished.
	if _, _, err := s.LoadVerification(context.Background(), handle, ids.New()); err == nil {
		t.Fatal("opened under another instance")
	}
	if _, found, err := s.LoadVerification(context.Background(), "verify-"+ids.New(), instance); found || err != nil {
		t.Fatalf("absent artifact: %v %v", found, err)
	}
	if err := os.WriteFile(filepath.Join(dir, handle), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.LoadVerification(context.Background(), handle, instance); found || err == nil {
		t.Fatalf("a corrupted artifact must be an error, not absence: %v %v", found, err)
	}
	if err := s.SaveVerification(context.Background(), "bad-handle", instance, original); err == nil {
		t.Fatal("unsafe handle accepted")
	}
}
