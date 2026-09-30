// Package authentication implements UC-00 local/test internal-account sessions.
package authentication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"orchestrator/internal/domain/identity"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/platform/password"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalidCredentials = errors.New("authentication: invalid credentials")
var ErrUnauthorized = errors.New("authentication: unauthorized")

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
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
