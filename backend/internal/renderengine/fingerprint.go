package renderengine

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
)

//go:embed funcs.go worker.go patch.go resources.go bundle.go
var engineSource embed.FS

// Fingerprint prevents a changed helper/worker implementation from silently rendering an old pin.
func Fingerprint() string {
	h := sha256.New()
	for _, name := range []string{"funcs.go", "worker.go", "patch.go", "resources.go", "bundle.go"} {
		source, _ := engineSource.ReadFile(name)
		h.Write([]byte(name))
		h.Write(source)
	}
	return hex.EncodeToString(h.Sum(nil))
}
