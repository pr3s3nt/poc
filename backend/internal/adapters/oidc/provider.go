// Package oidc adapts standards-based discovery/code exchange for Keycloak SSO.
package oidc

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/identity"
)

var errProvider = errors.New("oidc: provider authentication failed")

type Config struct {
	Issuer, ClientID, ClientSecret, PublicURL string
	DeveloperRole, EngineerRole, AdminRole    string
	AllowHTTP                                 bool
}
type Provider struct {
	config    Config
	oauth     oauth2.Config
	verifier  *coreoidc.IDTokenVerifier
	client    *http.Client
	revokeURL string
}

func New(ctx context.Context, c Config) (*Provider, error) {
	if c.ClientID == "" || c.ClientSecret == "" {
		return nil, errors.New("oidc: client ID and secret required")
	}
	for _, raw := range []string{c.Issuer, c.PublicURL} {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(c.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("oidc: valid HTTPS issuer/public URL required (HTTP requires test opt-in)")
		}
	}
	u, _ := url.Parse(c.PublicURL)
	if u.Path != "" && u.Path != "/" {
		return nil, errors.New("oidc: public URL must be an origin")
	}
	if c.DeveloperRole == "" {
		c.DeveloperRole = "developer"
	}
	if c.EngineerRole == "" {
		c.EngineerRole = "platform-engineer"
	}
	if c.AdminRole == "" {
		c.AdminRole = "admin"
	}
	if c.DeveloperRole == c.EngineerRole || c.AdminRole == c.EngineerRole || c.DeveloperRole == c.AdminRole {
		return nil, errors.New("oidc: role mappings must be distinct")
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errProvider
		}
		if req.URL.Scheme != "https" && !c.AllowHTTP {
			return errProvider
		}
		return nil
	}}
	ctx = coreoidc.ClientContext(ctx, client)
	provider, err := coreoidc.NewProvider(ctx, c.Issuer)
	if err != nil {
		return nil, errProvider
	}
	var metadata struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	if provider.Claims(&metadata) != nil {
		return nil, errProvider
	}
	endpoint := provider.Endpoint()
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.RevocationEndpoint} {
		v, e := url.Parse(raw)
		if e != nil || v.Host == "" || v.User != nil || (v.Scheme != "https" && !(c.AllowHTTP && v.Scheme == "http")) {
			return nil, errProvider
		}
	}
	return &Provider{config: c, oauth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: endpoint, RedirectURL: strings.TrimRight(c.PublicURL, "/") + "/api/v1/auth/oidc/callback", Scopes: []string{coreoidc.ScopeOpenID, "profile"}}, verifier: provider.Verifier(&coreoidc.Config{ClientID: c.ClientID, SupportedSigningAlgs: []string{"RS256", "ES256"}}), client: client, revokeURL: metadata.RevocationEndpoint}, nil
}
func (p *Provider) AuthorizationURL(state, nonce, verifier string) string {
	return p.oauth.AuthCodeURL(state, coreoidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}
func (p *Provider) Exchange(ctx context.Context, code, verifier, nonce string) (authentication.Grant, error) {
	t, err := p.oauth.Exchange(coreoidc.ClientContext(ctx, p.client), code, oauth2.VerifierOption(verifier))
	if err != nil {
		log.Print("oidc: code exchange failed")
		return authentication.Grant{}, errProvider
	}
	return p.grant(ctx, t, nonce)
}
func (p *Provider) Refresh(ctx context.Context, refresh string) (authentication.Grant, error) {
	t, err := p.oauth.TokenSource(coreoidc.ClientContext(ctx, p.client), &oauth2.Token{RefreshToken: refresh}).Token()
	if err != nil {
		return authentication.Grant{}, errProvider
	}
	return p.grant(ctx, t, "")
}
func (p *Provider) grant(ctx context.Context, t *oauth2.Token, nonce string) (authentication.Grant, error) {
	raw, ok := t.Extra("id_token").(string)
	if !ok {
		log.Print("oidc: missing ID token")
		return authentication.Grant{}, errProvider
	}
	id, err := p.verifier.Verify(coreoidc.ClientContext(ctx, p.client), raw)
	if err != nil || id.Subject == "" || (nonce != "" && id.Nonce != nonce) {
		log.Print("oidc: ID token validation failed")
		return authentication.Grant{}, errProvider
	}
	var claims struct {
		Username       string `json:"preferred_username"`
		ResourceAccess map[string]struct {
			Roles []string `json:"roles"`
		} `json:"resource_access"`
	}
	if id.Claims(&claims) != nil {
		return authentication.Grant{}, errProvider
	}
	roles := claims.ResourceAccess[p.config.ClientID].Roles
	has := func(role string) bool {
		for _, r := range roles {
			if r == role {
				return true
			}
		}
		return false
	}
	var role identity.Role
	switch {
	case has(p.config.AdminRole):
		role = identity.RoleAdmin
	case has(p.config.EngineerRole):
		role = identity.RolePlatformEngineer
	case has(p.config.DeveloperRole):
		role = identity.RoleDeveloper
	default:
		log.Print("oidc: no mapped client role in ID token")
		return authentication.Grant{}, errProvider
	}
	expiry := id.Expiry
	if t.Expiry.Before(expiry) {
		expiry = t.Expiry
	}
	if t.RefreshToken == "" || !expiry.After(time.Now()) {
		log.Print("oidc: missing refresh token or expired grant")
		return authentication.Grant{}, errProvider
	}
	return authentication.Grant{Subject: id.Subject, Username: claims.Username, Role: role, RefreshToken: t.RefreshToken, ExpiresAt: expiry}, nil
}
func (p *Provider) Revoke(ctx context.Context, refresh string) error {
	form := url.Values{"client_id": {p.config.ClientID}, "client_secret": {p.config.ClientSecret}, "token": {refresh}, "token_type_hint": {"refresh_token"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errProvider
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return errProvider
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errProvider
	}
	return nil
}
