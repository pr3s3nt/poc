package renderbundles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/renderregistry"
)

type runtime struct{ digest string }

func (r runtime) BinaryDigest() string { return r.digest }
func (r runtime) CheckBinary() error   { return nil }

type snapshot struct {
	tree registry.Tree
	err  error
}

func (s snapshot) Tree(string) (registry.Tree, error) { return s.tree, s.err }
func (s snapshot) Close()                             {}

// fetcher serves commits by ref and records every network-shaped call.
type fetcher struct {
	refs    map[string]string
	trees   map[string]registry.Tree
	treeErr error
	calls   int
}

func (f *fetcher) Resolve(_ context.Context, _ registry.GitSource, ref string) (string, error) {
	f.calls++
	commit, ok := f.refs[ref]
	if !ok {
		return "", moduleregistry.ErrUnavailable
	}
	return commit, nil
}
func (f *fetcher) Open(_ context.Context, _ registry.GitSource, commit string) (registry.Snapshot, error) {
	f.calls++
	tree, ok := f.trees[commit]
	if !ok {
		return nil, moduleregistry.ErrUnavailable
	}
	return snapshot{tree: tree, err: f.treeErr}, nil
}

func standardTree(t *testing.T) registry.Tree {
	t.Helper()
	dir := "../../../render-bundles/standard"
	out := registry.Tree{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		content, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = content
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func newRegistry(t *testing.T, f *fetcher) (*Registry, *store.Store) {
	t.Helper()
	st := store.New()
	for _, org := range []string{"acme", "other"} {
		if err := st.SaveOrganization(context.Background(), application.Organization{ID: org + "-id", Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	cache, err := OpenCache(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return &Registry{Cache: cache, Git: f, Store: st, Runtime: runtime{digest: strings.Repeat("b", 64)}}, st
}

const commitA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const commitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const url = "https://github.com/pr3s3nt/poc.git"

func TestInspectPinsCachesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	treeA := standardTree(t)
	treeB := standardTree(t)
	treeB["defaults.yaml"] = []byte(strings.Replace(string(treeB["defaults.yaml"]), "cpu: 10m", "cpu: 30m", 1))
	f := &fetcher{refs: map[string]string{"main": commitA}, trees: map[string]registry.Tree{commitA: treeA, commitB: treeB}}
	r, st := newRegistry(t, f)
	a, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Path: "backend/render-bundles/standard/", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Source.Revision != commitA || a.Source.Path != "backend/render-bundles/standard" || a.Source.ContentSHA256 != Digest(treeA) || a.Name() != "standard-workload" {
		t.Fatalf("pin = %+v", a.Source)
	}
	again, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Path: "backend/render-bundles/standard", Ref: commitA})
	if err != nil || again.ID != a.ID {
		t.Fatalf("same pin must be idempotent: %v %v", again.ID, err)
	}
	// The branch moved: a new immutable pin, the old one untouched.
	f.refs["main"] = commitB
	b, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Path: "backend/render-bundles/standard", Ref: "main"})
	if err != nil || b.ID == a.ID || b.Source.ContentSHA256 == a.Source.ContentSHA256 {
		t.Fatalf("moved branch: %+v %v", b, err)
	}
	foreign, err := r.Inspect(ctx, "other", renderregistry.Inspection{URL: url, Path: "backend/render-bundles/standard", Ref: commitA})
	if err != nil || foreign.ID == a.ID {
		t.Fatalf("each Organization gets its own record: %v", err)
	}
	if _, err := st.GetRenderBundle(ctx, "other", a.ID); err == nil {
		t.Fatal("foreign record readable")
	}
	// Rendering reads only the cache: no fetch.
	calls := f.calls
	loaded, err := r.Load(ctx, resource.GitRenderPin(a))
	if err != nil || loaded.Contract.Defaults["resources"] == nil || f.calls != calls {
		t.Fatalf("load from cache: %v (fetch calls %d -> %d)", err, calls, f.calls)
	}
	if err := r.Available(ctx, a); err != nil {
		t.Fatal(err)
	}
	other := r.Runtime
	r.Runtime = runtime{digest: strings.Repeat("e", 64)}
	if err := r.Available(ctx, a); !errors.Is(err, renderregistry.ErrBundleUnavailable) {
		t.Fatalf("binary change must make the pin unavailable: %v", err)
	}
	r.Runtime = other
}

func TestMissingOrCorruptCacheFailsClosedAndSamePinRepairs(t *testing.T) {
	ctx := context.Background()
	f := &fetcher{refs: map[string]string{}, trees: map[string]registry.Tree{commitA: standardTree(t)}}
	r, _ := newRegistry(t, f)
	a, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Path: ".", Ref: commitA})
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(r.Cache.dir, "bundles", a.Source.ContentSHA256)
	if err := os.WriteFile(filepath.Join(entry, "defaults.yaml"), []byte("tampered: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Available(ctx, a); !errors.Is(err, renderregistry.ErrBundleUnavailable) {
		t.Fatalf("corrupt cache = %v", err)
	}
	if err := os.RemoveAll(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Load(ctx, resource.GitRenderPin(a)); !errors.Is(err, renderregistry.ErrBundleUnavailable) {
		t.Fatalf("missing cache = %v", err)
	}
	repaired, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Path: ".", Ref: a.Source.Revision})
	if err != nil || repaired.ID != a.ID || r.Available(ctx, a) != nil {
		t.Fatalf("explicit same-pin inspection must repair the cache: %v", err)
	}
	// Different content for the identical pin is drift, never an update.
	tree := standardTree(t)
	tree["README.extra"] = []byte("changed")
	f.trees[commitA] = tree
	if _, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Path: ".", Ref: commitA}); !errors.Is(err, renderregistry.ErrDrift) {
		t.Fatalf("drift = %v", err)
	}
}

func TestInspectRejectsUnsafeSourcesWithoutFetching(t *testing.T) {
	ctx := context.Background()
	f := &fetcher{refs: map[string]string{}, trees: map[string]registry.Tree{}}
	r, st := newRegistry(t, f)
	for name, req := range map[string]renderregistry.Inspection{
		"http":        {URL: "http://github.com/x/y.git", Ref: "main"},
		"credentials": {URL: "https://user:token@github.com/x/y.git", Ref: "main"},
		"ssh":         {URL: "git@github.com:x/y.git", Ref: "main"},
		"query":       {URL: "https://github.com/x/y.git?a=b", Ref: "main"},
		"traversal":   {URL: url, Path: "../etc", Ref: "main"},
		"absolute":    {URL: url, Path: "/etc", Ref: "main"},
		"encoded":     {URL: url, Path: "a/%2e%2e/b", Ref: "main"},
		"option ref":  {URL: url, Ref: "--upload-pack=x"},
		"refspec":     {URL: url, Ref: "main:refs/x"},
		"empty ref":   {URL: url, Ref: ""},
	} {
		if _, err := r.Inspect(ctx, "acme", req); !errors.Is(err, renderregistry.ErrSourceInvalid) {
			t.Fatalf("%s: want invalid source, got %v", name, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("an unsafe source reached the network")
	}
	if _, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Ref: "missing"}); !errors.Is(err, renderregistry.ErrSourceUnavailable) {
		t.Fatalf("unresolvable ref = %v", err)
	}
	bad := standardTree(t)
	bad["provisioners/bindings.provisioners.yaml"] = []byte("- uri: cmd://sh\n  type: x\n  outputs: x\n")
	f.trees[commitA] = bad
	var invalid *BundleInvalidError
	if _, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Ref: commitA}); !errors.As(err, &invalid) || invalid.File != "provisioners/bindings.provisioners.yaml" {
		t.Fatalf("cmd provisioner = %v", err)
	}
	f.treeErr = registry.ErrUnsafeContent
	if _, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Ref: commitA}); !errors.Is(err, renderregistry.ErrBundleInvalid) {
		t.Fatalf("symlink/submodule/executable tree = %v", err)
	}
	f.treeErr = nil
	unsupported := standardTree(t)
	unsupported["bundle.yaml"] = []byte(strings.Replace(string(unsupported["bundle.yaml"]), "0.15.0", "0.16.0", 1))
	f.trees[commitA] = unsupported
	if _, err := r.Inspect(ctx, "acme", renderregistry.Inspection{URL: url, Ref: commitA}); !errors.Is(err, renderregistry.ErrContractUnsupported) {
		t.Fatalf("unsupported renderer = %v", err)
	}
	if list, _ := st.ListRenderBundles(ctx, "acme"); len(list) != 0 {
		t.Fatalf("rejected inspections recorded metadata: %d", len(list))
	}
}
