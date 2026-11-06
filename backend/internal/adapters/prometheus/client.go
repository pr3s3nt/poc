// Package prometheus implements bounded read-only external monitoring queries.
// Application authorization and target ownership must be established by the caller.
package prometheus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrEndpoint    = errors.New("monitoring: invalid endpoint")
	ErrUnavailable = errors.New("monitoring: provider unavailable")
	ErrResponse    = errors.New("monitoring: invalid or incomplete provider response")
	ErrBounds      = errors.New("monitoring: query bounds exceeded")
)

const MaxResponse = 4 << 20
const MaxSeries = 2000
const MaxSamples = 240

// Config is private adapter input; never return it in public API responses.
type Config struct {
	Endpoint    string
	BearerToken string
	CA          []byte
}
type Client struct {
	base  *url.URL
	http  *http.Client
	token string
}

func Endpoint(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrEndpoint
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return nil, ErrEndpoint
		}
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if h == "metadata.google.internal" || h == "metadata" {
		return nil, ErrEndpoint
	}
	if ip, e := netip.ParseAddr(h); e == nil && blocked(ip) {
		return nil, ErrEndpoint
	}
	return u, nil
}
func blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

func New(cfg Config) (*Client, error) {
	u, e := Endpoint(cfg.Endpoint)
	if e != nil {
		return nil, e
	}
	if strings.ContainsAny(cfg.BearerToken, "\r\n") {
		return nil, ErrEndpoint
	}
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if len(cfg.CA) > 0 && !roots.AppendCertsFromPEM(cfg.CA) {
		return nil, ErrEndpoint
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, MaxIdleConns: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 8 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, ErrUnavailable
		}
		addresses, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(addresses) == 0 {
			return nil, ErrUnavailable
		}
		for _, ip := range addresses {
			if blocked(ip) {
				return nil, ErrEndpoint
			}
		}
		var last error
		for _, ip := range addresses {
			c, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		_ = last
		return nil, ErrUnavailable
	}
	return &Client{base: u, token: cfg.BearerToken, http: &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrEndpoint }}}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return ErrEndpoint
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, e := c.http.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, MaxResponse+1))
	if e != nil || len(b) > MaxResponse {
		return ErrResponse
	}
	if json.Unmarshal(b, out) != nil {
		return ErrResponse
	}
	return nil
}

type Sample struct {
	Timestamp time.Time `json:"timestamp"`
	Value     *float64  `json:"value"`
}

func (s *Sample) UnmarshalJSON(b []byte) error {
	var pair []json.RawMessage
	if json.Unmarshal(b, &pair) != nil || len(pair) != 2 {
		return ErrResponse
	}
	var t float64
	var value string
	if json.Unmarshal(pair[0], &t) != nil || json.Unmarshal(pair[1], &value) != nil || math.IsNaN(t) || math.IsInf(t, 0) || t < 0 || t > 253402300799 {
		return ErrResponse
	}
	v, e := strconv.ParseFloat(value, 64)
	if e != nil {
		return ErrResponse
	}
	s.Timestamp = time.Unix(int64(t), int64((t-math.Floor(t))*1e9)).UTC()
	if !math.IsNaN(v) && !math.IsInf(v, 0) {
		s.Value = &v
	}
	return nil
}

type Series struct {
	Labels map[string]string `json:"metric"`
	Values []Sample          `json:"values"`
	Value  *Sample           `json:"value,omitempty"`
}
type result struct {
	Status   string   `json:"status"`
	Warnings []string `json:"warnings"`
	Data     struct {
		Type   string   `json:"resultType"`
		Result []Series `json:"result"`
	} `json:"data"`
}

func validate(r result, kind string) ([]Series, error) {
	if r.Status != "success" || len(r.Warnings) > 0 || r.Data.Type != kind || len(r.Data.Result) > MaxSeries {
		return nil, ErrResponse
	}
	for _, s := range r.Data.Result {
		if len(s.Labels) > 64 || len(s.Values) > MaxSamples {
			return nil, ErrResponse
		}
		if kind == "vector" && s.Value == nil {
			return nil, ErrResponse
		}
		var prior time.Time
		for _, v := range s.Values {
			if !prior.IsZero() && !v.Timestamp.After(prior) {
				return nil, ErrResponse
			}
			prior = v.Timestamp
		}
	}
	return r.Data.Result, nil
}

// Range is private to the adapter/application boundary; public callers never supply PromQL.
func (c *Client) Range(ctx context.Context, query string, from, to time.Time, step time.Duration) ([]Series, error) {
	if query == "" || len(query) > 16384 || !from.Before(to) || to.Sub(from) > 15*24*time.Hour || step < 30*time.Second || step%time.Second != 0 || int64(to.Sub(from)/step)+1 > MaxSamples {
		return nil, ErrBounds
	}
	var r result
	e := c.get(ctx, "/api/v1/query_range", url.Values{"query": {query}, "start": {strconv.FormatFloat(float64(from.UnixNano())/1e9, 'f', 3, 64)}, "end": {strconv.FormatFloat(float64(to.UnixNano())/1e9, 'f', 3, 64)}, "step": {fmt.Sprint(int64(step / time.Second))}, "timeout": {"8s"}, "limit": {fmt.Sprint(MaxSeries + 1)}}, &r)
	if e != nil {
		return nil, e
	}
	return validate(r, "matrix")
}
func (c *Client) Instant(ctx context.Context, query string, at time.Time) ([]Series, error) {
	if query == "" || len(query) > 16384 {
		return nil, ErrBounds
	}
	var r result
	e := c.get(ctx, "/api/v1/query", url.Values{"query": {query}, "time": {strconv.FormatInt(at.Unix(), 10)}, "timeout": {"8s"}, "limit": {fmt.Sprint(MaxSeries + 1)}}, &r)
	if e != nil {
		return nil, e
	}
	return validate(r, "vector")
}

// Alert is a scoped rule evaluation, not a notification-delivery receipt.
type Alert struct {
	Name        string            `json:"name"`
	State       string            `json:"state"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	ActiveAt    *time.Time        `json:"activeAt,omitempty"`
	Value       string            `json:"value"`
}

func (c *Client) Alerts(ctx context.Context, scope map[string]string) ([]Alert, error) {
	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			Groups []struct {
				Rules []struct {
					Type   string            `json:"type"`
					Labels map[string]string `json:"labels"`
					Alerts []Alert           `json:"alerts"`
				} `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	if e := c.get(ctx, "/api/v1/rules", url.Values{"type": {"alert"}}, &envelope); e != nil {
		return nil, e
	}
	if envelope.Status != "success" {
		return nil, ErrResponse
	}
	out := []Alert{}
	for _, g := range envelope.Data.Groups {
		for _, r := range g.Rules {
			matches := true
			for k, v := range scope {
				if r.Labels[k] != v {
					matches = false
				}
			}
			if !matches {
				continue
			}
			for _, a := range r.Alerts {
				for k, v := range scope {
					if a.Labels[k] != v {
						return nil, ErrResponse
					}
				}
				if a.State != "pending" && a.State != "firing" {
					return nil, ErrResponse
				}
				a.Name = a.Labels["alertname"]
				out = append(out, a)
				if len(out) > MaxSeries {
					return nil, ErrResponse
				}
			}
		}
	}
	return out, nil
}

func (c *Client) AlertmanagerStatus(ctx context.Context) error {
	var status struct {
		VersionInfo struct {
			Version string `json:"version"`
		} `json:"versionInfo"`
	}
	if e := c.get(ctx, "/api/v2/status", nil, &status); e != nil {
		return e
	}
	if status.VersionInfo.Version == "" {
		return ErrResponse
	}
	return nil
}
