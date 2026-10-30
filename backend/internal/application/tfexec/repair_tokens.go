package tfexec

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// SealReviewToken keeps retrievable same-actor confirmation tokens encrypted
// under the State key. Only its hash participates in database admission.
func (s *Service) SealReviewToken(_ context.Context, org, review string) (persistence.CipherArtifact, string, error) {
	if s.deps.Cipher == nil || org == "" || review == "" {
		return persistence.CipherArtifact{}, "", ErrArtifactEncoding
	}
	a := persistence.CipherArtifact{ID: ids.New(), OrganizationID: org, OwnerID: review, Purpose: "TERRAFORM_REVIEW_TOKEN", KeyID: s.deps.Cipher.KeyID(), AADVersion: 1, CreatedAt: time.Now().UTC()}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return a, "", ErrArtifactEncoding
	}
	token := hex.EncodeToString(bytes)
	hash := sha256.Sum256([]byte(token))
	var err error
	a.Nonce, a.Ciphertext, err = s.deps.Cipher.Seal([]byte(token), baselineAAD(a))
	return a, hex.EncodeToString(hash[:]), err
}
func (s *Service) OpenReviewToken(_ context.Context, org, review string, a persistence.CipherArtifact) (string, error) {
	if s.deps.Cipher == nil || a.OrganizationID != org || a.OwnerID != review || a.Purpose != "TERRAFORM_REVIEW_TOKEN" || a.AADVersion != 1 {
		return "", ErrArtifactEncoding
	}
	plain, err := s.deps.Cipher.Open(a.KeyID, a.Nonce, a.Ciphertext, baselineAAD(a))
	if err != nil || len(plain) != 64 {
		return "", errors.New("tfexec: review token unavailable")
	}
	return string(plain), nil
}
