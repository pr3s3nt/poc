package secretstores

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/ports/persistence"
)

// BootstrapKey is the stable reserved key of the Compose bootstrap store.
const BootstrapKey = "platform-vault"

// BootstrapMarker is stored in the verification record of a bootstrap-managed
// store. Only a marked store may have its credential replaced by bootstrap.
const BootstrapMarker = "compose"

// ErrBootstrapConflict stops bootstrap rather than overwrite a registration.
var ErrBootstrapConflict = errors.New("secret store: bootstrap conflicts with an existing store; resolve it before restarting")

// BootstrapOutcome reports what Bootstrap did.
type BootstrapOutcome string

// Bootstrap outcomes.
const (
	BootstrapCreated   BootstrapOutcome = "created"
	BootstrapConverted BootstrapOutcome = "converted"
	BootstrapRefreshed BootstrapOutcome = "refreshed"
	BootstrapUnchanged BootstrapOutcome = "unchanged"
)

// Bootstrap admits the explicitly configured Compose Vault as an ordinary
// READY store with the reserved key. It uses the registration validation, the
// real verifier and the platform credential store. The token is only bootstrap
// input: the persisted record carries an opaque credential reference exactly as
// interactive registration does.
//
//   - no record: verify, persist the credential, insert;
//   - matching Compose legacy record: verify, persist the credential, then
//     convert the same row (ID, key, endpoints, selections and references stay);
//   - matching bootstrap-managed record with the same token: nothing changes;
//   - managed record with another token: verify, persist a new credential,
//     replace the reference, then remove the replaced credential;
//   - any other state or endpoint difference: ErrBootstrapConflict.
//
// Admission is compare-and-set. A loser removes only its own attempt credential
// and re-evaluates the winner's state.
func (s *Service) Bootstrap(ctx context.Context, org string, cmd RegisterCommand) (BootstrapOutcome, error) {
	record, token, err := prepare(org, cmd)
	if err != nil {
		return "", err
	}
	record.Key = BootstrapKey
	if s.credentials == nil {
		return "", ErrCredentialStore
	}
	if s.verifier == nil {
		return "", fmt.Errorf("%w: no verifier is available", ErrVerification)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return "", err
	}
	scope := secretstore.CredentialScope(BootstrapKey)
	for attempt := 0; attempt < maxRetries; attempt++ {
		admission := persistence.ManagedStoreAdmission{}
		outcome := BootstrapCreated
		existing, err := s.store.GetSecretStore(ctx, org, BootstrapKey)
		switch {
		case errors.Is(err, persistence.ErrNotFound):
		case err != nil:
			return "", err
		default:
			if existing.Status != secretstore.StatusReady || existing.OrganizationKey != org || !existing.SameEndpoint(record) {
				return "", ErrBootstrapConflict
			}
			admission.ExpectedID, admission.ExpectedLegacy, admission.ExpectedCredentialRef = existing.ID, existing.Legacy, existing.CredentialRef
			managed := existing.Verification["bootstrap"] == BootstrapMarker
			switch {
			case existing.Legacy:
				outcome = BootstrapConverted
			case s.sameToken(ctx, existing, token):
				return BootstrapUnchanged, nil
			case managed:
				outcome = BootstrapRefreshed
			default:
				return "", ErrBootstrapConflict
			}
		}
		verification, err := s.verifier.Verify(ctx, verifyRequest(record, token))
		if err != nil {
			return "", verificationError(err)
		}
		ref, err := s.credentials.Put(ctx, org, scope, []byte(token))
		if err != nil {
			if ref != "" {
				if cleanupErr := s.cleanup(ctx, org, BootstrapKey, ref); cleanupErr != nil {
					return "", cleanupErr
				}
			}
			return "", ErrCredentialStore
		}
		next := record
		next.CredentialRef = ref
		next.Verification = map[string]any{}
		for k, v := range verification {
			next.Verification[k] = v
		}
		next.Verification["bootstrap"] = BootstrapMarker
		admission.Store = next
		if _, err = s.store.AdmitManagedSecretStore(ctx, admission); err == nil {
			if outcome == BootstrapRefreshed && existing.CredentialRef != "" {
				_ = s.cleanup(ctx, org, BootstrapKey, existing.CredentialRef) // replaced credential; failure is logged
			}
			return outcome, nil
		}
		if cleanupErr := s.cleanup(ctx, org, BootstrapKey, ref); cleanupErr != nil {
			return "", cleanupErr
		}
		switch {
		case errors.Is(err, persistence.ErrDuplicate), errors.Is(err, persistence.ErrVersionConflict):
			continue
		case errors.Is(err, persistence.ErrImmutable):
			return "", ErrBootstrapConflict
		}
		return "", err
	}
	return "", fmt.Errorf("secret store: bootstrap could not be admitted concurrently; retry")
}

// sameToken reports whether the store's persisted credential equals token.
func (s *Service) sameToken(ctx context.Context, store secretstore.Store, token string) bool {
	if store.CredentialRef == "" {
		return false
	}
	raw, err := s.credentials.Get(ctx, store.OrganizationKey, store.CredentialScope(), store.CredentialRef)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(raw))), []byte(token)) == 1
}
