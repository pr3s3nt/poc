package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestAcceptanceChecks(t *testing.T) {
	hash := sha256.Sum256([]byte("do-not-print-me"))
	got := acceptanceChecks("acceptance-config-ok", "do-not-print-me", hex.EncodeToString(hash[:]), nil)
	for _, name := range []string{"environment", "secret", "database"} {
		if !got[name] {
			t.Errorf("%s should pass", name)
		}
	}
	got = acceptanceChecks("", "", "", errors.New("unavailable"))
	for _, name := range []string{"environment", "secret", "database"} {
		if got[name] {
			t.Errorf("%s should fail", name)
		}
	}
}
