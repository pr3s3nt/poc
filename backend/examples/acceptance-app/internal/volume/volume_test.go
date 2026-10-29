package volume

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistenceAcrossStoreRecreation(t *testing.T) {
	dir := t.TempDir()
	first := New(dir, "pod-a")
	if s, e := first.Read(); e != nil || s.Present || !s.Configured {
		t.Fatalf("empty: %+v %v", s, e)
	}
	written, e := first.Write("proof-<public-value>")
	if e != nil {
		t.Fatal(e)
	}
	second := New(dir, "pod-b")
	read, e := second.Read()
	if e != nil || read.Value != written.Value || read.SHA256 != written.SHA256 || read.Process != "pod-b" {
		t.Fatalf("reopen: %+v %v", read, e)
	}
	if _, e := second.Write("replacement"); e != nil {
		t.Fatal(e)
	}
	read, e = first.Read()
	if e != nil || read.Value != "replacement" {
		t.Fatalf("replace: %+v %v", read, e)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 || files[0].Name() != "acceptance-value" {
		t.Fatal("temporary file left behind")
	}
}
func TestInvalidStorageDoesNotInventSuccess(t *testing.T) {
	for _, v := range []string{"", "  ", strings.Repeat("x", MaxValue+1), string([]byte{0xff})} {
		if _, e := New(t.TempDir(), "pod").Write(v); e != ErrValue {
			t.Fatalf("expected invalid value, got %v", e)
		}
	}
	if s, e := New("", "pod").Read(); e != nil || s.Configured {
		t.Fatal("disabled storage")
	}
	if _, e := New(filepath.Join(t.TempDir(), "missing"), "pod").Write("test"); e != ErrUnavailable {
		t.Fatal("must not create an unmounted directory")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if e := os.WriteFile(outside, []byte("outside"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(dir, "acceptance-value")); e != nil {
		t.Fatal(e)
	}
	if _, e := New(dir, "pod").Read(); e != ErrUnavailable {
		t.Fatal("symlink escape must fail")
	}
}
