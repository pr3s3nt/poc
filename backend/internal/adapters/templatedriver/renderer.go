// Package templatedriver renders immutable manifest templates without a platform-specific generator.
package templatedriver

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/renderengine"
)

const Version = "1.0.0"

//go:embed renderer.go git.go guard.go pod_volumes.go runtime_policy.go
var driverSource embed.FS

// Renderer executes prepared content only. There is no native or executable fallback.
type Renderer struct {
	git      BundleStore
	executor renderengine.Executor
}

func New() *Renderer { return &Renderer{} }

// rendererLibraries are the modules the renderer links for data handling
// (ADR-041). Their versions, with the Go toolchain, replace the digest of the
// whole executable so that unrelated rebuilds keep template pins valid.
var rendererLibraries = []string{"gopkg.in/yaml.v3", "github.com/tidwall/sjson", "github.com/tidwall/gjson", "github.com/tidwall/match", "github.com/tidwall/pretty"}

var runtimePin struct {
	once   sync.Once
	digest string
	err    error
}

// runtimeIdentity fingerprints what the renderer depends on outside its own
// embedded sources: the Go toolchain and the listed library versions.
func runtimeIdentity() (string, error) {
	runtimePin.once.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			runtimePin.err = fmt.Errorf("template: build information is unavailable")
			return
		}
		runtimePin.digest = identityFrom(runtime.Version(), info.Deps)
	})
	return runtimePin.digest, runtimePin.err
}

// identityFrom digests the Go toolchain and the renderer library versions; any
// other dependency of the backend does not affect it.
func identityFrom(goVersion string, deps []*debug.Module) string {
	versions := map[string]string{}
	for _, dep := range deps {
		module := dep
		if dep.Replace != nil {
			module = dep.Replace
		}
		versions[dep.Path] = module.Path + "@" + module.Version + "#" + module.Sum
	}
	h := sha256.New()
	h.Write([]byte("go:" + goVersion))
	for _, path := range rendererLibraries {
		h.Write([]byte("\n" + path + "=" + versions[path]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BinaryDigest is the stable renderer identity of ADR-041 (the persisted field
// keeps its historical name).
func (r *Renderer) BinaryDigest() string {
	identity, err := runtimeIdentity()
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(identity))
	h.Write([]byte(templatesource.EngineContract + ":" + Version + ":" + renderengine.Fingerprint()))
	for _, name := range []string{"renderer.go", "git.go", "guard.go", "pod_volumes.go", "runtime_policy.go"} {
		source, _ := driverSource.ReadFile(name)
		h.Write([]byte(name))
		h.Write(source)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (r *Renderer) CheckBinary() error {
	if _, err := runtimeIdentity(); err != nil {
		return fmt.Errorf("template: cannot fingerprint the running engine")
	}
	return nil
}
func (r *Renderer) ValidateSelection(ctx context.Context, selected resource.RenderingSelection) error {
	_, err := r.validateGit(ctx, selected)
	return err
}
func (r *Renderer) Render(ctx context.Context, req execution.RenderRequest) ([]execution.Manifest, error) {
	return r.renderGit(ctx, req)
}
