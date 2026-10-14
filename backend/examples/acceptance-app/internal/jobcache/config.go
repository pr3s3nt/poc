// Package jobcache is the Redis read-through/write-through cache of completed
// acceptance-app job results. PostgreSQL stays the durable authority: the
// worker commits a result there first and only then writes it through to
// Redis, and the backend reads Redis first and falls back to the same
// PostgreSQL row on a miss. Redis is a required dependency, so a cache failure
// is always reported; it is never silently absorbed by the database fallback.
package jobcache

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	// Embed the Mozilla public CA roots as the crypto/x509 fallback. The
	// workloads run in scratch images without /etc/ssl, and ElastiCache
	// certificates chain to a public root, so verification needs these roots.
	_ "golang.org/x/crypto/x509roots/fallback"
)

// Environment variable names of the Redis binding and the cache scope. The
// orchestrator injects the REDIS_* values from the redis resource outputs
// (host, port, username, password, tls).
const (
	EnvHost     = "REDIS_HOST"
	EnvPort     = "REDIS_PORT"
	EnvUsername = "REDIS_USERNAME"
	EnvPassword = "REDIS_PASSWORD"
	EnvTLS      = "REDIS_TLS"

	EnvApp        = "ACCEPTANCE_CACHE_APP"
	EnvEnv        = "ACCEPTANCE_CACHE_ENV"
	EnvRun        = "ACCEPTANCE_CACHE_RUN"
	EnvTTLSeconds = "ACCEPTANCE_CACHE_TTL_SECONDS"
)

const (
	// DefaultTTLSeconds is the cache entry lifetime when none is configured.
	DefaultTTLSeconds = 30
	// MaxTTLSeconds bounds the configurable lifetime so a short deterministic
	// expiry stays short.
	MaxTTLSeconds = 3600

	maxHostLen     = 253
	maxCredLen     = 512
	keyPartPattern = `^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`
)

var (
	keyPart  = regexp.MustCompile(keyPartPattern)
	hostPart = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]+$`)
)

// ErrConfig is wrapped by every configuration error. Messages name the
// offending variable but never contain its value.
var ErrConfig = errors.New("jobcache: invalid configuration")

// Config is the typed Redis binding plus the cache key scope and TTL.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      bool

	App string
	Env string
	Run string
	TTL time.Duration
}

// Addr returns host:port with IPv6 literals bracketed.
func (c Config) Addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

// ConfigFromEnv reads and validates the binding from the process environment.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	bad := func(name, why string) (Config, error) {
		return Config{}, fmt.Errorf("%w: %s %s", ErrConfig, name, why)
	}
	var cfg Config

	cfg.Host = getenv(EnvHost)
	if cfg.Host == "" {
		return bad(EnvHost, "is required")
	}
	if len(cfg.Host) > maxHostLen || !hostPart.MatchString(cfg.Host) {
		return bad(EnvHost, "is not a valid host name")
	}

	port, err := strconv.Atoi(getenv(EnvPort))
	if getenv(EnvPort) == "" {
		return bad(EnvPort, "is required")
	}
	if err != nil || port < 1 || port > 65535 {
		return bad(EnvPort, "must be a port number")
	}
	cfg.Port = port

	// Redis v2 always authenticates, in-cluster and managed.
	cfg.Username = getenv(EnvUsername)
	cfg.Password = getenv(EnvPassword)
	for _, f := range []struct{ name, value string }{{EnvUsername, cfg.Username}, {EnvPassword, cfg.Password}} {
		if f.value == "" {
			return bad(f.name, "is required because Redis authentication is mandatory")
		}
		if len(f.value) > maxCredLen || strings.ContainsFunc(f.value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return bad(f.name, "is too long or contains control characters")
		}
	}

	switch getenv(EnvTLS) {
	case "true":
		cfg.TLS = true
	case "false":
		cfg.TLS = false
	case "":
		return bad(EnvTLS, "is required")
	default:
		return bad(EnvTLS, `must be "true" or "false"`)
	}

	for _, f := range []struct {
		name string
		dst  *string
	}{{EnvApp, &cfg.App}, {EnvEnv, &cfg.Env}, {EnvRun, &cfg.Run}} {
		*f.dst = getenv(f.name)
		if *f.dst == "" {
			return bad(f.name, "is required")
		}
		if !keyPart.MatchString(*f.dst) {
			return bad(f.name, "must be 1-63 letters, digits, dot, underscore or dash")
		}
	}

	ttl := DefaultTTLSeconds
	if raw := getenv(EnvTTLSeconds); raw != "" {
		ttl, err = strconv.Atoi(raw)
		if err != nil || ttl < 1 || ttl > MaxTTLSeconds {
			return bad(EnvTTLSeconds, fmt.Sprintf("must be between 1 and %d seconds", MaxTTLSeconds))
		}
	}
	cfg.TTL = time.Duration(ttl) * time.Second
	return cfg, nil
}

// KeyFor returns the deterministic cache key of one job:
// acceptance:v1:<app>:<env>:<run>:job:<id>.
func (c Config) KeyFor(jobID int64) string {
	return "acceptance:v1:" + c.App + ":" + c.Env + ":" + c.Run + ":job:" + strconv.FormatInt(jobID, 10)
}
