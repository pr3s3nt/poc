package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"orchestrator/internal/domain/identity"
)

func TestProviderVerifiesTokensAndClientScopedRoles(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	claims := map[string]any{}
	var lastGrant, lastVerifier string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": server.URL, "authorization_endpoint": server.URL + "/auth", "token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys", "revocation_endpoint": server.URL + "/revoke", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			r.ParseForm()
			lastGrant = r.Form.Get("grant_type")
			lastVerifier = r.Form.Get("code_verifier")
			payload, _ := json.Marshal(claims)
			obj, _ := signer.Sign(payload)
			raw, _ := obj.CompactSerialize()
			json.NewEncoder(w).Encode(map[string]any{"access_token": "opaque-access", "refresh_token": "private-refresh", "expires_in": 120, "token_type": "Bearer", "id_token": raw})
		case "/revoke":
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	config := Config{Issuer: server.URL, ClientID: "platform", ClientSecret: "private", PublicURL: "http://console.test", AllowHTTP: true}
	p, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	authURL := p.AuthorizationURL("state", "nonce", "verifier")
	if !strings.Contains(authURL, "code_challenge_method=S256") || !strings.Contains(authURL, "nonce=nonce") {
		t.Fatal("PKCE/nonce missing")
	}
	reset := func() {
		claims = map[string]any{"iss": server.URL, "aud": "platform", "sub": "employee-id", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": "nonce", "preferred_username": "employee", "resource_access": map[string]any{"platform": map[string]any{"roles": []string{"developer"}}}}
	}
	reset()
	g, err := p.Exchange(context.Background(), "code", "verifier", "nonce")
	if err != nil || g.Role != identity.RoleDeveloper || lastGrant != "authorization_code" || lastVerifier != "verifier" {
		t.Fatal("exchange", err, g.Role)
	}
	reset()
	claims["resource_access"] = map[string]any{"platform": map[string]any{"roles": []string{"developer", "admin"}}}
	g, err = p.Refresh(context.Background(), "private-refresh")
	if err != nil || g.Role != identity.RoleAdmin || lastGrant != "refresh_token" {
		t.Fatal("refresh did not fetch current roles", err)
	}
	for _, kind := range []string{"issuer", "audience", "expired", "nonce", "other-client", "realm-admin", "unsigned"} {
		t.Run(kind, func(t *testing.T) {
			reset()
			switch kind {
			case "issuer":
				claims["iss"] = "https://attacker.test"
			case "audience":
				claims["aud"] = "other"
			case "expired":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "nonce":
				claims["nonce"] = "wrong"
			case "other-client":
				claims["resource_access"] = map[string]any{"other": map[string]any{"roles": []string{"admin"}}}
			case "realm-admin":
				delete(claims, "resource_access")
				claims["realm_access"] = map[string]any{"roles": []string{"admin"}}
			case "unsigned":
				claims["sub"] = ""
			}
			if _, err = p.Exchange(context.Background(), "code", "verifier", "nonce"); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	config.AllowHTTP = false
	if _, err = New(context.Background(), config); err == nil {
		t.Fatal("implicit HTTP accepted")
	}
}
