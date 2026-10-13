package statecipher

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformstate"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, KeySize) }

func TestSealUsesFreshNonceAndAuthenticatesScope(t *testing.T) {
	c, err := New("k1", key(7))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"secret":"hunter2"}`)
	aad := tfresource.StateAAD("org-a", strings.Repeat("a", 64), "k1", 1)
	n1, c1, err := c.Seal(plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	n2, c2, _ := c.Seal(plain, aad)
	if bytes.Equal(n1, n2) || bytes.Equal(c1, c2) {
		t.Fatal("two seals reused a nonce or produced identical ciphertext")
	}
	if bytes.Contains(c1, []byte("hunter2")) {
		t.Fatal("ciphertext contains plaintext")
	}
	if got, err := c.Open("k1", n1, c1, aad); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip: %v", err)
	}
	other, _ := New("k1", key(8))
	tampered := append([]byte(nil), c1...)
	tampered[0] ^= 1
	for name, open := range map[string]func() ([]byte, error){
		"wrong key":    func() ([]byte, error) { return other.Open("k1", n1, c1, aad) },
		"wrong key id": func() ([]byte, error) { return c.Open("k2", n1, c1, aad) },
		"other org": func() ([]byte, error) {
			return c.Open("k1", n1, c1, tfresource.StateAAD("org-b", strings.Repeat("a", 64), "k1", 1))
		},
		"other state": func() ([]byte, error) {
			return c.Open("k1", n1, c1, tfresource.StateAAD("org-a", strings.Repeat("b", 64), "k1", 1))
		},
		"other revision": func() ([]byte, error) {
			return c.Open("k1", n1, c1, tfresource.StateAAD("org-a", strings.Repeat("a", 64), "k1", 2))
		},
		"tampered":    func() ([]byte, error) { return c.Open("k1", n1, tampered, aad) },
		"short nonce": func() ([]byte, error) { return c.Open("k1", n1[:4], c1, aad) },
	} {
		if got, err := open(); !errors.Is(err, terraformstate.ErrUndecryptable) || got != nil {
			t.Errorf("%s: opened (%v)", name, err)
		}
	}
}

func TestLoadKeyFileRequiresPrivateWellFormedKey(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, err := LoadKeyFile("k1", write("hex", hex.EncodeToString(key(1))+"\n", 0o600)); err != nil {
		t.Fatalf("hex key: %v", err)
	}
	if _, err := LoadKeyFile("k1", write("b64", base64.StdEncoding.EncodeToString(key(2)), 0o400)); err != nil {
		t.Fatalf("base64 key: %v", err)
	}
	secret := hex.EncodeToString(key(3))
	for name, path := range map[string]string{
		"group readable": write("shared", secret, 0o640),
		"short":          write("short", hex.EncodeToString(key(1)[:16]), 0o600),
		"missing":        filepath.Join(dir, "absent"),
	} {
		_, err := LoadKeyFile("k1", path)
		if err == nil {
			t.Errorf("%s: accepted", name)
		} else if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), dir) {
			t.Errorf("%s: error leaks key or path", name)
		}
	}
	if _, err := New("bad id!", key(1)); err == nil {
		t.Error("unsafe key id accepted")
	}
}
