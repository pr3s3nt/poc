package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
)

func TestJournalRecordsExactlyTheObjectsThisProcessWroteAndDeleted(t *testing.T) {
	ctx := context.Background()
	inner := credentialmemory.New()
	path := filepath.Join(t.TempDir(), "journal")
	j := &journaledCredentials{inner: inner, path: path}
	first, err := j.Put(ctx, "acme", "lab", []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := j.Put(ctx, "acme", "lab", []byte("two"))
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Delete(ctx, "acme", "lab", second); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "put " + first + "\nput " + second + "\ndel " + second + "\n"
	if string(raw) != want {
		t.Fatalf("journal:\n%s\nwant:\n%s", raw, want)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode %v", info.Mode().Perm())
	}
	if strings.Contains(string(raw), "one") || strings.Contains(string(raw), "two") {
		t.Fatal("journal holds credential values")
	}
}

func TestJournalFailureRemovesTheWriteAndReportsIt(t *testing.T) {
	ctx := context.Background()
	inner := credentialmemory.New()
	j := &journaledCredentials{inner: inner, path: filepath.Join(t.TempDir(), "missing-dir", "journal")}
	if ref, err := j.Put(ctx, "acme", "lab", []byte("one")); err == nil || ref != "" {
		t.Fatalf("put with an unwritable journal: %q %v", ref, err)
	}
	if inner.Len() != 0 {
		t.Fatal("an untracked object was left in the store")
	}
}
