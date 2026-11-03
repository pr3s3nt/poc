package scorek8s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

type retainedRenderer struct {
	path   string
	bundle resource.RenderBundle
}

// RetainTools preserves each verified renderer binary in the durable bundle
// cache. Stored Git bundle contents remain independent and digest-verified.
func (r *Renderer) RetainTools(dir string) error {
	root := filepath.Join(dir, "toolchains")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	target := filepath.Join(root, r.bundle.BinaryDigest)
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	if err := storeToolFile(filepath.Join(target, "score-k8s"), data, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(r.bundle)
	if err != nil {
		return err
	}
	if err := storeToolFile(filepath.Join(target, "bundle.json"), raw, 0600); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	r.retained = map[string]retainedRenderer{}
	for _, e := range entries {
		if !e.IsDir() || !tfmodule.ValidDigest(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, e.Name(), "bundle.json"))
		if err != nil || len(raw) > 64<<10 {
			return fmt.Errorf("score-k8s: retained engine metadata unavailable")
		}
		var b resource.RenderBundle
		if json.Unmarshal(raw, &b) != nil || b.BinaryDigest != e.Name() || b.Version == "" {
			return fmt.Errorf("score-k8s: retained engine metadata drift")
		}
		name := filepath.Join(root, e.Name(), "score-k8s")
		info, err := os.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("score-k8s: retained binary unavailable")
		}
		digest, err := binaryDigest(name)
		if err != nil || digest != e.Name() {
			return fmt.Errorf("score-k8s: retained binary drift")
		}
		r.retained[e.Name()] = retainedRenderer{path: name, bundle: b}
	}
	return nil
}
func storeToolFile(name string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if os.IsExist(err) {
		info, e := os.Lstat(name)
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("score-k8s: retained file is not regular")
		}
		old, e := os.ReadFile(name)
		if e != nil || !bytes.Equal(old, data) {
			return fmt.Errorf("score-k8s: retained file drift")
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (r *Renderer) CheckPinnedBinary(digest, version string) error {
	if digest == r.bundle.BinaryDigest && version == r.bundle.Version {
		return r.CheckBinary()
	}
	old, ok := r.retained[digest]
	if !ok || old.bundle.Version != version {
		return ErrBundleUnavailable
	}
	got, err := binaryDigest(old.path)
	if err != nil || got != digest {
		return ErrBundleUnavailable
	}
	return nil
}
func (r *Renderer) forRetained(pin resource.RenderBundle) (*Renderer, error) {
	if err := r.CheckPinnedBinary(pin.BinaryDigest, pin.RendererVersion); err != nil {
		return nil, err
	}
	old, ok := r.retained[pin.BinaryDigest]
	if !ok {
		return nil, ErrBundleUnavailable
	}
	clone := *r
	clone.path = old.path
	clone.bundle = old.bundle
	clone.retained = nil
	return &clone, nil
}
