package awsaccess

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"orchestrator/internal/ports/terraformexec"
)

// credentialDocument is the AWS container credential protocol, private to the
// authenticated loopback bridge. It must never be persisted or logged.
type credentialDocument struct {
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string
	Token           string
	Expiration      time.Time
}

// StartCredentialEndpoint provides refreshable credentials to one local
// Terraform subprocess. Every request resolves and validates the pinned source.
func StartCredentialEndpoint(ctx context.Context, refresh func(context.Context) (terraformexec.AWSConnection, error)) (string, string, func() error, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", nil, ErrCall
	}
	token := hex.EncodeToString(raw[:])
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", "", nil, ErrCall
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet || r.URL.Path != "/credentials" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(token)) != 1 {
			http.Error(w, "unavailable", http.StatusForbidden)
			return
		}
		callCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		conn, err := refresh(callCtx)
		if err != nil || conn.AccessKeyID == "" || conn.SecretAccessKey == "" || conn.SessionToken == "" || !conn.Expiration.After(time.Now().Add(time.Minute)) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(credentialDocument{AccessKeyID: conn.AccessKeyID, SecretAccessKey: conn.SecretAccessKey, Token: conn.SessionToken, Expiration: conn.Expiration})
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-done:
		}
	}()
	return "http://" + listener.Addr().String() + "/credentials", token, server.Close, nil
}

// ReadCredentialEndpoint is used by readiness for the same invocation. Restrict
// even internally supplied URLs to loopback and never follow redirects/proxies.
func ReadCredentialEndpoint(ctx context.Context, uri, token, region string) (terraformexec.AWSConnection, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "/credentials" || u.RawQuery != "" || u.Fragment != "" || token == "" {
		return terraformexec.AWSConnection{}, ErrCall
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return terraformexec.AWSConnection{}, ErrCall
	}
	req.Header.Set("Authorization", token)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return terraformexec.AWSConnection{}, ErrCall
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return terraformexec.AWSConnection{}, ErrCall
	}
	var doc credentialDocument
	if err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 64<<10)).Decode(&doc); err != nil || doc.AccessKeyID == "" || doc.SecretAccessKey == "" || doc.Token == "" || !doc.Expiration.After(time.Now()) {
		return terraformexec.AWSConnection{}, errors.New("AWS invocation credential unavailable")
	}
	return terraformexec.AWSConnection{AccessKeyID: doc.AccessKeyID, SecretAccessKey: doc.SecretAccessKey, SessionToken: doc.Token, Expiration: doc.Expiration, Region: region}, nil
}
