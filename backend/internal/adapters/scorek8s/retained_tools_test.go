package scorek8s

import (
	"context"
	"orchestrator/internal/domain/resource"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedRendererUsesExactBinaryAfterRestart(t *testing.T) {
	root := t.TempDir()
	renderer := func(marker string) *Renderer {
		t.Helper()
		name := filepath.Join(t.TempDir(), "score-k8s")
		data := []byte("#!/bin/sh\nprintf 'score-k8s " + Version + " (test)\\n'\n# " + marker + "\n")
		if err := os.WriteFile(name, data, 0700); err != nil {
			t.Fatal(err)
		}
		r, err := New(context.Background(), name, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.RetainTools(root); err != nil {
			t.Fatal(err)
		}
		return r
	}
	old := renderer("old")
	current := renderer("new")
	if current.bundle.BinaryDigest == old.bundle.BinaryDigest {
		t.Fatal("identical fixture binaries")
	}
	pin := resource.RenderBundle{BinaryDigest: old.bundle.BinaryDigest, RendererVersion: Version}
	retained, err := current.forRetained(pin)
	if err != nil || retained.bundle != old.bundle {
		t.Fatal("old binary unavailable after upgrade", err)
	}
	if err := current.CheckPinnedBinary(old.bundle.BinaryDigest, Version); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retained.path, []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := current.CheckPinnedBinary(old.bundle.BinaryDigest, Version); err == nil {
		t.Fatal("corrupt historical binary accepted")
	}
	if err := current.RetainTools(root); err == nil {
		t.Fatal("corrupt archive accepted on restart")
	}
}
