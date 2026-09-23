// Package password provides the local/test password hashing primitive.
package password

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

const rounds = 100000

func Hash(raw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt) + ":" + digest(salt, raw), nil
}

func Verify(encoded, raw string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got := digestBytes(salt, raw)
	return subtle.ConstantTimeCompare(want, got) == 1
}

func digest(salt []byte, raw string) string { return hex.EncodeToString(digestBytes(salt, raw)) }
func digestBytes(salt []byte, raw string) []byte {
	sum := append(append([]byte(nil), salt...), []byte(raw)...)
	for range rounds {
		h := sha256.Sum256(sum)
		sum = h[:]
	}
	return sum
}
