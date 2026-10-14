package v2run

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// writeJSONPrivate atomically writes v as JSON with mode 0600 (temp file in
// the same directory, fsync, rename).
func writeJSONPrivate(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFilePrivate(path, append(raw, '\n'))
}

// WriteFilePrivate atomically replaces path with data at mode 0600.
func WriteFilePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
