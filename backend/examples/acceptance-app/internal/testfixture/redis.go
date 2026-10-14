// Package testfixture provisions the disposable real services the acceptance
// app integration tests run against: an auth-enabled Redis (optionally TLS) it
// starts and owns, and a PostgreSQL login derived from the shared test server.
// Nothing here is a fake: a missing service fails the test when it is required.
package testfixture

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"orchestrator/examples/acceptance-app/internal/jobstore"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// Environment variables of the fixture.
const (
	// RequireRedisEnv set to "1" makes a missing redis-server a failure. It is
	// also implied by REQUIRE_POSTGRES=1, so the standard validation command
	// cannot pass with the Redis suites skipped.
	RequireRedisEnv = "REQUIRE_REDIS"
	// RedisServerEnv overrides the redis-server binary path.
	RedisServerEnv = "ACCEPTANCE_REDIS_SERVER"
	// RunIDEnv labels every directory and process the fixture owns.
	RunIDEnv = "ACCEPTANCE_FIXTURE_RUN_ID"
)

// Redis is one disposable redis-server owned by a test.
type Redis struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      bool
	// CA is the PEM of the fixture CA that signed the server certificate (TLS only).
	CA *x509.CertPool

	t    testing.TB
	conf string
	dir  string
	cmd  *exec.Cmd
	done chan struct{}
}

// RedisOptions selects the fixture shape.
type RedisOptions struct {
	// TLS serves TLS only. CertNames are the DNS names/IPs of the server certificate.
	TLS       bool
	CertNames []string
	// Password overrides the random one (for example to exercise reserved characters).
	Password string
}

func redisRequired() bool {
	return os.Getenv(RequireRedisEnv) == "1" || os.Getenv(persistencetest.RequirePostgresEnv) == "1"
}

func runID() string {
	if id := os.Getenv(RunIDEnv); id != "" {
		return id
	}
	return "local"
}

func randomHex(t testing.TB, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func freePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// StartRedis starts a redis-server with an ACL user (the default user is
// disabled, so unauthenticated and wrong-password clients are refused). It
// stops and removes everything it created when the test ends.
func StartRedis(t testing.TB, opts RedisOptions) *Redis {
	t.Helper()
	bin := os.Getenv(RedisServerEnv)
	if bin == "" {
		bin = "redis-server"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		if redisRequired() {
			t.Fatalf("redis-server is required but not found (%s=1 or REQUIRE_POSTGRES=1): %v", RequireRedisEnv, err)
		}
		t.Skipf("redis-server not found: %v", err)
	}
	dir, err := os.MkdirTemp("", "acc-redis-"+runID()+"-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	r := &Redis{t: t, dir: dir, Host: "127.0.0.1", Username: "acc" + randomHex(t, 3), Password: opts.Password, TLS: opts.TLS}
	if r.Password == "" {
		r.Password = randomHex(t, 16)
	}
	t.Cleanup(func() {
		r.Stop()
		_ = os.RemoveAll(dir)
	})

	if opts.TLS {
		names := opts.CertNames
		if len(names) == 0 {
			names = []string{"localhost"}
		}
		r.Host = names[0]
		r.CA = r.writeCerts(names)
	}
	r.Port = freePort(t)
	r.conf = filepath.Join(dir, "redis.conf")
	r.writeConf()
	r.start(path)
	return r
}

func (r *Redis) writeConf() {
	var b strings.Builder
	fmt.Fprintf(&b, "bind 127.0.0.1\ndir %s\nsave \"\"\nappendonly no\nprotected-mode yes\n", r.dir)
	// The ACL password is given as its SHA-256 so any characters are safe in the file.
	sum := sha256.Sum256([]byte(r.Password))
	fmt.Fprintf(&b, "user default off\nuser %s on #%s ~* &* +@all\n", r.Username, hex.EncodeToString(sum[:]))
	if r.TLS {
		fmt.Fprintf(&b, "port 0\ntls-port %d\ntls-cert-file %s\ntls-key-file %s\ntls-ca-cert-file %s\ntls-auth-clients no\n",
			r.Port, filepath.Join(r.dir, "server.crt"), filepath.Join(r.dir, "server.key"), filepath.Join(r.dir, "ca.crt"))
	} else {
		fmt.Fprintf(&b, "port %d\n", r.Port)
	}
	if err := os.WriteFile(r.conf, []byte(b.String()), 0o600); err != nil {
		r.t.Fatalf("write redis conf: %v", err)
	}
}

func (r *Redis) start(path string) {
	r.t.Helper()
	cmd := exec.Command(path, r.conf)
	cmd.Dir = r.dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	logFile, err := os.OpenFile(filepath.Join(r.dir, "redis.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		r.t.Fatalf("redis log: %v", err)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		r.t.Fatalf("start redis-server: %v", err)
	}
	r.cmd = cmd
	r.done = make(chan struct{})
	go func() { _ = cmd.Wait(); logFile.Close(); close(r.done) }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-r.done:
			r.t.Fatalf("redis-server exited at start: %s", r.logTail())
		default:
		}
		if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(r.Port)), 200*time.Millisecond); err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.t.Fatalf("redis-server did not listen: %s", r.logTail())
}

func (r *Redis) logTail() string {
	f, err := os.Open(filepath.Join(r.dir, "redis.log"))
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return strings.Join(lines, " | ")
}

// Stop kills the server (an outage). It is idempotent.
func (r *Redis) Stop() {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGKILL)
	<-r.done
	r.cmd = nil
}

// Restart brings the server back on the same port with the same credentials
// and an empty keyspace, as after a Redis restart.
func (r *Redis) Restart() {
	r.t.Helper()
	r.Stop()
	path, err := exec.LookPath(firstNonEmpty(os.Getenv(RedisServerEnv), "redis-server"))
	if err != nil {
		r.t.Fatalf("redis-server: %v", err)
	}
	r.start(path)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Alive reports whether the owned process is still running.
func (r *Redis) Alive() bool {
	if r.done == nil {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return r.cmd != nil
	}
}

// writeCerts creates a throwaway CA and a server certificate for names, and
// returns a pool trusting only that CA. The CA key never leaves memory.
func (r *Redis) writeCerts(names []string) *x509.CertPool {
	r.t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		r.t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "acceptance fixture CA " + randomHex(r.t, 4)},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		r.t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		r.t.Fatal(err)
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: names[0]},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, n := range names {
		if ip := net.ParseIP(n); ip != nil {
			srvTmpl.IPAddresses = append(srvTmpl.IPAddresses, ip)
		} else {
			srvTmpl.DNSNames = append(srvTmpl.DNSNames, n)
		}
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		r.t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(srvKey)
	if err != nil {
		r.t.Fatal(err)
	}
	write := func(name, typ string, der []byte) {
		if err := os.WriteFile(filepath.Join(r.dir, name), pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
			r.t.Fatal(err)
		}
	}
	write("ca.crt", "CERTIFICATE", caDER)
	write("server.crt", "CERTIFICATE", srvDER)
	write("server.key", "EC PRIVATE KEY", keyDER)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return pool
}

// NewCA returns a pool with an unrelated throwaway CA, for "wrong CA" tests.
func NewCA(t testing.TB) *x509.CertPool {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "unrelated CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool
}

// PostgresConfig returns a jobstore.Config that logs in as the owner of a fresh
// disposable database on the shared test server. It skips, or fails under
// REQUIRE_POSTGRES=1, when no server is configured.
func PostgresConfig(t *testing.T) jobstore.Config {
	t.Helper()
	db := persistencetest.FreshPostgresRoles(t)
	return configFromURL(t, db.Owner)
}

// PostgresConfigWithPassword is PostgresConfig after resetting the owner role
// password, for exercising reserved characters.
func PostgresConfigWithPassword(t *testing.T, password string) jobstore.Config {
	t.Helper()
	db := persistencetest.FreshPostgresRoles(t)
	cfg := configFromURL(t, db.Owner)
	admin := connectAdmin(t, db.Admin)
	if _, err := admin.Exec(context.Background(), `ALTER ROLE "`+db.OwnerRole+`" PASSWORD '`+strings.ReplaceAll(password, "'", "''")+`'`); err != nil {
		t.Fatalf("set password: %v", err)
	}
	cfg.Password = password
	return cfg
}

func configFromURL(t testing.TB, raw string) jobstore.Config {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		port = 5432
	}
	pw, _ := u.User.Password()
	return jobstore.Config{Host: u.Hostname(), Port: port, Database: strings.TrimPrefix(u.Path, "/"), User: u.User.Username(), Password: pw}
}

// Client returns an independent go-redis client for inspecting keys directly,
// as the acceptance runner's private read-only assertions do. TLS fixtures are
// verified against the fixture CA.
func (r *Redis) Client(t testing.TB) *redis.Client {
	t.Helper()
	opts := &redis.Options{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(r.Port)), Username: r.Username, Password: r.Password, MaxRetries: -1}
	if r.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: r.Host, RootCAs: r.CA}
	}
	c := redis.NewClient(opts)
	t.Cleanup(func() { _ = c.Close() })
	return c
}
