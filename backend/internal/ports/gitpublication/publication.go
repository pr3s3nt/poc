// Package gitpublication separates Git side effects from individual CD controllers.
package gitpublication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

type Intent struct {
	ID string
	// DestinationKey identifies the leased repository/branch, without credentials.
	DestinationKey string
	Branch         string
	Path           string
	// Files contains only validated non-secret, generated manifests/provider config.
	Files      map[string][]byte
	TreeDigest string
	Delete     bool
}

// TreeVerifier verifies the exact unit tree of a controller-reported commit;
// later commits to unrelated paths do not invalidate an admitted revision.
type TreeVerifier interface {
	VerifyTree(context.Context, string, string, string, string) (bool, error)
}
type Receipt struct{ IntentID, Commit, TreeDigest string }

// Guard runs before every remote write. It verifies the durable publication
// lease and Environment owner/fence; expiry cannot grant unknown-outcome takeover.
type Guard func(context.Context) error
type Publisher interface {
	Publish(context.Context, Intent, Guard) (Receipt, error)
	ObservePublication(context.Context, Intent) (Receipt, bool, error)
}

// Digest includes lengths/paths/content, independent of map ordering.
func Digest(files map[string][]byte) string {
	h := sha256.New()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(sum[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
