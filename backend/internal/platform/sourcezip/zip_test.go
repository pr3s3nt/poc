package sourcezip

import (
	"archive/zip"
	"bytes"
	"context"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

type entry struct {
	name, body string
	mode       fs.FileMode
}

func archive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestReadPreservesRepositoryLayout(t *testing.T) {
	data := archive(t, entry{name: "repo/", mode: fs.ModeDir | 0755}, entry{name: "repo/modules/db/main.tf", body: "module"}, entry{name: "repo/modules/child/main.tf", body: "child"})
	files, err := Read(context.Background(), data)
	if err != nil || len(files) != 2 || string(files["repo/modules/child/main.tf"]) != "child" {
		t.Fatalf("layout: %v %v", files, err)
	}
}
func TestRejectUnsafeOrAmbiguousArchive(t *testing.T) {
	for _, name := range []string{"../escape.tf", "/tmp/file", "C:/file", "a\\file", "a/../file", "a//file", "./file", "a/./file", "a\x00file", ".git/config", "a/.terraform/file", "terraform.tfstate", "state.tfstate.backup", "terraform.tfvars"} {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(context.Background(), archive(t, entry{name: name, body: "secret"})); err == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
	for name, entries := range map[string][]entry{
		"duplicate":          {{name: "a", body: "x"}, {name: "a", body: "y"}},
		"file-directory":     {{name: "a", body: "x"}, {name: "a/b", body: "y"}},
		"explicit-directory": {{name: "a/", mode: fs.ModeDir | 0755}, {name: "a", body: "x"}},
		"symlink":            {{name: "a", body: "/tmp/file", mode: fs.ModeSymlink | 0777}},
		"executable":         {{name: "a", body: "code", mode: 0755}},
		"special":            {{name: "a", mode: fs.ModeNamedPipe | 0600}},
		"zip-bomb":           {{name: "a", body: strings.Repeat("x", MaxFileBytes+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(context.Background(), archive(t, entries...)); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
	if _, err := Read(context.Background(), []byte("not a ZIP")); err == nil {
		t.Fatal("accepted malformed archive")
	}
	data := archive(t, entry{name: "main.tf", body: "content"})
	data[len(data)/3] ^= 255
	if _, err := Read(context.Background(), data); err == nil {
		t.Fatal("accepted corrupt archive")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, archive(t, entry{name: "main.tf"})); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestArchiveEntryLimit(t *testing.T) {
	entries := make([]entry, MaxEntries+1)
	for i := range entries {
		entries[i] = entry{name: "file-" + strconv.Itoa(i)}
	}
	if _, err := Read(context.Background(), archive(t, entries...)); err == nil {
		t.Fatal("accepted too many entries")
	}
}
