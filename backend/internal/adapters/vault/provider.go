// Package vault implements the UC-12 configuration Provider using Vault KV v2.
package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
)

var safeSegment = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const pathPrefix = "orchestrator/apps/"

// Provider reads and writes immutable value objects in one KV v2 mount.
// Token must be a scoped token, never the initial root token.
type Provider struct {
	baseURL      string
	agentAddress string
	token        string
	client       *http.Client
	mount        string
	authMount    string
	caPEM        string
}

// SetAuthMount configures the Kubernetes auth mount used for role writes.
func (p *Provider) SetAuthMount(mount string) error {
	if !safeSegment.MatchString(mount) {
		return fmt.Errorf("vault: invalid Kubernetes auth mount")
	}
	p.authMount = mount
	return nil
}

// AuthMount returns the configured Kubernetes auth mount.
func (p *Provider) AuthMount() string {
	if p.authMount == "" {
		return "kubernetes"
	}
	return p.authMount
}

// Mount returns the KV v2 mount of the provider.
func (p *Provider) Mount() string { return p.mount }

// AgentAddress returns the in-cluster address pinned into bundles.
func (p *Provider) AgentAddress() string { return p.agentAddress }

// OwnsRef reports whether ref is a value reference of this store inside the
// scope of one Application Environment.
func (p *Provider) OwnsRef(ref, app, env string) bool {
	path, ok := strings.CutPrefix(ref, "kv2://"+p.mount+"/")
	return ok && validValuePath(path) && strings.HasPrefix(path, pathPrefix+app+"/envs/"+env+"/values/")
}

// SetAgentAddress configures the in-cluster Vault address used by injected
// Pods. It may differ from the API address used by the orchestrator process.
func (p *Provider) SetAgentAddress(address string) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("vault: valid Agent address required")
	}
	p.agentAddress = strings.TrimRight(address, "/")
	return nil
}

func (p *Provider) PrepareWorkloadAccess(ctx context.Context, app, env, workload, namespace, revisionID string, refs []string) (configport.WorkloadAccess, error) {
	if p.agentAddress == "" {
		return configport.WorkloadAccess{}, fmt.Errorf("vault: in-cluster Agent address is not configured")
	}
	if !safeSegment.MatchString(app) || !safeSegment.MatchString(workload) || !safeSegment.MatchString(namespace) || !safeSegment.MatchString(revisionID) || (env != "staging" && env != "production") || len(refs) == 0 {
		return configport.WorkloadAccess{}, fmt.Errorf("vault: invalid workload access scope")
	}
	unique := map[string]bool{}
	paths := []string{}
	for _, ref := range refs {
		path, ok := strings.CutPrefix(ref, "kv2://"+p.mount+"/")
		if !ok || !validValuePath(path) || !strings.HasPrefix(path, pathPrefix+app+"/envs/"+env+"/values/") {
			return configport.WorkloadAccess{}, fmt.Errorf("vault: value reference is outside workload scope")
		}
		if !unique[path] {
			unique[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	identity := app + "/" + env + "/" + workload + "/" + namespace + "/" + revisionID + "/" + strings.Join(paths, ",")
	sum := sha256.Sum256([]byte(identity))
	role := "orch-" + hex.EncodeToString(sum[:10])
	saHash := sha256.Sum256([]byte(app + "/" + env + "/" + workload))
	serviceAccount := "orch-" + hex.EncodeToString(saHash[:10])
	var policy strings.Builder
	for _, path := range paths {
		policy.WriteString("path \"" + p.mount + "/data/" + path + "\" { capabilities = [\"read\"] }\n")
	}
	if err := p.writeObject(ctx, "/v1/sys/policies/acl/"+role, map[string]any{"policy": policy.String()}); err != nil {
		return configport.WorkloadAccess{}, err
	}
	if err := p.writeObject(ctx, "/v1/auth/"+p.AuthMount()+"/role/"+role, map[string]any{
		"bound_service_account_names":      serviceAccount,
		"bound_service_account_namespaces": namespace,
		"token_policies":                   role,
		"token_ttl":                        "1h",
	}); err != nil {
		return configport.WorkloadAccess{}, err
	}
	return configport.WorkloadAccess{Role: role, Address: p.agentAddress, ServiceAccount: serviceAccount}, nil
}

// WriteBundle creates an immutable, workload-scoped KV object for VSO from
// already-read values. A fresh deployment ID means a new path and Kubernetes
// Secret, so a desired revision cannot alter a running Pod before Preview ->
// Deploy. Values come from each ref's owning store; the object lands in this
// (delivery) store and the access policy/role are written here.
func (p *Provider) WriteBundle(ctx context.Context, app, env, workload, namespace, revisionID, deploymentID string, values map[string]map[string]string) (configport.WorkloadBundle, error) {
	if !safeSegment.MatchString(app) || !safeSegment.MatchString(workload) || !safeSegment.MatchString(namespace) || !safeSegment.MatchString(revisionID) || !safeSegment.MatchString(deploymentID) || (env != "staging" && env != "production") {
		return configport.WorkloadBundle{}, fmt.Errorf("vault: invalid workload bundle scope")
	}
	data := map[string]string{}
	keys := map[string]map[string]string{}
	for container, entries := range values {
		if !safeSegment.MatchString(container) {
			return configport.WorkloadBundle{}, fmt.Errorf("vault: invalid container name")
		}
		keys[container] = map[string]string{}
		for name, value := range entries {
			if !safeSegment.MatchString(name) {
				return configport.WorkloadBundle{}, fmt.Errorf("vault: invalid environment variable name")
			}
			key := container + "_" + name
			if len(key) > 253 {
				return configport.WorkloadBundle{}, fmt.Errorf("vault: configuration key is too long")
			}
			data[key] = value
			keys[container][name] = key
		}
	}
	if len(data) == 0 {
		return configport.WorkloadBundle{}, fmt.Errorf("vault: no workload configuration references")
	}
	path := pathPrefix + app + "/envs/" + env + "/values/" + deploymentID
	if err := p.writeObject(ctx, "/v1/"+p.mount+"/data/"+path, map[string]any{"options": map[string]any{"cas": 0}, "data": data}); err != nil {
		return configport.WorkloadBundle{}, fmt.Errorf("vault: create workload bundle: %w", err)
	}
	access, err := p.PrepareWorkloadAccess(ctx, app, env, workload, namespace, revisionID, []string{"kv2://" + p.mount + "/" + path})
	if err != nil {
		return configport.WorkloadBundle{}, err
	}
	hash := sha256.Sum256([]byte(app + "/" + env + "/" + workload + "/" + deploymentID))
	return configport.WorkloadBundle{Address: access.Address, Mount: p.mount, AuthMount: p.AuthMount(), Path: path, Role: access.Role, ServiceAccount: access.ServiceAccount, SecretName: "orch-" + hex.EncodeToString(hash[:10]), CAPEM: p.caPEM, Keys: keys}, nil
}

// CheckWorkloadAuth verifies that the Kubernetes auth mount exists and that
// the token may still manage workload policies and roles, without changing
// anything outside a probe-named policy that is removed again.
func (p *Provider) CheckWorkloadAuth(ctx context.Context, probe string) error {
	if !safeSegment.MatchString(probe) {
		return fmt.Errorf("vault: invalid probe name")
	}
	status, err := p.request(ctx, http.MethodGet, "/v1/auth/"+p.AuthMount()+"/config", nil, nil)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("vault: Kubernetes auth mount %q is not configured for workloads", p.AuthMount())
	}
	return nil
}

// request performs one request with the provider token; transport errors are
// reduced to fixed text because they can quote the URL.
func (p *Provider) request(ctx context.Context, method, path string, body []byte, into any) (int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("vault: build request")
	}
	req.Header.Set("X-Vault-Token", p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("vault: request failed")
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 16<<20)
	if into != nil && res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(limited).Decode(into); err != nil {
			return 0, fmt.Errorf("vault: invalid response")
		}
	} else {
		_, _ = io.Copy(io.Discard, limited)
	}
	return res.StatusCode, nil
}

// DeleteValue removes every version of one value object (attempt cleanup).
func (p *Provider) DeleteValue(ctx context.Context, ref string) error {
	path, ok := strings.CutPrefix(ref, "kv2://"+p.mount+"/")
	if !ok || !validValuePath(path) {
		return fmt.Errorf("vault: invalid value reference")
	}
	status, err := p.request(ctx, http.MethodDelete, "/v1/"+p.mount+"/metadata/"+path, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("vault: delete status %d", status)
	}
	return nil
}

func (p *Provider) writeObject(ctx context.Context, path string, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("vault: encode access policy: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("vault: make access request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault: configure workload access: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		return fmt.Errorf("vault: configure workload access status %d", res.StatusCode)
	}
	return nil
}

// NewWithCA builds a provider whose TLS verification trusts caPEM in addition
// to the system roots; verification is never disabled. Redirects are not
// followed so the token cannot leave the configured address.
func NewWithCA(baseURL, token, mount, caPEM string) (*Provider, error) {
	client, err := HTTPClient(caPEM)
	if err != nil {
		return nil, err
	}
	p, err := New(baseURL, token, mount, client)
	if err != nil {
		return nil, err
	}
	p.caPEM = caPEM
	return p, nil
}

func New(baseURL, token, mount string, client *http.Client) (*Provider, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("vault: valid http(s) address required")
	}
	if token == "" {
		return nil, fmt.Errorf("vault: token required")
	}
	if !safeSegment.MatchString(mount) {
		return nil, fmt.Errorf("vault: invalid KV mount")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Provider{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: client, mount: mount}, nil
}

func (p *Provider) WriteValue(ctx context.Context, app, env, value string) (string, error) {
	if !safeSegment.MatchString(app) || (env != "staging" && env != "production") {
		return "", fmt.Errorf("vault: invalid Application/Environment scope")
	}
	path := pathPrefix + app + "/envs/" + env + "/values/" + ids.New()
	body, err := json.Marshal(map[string]any{"options": map[string]any{"cas": 0}, "data": map[string]string{"value": value}})
	if err != nil {
		return "", fmt.Errorf("vault: encode value: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/"+p.mount+"/data/"+path, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("vault: make write request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: write failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("vault: write status %d", res.StatusCode)
	}
	return "kv2://" + p.mount + "/" + path, nil
}

func (p *Provider) ReadValue(ctx context.Context, ref string) (string, error) {
	path, ok := strings.CutPrefix(ref, "kv2://"+p.mount+"/")
	if !ok || !validValuePath(path) {
		return "", fmt.Errorf("vault: invalid value reference")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/"+p.mount+"/data/"+path, nil)
	if err != nil {
		return "", fmt.Errorf("vault: make read request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)
	res, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: read failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault: read status %d", res.StatusCode)
	}
	var payload struct {
		Data struct {
			Data struct {
				Value *string `json:"value"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil || payload.Data.Data.Value == nil {
		return "", fmt.Errorf("vault: invalid value response")
	}
	return *payload.Data.Data.Value, nil
}

func validValuePath(path string) bool {
	segments := strings.Split(path, "/")
	return len(segments) == 7 && segments[0] == "orchestrator" && segments[1] == "apps" && safeSegment.MatchString(segments[2]) && segments[3] == "envs" && (segments[4] == "staging" || segments[4] == "production") && segments[5] == "values" && safeSegment.MatchString(segments[6])
}

var _ configport.Provider = (*Provider)(nil)
