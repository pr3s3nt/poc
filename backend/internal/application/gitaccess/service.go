// Package gitaccess manages the narrow platform Git access setting.
package gitaccess

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"

	"orchestrator/internal/ports/credentials"
	access "orchestrator/internal/ports/gitaccess"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("git access: invalid settings")
var ErrUnavailable = errors.New("git access: settings unavailable")

const credentialScope = "platform-git"

type Draft struct {
	URL             string `json:"url"`
	Username        string `json:"username"`
	Authenticated   bool   `json:"authenticated"`
	Token           string `json:"token"`
	CAPEM           string `json:"caPem"`
	ExpectedVersion int64  `json:"expectedVersion"`
}
type View struct {
	URL             string     `json:"url"`
	Username        string     `json:"username"`
	Authenticated   bool       `json:"authenticated"`
	CAPEM           string     `json:"caPem"`
	TokenConfigured bool       `json:"tokenConfigured"`
	Version         int64      `json:"version"`
	UpdatedAt       *time.Time `json:"updatedAt,omitempty"`
}
type Service struct {
	store       persistence.Store
	repo        persistence.GitSettingsRepository
	credentials credentials.Store
}

func New(st persistence.Store, creds credentials.Store) *Service {
	repo, _ := st.(persistence.GitSettingsRepository)
	return &Service{store: st, repo: repo, credentials: creds}
}
func view(v persistence.GitSettings) View {
	out := View{URL: v.URL, Username: v.Username, Authenticated: v.Authenticated, CAPEM: v.CAPEM, TokenConfigured: v.CredentialRef != "", Version: v.Version}
	if !v.UpdatedAt.IsZero() {
		out.UpdatedAt = &v.UpdatedAt
	}
	return out
}
func (s *Service) read(ctx context.Context, org string) (persistence.GitSettings, error) {
	if s == nil || s.repo == nil {
		return persistence.GitSettings{}, ErrUnavailable
	}
	v, err := s.repo.GetGitSettings(ctx, org)
	if errors.Is(err, persistence.ErrNotFound) {
		return persistence.GitSettings{}, nil
	}
	if err != nil {
		return persistence.GitSettings{}, ErrUnavailable
	}
	return v, nil
}
func (s *Service) Get(ctx context.Context, org string) (View, error) {
	v, err := s.read(ctx, org)
	return view(v), err
}

// ScopeURL rejects ambiguous/normalized paths, userinfo, query and fragments.
func ScopeURL(raw string) (*url.URL, error) {
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, "\\%?#") || strings.IndexFunc(raw, unicode.IsSpace) >= 0 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return nil, ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return nil, ErrInvalid
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return nil, ErrInvalid
		}
	}
	if strings.Contains(u.Path, "//") {
		return nil, ErrInvalid
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}
func Matches(scope, repo string) bool {
	a, err := ScopeURL(scope)
	if err != nil {
		return false
	}
	b, err := ScopeURL(repo)
	if err != nil {
		return false
	}
	return a.Host == b.Host && (a.Path == "" || b.Path == a.Path || strings.HasPrefix(b.Path, a.Path+"/"))
}
func validate(d Draft) (Draft, error) {
	u, err := ScopeURL(strings.TrimSpace(d.URL))
	if err != nil {
		return d, err
	}
	d.URL = u.String()
	if d.ExpectedVersion < 0 || len(d.Username) > 256 || strings.Contains(d.Username, ":") || strings.IndexFunc(d.Username, unicode.IsControl) >= 0 || len(d.Token) > 4096 || strings.IndexFunc(d.Token, unicode.IsControl) >= 0 || len(d.CAPEM) > 64<<10 {
		return d, ErrInvalid
	}
	d.CAPEM = strings.TrimSpace(d.CAPEM)
	if d.Authenticated && strings.TrimSpace(d.Username) == "" {
		return d, ErrInvalid
	}
	rest := []byte(d.CAPEM)
	for len(rest) > 0 {
		if !strings.HasPrefix(string(rest), "-----BEGIN CERTIFICATE-----") {
			return d, ErrInvalid
		}
		block, tail := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) > 0 {
			return d, ErrInvalid
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return d, ErrInvalid
		}
		rest = []byte(strings.TrimSpace(string(tail)))
	}
	if !d.Authenticated {
		d.Username = ""
		d.Token = ""
	}
	return d, nil
}
func (s *Service) prepare(ctx context.Context, org string, d Draft) (Draft, persistence.GitSettings, error) {
	if s == nil {
		return d, persistence.GitSettings{}, ErrUnavailable
	}
	d, err := validate(d)
	if err != nil {
		return d, persistence.GitSettings{}, err
	}
	current, err := s.read(ctx, org)
	if err != nil {
		return d, current, err
	}
	if d.ExpectedVersion != current.Version {
		return d, current, persistence.ErrVersionConflict
	}
	if d.Authenticated && d.Token == "" && (current.CredentialRef == "" || current.URL != d.URL || current.Username != d.Username) {
		return d, current, ErrInvalid
	}
	return d, current, nil
}
func (s *Service) Save(ctx context.Context, org, actor string, d Draft) (View, error) {
	d, current, err := s.prepare(ctx, org, d)
	if err != nil {
		return View{}, err
	}
	next := persistence.GitSettings{OrganizationKey: org, URL: d.URL, Username: d.Username, Authenticated: d.Authenticated, CAPEM: d.CAPEM, Version: current.Version + 1, UpdatedAt: time.Now().UTC(), UpdatedBy: actor}
	if d.Authenticated {
		next.CredentialRef = current.CredentialRef
		if s.credentials == nil {
			return View{}, ErrUnavailable
		}
		if d.Token != "" {
			payload, _ := json.Marshal(access.Access{Username: d.Username, Token: d.Token})
			ref, putErr := s.credentials.Put(ctx, org, credentialScope, payload)
			if putErr != nil || ref == "" {
				if ref != "" {
					cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					_ = s.credentials.Delete(cleanup, org, credentialScope, ref)
					cancel()
				}
				return View{}, ErrUnavailable
			}
			next.CredentialRef = ref
		}
	}
	err = s.store.Transact(ctx, func(ctx context.Context) error { return s.repo.SaveGitSettings(ctx, current.Version, next) })
	// Immutable credential objects are retained: a transport may have read the old
	// reference, and an uncertain commit may already point at the new object.
	if err != nil {
		if errors.Is(err, persistence.ErrVersionConflict) && next.CredentialRef != "" && next.CredentialRef != current.CredentialRef {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_ = s.credentials.Delete(cleanup, org, credentialScope, next.CredentialRef)
			cancel()
		}
		return View{}, err
	}
	return view(next), nil
}
func (s *Service) credential(ctx context.Context, org string, v persistence.GitSettings) (access.Access, error) {
	out := access.Access{CAPEM: v.CAPEM}
	if !v.Authenticated {
		return out, nil
	}
	if s.credentials == nil || v.CredentialRef == "" {
		return access.Access{}, ErrUnavailable
	}
	payload, err := s.credentials.Get(ctx, org, credentialScope, v.CredentialRef)
	var secret access.Access
	if err != nil || json.Unmarshal(payload, &secret) != nil || secret.Token == "" || secret.Username != v.Username {
		return access.Access{}, ErrUnavailable
	}
	out.Username, out.Token = secret.Username, secret.Token
	return out, nil
}
func (s *Service) ResolveGitAccess(ctx context.Context, org, repo string) (access.Access, error) {
	// Unscoped legacy preparation never borrows another Organization's secret.
	if org == "" {
		return access.Access{}, nil
	}
	v, err := s.read(ctx, org)
	if err != nil {
		return access.Access{}, err
	}
	if v.URL == "" || !Matches(v.URL, repo) {
		return access.Access{}, nil
	}
	return s.credential(ctx, org, v)
}

// DraftAccess verifies retention/CAS and scope without saving or replacing a token.
func (s *Service) DraftAccess(ctx context.Context, org, repository string, d Draft) (access.Access, error) {
	d, current, err := s.prepare(ctx, org, d)
	if err != nil {
		return access.Access{}, err
	}
	if !Matches(d.URL, repository) {
		return access.Access{}, ErrInvalid
	}
	out := access.Access{CAPEM: d.CAPEM}
	if !d.Authenticated {
		return out, nil
	}
	if d.Token != "" {
		out.Username, out.Token = d.Username, d.Token
		return out, nil
	}
	out, err = s.credential(ctx, org, current)
	out.CAPEM = d.CAPEM
	return out, err
}
