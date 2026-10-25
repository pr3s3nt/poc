package vault

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"time"

	"orchestrator/internal/adapters/tlsprobe"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/ports/credentials"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// StoreInspector reads real, safe evidence about a registered workload secret
// store: its TLS trust and certificate expiry, whether Vault is initialized and
// unsealed, and whether the saved platform token is still accepted. It reads
// no workload secret value. The workload-side address and the in-cluster auth
// path are outside what the platform runtime can observe.
type StoreInspector struct {
	Stores interface {
		GetSecretStore(ctx context.Context, organizationKey, key string) (secretstore.Store, error)
	}
	Credentials credentials.Store
}

var _ probes.StoreInspector = StoreInspector{}

// Inspect implements probes.StoreInspector.
func (i StoreInspector) Inspect(ctx context.Context, org, key string) domain.Finding {
	rec, err := i.Stores.GetSecretStore(ctx, org, key)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "SECRET_STORE_NOT_FOUND", Remediation: "select-secret-store"}
		}
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_READ_FAILED", Remediation: "rerun"}
	}
	if rec.Status != secretstore.StatusReady {
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "SECRET_STORE_NOT_READY", Remediation: "verify-secret-store"}
	}
	address, err := url.Parse(rec.BackendAddress)
	if err != nil || address.Host == "" {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_ADDRESS_INVALID"}
	}
	var roots *x509.CertPool
	if rec.TLSCAPEM != "" {
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(rec.TLSCAPEM)) {
			return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_CA_INVALID"}
		}
	}
	facts := map[string]string{}
	if address.Scheme == "https" {
		port := address.Port()
		if port == "" {
			port = "443"
		}
		tlsResult := tlsprobe.Check(ctx, address.Hostname()+":"+port, address.Hostname(), roots, false)
		if tlsResult.Verdict != domain.VerdictPass {
			return tlsResult
		}
		for k, v := range tlsResult.Facts {
			if k == "notAfter" || k == "daysRemaining" {
				facts[k] = v
			}
		}
	}
	token := ""
	if i.Credentials != nil && rec.CredentialRef != "" {
		if raw, err := i.Credentials.Get(ctx, org, rec.CredentialScope(), rec.CredentialRef); err == nil {
			token = string(raw)
		}
	}
	if token == "" {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_TOKEN_UNAVAILABLE", Facts: facts}
	}
	p, err := NewWithCA(rec.BackendAddress, token, rec.Mount, rec.TLSCAPEM)
	if err != nil {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_ADDRESS_INVALID", Facts: facts}
	}
	wait, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// Vault answers its health endpoint with the state in the status code.
	status, err := p.request(wait, http.MethodGet, "/v1/sys/health?standbyok=true", nil, nil)
	if err != nil {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_UNREACHABLE", Remediation: "check-endpoint", Facts: facts}
	}
	switch status {
	case http.StatusServiceUnavailable:
		facts["sealed"] = "true"
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "STORE_SEALED", Remediation: "unseal-store", Facts: facts}
	case http.StatusNotImplemented:
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "STORE_UNINITIALIZED", Remediation: "initialize-store", Facts: facts}
	case http.StatusOK, http.StatusTooManyRequests, 472, 473:
	default:
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_STATE_UNKNOWN", Facts: facts}
	}
	var self struct {
		Data struct {
			ExpireTime string `json:"expire_time"`
		} `json:"data"`
	}
	status, err = p.request(wait, http.MethodGet, "/v1/auth/token/lookup-self", nil, &self)
	switch {
	case err != nil:
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_UNREACHABLE", Remediation: "check-endpoint", Facts: facts}
	case status != http.StatusOK:
		facts["tokenAccepted"] = "false"
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "STORE_TOKEN_REJECTED", Remediation: "replace-store-token", Facts: facts}
	}
	facts["tokenAccepted"] = "true"
	if at, err := time.Parse(time.RFC3339, self.Data.ExpireTime); err == nil {
		facts["daysRemaining"] = domain.Itoa(int(time.Until(at).Hours() / 24))
	}
	// An accepted token is not a usable store: the mount must be KV v2 and the
	// token must hold the capabilities the product needs on the configured
	// paths. This reads capabilities and mount metadata only; it creates no
	// policy, role or value and reads no secret.
	var mountInfo struct {
		Data struct {
			Type    string            `json:"type"`
			Options map[string]string `json:"options"`
		} `json:"data"`
	}
	status, err = p.request(wait, http.MethodGet, "/v1/sys/internal/ui/mounts/"+rec.Mount, nil, &mountInfo)
	switch {
	case err != nil:
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_UNREACHABLE", Remediation: "check-endpoint", Facts: facts}
	case status != http.StatusOK:
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_CAPABILITIES_UNOBSERVED", Facts: facts}
	case (mountInfo.Data.Type != "kv" && mountInfo.Data.Type != "generic") || mountInfo.Data.Options["version"] != "2":
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "STORE_NOT_KV2", Remediation: "fix-store-mount", Facts: facts}
	}
	required := requiredCapabilities(rec.Mount, rec.AuthMount, "diagnostic")
	paths := make([]string, 0, len(required))
	for path := range required {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	body, _ := json.Marshal(map[string]any{"paths": paths})
	var caps map[string]any
	status, err = p.request(wait, http.MethodPost, "/v1/sys/capabilities-self", body, &caps)
	if err != nil || status != http.StatusOK {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_CAPABILITIES_UNOBSERVED", Facts: facts}
	}
	missing := missingCapabilities(caps, required)
	facts["allowed"], facts["denied"] = domain.Itoa(len(required)-len(missing)), domain.Itoa(len(missing))
	if len(missing) > 0 {
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "STORE_CAPABILITY_MISSING", Remediation: "grant-store-capabilities", Facts: facts}
	}
	// The workload login path (Kubernetes auth config) is read when the token
	// may; what the pods can reach from inside the cluster is never observable
	// from here.
	if s, err := p.request(wait, http.MethodGet, "/v1/auth/"+rec.AuthMount+"/config", nil, nil); err != nil || s != http.StatusOK {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_AUTH_UNOBSERVED", Facts: facts}
	}
	facts["authConfigured"] = "true"
	return domain.Finding{Verdict: domain.VerdictPass, Reason: "STORE_AVAILABLE", Facts: facts}
}
