package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/credentials"
)

// ConnectionCredentials is the durable UC-04 Connection credential store. It
// writes one immutable KV v2 object per registration attempt under
// orchestrator/connections/<org>/<connection>/credentials/<object-id>, separate
// from the UC-12 orchestrator/apps/... namespace and workload ACLs. It never
// writes Vault policies or roles. The token is a scoped token read from a file
// by bootstrap; it is never logged or persisted.
type ConnectionCredentials struct {
	baseURL string
	token   string
	mount   string
	client  *http.Client
}

const credentialScheme = "kv2"

// NewConnectionCredentials validates the Vault address, token and mount.
func NewConnectionCredentials(baseURL, token, mount string, client *http.Client) (*ConnectionCredentials, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("vault: valid http(s) connection credential store address required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("vault: connection credential store token required")
	}
	if !credentials.ValidMount(mount) {
		return nil, fmt.Errorf("vault: invalid connection credential KV mount")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	// Never follow a redirect with the token to another destination.
	guarded := *client
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &ConnectionCredentials{baseURL: strings.TrimRight(baseURL, "/"), token: strings.TrimSpace(token), mount: mount, client: &guarded}, nil
}

func (s *ConnectionCredentials) Durable() bool { return true }

// Put creates the object with check-and-set 0, so an existing path is never
// overwritten. The reference is returned even on failure so the caller can
// delete a write whose response was lost.
func (s *ConnectionCredentials) Put(ctx context.Context, org, connection string, value []byte) (string, error) {
	if !credentials.ValidScope(org, connection) || len(value) == 0 {
		return "", credentials.ErrInvalidReference
	}
	objectID := ids.New()
	path := credentials.ObjectPath(org, connection, objectID)
	ref := credentials.Reference(credentialScheme, s.mount, org, connection, objectID)
	body, err := json.Marshal(map[string]any{
		"options": map[string]any{"cas": 0},
		"data":    map[string]string{"encoding": "base64", "value": base64.StdEncoding.EncodeToString(value)},
	})
	if err != nil {
		return "", fmt.Errorf("vault: encode connection credential")
	}
	status, err := s.do(ctx, http.MethodPost, "/v1/"+s.mount+"/data/"+path, body, nil)
	if err != nil {
		return ref, err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return ref, fmt.Errorf("%w: write status %d", credentials.ErrUnavailable, status)
	}
	return ref, nil
}

func (s *ConnectionCredentials) Get(ctx context.Context, org, connection, ref string) ([]byte, error) {
	path, err := credentials.ParseReference(ref, credentialScheme, s.mount, org, connection)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			Data struct {
				Encoding string  `json:"encoding"`
				Value    *string `json:"value"`
			} `json:"data"`
		} `json:"data"`
	}
	status, err := s.do(ctx, http.MethodGet, "/v1/"+s.mount+"/data/"+path, nil, &payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, credentials.ErrNotFound
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: read status %d", credentials.ErrUnavailable, status)
	}
	if payload.Data.Data.Value == nil || payload.Data.Data.Encoding != "base64" {
		return nil, fmt.Errorf("%w: invalid credential object", credentials.ErrUnavailable)
	}
	value, err := base64.StdEncoding.DecodeString(*payload.Data.Data.Value)
	if err != nil || len(value) == 0 {
		return nil, fmt.Errorf("%w: invalid credential object", credentials.ErrUnavailable)
	}
	return value, nil
}

// Delete removes the metadata and every version of the object, so a rolled
// back registration keeps no recoverable credential. A missing object is not
// an error.
func (s *ConnectionCredentials) Delete(ctx context.Context, org, connection, ref string) error {
	path, err := credentials.ParseReference(ref, credentialScheme, s.mount, org, connection)
	if err != nil {
		return err
	}
	status, err := s.do(ctx, http.MethodDelete, "/v1/"+s.mount+"/metadata/"+path, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("%w: delete status %d", credentials.ErrUnavailable, status)
	}
	return nil
}

// do performs one request. Transport errors are reduced to a fixed message:
// they can include the request URL, and remote bodies are never surfaced.
func (s *ConnectionCredentials) do(ctx context.Context, method, path string, body []byte, into any) (int, error) {
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
	limited := io.LimitReader(res.Body, 16<<20)
	if into != nil && res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(limited).Decode(into); err != nil {
			return 0, fmt.Errorf("%w: invalid response", credentials.ErrUnavailable)
		}
	} else {
		_, _ = io.Copy(io.Discard, limited)
	}
	return res.StatusCode, nil
}

var _ credentials.Store = (*ConnectionCredentials)(nil)
