package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRetainedProviderMirrorKeepsVersionsAndRejectsDrift(t *testing.T) {
	release, err := DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	release.Providers = release.Providers[:1]
	source, destination := t.TempDir(), t.TempDir()
	install := func(version, content string) string {
		t.Helper()
		release.Providers[0].Version = version
		sum := sha256.Sum256([]byte(content))
		release.Providers[0].Hashes = []string{"zh:" + hex.EncodeToString(sum[:])}
		dir := filepath.Join(source, filepath.FromSlash(release.Providers[0].Source))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", filepath.Base(dir), version, runtime.GOOS, runtime.GOARCH)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := RetainProviderMirror(release, source, destination); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(destination, filepath.FromSlash(release.Providers[0].Source), name)
	}
	old := install("1.0.0", "old verified archive")
	current := install("2.0.0", "new verified archive")
	if b, err := os.ReadFile(old); err != nil || string(b) != "old verified archive" {
		t.Fatal("old archive lost", err)
	}
	if err := RetainProviderMirror(release, source, destination); err != nil {
		t.Fatal("repeat retain", err)
	}
	if err := os.WriteFile(current, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RetainProviderMirror(release, source, destination); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	release.Providers[0].Source = "registry.terraform.io/../../escape"
	if err := RetainProviderMirror(release, source, destination); err == nil {
		t.Fatal("unsafe provider path accepted")
	}
}
