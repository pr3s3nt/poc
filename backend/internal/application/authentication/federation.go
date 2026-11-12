package authentication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/domain/identity"
	"orchestrator/internal/ports/persistence"
)

// Grant contains provider credentials only in memory, never in HTTP DTOs/logs.
type Grant struct {
	Subject, Username, RefreshToken string
	Role                            identity.Role
	ExpiresAt                       time.Time
}
type OIDCProvider interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(context.Context, string, string, string) (Grant, error)
	Refresh(context.Context, string) (Grant, error)
	Revoke(context.Context, string) error
}
type transaction struct {
	binding, nonce, verifier string
	expires                  time.Time
}
type federationSession struct {
	sync.Mutex
	grant            Grant
	checked, expires time.Time
}
type Federation struct {
	Provider                OIDCProvider
	Issuer, OrganizationKey string
	Secure                  bool
	mu                      sync.Mutex
	pending                 map[string]transaction
	sessions                map[string]*federationSession
}

func (s *Service) EnableOIDC(f *Federation) {
	f.pending = map[string]transaction{}
	f.sessions = map[string]*federationSession{}
	s.federation = f
}
func (s *Service) OIDCEnabled() bool  { return s.federation != nil }
func (s *Service) SecureCookie() bool { return s.federation != nil && s.federation.Secure }
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func (s *Service) BeginOIDC() (url, binding string, err error) {
	f := s.federation
	if f == nil {
		return "", "", ErrForbidden
	}
	state, err := randomToken()
	if err != nil {
		return "", "", err
	}
	binding, err = randomToken()
	if err != nil {
		return "", "", err
	}
	nonce, err := randomToken()
	if err != nil {
		return "", "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range f.pending {
		if !v.expires.After(s.now()) {
			delete(f.pending, k)
		}
	}
	if len(f.pending) >= 1024 {
		return "", "", ErrUnauthorized
	}
	f.pending[hashToken(state)] = transaction{hashToken(binding), nonce, verifier, s.now().Add(5 * time.Minute)}
	return f.Provider.AuthorizationURL(state, nonce, verifier), binding, nil
}
func (s *Service) CompleteOIDC(ctx context.Context, state, binding, code string) (string, Identity, error) {
	f := s.federation
	if f == nil || state == "" || binding == "" || code == "" {
		return "", Identity{}, ErrUnauthorized
	}
	f.mu.Lock()
	t, ok := f.pending[hashToken(state)]
	delete(f.pending, hashToken(state))
	f.mu.Unlock()
	if !ok || t.binding != hashToken(binding) || !t.expires.After(s.now()) {
		return "", Identity{}, ErrUnauthorized
	}
	grant, err := f.Provider.Exchange(ctx, code, t.verifier, t.nonce)
	if err != nil {
		return "", Identity{}, ErrUnauthorized
	}
	account, err := s.saveExternalIdentity(ctx, grant)
	if err != nil {
		return "", Identity{}, err
	}
	token, who, err := s.newSession(ctx, account)
	if err != nil {
		return "", Identity{}, err
	}
	f.mu.Lock()
	for k, v := range f.sessions {
		if !v.expires.After(s.now()) {
			delete(f.sessions, k)
		}
	}
	if len(f.sessions) >= 10000 {
		f.mu.Unlock()
		_ = s.SignOut(ctx, token)
		return "", Identity{}, ErrUnauthorized
	}
	f.sessions[hashToken(token)] = &federationSession{grant: grant, checked: s.now(), expires: s.now().Add(8 * time.Hour)}
	f.mu.Unlock()
	who.AuthProvider = "oidc"
	return token, who, nil
}
func externalID(issuer, subject string) string {
	b := sha256.Sum256([]byte(issuer + "\x00" + subject))
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:])
}
func (s *Service) saveExternalIdentity(ctx context.Context, g Grant) (identity.UserAccount, error) {
	f := s.federation
	if g.Subject == "" || g.RefreshToken == "" || !g.ExpiresAt.After(s.now()) {
		return identity.UserAccount{}, ErrUnauthorized
	}
	switch g.Role {
	case identity.RoleAdmin, identity.RolePlatformEngineer, identity.RoleDeveloper:
	default:
		return identity.UserAccount{}, ErrForbidden
	}
	username := strings.TrimSpace(g.Username)
	if username == "" || len(username) > 255 || strings.ContainsAny(username, "\r\n\x00") {
		return identity.UserAccount{}, ErrUnauthorized
	}
	account := identity.UserAccount{ID: externalID(f.Issuer, g.Subject), OrganizationKey: f.OrganizationKey, Username: username, Role: g.Role, Status: identity.AccountActive, OIDCIssuer: f.Issuer, OIDCSubject: g.Subject}
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		old, err := s.store.GetUserAccount(ctx, account.ID)
		if err == nil {
			if old.OIDCIssuer != f.Issuer || old.OIDCSubject != g.Subject || old.OrganizationKey != f.OrganizationKey || !s.accepted(old) {
				return ErrForbidden
			}
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		named, err := s.store.GetUserAccountByUsername(ctx, username)
		if err == nil && named.ID != account.ID {
			return ErrForbidden
		}
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		return s.store.SaveUserAccount(ctx, account)
	})
	return account, err
}
func (s *Service) externalIdentity(ctx context.Context, token string, account identity.UserAccount) (Identity, error) {
	f := s.federation
	if f == nil || account.OIDCIssuer != f.Issuer || account.OrganizationKey != f.OrganizationKey {
		return Identity{}, ErrUnauthorized
	}
	key := hashToken(token)
	f.mu.Lock()
	live := f.sessions[key]
	f.mu.Unlock()
	if live == nil {
		return Identity{}, ErrUnauthorized
	}
	live.Lock()
	defer live.Unlock()
	if !live.expires.After(s.now()) {
		return Identity{}, ErrUnauthorized
	}
	if s.now().Sub(live.checked) >= 30*time.Second || !live.grant.ExpiresAt.After(s.now().Add(5*time.Second)) {
		g, err := f.Provider.Refresh(ctx, live.grant.RefreshToken)
		if err != nil || g.Subject != account.OIDCSubject {
			f.mu.Lock()
			delete(f.sessions, key)
			f.mu.Unlock()
			return Identity{}, ErrUnauthorized
		}
		updated, err := s.saveExternalIdentity(ctx, g)
		if err != nil {
			f.mu.Lock()
			delete(f.sessions, key)
			f.mu.Unlock()
			return Identity{}, ErrUnauthorized
		}
		account = updated
		live.grant = g
		live.checked = s.now()
	}
	return Identity{UserID: account.ID, OrganizationKey: account.OrganizationKey, Username: live.grant.Username, Role: live.grant.Role, AuthProvider: "oidc"}, nil
}
func (s *Service) revokeExternal(ctx context.Context, token string, provider bool) {
	f := s.federation
	if f == nil {
		return
	}
	key := hashToken(token)
	f.mu.Lock()
	live := f.sessions[key]
	delete(f.sessions, key)
	f.mu.Unlock()
	if live != nil {
		live.Lock()
		defer live.Unlock()
		live.expires = s.now()
		if provider {
			_ = f.Provider.Revoke(ctx, live.grant.RefreshToken)
		}
	}
}
