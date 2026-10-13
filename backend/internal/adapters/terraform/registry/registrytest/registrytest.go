// Package registrytest opens the real module registry for tests: the
// embedded release, no Git sources, a private temporary cache and platform
// preparation of the packaged modules into the given store. It fakes nothing.
package registrytest

import (
	"context"
	"os"
	"testing"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/planning"
)

// Open returns a prepared packaged-only registry backed by st.
func Open(t testing.TB, st registry.BundleStore) *registry.Service {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	svc, err := registry.Open(context.Background(), registry.Options{CacheDir: dir, Store: st})
	if err != nil {
		t.Fatalf("open module registry: %v", err)
	}
	return svc
}

// Inspector returns the product planning inspector over Open(t, st).
func Inspector(t testing.TB, st registry.BundleStore) planning.RegistryInspector {
	t.Helper()
	return planning.RegistryInspector{Metadata: Open(t, st)}
}
