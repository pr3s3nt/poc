// Package statecipher implements the Terraform state cipher with AES-256-GCM.
// The key is read from platform configuration outside PostgreSQL; every Seal
// draws a fresh random nonce and authenticates the caller's AAD.
package statecipher

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"

	"orchestrator/internal/ports/terraformstate"
)

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// AESGCM is a single-key AES-256-GCM state cipher.
type AESGCM struct {
	keyID string
	aead  cipher.AEAD
}

// New builds a cipher from a 32-byte key. The key bytes are not retained
// beyond the AES key schedule.
func New(keyID string, key []byte) (*AESGCM, error) {
	if !keyIDPattern.MatchString(keyID) {
		return nil, errors.New("statecipher: key id must be 1-64 safe characters")
	}
	if len(key) != KeySize {
		return nil, errors.New("statecipher: key must be 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("statecipher: cannot initialize AES")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("statecipher: cannot initialize GCM")
	}
	return &AESGCM{keyID: keyID, aead: aead}, nil
}

// LoadKeyFile reads a key file holding 32 bytes as 64 hex characters or as
// standard base64. The file must be readable by its owner only. Errors never
// echo file contents.
func LoadKeyFile(keyID, path string) (*AESGCM, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.New("statecipher: state key file is not readable")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("statecipher: state key file must not be accessible by group or others")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("statecipher: state key file is not readable")
	}
	text := bytes.TrimSpace(raw)
	var key []byte
	if len(text) == 2*KeySize {
		key = make([]byte, KeySize)
		if _, err = hex.Decode(key, text); err != nil {
			key = nil
		}
	}
	if key == nil {
		decoded, decErr := base64.StdEncoding.DecodeString(string(text))
		if decErr != nil || len(decoded) != KeySize {
			return nil, fmt.Errorf("statecipher: state key file must hold %d bytes as hex or base64", KeySize)
		}
		key = decoded
	}
	c, err := New(keyID, key)
	clear(key)
	clear(raw)
	return c, err
}

// KeyID names the key used by Seal.
func (c *AESGCM) KeyID() string { return c.keyID }

// Seal encrypts plaintext with a new random nonce.
func (c *AESGCM) Seal(plaintext, aad []byte) ([]byte, []byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, errors.New("statecipher: cannot draw a nonce")
	}
	return nonce, c.aead.Seal(nil, nonce, plaintext, aad), nil
}

// Open decrypts; any mismatch of key ID, key, nonce, bytes or AAD fails closed.
func (c *AESGCM) Open(keyID string, nonce, ciphertext, aad []byte) ([]byte, error) {
	if keyID != c.keyID || len(nonce) != c.aead.NonceSize() {
		return nil, terraformstate.ErrUndecryptable
	}
	out, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, terraformstate.ErrUndecryptable
	}
	return out, nil
}

var _ terraformstate.Cipher = (*AESGCM)(nil)
