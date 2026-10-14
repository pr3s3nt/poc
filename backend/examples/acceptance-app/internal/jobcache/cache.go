package jobcache

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"orchestrator/examples/acceptance-app/internal/jobstore"
)

const opTimeout = 2 * time.Second

// Reason is a safe, closed classification of a cache failure. It is the only
// description of a Redis error that may reach logs, HTTP bodies or evidence.
type Reason string

const (
	ReasonNone        Reason = ""
	ReasonUnreachable Reason = "unreachable"
	ReasonAuth        Reason = "auth"
	ReasonTLS         Reason = "tls"
	ReasonTimeout     Reason = "timeout"
	ReasonConfig      Reason = "config"
	ReasonInvalid     Reason = "invalid-entry"
	ReasonOther       Reason = "error"
)

// Classify maps an error to a Reason without exposing its text.
func Classify(err error) Reason {
	if err == nil {
		return ReasonNone
	}
	var (
		unknownAuth x509.UnknownAuthorityError
		hostname    x509.HostnameError
		invalid     x509.CertificateInvalidError
		recordHdr   tls.RecordHeaderError
		certVerify  *tls.CertificateVerificationError
	)
	switch {
	case errors.Is(err, ErrConfig):
		return ReasonConfig
	case errors.As(err, &unknownAuth), errors.As(err, &hostname), errors.As(err, &invalid),
		errors.As(err, &recordHdr), errors.As(err, &certVerify):
		return ReasonTLS
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "tls:"), strings.Contains(msg, "x509:"), strings.Contains(msg, "certificate"):
		return ReasonTLS
	case strings.Contains(msg, "wrongpass"), strings.Contains(msg, "noauth"),
		strings.Contains(msg, "invalid password"), strings.Contains(msg, "invalid username-password"),
		strings.Contains(msg, "without any password configured"), strings.Contains(msg, "noperm"):
		return ReasonAuth
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ReasonTimeout
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ReasonUnreachable
	}
	if strings.Contains(msg, "eof") || strings.Contains(msg, "connection") || strings.Contains(msg, "refused") {
		return ReasonUnreachable
	}
	return ReasonOther
}

// Cache stores completed jobs in Redis.
type Cache struct {
	cfg    Config
	client *redis.Client
}

// Option customizes New. It exists for tests: verification of the server
// certificate always runs, only the trusted roots can be replaced.
type Option func(*options)

type options struct{ roots *x509.CertPool }

// WithRootCAs replaces the trusted roots used to verify a TLS Redis server.
func WithRootCAs(pool *x509.CertPool) Option { return func(o *options) { o.roots = pool } }

// New builds the client. It does not connect. Operations are not retried
// implicitly: a failure is reported once.
func New(cfg Config, opts ...Option) *Cache {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	ro := &redis.Options{
		Addr:            cfg.Addr(),
		Username:        cfg.Username,
		Password:        cfg.Password,
		DialTimeout:     opTimeout,
		ReadTimeout:     opTimeout,
		WriteTimeout:    opTimeout,
		MaxRetries:      -1,
		PoolSize:        4,
		DisableIdentity: true,
	}
	if cfg.TLS {
		// ServerName drives both SNI and the hostname check. Roots default
		// to the system pool, which falls back to the embedded public roots.
		ro.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Host, RootCAs: o.roots}
	}
	return &Cache{cfg: cfg, client: redis.NewClient(ro)}
}

// Config returns the binding the cache was built from.
func (c *Cache) Config() Config { return c.cfg }

// Close releases the connection pool.
func (c *Cache) Close() error { return c.client.Close() }

// Ping verifies reachability, TLS and authentication.
func (c *Cache) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return c.client.Ping(ctx).Err()
}

// Probe exercises the operations real lookups need (SET with TTL, GET, DEL) on
// a throwaway key in the cache scope. PING alone can succeed while an ACL denies
// these commands. The key expires on its own if DEL is not permitted.
func (c *Cache) Probe(ctx context.Context) error {
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	key := c.cfg.ProbeKey(hex.EncodeToString(rnd[:]))
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	ttl := c.cfg.TTL
	if ttl > 10*time.Second {
		ttl = 10 * time.Second
	}
	if err := c.client.Set(ctx, key, "ok", ttl).Err(); err != nil {
		return err
	}
	if got, err := c.client.Get(ctx, key).Result(); err != nil || got != "ok" {
		if err == nil {
			err = ErrInvalidEntry
		}
		return err
	}
	return c.client.Del(ctx, key).Err()
}

// Put writes a completed job with the configured TTL. Only completed jobs are
// cached, so a pending job is never frozen by a cache entry.
func (c *Cache) Put(ctx context.Context, job jobstore.Job) error {
	if job.Status != jobstore.StatusDone {
		return fmt.Errorf("jobcache: only completed jobs are cached")
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("jobcache: encode: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return c.client.Set(ctx, c.cfg.KeyFor(job.ID), raw, c.cfg.TTL).Err()
}

// ErrInvalidEntry reports a cached value that is not a completed job of the
// requested id. It is handled like a miss that overwrites the entry.
var ErrInvalidEntry = errors.New("jobcache: invalid cache entry")

// Get returns the cached completed job. found=false with a nil error is a
// cache miss; a non-nil error is a cache failure and never a miss.
func (c *Cache) Get(ctx context.Context, id int64) (job jobstore.Job, found bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	raw, err := c.client.Get(ctx, c.cfg.KeyFor(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return jobstore.Job{}, false, nil
	}
	if err != nil {
		return jobstore.Job{}, false, err
	}
	if err := json.Unmarshal(raw, &job); err != nil || job.ID != id || job.Status != jobstore.StatusDone {
		return jobstore.Job{}, false, ErrInvalidEntry
	}
	return job, true, nil
}
