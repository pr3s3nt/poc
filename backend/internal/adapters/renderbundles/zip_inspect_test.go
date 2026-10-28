package renderbundles

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/renderregistry"
)

func zipBundle(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, body := range files {
		f, err := w.Create("repo/bundles/standard/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestZIPBundlePinScopeRestartUpdateAndRepair(t *testing.T) {
	ctx := context.Background()
	r, st := newRegistry(t, nil)
	tree := standardTree(t)
	req := renderregistry.ZIPInspection{Path: "repo/bundles/standard", Data: zipBundle(t, tree)}
	a, err := r.InspectZIP(ctx, "acme", req)
	if err != nil {
		t.Fatal(err)
	}
	if a.Source.Kind != "zip" || a.Source.URL != "zip:sha256:"+a.Source.Revision {
		t.Fatalf("pin %+v", a.Source)
	}
	again, err := r.InspectZIP(ctx, "acme", req)
	if err != nil || again.ID != a.ID {
		t.Fatalf("repeat %v", err)
	}
	other, err := r.InspectZIP(ctx, "other", req)
	if err != nil || other.ID == a.ID {
		t.Fatalf("scope %v", err)
	}
	if _, err := st.GetRenderBundle(ctx, "other", a.ID); err == nil {
		t.Fatal("foreign pin readable")
	}
	restarted := &Registry{Cache: r.Cache, Store: st, Runtime: r.Runtime}
	if _, err := restarted.Load(ctx, resource.GitRenderPin(a)); err != nil {
		t.Fatal(err)
	}
	tree["README.md"] = []byte("new version")
	req.Data = zipBundle(t, tree)
	b, err := restarted.InspectZIP(ctx, "acme", req)
	if err != nil || b.ID == a.ID {
		t.Fatalf("new version %v", err)
	}
	if _, err := restarted.Load(ctx, resource.GitRenderPin(a)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(r.Cache.dir, "bundles", a.Source.ContentSHA256)); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Available(ctx, a); err == nil {
		t.Fatal("missing cache accepted")
	}
	req.Data = zipBundle(t, standardTree(t))
	repaired, err := restarted.InspectZIP(ctx, "acme", req)
	if err != nil || repaired.ID != a.ID {
		t.Fatalf("repair %v", err)
	}
	if err := restarted.Available(ctx, a); err != nil {
		t.Fatal(err)
	}
	req.Path = "."
	if _, err := restarted.InspectZIP(ctx, "acme", req); err == nil {
		t.Fatal("implicitly flattened archive")
	}
}
