package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	oidcadapter "orchestrator/internal/adapters/oidc"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/ports/persistence"
)

// OIDCConfig is read from an owner-only file; its client secret never enters logs.
type OIDCConfig struct {
	Issuer                 string `json:"issuer"`
	ClientID               string `json:"clientId"`
	ClientSecret           string `json:"clientSecret"`
	PublicURL              string `json:"publicUrl"`
	OrganizationKey        string `json:"organizationKey"`
	DeveloperRole          string `json:"developerRole"`
	EngineerRole           string `json:"engineerRole"`
	AdminRole              string `json:"adminRole"`
	LocalTestAdminUsername string `json:"localTestAdminUsername"`
	AllowHTTP              bool   `json:"allowHttp"`
}

func configureOIDC(ctx context.Context, st persistence.Store, auth *authentication.Service, file string) error {
	if file == "" {
		return nil
	}
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("bootstrap: OIDC configuration must be an owner-only regular file")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return errors.New("bootstrap: cannot read OIDC configuration")
	}
	var c OIDCConfig
	if json.Unmarshal(raw, &c) != nil || c.OrganizationKey == "" {
		return errors.New("bootstrap: invalid OIDC configuration")
	}
	if _, err = st.GetOrganization(ctx, c.OrganizationKey); err != nil {
		return errors.New("bootstrap: OIDC Organization must already exist")
	}
	p, err := oidcadapter.New(ctx, oidcadapter.Config{Issuer: c.Issuer, ClientID: c.ClientID, ClientSecret: c.ClientSecret, PublicURL: c.PublicURL, DeveloperRole: c.DeveloperRole, EngineerRole: c.EngineerRole, AdminRole: c.AdminRole, AllowHTTP: c.AllowHTTP})
	if err != nil {
		return err
	}
	auth.EnableOIDC(&authentication.Federation{Provider: p, Issuer: c.Issuer, OrganizationKey: c.OrganizationKey, Secure: strings.HasPrefix(c.PublicURL, "https://"), LocalTestAdminUsername: strings.ToLower(strings.TrimSpace(c.LocalTestAdminUsername))})
	return nil
}
