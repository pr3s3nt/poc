// Package installationsetup implements operator-authorized, one-time initialization.
package installationsetup

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"regexp"
	"strings"
)

var ErrClosed = errors.New("setup: unavailable")
var ErrToken = errors.New("setup: invalid authorization")
var ErrInvalid = errors.New("setup: invalid identity")

type Service struct {
	store        persistence.Store
	installation persistence.InstallationSetup
	token        [32]byte
	enabled      bool
}
type Claim struct {
	Token            string `json:"token"`
	OrganizationKey  string `json:"organizationKey"`
	OrganizationName string `json:"organizationName"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	ConfirmPassword  string `json:"confirmPassword"`
}

func New(st persistence.Store, token string) *Service {
	repo, _ := st.(persistence.InstallationSetup)
	return &Service{store: st, installation: repo, token: sha256.Sum256([]byte(token)), enabled: len(token) >= 32 && repo != nil}
}
func (s *Service) Available(ctx context.Context) (bool, error) {
	if s == nil || !s.enabled {
		return false, nil
	}
	orgs, err := s.installation.ListOrganizations(ctx)
	return len(orgs) == 0 && err == nil, err
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)

func (s *Service) Initialize(ctx context.Context, d Claim) error {
	if s == nil || !s.enabled {
		return ErrClosed
	}
	token := sha256.Sum256([]byte(d.Token))
	if subtle.ConstantTimeCompare(token[:], s.token[:]) != 1 {
		return ErrToken
	}
	d.Username = strings.ToLower(strings.TrimSpace(d.Username))
	if (seed.OrganizationIdentity{Key: d.OrganizationKey, Name: d.OrganizationName}).Validate() != nil || !usernamePattern.MatchString(d.Username) || len(d.Password) < authentication.MinPasswordLength || len(d.Password) > 1024 || d.Password != d.ConfirmPassword {
		return ErrInvalid
	}
	hash, err := password.Hash(d.Password)
	if err != nil {
		return err
	}
	return s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.installation.LockInstallationSetup(ctx); err != nil {
			return err
		}
		available, err := s.Available(ctx)
		if err != nil {
			return err
		}
		if !available {
			return ErrClosed
		}
		if err := s.store.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: d.OrganizationKey, Name: d.OrganizationName}); err != nil {
			return err
		}
		return s.store.SaveUserAccount(ctx, identity.UserAccount{ID: ids.New(), OrganizationKey: d.OrganizationKey, Username: d.Username, PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive})
	})
}
