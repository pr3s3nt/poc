package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/tfmodule"
)

func TestReadPinnedUsesOnlyExactVerifiedCacheContent(t *testing.T) {
	cache := &Cache{dir: t.TempDir()}
	tree := Tree{"main.tf": []byte("resource \"aws_vpc\" \"main\" {}\n")}
	ref := tfmodule.SourceRef{SourceID: "packaged", ModuleID: "vpc", Revision: strings.Repeat("a", 40), Subdirectory: ".", ContentSHA256: tree.Digest()}
	root := sourceDir(cache.bundleDir(ref), rootDir(ref))
	if err := tree.write(root); err != nil {
		t.Fatal(err)
	}
	svc := &Service{cfg: Config{Cache: cache}, prepared: map[string]tfmodule.Bundle{ref.Pin(): {Ref: ref, Contract: tfmodule.Contract{Transitive: []tfmodule.TransitiveModule{}}}}}
	files, err := svc.ReadPinned(context.Background(), ref)
	if err != nil || len(files) != 1 {
		t.Fatalf("verified source unavailable: %v %#v", err, files)
	}
	if string(files["main.tf"]) != string(tree["main.tf"]) {
		t.Fatal("wrong pinned content")
	}
	if err := os.WriteFile(filepath.Join(root, "main.tf"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadPinned(context.Background(), ref); err == nil {
		t.Fatal("drifted source served")
	}
	other := ref
	other.ContentSHA256 = strings.Repeat("0", 64)
	if _, err := svc.ReadPinned(context.Background(), other); err == nil {
		t.Fatal("different full pin accepted")
	}
}
