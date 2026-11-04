// Package templatedriver renders immutable manifest templates without a platform-specific generator.
package templatedriver

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/renderengine"
)

const Version = "1.0.0"

//go:embed renderer.go git.go guard.go
var driverSource embed.FS

// Renderer executes prepared content only. There is no native or executable fallback.
type Renderer struct {
	git      BundleStore
	executor renderengine.Executor
}

func New() *Renderer { return &Renderer{} }

var executablePin struct {
	once   sync.Once
	digest string
	err    error
}

// Pin the executable too: delivery preparation, domain methods and linked helpers
// can affect output even when the template worker source itself did not change.
func executableDigest() (string, error) {
	executablePin.once.Do(func() {
		path, err := os.Executable()
		if err != nil {
			executablePin.err = err
			return
		}
		file, err := os.Open("/proc/self/exe")
		if err != nil {
			file, err = os.Open(path)
		}
		if err != nil {
			executablePin.err = err
			return
		}
		defer file.Close()
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err != nil {
			executablePin.err = err
			return
		}
		executablePin.digest = hex.EncodeToString(hash.Sum(nil))
	})
	return executablePin.digest, executablePin.err
}

func (r *Renderer) BinaryDigest() string {
	binary, err := executableDigest()
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(binary))
	h.Write([]byte(templatesource.EngineContract + ":" + Version + ":" + renderengine.Fingerprint()))
	for _, name := range []string{"renderer.go", "git.go", "guard.go"} {
		source, _ := driverSource.ReadFile(name)
		h.Write([]byte(name))
		h.Write(source)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (r *Renderer) CheckBinary() error {
	if _, err := executableDigest(); err != nil {
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
