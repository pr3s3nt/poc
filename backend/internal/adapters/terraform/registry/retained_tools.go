package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"orchestrator/internal/domain/tfmodule"
)

// retainToolchains records only the trusted current release and a hash-verified
// binary. Existing manifests are immutable. The private cache is preserved across
// platform recreation; execution still checks the complete original bundle pin.
func retainToolchains(cfg *Config) error {
	if cfg.TerraformBinary == "" {
		return nil
	}
	root := filepath.Join(cfg.Cache.dir, "toolchains")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	dir := filepath.Join(root, cfg.Release.Terraform.BinarySHA256)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	target := filepath.Join(dir, "terraform")
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		temp, err := os.MkdirTemp(root, ".preparing-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		staged := filepath.Join(temp, "terraform")
		if err := copyVerifiedBinary(cfg.TerraformBinary, staged, cfg.Release.Terraform.BinarySHA256); err != nil {
			return err
		}
		if err := os.Link(staged, target); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	if !regularDigest(target, cfg.Release.Terraform.BinarySHA256) {
		return errors.New("registry: retained Terraform binary is unavailable or corrupted")
	}
	raw, err := json.Marshal(cfg.Release)
	if err != nil {
		return err
	}
	// A binary may serve several provider releases; name each manifest by full digest.
	sum := toolDigest(raw)
	name := filepath.Join(dir, sum+".json")
	if old, err := os.ReadFile(name); err == nil {
		if !bytes.Equal(old, raw) {
			return errors.New("registry: retained release manifest drift")
		}
	} else if os.IsNotExist(err) {
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(raw)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else {
		return err
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range dirs {
		if !entry.IsDir() || !tfmodule.ValidDigest(entry.Name()) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			return err
		}
		for _, file := range files {
			if filepath.Ext(file.Name()) != ".json" {
				continue
			}
			info, err := file.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				return errors.New("registry: retained release metadata is invalid")
			}
			data, err := os.ReadFile(filepath.Join(root, entry.Name(), file.Name()))
			if err != nil {
				return err
			}
			var release Release
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&release) != nil || decoder.Decode(&struct{}{}) != io.EOF || release.validate() != nil || release.Terraform.BinarySHA256 != entry.Name() || toolDigest(data)+".json" != file.Name() {
				return errors.New("registry: retained release metadata drift")
			}
			binary := filepath.Join(root, entry.Name(), "terraform")
			if !regularDigest(binary, entry.Name()) {
				return errors.New("registry: retained Terraform binary drift")
			}
			cfg.Retained = append(cfg.Retained, RetainedRelease{Release: release, TerraformBinary: binary})
		}
	}
	return nil
}
func regularDigest(name, want string) bool {
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	raw, err := os.ReadFile(name)
	return err == nil && toolDigest(raw) == want
}

func toolDigest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
