package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
)

const maxResponseBytes = 8 << 20

type apiError struct {
	Status int
	Code   string
	Field  string
}

func (e *apiError) Error() string {
	message := fmt.Sprintf("orchestrator API returned status %d", e.Status)
	if e.Code != "" {
		message += " code=" + e.Code
	}
	if e.Field != "" {
		message += " field=" + e.Field
	}
	return message
}

type apiClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client

	mu       sync.Mutex
	signedIn bool
}

func newAPIClient(rawURL, username, password string) (*apiClient, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("ORCHESTRATOR_API_URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("ORCHESTRATOR_API_URL must not contain credentials, query or fragment")
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("ORCHESTRATOR_USERNAME and ORCHESTRATOR_PASSWORD are required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create session cookie jar: %w", err)
	}
	return &apiClient{
		baseURL:  strings.TrimRight(rawURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Jar: jar},
	}, nil
}

func (c *apiClient) do(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, int, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, 0, err
	}
	raw, status, err := c.request(ctx, method, path, query, body)
	if err == nil || status != http.StatusUnauthorized {
		return raw, status, err
	}
	c.mu.Lock()
	c.signedIn = false
	err = c.signInLocked(ctx)
	c.mu.Unlock()
	if err != nil {
		return nil, status, err
	}
	return c.request(ctx, method, path, query, body)
}

func (c *apiClient) ensureSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.signedIn {
		return nil
	}
	return c.signInLocked(ctx)
}

func (c *apiClient) signInLocked(ctx context.Context) error {
	_, status, err := c.request(ctx, http.MethodPost, "/api/v1/auth/sign-in", nil, map[string]string{
		"username": c.username,
		"password": c.password,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("orchestrator sign-in returned status %d", status)
	}
	c.signedIn = true
	return nil
}

func (c *apiClient) request(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, int, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("encode API request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, 0, fmt.Errorf("create API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("call orchestrator API: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read orchestrator API response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("orchestrator API response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, decodeAPIError(resp.StatusCode, raw)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	return raw, resp.StatusCode, nil
}

func decodeAPIError(status int, raw []byte) *apiError {
	var body struct {
		Code  string `json:"code"`
		Field string `json:"field"`
	}
	_ = json.Unmarshal(raw, &body)
	return &apiError{Status: status, Code: body.Code, Field: body.Field}
}
