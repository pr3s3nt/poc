package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ReadCredentialFile reads one connection credential (for example a
// PostgreSQL URL with a password) from an owner-only regular file. A file
// readable or writable by group or others, an empty file or a missing file is
// refused. Errors name only the purpose, never the path contents.
func ReadCredentialFile(purpose, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("bootstrap: no %s file configured", purpose)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("bootstrap: %s file is not accessible", purpose)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("bootstrap: %s file is not a regular file", purpose)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("bootstrap: %s file must be owner-only (mode 0600 or 0400)", purpose)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("bootstrap: %s file cannot be read", purpose)
	}
	text := strings.TrimSpace(string(value))
	if text == "" {
		return "", fmt.Errorf("bootstrap: %s file is empty", purpose)
	}
	return text, nil
}

// DistinctFiles fails when two configured paths name the same file, so a
// state key can never double as a database credential (or the reverse).
func DistinctFiles(paths map[string]string) error {
	type entry struct {
		purpose string
		info    os.FileInfo
	}
	var seen []entry
	for purpose, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("bootstrap: %s file is not accessible", purpose)
		}
		for _, other := range seen {
			if os.SameFile(info, other.info) {
				return errors.New("bootstrap: the " + purpose + " and " + other.purpose + " must be separate files")
			}
		}
		seen = append(seen, entry{purpose, info})
	}
	return nil
}
