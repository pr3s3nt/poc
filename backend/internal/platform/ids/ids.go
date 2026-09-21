// Package ids generates the stable identifiers used by orchestrator aggregates.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// New returns a random UUID v4 string.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("ids: entropy unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return strings.Join([]string{h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]}, "-")
}

// NewRunID returns a short, sortable identifier used to tag verification runs.
func NewRunID(prefix string, now time.Time) string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("ids: entropy unavailable: %v", err))
	}
	return fmt.Sprintf("%s-%s-%s", prefix, now.UTC().Format("20060102t150405"), hex.EncodeToString(b[:]))
}
