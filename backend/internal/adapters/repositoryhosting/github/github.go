// Package github provisions private, app-scoped deployment repositories.
package github

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/application/gitaccess"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
	port "orchestrator/internal/ports/repositoryhosting"
)

var ErrUnavailable = port.ErrUnavailable
var ErrIdentity = port.ErrIdentity

type Factory struct {
	Credentials   credentials.Store
	Organizations persistence.OrganizationIdentityReader
	Client        *http.Client
}

func (f *Factory) Open(ctx context.Context, b domain.BindingVersion, p domain.RepositoryPolicy) (port.Host, error) {
	if p.Validate() != nil || f.Credentials == nil || !f.Credentials.Durable() || f.Organizations == nil || b.WriterCredentialRef == "" {
		return nil, ErrUnavailable
	}
	org, err := f.Organizations.GetOrganizationByID(ctx, b.OrganizationID)
	if err != nil {
		return nil, ErrUnavailable
	}
	var raw []byte
	caPEM := b.Config["caBundle"]
	if b.WriterCredentialRef == gitaccess.PlatformWriterReference {
		settings, _ := f.Organizations.(persistence.GitSettingsRepository)
		destination := strings.TrimRight(b.Config["repositoryHost"], "/") + "/" + b.Config["repositoryOwner"]
		raw, caPEM, err = gitaccess.Writer(ctx, settings, f.Credentials, org.Key, destination)
	} else if strings.HasPrefix(b.WriterCredentialRef, gitaccess.ConnectionWriterPrefix) {
		conns, ok := f.Organizations.(gitaccess.ConnectionReader)
		if !ok {
			return nil, ErrUnavailable
		}
		raw, err = gitaccess.ConnectionCredential(ctx, conns, f.Credentials, org.Key, b.WriterCredentialRef, strings.TrimRight(p.CloneBase, "/")+"/"+p.Owner)
	} else {
		raw, err = f.Credentials.Get(ctx, org.Key, b.ID, b.WriterCredentialRef)
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	var secret struct{ Username, Password string }
	if json.Unmarshal(raw, &secret) != nil || secret.Password == "" || strings.ContainsAny(secret.Password, "\x00\r\n") {
		return nil, ErrUnavailable
	}

	client, err := repositoryHTTPClient(f.Client, caPEM)
	if err != nil {
		return nil, err
	}
	return &Host{policy: p, token: secret.Password, client: client}, nil

}

// Trust the declared enterprise CA without disabling certificate verification or
// forwarding credentials on redirects. Clone transports so callers stay unchanged.
func repositoryHTTPClient(base *http.Client, caPEM string) (*http.Client, error) {
	client := http.Client{Timeout: 20 * time.Second}
	if base != nil {
		client = *base
	}
	if caPEM != "" {
		transport := client.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		original, ok := transport.(*http.Transport)
		if !ok {
			return nil, ErrUnavailable
		}
		cloned := original.Clone()
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if cloned.TLSClientConfig != nil && cloned.TLSClientConfig.RootCAs != nil {
			roots = cloned.TLSClientConfig.RootCAs.Clone()
		}
		if !roots.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, ErrUnavailable
		}
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if cloned.TLSClientConfig != nil {
			tlsConfig = cloned.TLSClientConfig.Clone()
		}
		tlsConfig.RootCAs = roots
		tlsConfig.InsecureSkipVerify = false
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
		cloned.TLSClientConfig = tlsConfig
		client.Transport = cloned
	}
	if client.Timeout <= 0 {
		client.Timeout = 20 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client, nil
}

type Host struct {
	policy domain.RepositoryPolicy
	token  string
	client *http.Client
}
type owner struct{ Login, Type string }
type repository struct {
	ID                                                   int64 `json:"id"`
	Name, FullName, CloneURL, DefaultBranch, Description string
	Private, Archived                                    bool
}

func (r *repository) UnmarshalJSON(raw []byte) error {
	var v struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
		Description   string `json:"description"`
		Private       bool   `json:"private"`
		Archived      bool   `json:"archived"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*r = repository{v.ID, v.Name, v.FullName, v.CloneURL, v.DefaultBranch, v.Description, v.Private, v.Archived}
	return nil
}
func (h *Host) request(ctx context.Context, method, path string, body any, into any) (int, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, ErrUnavailable
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(h.policy.APIBase, "/")+path, bytes.NewReader(data))
	if err != nil {
		return 0, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(req)
	if err != nil {
		return 0, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 && into != nil {
		if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(into) != nil {
			return response.StatusCode, ErrUnavailable
		}
	}
	return response.StatusCode, nil
}
func (h *Host) owner(ctx context.Context) (owner, error) {
	var identity owner
	status, err := h.request(ctx, "GET", "/user", nil, &identity)
	if err != nil || status != 200 || identity.Login == "" {
		return owner{}, ErrUnavailable
	}
	var target owner
	status, err = h.request(ctx, "GET", "/users/"+url.PathEscape(h.policy.Owner), nil, &target)
	if err != nil || status != 200 || !strings.EqualFold(target.Login, h.policy.Owner) {
		return owner{}, ErrUnavailable
	}
	if target.Type == "User" {
		if !strings.EqualFold(target.Login, identity.Login) {
			return owner{}, ErrUnavailable
		}
		return target, nil
	}
	if target.Type != "Organization" {
		return owner{}, ErrUnavailable
	}
	var membership struct{ State string }
	status, err = h.request(ctx, "GET", "/user/memberships/orgs/"+url.PathEscape(target.Login), nil, &membership)
	if err != nil || status != 200 || membership.State != "active" {
		return owner{}, ErrUnavailable
	}
	return target, nil
}
func (h *Host) Verify(ctx context.Context) error { _, err := h.owner(ctx); return err }
func (h *Host) repoPath(r domain.ApplicationRepository) string {
	return "/repos/" + url.PathEscape(h.policy.Owner) + "/" + url.PathEscape(r.Name)
}
func (h *Host) validate(r repository, in domain.ApplicationRepository) (domain.ApplicationRepository, error) {
	expected := strings.TrimRight(h.policy.CloneBase, "/") + "/" + h.policy.Owner + "/" + in.Name + ".git"
	if r.ID < 1 || !r.Private || r.Archived || r.Description != in.Marker || !strings.EqualFold(r.FullName, h.policy.Owner+"/"+in.Name) || !strings.EqualFold(r.Name, in.Name) || !strings.EqualFold(r.CloneURL, expected) || r.DefaultBranch == "" {
		return in, ErrIdentity
	}
	if in.Status == "READY" && (in.RemoteID != strconv.FormatInt(r.ID, 10) || in.CloneURL != r.CloneURL || in.Branch != r.DefaultBranch) {
		return in, ErrIdentity
	}
	in.Status = "READY"
	in.RemoteID = strconv.FormatInt(r.ID, 10)
	in.CloneURL = r.CloneURL
	in.Branch = r.DefaultBranch
	return in, in.Validate()
}
func (h *Host) Ensure(ctx context.Context, in domain.ApplicationRepository) (domain.ApplicationRepository, error) {
	if in.Validate() != nil {
		return in, ErrIdentity
	}
	var remote repository
	status, err := h.request(ctx, "GET", h.repoPath(in), nil, &remote)
	if err != nil {
		return in, err
	}
	if status == 200 {
		return h.validate(remote, in)
	}
	if status != 404 {
		return in, ErrUnavailable
	}
	if in.Status == "READY" {
		return in, ErrIdentity
	}
	target, err := h.owner(ctx)
	if err != nil {
		return in, err
	}
	endpoint := "/user/repos"
	if target.Type == "Organization" {
		endpoint = "/orgs/" + url.PathEscape(target.Login) + "/repos"
	}
	// Always observe after creation, even after a lost response or a concurrent
	// 422. The durable allocation marker and GitHub's name uniqueness prevent
	// duplicate repositories and forbid takeover of a foreign same-name repo.
	_, createErr := h.request(ctx, "POST", endpoint, map[string]any{"name": in.Name, "description": in.Marker, "private": true, "auto_init": true}, nil)
	for attempt := 0; attempt < 5; attempt++ {
		remote = repository{}
		status, err = h.request(ctx, "GET", h.repoPath(in), nil, &remote)
		if err == nil && status == 200 && remote.DefaultBranch != "" {
			return h.validate(remote, in)
		}
		if status != 404 && status != 200 {
			break
		}
		select {
		case <-ctx.Done():
			return in, ErrUnavailable
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = createErr
	return in, ErrUnavailable
}
