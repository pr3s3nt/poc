package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

func zipFiles(t *testing.T, files map[string]string, methods ...uint16) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, body := range files {
		method := uint16(zip.Deflate)
		if len(methods) > 0 {
			method = methods[0]
		}
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestZIPModuleInspectReuseRestartAndTamper(t *testing.T) {
	ctx := context.Background()
	f := newFixtureWithBinary(t, nil, "stand-in Terraform binary")
	f.svc.cfg.Git = nil
	f.svc.cfg.Policy = Policy{}
	files := map[string]string{"repo/modules/db/main.tf": rootModule}
	req := moduleregistry.ZIPInspection{OrganizationKey: "acme", ResourceType: "redis", Path: "repo/modules/db", Data: zipFiles(t, files)}
	b, err := f.svc.InspectZIP(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if b.Contract.SourceOrganizationKey != "acme" || b.Contract.SourceKind != tfmodule.SourceKindZIP || b.Contract.SourceURL != "" || len(b.Ref.Revision) != 64 || !strings.HasPrefix(b.Ref.SourceID, "zip-") {
		t.Fatalf("pin %+v", b)
	}
	// Repackaging preserves content identity, independent of compression/order.
	req.Data = zipFiles(t, files, zip.Store)
	again, err := f.svc.InspectZIP(ctx, req)
	if err != nil || again.ID != b.ID {
		t.Fatalf("repeat %v", err)
	}
	restarted, err := New(Config{Release: f.release, Cache: f.cache, Store: f.store, Packaged: terraform.PackagedModules(), TerraformBinary: f.svc.cfg.TerraformBinary})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := restarted.Verify(ctx, b.Ref); err != nil || got.ID != b.ID {
		t.Fatalf("restart %v", err)
	}
	artifact, err := restarted.VerifyForExecution(ctx, b.Ref)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(artifact.ModuleDir, "main.tf"))
	if err != nil || string(content) != rootModule {
		t.Fatalf("execution content differs: %v", err)
	}
	if err := artifact.Cleanup(); err != nil {
		t.Fatal(err)
	}
	bad := b.Ref
	bad.ContentSHA256 = strings.Repeat("0", 64)
	if _, err := restarted.Verify(ctx, bad); !errors.Is(err, moduleregistry.ErrDrift) {
		t.Fatalf("tampered: %v", err)
	}
	files["repo/modules/db/main.tf"] += "\n# new version\n"
	req.Data = zipFiles(t, files)
	newer, err := restarted.InspectZIP(ctx, req)
	if err != nil || newer.ID == b.ID || newer.Ref.Revision == b.Ref.Revision {
		t.Fatalf("update %v", err)
	}
	if got, err := restarted.Verify(ctx, b.Ref); err != nil || got.ID != b.ID {
		t.Fatalf("old pin %v", err)
	}
	req.Path = "modules/db"
	if _, err := restarted.InspectZIP(ctx, req); err == nil {
		t.Fatal("implicitly flattened archive")
	}
}
func TestZIPModuleResolvesLocalChildrenAndRetainsContractGuards(t *testing.T) {
	f := newFixture(t, nil)
	f.svc.cfg.Git = nil
	files := map[string]string{"modules/root/main.tf": rootModule + `
module "nested" { source = "../child" }`, "modules/child/main.tf": rootModule}
	req := moduleregistry.ZIPInspection{OrganizationKey: "acme", ResourceType: "redis", Path: "modules/root", Data: zipFiles(t, files)}
	b, err := f.svc.InspectZIP(context.Background(), req)
	if err != nil || len(b.Contract.Transitive) != 1 {
		t.Fatalf("local sibling: %v", err)
	}
	if _, err := f.svc.Verify(context.Background(), b.Ref); err != nil {
		t.Fatal(err)
	}
	files["modules/child/main.tf"] += `
resource "null_resource" "bad" { provisioner "local-exec" { command = "false" } }`
	req.Data = zipFiles(t, files)
	if _, err := f.svc.InspectZIP(context.Background(), req); err == nil {
		t.Fatal("ZIP bypassed Terraform guard")
	}
}
