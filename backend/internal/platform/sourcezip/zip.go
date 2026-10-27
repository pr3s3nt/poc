// Package sourcezip reads bounded source archives without extracting to disk.
package sourcezip

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 32 << 20
const MaxFileBytes = 1 << 20
const MaxEntries = 2000

var ErrInvalid = errors.New("sourcezip: invalid or unsafe ZIP archive")

// Read validates the entire archive, including files outside the selected
// directory. No archive names or content are included in errors.
func Read(ctx context.Context, data []byte) (map[string][]byte, error) {
	if len(data) == 0 || len(data) > MaxBytes {
		return nil, ErrInvalid
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) == 0 || len(z.File) > MaxEntries {
		return nil, ErrInvalid
	}
	out := map[string][]byte{}
	names := map[string]bool{}
	files := map[string]bool{}
	total := 0
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mode := f.Mode()
		dir := mode.IsDir()
		name := f.Name
		if dir {
			name = strings.TrimSuffix(name, "/")
		}
		if !safePath(name) || names[name] || f.Flags&1 != 0 || (!dir && (!mode.IsRegular() || mode.Perm()&0111 != 0)) || (dir && mode.Type() != fs.ModeDir) {
			return nil, ErrInvalid
		}
		names[name] = true
		if dir {
			continue
		}
		files[name] = true
		if f.UncompressedSize64 > MaxFileBytes {
			return nil, ErrInvalid
		}
		rc, err := f.Open()
		if err != nil {
			return nil, ErrInvalid
		}
		content, readErr := io.ReadAll(io.LimitReader(rc, MaxFileBytes+1))
		closeErr := rc.Close()
		total += len(content)
		if readErr != nil || closeErr != nil || len(content) > MaxFileBytes || total > MaxBytes {
			return nil, ErrInvalid
		}
		out[name] = content
	}
	if len(out) == 0 {
		return nil, ErrInvalid
	}
	for name := range names {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if files[parent] {
				return nil, ErrInvalid
			}
		}
	}
	return out, nil
}

func safePath(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." || segment == "" || segment == ".git" || segment == ".terraform" {
			return false
		}
		for _, r := range segment {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	base := strings.ToLower(path.Base(name))
	return !strings.Contains(base, ".tfstate") && base != "terraform.tfvars" && !strings.HasSuffix(base, ".auto.tfvars") && !strings.HasSuffix(base, ".tfvars.json")
}
