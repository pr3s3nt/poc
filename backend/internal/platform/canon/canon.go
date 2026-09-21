// Package canon produces canonical JSON encodings and deterministic fingerprints.
package canon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Bytes encodes v as canonical JSON. Go sorts map keys, so the encoding is stable.
func Bytes(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canon: marshal: %w", err)
	}
	return b, nil
}

// Hash returns the hex SHA-256 fingerprint of the canonical encoding of v.
func Hash(v any) (string, error) {
	b, err := Bytes(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Map converts any JSON-serialisable value into a generic document tree.
func Map(v any) (map[string]any, error) {
	b, err := Bytes(v)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("canon: unmarshal: %w", err)
	}
	return out, nil
}

// Clone deep-copies a JSON document tree.
func Clone(v any) (any, error) {
	b, err := Bytes(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("canon: unmarshal: %w", err)
	}
	return out, nil
}
