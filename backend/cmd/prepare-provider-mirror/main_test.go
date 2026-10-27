package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/terraform/registry"
)

func TestDownloadVerifiesReleaseBeforePublishing(t *testing.T) {
	archive := []byte("fixture provider archive")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/terraform-provider-kubernetes/2.38.0/terraform-provider-kubernetes_2.38.0_linux_amd64.zip" {
			t.Errorf("unexpected release URL: %s", r.URL.Path)
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	p := registry.ReleaseProvider{Source: "registry.terraform.io/hashicorp/kubernetes", Version: "2.38.0", Hashes: []string{fmt.Sprintf("zh:%x", sha256.Sum256(archive))}}
	dir := t.TempDir()
	if err := download(context.Background(), server.Client(), server.URL, dir, "linux_amd64", p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, p.Source, "terraform-provider-kubernetes_2.38.0_linux_amd64.zip")
	if got, err := os.ReadFile(path); err != nil || string(got) != string(archive) {
		t.Fatalf("verified mirror archive: %q %v", got, err)
	}
	// A corrupt update must neither replace good bytes nor leave temp files.
	p.Hashes = []string{"zh:wrong"}
	if err := download(context.Background(), server.Client(), server.URL, dir, "linux_amd64", p); err == nil {
		t.Fatal("accepted an archive outside release hashes")
	}
	got, _ := os.ReadFile(path)
	files, _ := os.ReadDir(filepath.Dir(path))
	if string(got) != string(archive) || len(files) != 1 {
		t.Fatal("failed verification changed the published mirror")
	}
	p.Source = "registry.terraform.io/hashicorp/../escape"
	if err := download(context.Background(), server.Client(), server.URL, dir, "linux_amd64", p); err == nil {
		t.Fatal("accepted an unsafe source")
	}
}
