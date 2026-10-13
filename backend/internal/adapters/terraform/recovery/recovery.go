// Package recovery keeps the private encrypted recovery artifact of a
// Terraform state the backend could not store. The artifact is the only copy
// of that state: it is encrypted with the state key (AES-256-GCM, AAD binding
// instance and attempt), written owner-only into a private directory outside
// any Git work tree, never published, and never deleted by the executor.
// Manual recovery (D12) opens it with the same key.
package recovery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

// Store implements terraformexec.RecoveryStore.
type Store struct {
	dir    string
	cipher terraformstate.Cipher
}

var (
	_            terraformexec.RecoveryStore     = (*Store)(nil)
	_            terraformexec.VerificationStore = (*Store)(nil)
	verifyHandle                                 = regexp.MustCompile(`^verify-[0-9a-fA-F-]{36}$`)
	idShape                                      = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)
)

// New opens the recovery directory. It must be absolute, outside a Git work
// tree and private to the owner (0700); it is created when missing.
func New(dir string, cipher terraformstate.Cipher) (*Store, error) {
	if dir == "" || !filepath.IsAbs(dir) || cipher == nil {
		return nil, errors.New("recovery: an absolute directory and the state cipher are required")
	}
	dir = filepath.Clean(dir)
	for parent := dir; ; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return nil, errors.New("recovery: the directory must be outside a Git work tree")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.New("recovery: cannot create the directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("recovery: the directory must be a private (0700) directory")
	}
	return &Store{dir: dir, cipher: cipher}, nil
}

type envelope struct {
	KeyID      string `json:"keyId"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func aad(instanceID, attemptID string) []byte {
	return []byte("orchestrator-terraform-recovery|instance=" + instanceID + "|attempt=" + attemptID)
}

// Save encrypts state and writes it atomically. An existing artifact of the
// same attempt is never overwritten.
func (s *Store) Save(_ context.Context, instanceID, attemptID string, state []byte) (string, error) {
	if !idShape.MatchString(instanceID) || !idShape.MatchString(attemptID) || len(state) == 0 {
		return "", errors.New("recovery: invalid artifact identity")
	}
	nonce, ciphertext, err := s.cipher.Seal(state, aad(instanceID, attemptID))
	if err != nil {
		return "", errors.New("recovery: cannot encrypt the state")
	}
	data, err := json.Marshal(envelope{KeyID: s.cipher.KeyID(), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)})
	if err != nil {
		return "", errors.New("recovery: cannot encode the artifact")
	}
	handle := "recovery-" + attemptID
	tmp, err := os.CreateTemp(s.dir, ".tmp-")
	if err != nil {
		return "", errors.New("recovery: cannot create the artifact")
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", errors.New("recovery: cannot write the artifact")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", errors.New("recovery: cannot sync the artifact")
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", errors.New("recovery: cannot close the artifact")
	}
	final := filepath.Join(s.dir, handle)
	// Link, not rename: an existing artifact must never be replaced.
	if err := os.Link(name, final); err != nil {
		os.Remove(name)
		if os.IsExist(err) {
			return handle, nil
		}
		return "", errors.New("recovery: cannot publish the artifact")
	}
	if err := os.Remove(name); err != nil {
		return "", fmt.Errorf("recovery: cannot finish the artifact")
	}
	return handle, nil
}

// Open decrypts the artifact of a handle for an operator-driven recovery.
func (s *Store) Open(instanceID, attemptID string) ([]byte, error) {
	if !idShape.MatchString(instanceID) || !idShape.MatchString(attemptID) {
		return nil, errors.New("recovery: invalid artifact identity")
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "recovery-"+attemptID))
	if err != nil {
		return nil, errors.New("recovery: the artifact cannot be read")
	}
	var env envelope
	if json.Unmarshal(data, &env) != nil {
		return nil, errors.New("recovery: the artifact is malformed")
	}
	nonce, err1 := base64.StdEncoding.DecodeString(env.Nonce)
	ciphertext, err2 := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err1 != nil || err2 != nil {
		return nil, errors.New("recovery: the artifact is malformed")
	}
	return s.cipher.Open(env.KeyID, nonce, ciphertext, aad(instanceID, attemptID))
}

func verifyAAD(handle, instanceID string) []byte {
	return []byte("orchestrator-terraform-verification|instance=" + instanceID + "|handle=" + handle)
}

// SaveVerification implements terraformexec.VerificationStore.
func (s *Store) SaveVerification(_ context.Context, handle, instanceID string, state []byte) error {
	if !verifyHandle.MatchString(handle) || !idShape.MatchString(instanceID) {
		return errors.New("recovery: invalid verification identity")
	}
	nonce, ciphertext, err := s.cipher.Seal(state, verifyAAD(handle, instanceID))
	if err != nil {
		return errors.New("recovery: cannot encrypt the verification state")
	}
	data, err := json.Marshal(envelope{KeyID: s.cipher.KeyID(), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)})
	if err != nil {
		return errors.New("recovery: cannot encode the verification artifact")
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-")
	if err != nil {
		return errors.New("recovery: cannot create the verification artifact")
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return errors.New("recovery: cannot write the verification artifact")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return errors.New("recovery: cannot sync the verification artifact")
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return errors.New("recovery: cannot close the verification artifact")
	}
	// Link, never rename: the original pre-destroy state is never replaced.
	err = os.Link(name, filepath.Join(s.dir, handle))
	os.Remove(name)
	if err != nil && !os.IsExist(err) {
		return errors.New("recovery: cannot publish the verification artifact")
	}
	return nil
}

// LoadVerification implements terraformexec.VerificationStore.
func (s *Store) LoadVerification(_ context.Context, handle, instanceID string) ([]byte, bool, error) {
	if !verifyHandle.MatchString(handle) || !idShape.MatchString(instanceID) {
		return nil, false, errors.New("recovery: invalid verification identity")
	}
	data, err := os.ReadFile(filepath.Join(s.dir, handle))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("recovery: the verification artifact cannot be read")
	}
	var env envelope
	if json.Unmarshal(data, &env) != nil {
		return nil, false, errors.New("recovery: the verification artifact is malformed")
	}
	nonce, err1 := base64.StdEncoding.DecodeString(env.Nonce)
	ciphertext, err2 := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err1 != nil || err2 != nil {
		return nil, false, errors.New("recovery: the verification artifact is malformed")
	}
	state, err := s.cipher.Open(env.KeyID, nonce, ciphertext, verifyAAD(handle, instanceID))
	if err != nil {
		return nil, false, errors.New("recovery: the verification artifact cannot be decrypted")
	}
	return state, true, nil
}
