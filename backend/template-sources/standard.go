// Package standardtemplates contains the platform's initial, versioned manifest templates.
package standardtemplates

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"orchestrator/internal/adapters/templatedriver"
	"sort"
	"strings"
)

//go:embed standard
var content embed.FS

func Files() map[string]string {
	files := map[string]string{}
	_ = fs.WalkDir(content, "standard", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Only runtime files define the source; examples and documentation are independent.
		if strings.Contains(path, "/examples/") || strings.HasSuffix(path, "README.md") {
			return nil
		}
		raw, err := content.ReadFile(path)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(path, "standard/")] = string(raw)
		return nil
	})
	return files
}
func ID(org string) string {
	h := sha256.New()
	h.Write([]byte(org + "\x00" + templatedriver.New().BinaryDigest()))
	files := Files()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h.Write([]byte(name + "\x00" + files[name] + "\x00"))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return sum[:8] + "-" + sum[8:12] + "-4" + sum[13:16] + "-8" + sum[17:20] + "-" + sum[20:32]
}
