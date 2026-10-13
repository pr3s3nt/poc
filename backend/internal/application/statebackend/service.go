// Package statebackend implements OC-18: the Terraform HTTP state backend
// operations (GET/LOCK/POST/UNLOCK) over the restricted PostgreSQL repository
// and the state cipher. Every request verifies its invocation capability and
// the current Environment claim owner in one short transaction; lock, lineage
// and serial checks and the encrypted commit are atomic. No request releases a
// lock because a deadline passed, and no request purges state.
package statebackend

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformstate"
)

// MaxCapabilityTTL bounds one invocation capability.
const MaxCapabilityTTL = 6 * time.Hour

// MaxStateBytes bounds one state document.
const MaxStateBytes = 32 << 20

// Service is the state backend application service.
type Service struct {
	repo   terraformstate.Repository
	cipher terraformstate.Cipher
	now    func() time.Time
}

// New builds the service. A missing cipher fails closed: there is no
// plaintext or keyless mode.
func New(repo terraformstate.Repository, cipher terraformstate.Cipher) (*Service, error) {
	if repo == nil || cipher == nil {
		return nil, errors.New("statebackend: a PostgreSQL repository and a state key are required")
	}
	return &Service{repo: repo, cipher: cipher, now: func() time.Time { return time.Now().UTC() }}, nil
}

// SetClock replaces the time source (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// HashToken returns the stored form of a capability token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IssueCapability creates a short-lived invocation token for one instance,
// pinned to the claim owner in ctx (persistence.WithOwner) and its current
// fence. Only the hash is stored; the token is returned once and must reach
// the executor through its private subprocess environment.
func (s *Service) IssueCapability(ctx context.Context, instanceID string, action tfresource.CapabilityAction, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 || ttl > MaxCapabilityTTL || !action.Valid() {
		return "", time.Time{}, fmt.Errorf("%w: capability action or lifetime", tfresource.ErrInvalid)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, errors.New("statebackend: cannot draw a capability token")
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := s.now().Add(ttl)
	err := s.repo.InsertCapability(ctx, tfresource.Capability{TokenHash: HashToken(token), InstanceID: instanceID, Action: action, ExpiresAt: expires})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Service) authorize(ctx context.Context, token, stateKey string) (terraformstate.Invocation, error) {
	if token == "" || len(token) > 256 {
		return terraformstate.Invocation{}, terraformstate.ErrUnauthorized
	}
	if !tfresource.ValidStateKey(stateKey) {
		return terraformstate.Invocation{}, terraformstate.ErrForbidden
	}
	return s.repo.AuthorizeForUpdate(ctx, HashToken(token), stateKey, s.now())
}

func (s *Service) open(inv terraformstate.Invocation, blob *tfresource.StateBlob) ([]byte, error) {
	if blob.OrganizationID != inv.OrganizationID {
		return nil, terraformstate.ErrForbidden
	}
	return s.cipher.Open(blob.KeyID, blob.Nonce, blob.Ciphertext, tfresource.StateAAD(inv.OrganizationID, inv.StateKey, blob.KeyID, blob.AADRevision))
}

// Read returns the decrypted state, or found=false when no state was ever
// acknowledged. It never creates state; undecryptable ciphertext fails closed.
func (s *Service) Read(ctx context.Context, token, stateKey string) (state []byte, found bool, err error) {
	err = s.repo.Transact(ctx, func(ctx context.Context) error {
		inv, err := s.authorize(ctx, token, stateKey)
		if err != nil {
			return err
		}
		blob, err := s.repo.StateForUpdate(ctx, inv.InstanceID)
		if err != nil || blob == nil {
			return err
		}
		state, err = s.open(inv, blob)
		found = err == nil
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return state, found, nil
}

// LockRequest is the parsed Terraform lock info. Only ID and the safe
// metadata are kept; Who, Info and Path are discarded.
type LockRequest struct {
	ID       string
	Metadata tfresource.LockMetadata
}

// Lock claims the state lock for the capability's operation and fence. The
// same lock ID, operation and fence is acknowledged idempotently; any other
// holder yields ErrLockConflict with the held lock's safe metadata.
func (s *Service) Lock(ctx context.Context, token, stateKey string, req LockRequest) (*tfresource.Lock, error) {
	if !validLockID(req.ID) {
		return nil, fmt.Errorf("%w: lock id", terraformstate.ErrInvalidState)
	}
	var held *tfresource.Lock
	err := s.repo.Transact(ctx, func(ctx context.Context) error {
		inv, err := s.authorize(ctx, token, stateKey)
		if err != nil {
			return err
		}
		now := s.now()
		inserted, err := s.repo.InsertLock(ctx, tfresource.Lock{InstanceID: inv.InstanceID, LockID: req.ID, OperationID: inv.OperationID, Owner: inv.Owner, Fence: inv.Fence, Metadata: req.Metadata, AcquiredAt: now, Deadline: inv.ExpiresAt})
		if err != nil || inserted {
			return err
		}
		held, err = s.repo.LockForUpdate(ctx, inv.InstanceID)
		if err != nil {
			return err
		}
		if held != nil && held.LockID == req.ID && held.OperationID == inv.OperationID && held.Fence == inv.Fence {
			held = nil
			return nil
		}
		return terraformstate.ErrLockConflict
	})
	if errors.Is(err, terraformstate.ErrLockConflict) {
		return held, err
	}
	return nil, err
}

// Unlock releases the lock only for its exact lock ID, operation and fence.
// An absent lock is acknowledged; nothing else ever deletes a lock.
func (s *Service) Unlock(ctx context.Context, token, stateKey, lockID string) error {
	return s.repo.Transact(ctx, func(ctx context.Context) error {
		inv, err := s.authorize(ctx, token, stateKey)
		if err != nil {
			return err
		}
		held, err := s.repo.LockForUpdate(ctx, inv.InstanceID)
		if err != nil || held == nil {
			return err
		}
		if held.LockID != lockID || held.OperationID != inv.OperationID || held.Fence != inv.Fence {
			return terraformstate.ErrLockMismatch
		}
		return s.repo.DeleteLock(ctx, inv.InstanceID, lockID, inv.OperationID, inv.Fence)
	})
}

type stateHeader struct {
	Version *int   `json:"version"`
	Serial  *int64 `json:"serial"`
	Lineage string `json:"lineage"`
}

// Write commits a new state under the held lock. Lineage must not change and
// serial must advance; replay of identical bytes at the current serial is
// acknowledged without a write, different bytes at it are rejected. The
// ciphertext, serial and version commit atomically with the attempt's serial.
func (s *Service) Write(ctx context.Context, token, stateKey, lockID string, body []byte) error {
	if len(body) == 0 || len(body) > MaxStateBytes {
		return terraformstate.ErrInvalidState
	}
	var header stateHeader
	if err := json.Unmarshal(body, &header); err != nil || header.Version == nil || header.Serial == nil || *header.Serial < 0 || header.Lineage == "" || len(header.Lineage) > 128 {
		return terraformstate.ErrInvalidState
	}
	return s.repo.Transact(ctx, func(ctx context.Context) error {
		inv, err := s.authorize(ctx, token, stateKey)
		if err != nil {
			return err
		}
		if !inv.Action.MayWrite() {
			return fmt.Errorf("%w: capability may not write state", terraformstate.ErrForbidden)
		}
		held, err := s.repo.LockForUpdate(ctx, inv.InstanceID)
		if err != nil {
			return err
		}
		if lockID == "" || held == nil || held.LockID != lockID || held.OperationID != inv.OperationID || held.Fence != inv.Fence {
			return terraformstate.ErrLockMismatch
		}
		current, err := s.repo.StateForUpdate(ctx, inv.InstanceID)
		if err != nil {
			return err
		}
		var expected int64
		if current != nil {
			// The stored state must open with the configured key before it is
			// replaced: a wrong key never overwrites state it cannot read.
			plain, err := s.open(inv, current)
			if err != nil {
				return err
			}
			if current.Lineage != header.Lineage || *header.Serial < current.Serial {
				return terraformstate.ErrStateConflict
			}
			if *header.Serial == current.Serial {
				if bytes.Equal(plain, body) {
					return nil
				}
				return terraformstate.ErrStateConflict
			}
			expected = current.Version
		}
		version := expected + 1
		keyID := s.cipher.KeyID()
		nonce, ciphertext, err := s.cipher.Seal(body, tfresource.StateAAD(inv.OrganizationID, inv.StateKey, keyID, version))
		if err != nil {
			return err
		}
		if err := s.repo.RecordStateSerial(ctx, inv.OperationID, inv.InstanceID, *header.Serial); err != nil {
			return err
		}
		return s.repo.PutState(ctx, tfresource.StateBlob{InstanceID: inv.InstanceID, OrganizationID: inv.OrganizationID, Ciphertext: ciphertext, Nonce: nonce, KeyID: keyID, AADRevision: version, Lineage: header.Lineage, Serial: *header.Serial, Version: version, UpdatedAt: s.now()}, expected)
	})
}

func validLockID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}
