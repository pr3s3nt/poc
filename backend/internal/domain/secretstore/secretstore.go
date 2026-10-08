// Package secretstore models the Organization-scoped workload Secret Store
// Connection of ADR-012. It is separate from execution Connections: it holds
// only nonsecret addressing metadata, the opaque reference of a token kept in
// the platform credential store, and READY verification status.
package secretstore

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Provider names the first supported secret-store adapter.
type Provider string

// ProviderVaultKV2 is HashiCorp Vault KV version 2 with token authentication.
const ProviderVaultKV2 Provider = "VAULT_KV_V2"

// Status is the store state machine value.
type Status string

// Store states. A store is only persisted READY after verification.
const (
	StatusReady    Status = "READY"
	StatusRejected Status = "REJECTED"
)

// Default mounts of the first delivery.
const (
	DefaultKVMount   = "kv"
	DefaultAuthMount = "kubernetes"
)

var (
	// KeyPattern is the generated store key shape.
	KeyPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
	mountPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// Store is one registered Vault KV v2 workload secret store.
type Store struct {
	ID              string   `json:"id"`
	Key             string   `json:"key"`
	OrganizationKey string   `json:"organizationKey"`
	Name            string   `json:"name"`
	Provider        Provider `json:"provider"`
	BackendAddress  string   `json:"backendAddress"`
	WorkloadAddress string   `json:"workloadAddress"`
	Mount           string   `json:"mount"`
	AuthMount       string   `json:"authMount"`
	TLSCAPEM        string   `json:"tlsCaPem,omitempty"`
	CredentialRef   string   `json:"credentialRef,omitempty"`
	Status          Status   `json:"status"`
	// Legacy marks the explicit per-Organization platform store backfilled
	// from the flag-configured Vault of earlier releases.
	Legacy       bool           `json:"legacy,omitempty"`
	Verification map[string]any `json:"verification,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
}

// CredentialScope is the credential-store connection key of the token. It is
// prefixed so a store never shares a credential path with an execution
// Connection of the same key.
func (s Store) CredentialScope() string { return CredentialScope(s.Key) }

// CredentialScope derives the credential-store scope of a store key.
func CredentialScope(key string) string { return "ss-" + key }

// ValidMount reports whether a mount is a safe path segment.
func ValidMount(mount string) bool { return mountPattern.MatchString(mount) }

// ValidateAddress checks an http(s) address without credentials, query or fragment.
func ValidateAddress(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("secretstore: a valid http(s) address is required")
	}
	if strings.ContainsAny(raw, " \t\r\n\x00") {
		return fmt.Errorf("secretstore: address contains invalid characters")
	}
	return nil
}

// Validate reports whether the persisted record is well formed.
func (s Store) Validate() error {
	if !KeyPattern.MatchString(s.Key) || s.OrganizationKey == "" || strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("secretstore: key, organization and name are required")
	}
	if s.Provider != ProviderVaultKV2 {
		return fmt.Errorf("secretstore: unsupported provider")
	}
	if err := ValidateAddress(s.BackendAddress); err != nil {
		return err
	}
	if err := ValidateAddress(s.WorkloadAddress); err != nil {
		return err
	}
	if !ValidMount(s.Mount) || !ValidMount(s.AuthMount) {
		return fmt.Errorf("secretstore: invalid mount")
	}
	return nil
}
