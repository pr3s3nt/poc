package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"orchestrator/internal/domain/tfmodule"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// RetainProviderMirror copies already-installed release archives to a durable
// union mirror. Every archive is checked against the trusted release zh hashes.
// Existing versions are retained; missing/drifted archives never trigger a fetch.
func RetainProviderMirror(release Release, source, destination string) error {
	if err := release.validate(); err != nil {
		return err
	}
	for _, p := range release.Providers {
		parts := strings.Split(p.Source, "/")
		if len(parts) != 3 || !tfmodule.ValidIdentifier(parts[1]) || !tfmodule.ValidIdentifier(parts[2]) || strings.Contains(p.Version, "/") {
			return errors.New("registry: invalid provider archive identity")
		}
		file := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", parts[2], p.Version, runtime.GOOS, runtime.GOARCH)
		original := filepath.Join(source, filepath.FromSlash(p.Source), file)
		targetDir := filepath.Join(destination, filepath.FromSlash(p.Source))
		if err := os.MkdirAll(targetDir, 0700); err != nil {
			return err
		}
		target := filepath.Join(targetDir, file)
		verify := func(name string) error {
			info, err := os.Lstat(name)
			if err != nil || !info.Mode().IsRegular() {
				return errors.New("registry: retained provider archive unavailable")
			}
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			defer f.Close()
			hash := sha256.New()
			if _, err = io.Copy(hash, f); err != nil {
				return err
			}
			if !slices.Contains(p.Hashes, "zh:"+hex.EncodeToString(hash.Sum(nil))) {
				return errors.New("registry: provider archive checksum mismatch")
			}
			return nil
		}
		if err := verify(original); err != nil {
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			if err := verify(target); err != nil {
				return err
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		input, err := os.Open(original)
		if err != nil {
			return err
		}
		stage, err := os.CreateTemp(targetDir, ".prepare-")
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(stage, input)
		input.Close()
		closeErr := stage.Close()
		if copyErr != nil || closeErr != nil {
			os.Remove(stage.Name())
			return errors.New("registry: retain provider archive failed")
		}
		if err := verify(stage.Name()); err != nil {
			os.Remove(stage.Name())
			return err
		}
		err = os.Link(stage.Name(), target)
		os.Remove(stage.Name())
		if err != nil && !os.IsExist(err) {
			return err
		}
		if err := verify(target); err != nil {
			return err
		}
	}
	return nil
}
