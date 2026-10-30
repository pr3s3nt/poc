// Package authentication implements UC-00 local/test internal-account sessions.
package authentication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalidCredentials = errors.New("authentication: invalid credentials")
var ErrUnauthorized = errors.New("authentication: unauthorized")

// ErrForbidden denies a user-management call of a non-Admin caller (BR-09).
var ErrForbidden = errors.New("authentication: forbidden")

// ErrUsernameTaken refuses an account whose normalized username already exists.
var ErrUsernameTaken = errors.New("authentication: username already exists")

// ErrInvalidUsername refuses a username outside the public username rule.
var ErrInvalidUsername = errors.New("authentication: invalid username")

// ErrInvalidRole refuses a role outside the three UC-00 roles (BR-03).
var ErrInvalidRole = errors.New("authentication: invalid role")

// ErrInvalidPassword refuses a new password below the minimum policy.
var ErrInvalidPassword = errors.New("authentication: invalid password")

// ErrUserNotFound reports an Admin password-reset target that does not exist
// in the caller's Organization (BR-11); the caller learns nothing about
// accounts of other Organizations.
var ErrUserNotFound = errors.New("authentication: user not found")

// MinPasswordLength is the minimum length of a password chosen through the
// product (change password, Admin creation, first-Admin bootstrap).
const MinPasswordLength = 8

// usernamePattern is the public username rule: lowercase letters, digits, dot,
// underscore and hyphen, starting with a letter or digit.
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)

type Identity struct {
	UserID, OrganizationKey, Username string
	Role                              identity.Role
}
type Service struct {
	store    persistence.Store
	now      func() time.Time
	rejected map[string]bool
}

// NewService builds the UC-00 service. Sign-in and session restore are refused
// for rejectedAccountIDs; the production profile passes the fixed local/test
// accounts so a reused database never accepts them (UC-00 BR-05). The set is
// fixed at construction and only read afterwards.
func NewService(store persistence.Store, rejectedAccountIDs ...string) *Service {
	rejected := make(map[string]bool, len(rejectedAccountIDs))
	for _, id := range rejectedAccountIDs {
		rejected[strings.ToLower(id)] = true
	}
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }, rejected: rejected}
}

func (s *Service) accepted(account identity.UserAccount) bool {
	return account.Status == identity.AccountActive && !s.rejected[strings.ToLower(account.ID)]
}

func (s *Service) SignIn(ctx context.Context, username, rawPassword string) (string, Identity, error) {
	account, err := s.store.GetUserAccountByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil || !s.accepted(account) || !password.Verify(account.PasswordHash, rawPassword) {
		return "", Identity{}, ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Identity{}, err
	}
	token := hex.EncodeToString(raw)
	expires := s.now().Add(8 * time.Hour)
	session := identity.Session{ID: ids.New(), UserAccountID: account.ID, TokenHash: hashToken(token), ExpiresAt: expires}
	if err := s.store.Transact(ctx, func(ctx context.Context) error { return s.store.SaveSession(ctx, session) }); err != nil {
		return "", Identity{}, err
	}
	return token, Identity{UserID: account.ID, OrganizationKey: account.OrganizationKey, Username: account.Username, Role: account.Role}, nil
}
func (s *Service) IdentityForToken(ctx context.Context, token string) (Identity, error) {
	session, err := s.store.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil || session.RevokedAt != nil || !session.ExpiresAt.After(s.now()) {
		return Identity{}, ErrUnauthorized
	}
	account, err := s.store.GetUserAccount(ctx, session.UserAccountID)
	if err != nil || !s.accepted(account) {
		return Identity{}, ErrUnauthorized
	}
	return Identity{UserID: account.ID, OrganizationKey: account.OrganizationKey, Username: account.Username, Role: account.Role}, nil
}
func (s *Service) SignOut(ctx context.Context, token string) error {
	session, err := s.store.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return nil
	}
	now := s.now()
	session.RevokedAt = &now
	return s.store.Transact(ctx, func(ctx context.Context) error { return s.store.SaveSession(ctx, session) })
}
// ChangePassword replaces the account password after verifying the current one
// and revokes every session of the account in the same transaction, so a
// successful change ends all sign-ins including the current one (BR-08).
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < MinPasswordLength {
		return ErrInvalidPassword
	}
	account, err := s.store.GetUserAccount(ctx, userID)
	if err != nil || !s.accepted(account) || !password.Verify(account.PasswordHash, currentPassword) {
		return ErrInvalidCredentials
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	account.PasswordHash = hash
	return s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.store.SaveUserAccount(ctx, account); err != nil {
			return err
		}
		return s.store.RevokeSessionsForUser(ctx, account.ID, s.now())
	})
}

// ListUsers returns the accounts of the caller's Organization; only an Admin
// may list them (BR-09). The caller identity comes from the session.
func (s *Service) ListUsers(ctx context.Context, caller Identity) ([]identity.UserAccount, error) {
	if caller.Role != identity.RoleAdmin {
		return nil, ErrForbidden
	}
	return s.store.ListUserAccounts(ctx, caller.OrganizationKey)
}

// CreateUser registers an account in the caller's Organization; only an Admin
// may create accounts (BR-09). The password is stored as a hash only (BR-02).
func (s *Service) CreateUser(ctx context.Context, caller Identity, username, rawPassword string, role identity.Role) (identity.UserAccount, error) {
	if caller.Role != identity.RoleAdmin {
		return identity.UserAccount{}, ErrForbidden
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return identity.UserAccount{}, ErrInvalidUsername
	}
	switch role {
	case identity.RoleAdmin, identity.RolePlatformEngineer, identity.RoleDeveloper:
	default:
		return identity.UserAccount{}, ErrInvalidRole
	}
	if len(rawPassword) < MinPasswordLength {
		return identity.UserAccount{}, ErrInvalidPassword
	}
	hash, err := password.Hash(rawPassword)
	if err != nil {
		return identity.UserAccount{}, err
	}
	account := identity.UserAccount{
		ID: ids.New(), OrganizationKey: caller.OrganizationKey, Username: username,
		PasswordHash: hash, Role: role, Status: identity.AccountActive,
	}
	// The duplicate check runs inside the transaction so a concurrent create of
	// the same username cannot slip between check and insert.
	err = s.store.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetUserAccountByUsername(ctx, username); err == nil {
			return ErrUsernameTaken
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		return s.store.SaveUserAccount(ctx, account)
	})
	if err != nil {
		return identity.UserAccount{}, err
	}
	return account, nil
}

// ResetUserPassword replaces the password of an account in the caller's
// Organization and revokes every session of that account in the same
// transaction, so a successful reset ends all of its sign-ins (BR-11). Only an
// Admin may reset passwords; the Admin never reads the current password. The
// new password is stored as a hash only (BR-02).
func (s *Service) ResetUserPassword(ctx context.Context, caller Identity, targetUserID, newPassword string) error {
	if caller.Role != identity.RoleAdmin {
		return ErrForbidden
	}
	if len(newPassword) < MinPasswordLength {
		return ErrInvalidPassword
	}
	account, err := s.store.GetUserAccount(ctx, targetUserID)
	if err != nil || account.OrganizationKey != caller.OrganizationKey {
		return ErrUserNotFound
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	account.PasswordHash = hash
	return s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.store.SaveUserAccount(ctx, account); err != nil {
			return err
		}
		return s.store.RevokeSessionsForUser(ctx, account.ID, s.now())
	})
}

// BootstrapAdmin creates the first ADMIN account of the Organization (BR-10).
// It runs without a caller identity from the CLI; the password arrives from an
// owner-supplied file, never from code or a default. The Organization must
// already exist (seed.Apply guarantees it).
func (s *Service) BootstrapAdmin(ctx context.Context, organizationKey, username, rawPassword string) (identity.UserAccount, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return identity.UserAccount{}, ErrInvalidUsername
	}
	if len(rawPassword) < MinPasswordLength {
		return identity.UserAccount{}, ErrInvalidPassword
	}
	hash, err := password.Hash(rawPassword)
	if err != nil {
		return identity.UserAccount{}, err
	}
	account := identity.UserAccount{
		ID: ids.New(), OrganizationKey: organizationKey, Username: username,
		PasswordHash: hash, Role: identity.RoleAdmin, Status: identity.AccountActive,
	}
	err = s.store.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetUserAccountByUsername(ctx, username); err == nil {
			return ErrUsernameTaken
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		return s.store.SaveUserAccount(ctx, account)
	})
	if err != nil {
		return identity.UserAccount{}, err
	}
	return account, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
