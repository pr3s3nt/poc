package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/execution"
)

// OutputSecrets is the durable UC-08 output-secret store: Terraform secret
// outputs and executor-generated passwords live in Vault KV v2 under
// orchestrator/outputs/<name-digest>/<object-id>, create-only (CAS 0) and
// separate from the UC-04 Connection credential namespace and the UC-12
// workload namespace. The token is scoped to that path only. References are
// opaque, stable across process restarts and never contain the value.
type OutputSecrets struct {
	baseURL string
	token   string
	mount   string
	client  *http.Client
}

const outputsPrefix = "orchestrator/outputs/"

var outputObject = regexp.MustCompile(`^[0-9a-f]{16}/[0-9a-zA-Z-]{8,64}$`)

// NewOutputSecrets validates the Vault address, token and mount.
func NewOutputSecrets(baseURL, token, mount string, client *http.Client) (*OutputSecrets, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("vault: valid http(s) output secret store address required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("vault: output secret store token required")
	}
	if !credentials.ValidMount(mount) {
		return nil, fmt.Errorf("vault: invalid output secret KV mount")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	guarded := *client
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &OutputSecrets{baseURL: strings.TrimRight(baseURL, "/"), token: strings.TrimSpace(token), mount: mount, client: &guarded}, nil
}

// Durable reports that references survive a process restart.
func (s *OutputSecrets) Durable() bool { return true }

func (s *OutputSecrets) refPrefix() string { return "secret://vault/" + s.mount + "/" + outputsPrefix }

// Put writes a new immutable object and returns its reference. A failed or
// ambiguous write returns an error and no reference is ever recorded.
func (s *OutputSecrets) Put(ctx context.Context, name string, value string) (string, error) {
	if name == "" || value == "" {
		return "", credentials.ErrInvalidReference
	}
	sum := sha256.Sum256([]byte(name))
	object := hex.EncodeToString(sum[:8]) + "/" + ids.New()
	body, err := json.Marshal(map[string]any{
		"options": map[string]any{"cas": 0},
		"data":    map[string]string{"encoding": "base64", "value": base64.StdEncoding.EncodeToString([]byte(value))},
	})
	if err != nil {
		return "", fmt.Errorf("vault: encode output secret")
	}
	status, err := s.do(ctx, http.MethodPost, "/v1/"+s.mount+"/data/"+outputsPrefix+object, body, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return "", fmt.Errorf("%w: output secret write status %d", credentials.ErrUnavailable, status)
	}
	return s.refPrefix() + object, nil
}

// Get resolves a reference this store issued. Anything else (another scheme,
// mount or path, traversal) is simply not found; Vault outages are also not
// found, so a caller never mistakes an outage for an empty secret.
func (s *OutputSecrets) Get(ctx context.Context, ref string) (string, bool) {
	prefix := s.refPrefix()
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	object := strings.TrimPrefix(ref, prefix)
	if !outputObject.MatchString(object) {
		return "", false
	}
	var payload struct {
		Data struct {
			Data struct {
				Encoding string  `json:"encoding"`
				Value    *string `json:"value"`
			} `json:"data"`
		} `json:"data"`
	}
	status, err := s.do(ctx, http.MethodGet, "/v1/"+s.mount+"/data/"+outputsPrefix+object, nil, &payload)
	if err != nil || status != http.StatusOK || payload.Data.Data.Value == nil || payload.Data.Data.Encoding != "base64" {
		return "", false
	}
	value, err := base64.StdEncoding.DecodeString(*payload.Data.Data.Value)
	if err != nil || len(value) == 0 {
		return "", false
	}
	return string(value), true
}

func (s *OutputSecrets) do(ctx context.Context, method, path string, body []byte, into any) (int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("%w: build request", credentials.ErrUnavailable)
	}
	req.Header.Set("X-Vault-Token", s.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: request failed", credentials.ErrUnavailable)
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 1<<20)
	if into != nil && res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(limited).Decode(into); err != nil {
			return 0, fmt.Errorf("%w: invalid response", credentials.ErrUnavailable)
		}
	} else {
		_, _ = io.Copy(io.Discard, limited)
	}
	return res.StatusCode, nil
}

var _ execution.SecretStore = (*OutputSecrets)(nil)
