// Package secretstores implements PE/Admin registration and Developer choices
// of Organization-scoped workload Secret Store Connections (UC-04 SS-01..06).
package secretstores

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
)

// Errors mapped by delivery. Their text is fixed and safe for the caller.
var (
	ErrInvalid         = errors.New("secret store: invalid registration")
	ErrVerification    = errors.New("secret store: verification failed")
	ErrCredentialStore = errors.New("secret store: the token could not be stored; no store was saved, retry later")
)

// CleanupError reports a failed rollback of the attempt's own credential.
type CleanupError struct{ Reference string }

func (e *CleanupError) Error() string {
	return "secret store: registration failed and its credential cleanup did not complete (reference " + e.Reference + "); contact an operator"
}

const (
	maxNameLength  = 100
	maxKeySlug     = 32
	maxTokenLength = 4096
	maxCAPEM       = 64 << 10
	maxRetries     = 5
	cleanupTimeout = 15 * time.Second
)

// RegisterCommand is the PE/Admin registration input. Token is concealed and
// never returned or logged.
type RegisterCommand struct {
	Name            string
	BackendAddress  string
	WorkloadAddress string
	Mount           string
	AuthMount       string
	TLSCAPEM        string
	Token           string
}

// Service registers and lists stores.
type Service struct {
	store       persistence.Store
	verifier    configport.Verifier
	credentials credentials.Store
}

// NewService wires the repository, verifier and platform credential store.
func NewService(store persistence.Store, verifier configport.Verifier, creds credentials.Store) *Service {
	return &Service{store: store, verifier: verifier, credentials: creds}
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: a name is required (at most %d characters, no control characters)", ErrInvalid, maxNameLength)
	}
	return name, nil
}

func validToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenLength || strings.IndexFunc(token, unicode.IsControl) >= 0 || !utf8.ValidString(token) {
		return fmt.Errorf("%w: a token is required (at most %d characters, no control characters)", ErrInvalid, maxTokenLength)
	}
	return nil
}

func validCA(pemText string) error {
	if pemText == "" {
		return nil
	}
	if len(pemText) > maxCAPEM {
		return fmt.Errorf("%w: the CA certificate is too large", ErrInvalid)
	}
	rest := []byte(pemText)
	found := false
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("%w: only CERTIFICATE PEM blocks are accepted", ErrInvalid)
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return fmt.Errorf("%w: the CA certificate is not valid", ErrInvalid)
		}
		found = true
	}
	if !found || len(strings.TrimSpace(string(rest))) > 0 {
		return fmt.Errorf("%w: the CA certificate is not valid PEM", ErrInvalid)
	}
	return nil
}

// prepare validates one registration input and returns the unsaved record and
// the trimmed token. Interactive registration and Compose bootstrap share it.
func prepare(org string, cmd RegisterCommand) (secretstore.Store, string, error) {
	name, err := validName(cmd.Name)
	if err != nil {
		return secretstore.Store{}, "", err
	}
	mount, authMount := strings.TrimSpace(cmd.Mount), strings.TrimSpace(cmd.AuthMount)
	if mount == "" {
		mount = secretstore.DefaultKVMount
	}
	if authMount == "" {
		authMount = secretstore.DefaultAuthMount
	}
	record := secretstore.Store{
		Name: name, OrganizationKey: org, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
		BackendAddress: strings.TrimRight(strings.TrimSpace(cmd.BackendAddress), "/"), WorkloadAddress: strings.TrimRight(strings.TrimSpace(cmd.WorkloadAddress), "/"),
		Mount: mount, AuthMount: authMount, TLSCAPEM: strings.TrimSpace(cmd.TLSCAPEM),
	}
	for _, check := range []error{
		secretstore.ValidateAddress(record.BackendAddress), secretstore.ValidateAddress(record.WorkloadAddress),
	} {
		if check != nil {
			return secretstore.Store{}, "", fmt.Errorf("%w: backend and workload addresses must be http(s) URLs without credentials", ErrInvalid)
		}
	}
	if !secretstore.ValidMount(mount) || !secretstore.ValidMount(authMount) {
		return secretstore.Store{}, "", fmt.Errorf("%w: mounts may contain only letters, digits, hyphen and underscore", ErrInvalid)
	}
	if err := validCA(record.TLSCAPEM); err != nil {
		return secretstore.Store{}, "", err
	}
	if err := validToken(cmd.Token); err != nil {
		return secretstore.Store{}, "", err
	}
	return record, strings.TrimSpace(cmd.Token), nil
}

// Register verifies the store, stores its token privately and inserts a READY
// record. A failure after the credential was written removes only this
// attempt's credential.
func (s *Service) Register(ctx context.Context, org string, cmd RegisterCommand) (secretstore.Store, error) {
	record, token, err := prepare(org, cmd)
	if err != nil {
		return secretstore.Store{}, err
	}
	name := record.Name
	if s.credentials == nil {
		return secretstore.Store{}, ErrCredentialStore
	}
	if s.verifier == nil {
		return secretstore.Store{}, fmt.Errorf("%w: no verifier is available", ErrVerification)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return secretstore.Store{}, err
	}
	verification, err := s.verifier.Verify(ctx, verifyRequest(record, token))
	if err != nil {
		return secretstore.Store{}, verificationError(err)
	}
	record.Verification = verification
	base := keySlug(name)
	for attempt := 0; attempt < maxRetries; attempt++ {
		key, err := s.freeKey(ctx, org, base)
		if err != nil {
			return secretstore.Store{}, err
		}
		record.Key = key
		ref, err := s.credentials.Put(ctx, org, secretstore.CredentialScope(key), []byte(token))
		if err != nil {
			if ref != "" {
				if cleanupErr := s.cleanup(ctx, org, key, ref); cleanupErr != nil {
					return secretstore.Store{}, cleanupErr
				}
			}
			return secretstore.Store{}, ErrCredentialStore
		}
		record.CredentialRef = ref
		record.CreatedAt = time.Now().UTC()
		err = s.store.Transact(ctx, func(ctx context.Context) error {
			if _, err := s.store.GetOrganization(ctx, org); err != nil {
				return err
			}
			return s.store.CreateSecretStore(ctx, record)
		})
		if err == nil {
			return record, nil
		}
		if cleanupErr := s.cleanup(ctx, org, key, ref); cleanupErr != nil {
			return secretstore.Store{}, cleanupErr
		}
		if !errors.Is(err, persistence.ErrDuplicate) {
			return secretstore.Store{}, err
		}
	}
	return secretstore.Store{}, fmt.Errorf("secret store: could not allocate a unique store key; retry")
}

func verifyRequest(record secretstore.Store, token string) configport.VerifyRequest {
	return configport.VerifyRequest{BackendAddress: record.BackendAddress, Mount: record.Mount, AuthMount: record.AuthMount, CAPEM: record.TLSCAPEM, Token: token}
}

func verificationError(err error) error {
	switch {
	case errors.Is(err, configport.ErrUnreachable):
		return fmt.Errorf("%w: the secret store could not be reached; check the backend address, network access and TLS settings", ErrVerification)
	case errors.Is(err, configport.ErrTokenRejected):
		return fmt.Errorf("%w: the secret store rejected the token", ErrVerification)
	case errors.Is(err, configport.ErrNotKV2):
		return fmt.Errorf("%w: the mount is not a Vault KV version 2 engine", ErrVerification)
	case errors.Is(err, configport.ErrCapability):
		// The category text names only capability classes, never values.
		return fmt.Errorf("%w: %s", ErrVerification, strings.TrimPrefix(err.Error(), "the token lacks a required capability: "))
	case errors.Is(err, configport.ErrProbeCleanup):
		return fmt.Errorf("%w: the verification probe object could not be removed; the store was not registered", ErrVerification)
	case errors.Is(err, configport.ErrProbe):
		return fmt.Errorf("%w: the read/write probe failed", ErrVerification)
	}
	return fmt.Errorf("%w: check the address, mounts and token permissions", ErrVerification)
}

func (s *Service) cleanup(ctx context.Context, org, key, ref string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := s.credentials.Delete(cleanupCtx, org, secretstore.CredentialScope(key), ref); err != nil {
		reference := ids.New()
		log.Printf("secretstore: credential cleanup failed (reference %s, organization %s, store %s)", reference, org, key)
		return &CleanupError{Reference: reference}
	}
	return nil
}

func (s *Service) freeKey(ctx context.Context, org, base string) (string, error) {
	existing, err := s.store.ListSecretStores(ctx, org)
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, store := range existing {
		taken[store.Key] = true
	}
	if !taken[base] {
		return base, nil
	}
	for n := 2; n < 100; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !taken[candidate] {
			return candidate, nil
		}
	}
	for {
		candidate := base + "-" + strings.ReplaceAll(ids.New(), "-", "")[:6]
		if !taken[candidate] {
			return candidate, nil
		}
	}
}

func keySlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'đ':
			r = 'd'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > maxKeySlug {
		slug = strings.TrimRight(slug[:maxKeySlug], "-")
	}
	if slug == "" {
		slug = "store"
	}
	return slug
}

// List returns every store of the Organization (platform management view).
func (s *Service) List(ctx context.Context, org string) ([]secretstore.Store, error) {
	return s.store.ListSecretStores(ctx, org)
}

// Choices returns the READY stores a Developer may select.
func (s *Service) Choices(ctx context.Context, org string) ([]secretstore.Store, error) {
	all, err := s.store.ListSecretStores(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []secretstore.Store{}
	for _, store := range all {
		if store.Status == secretstore.StatusReady && store.OrganizationKey == org {
			out = append(out, store)
		}
	}
	return out, nil
}
