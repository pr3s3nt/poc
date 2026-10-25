package tlsprobe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "orchestrator/internal/domain/diagnostics"
)

// pki issues a CA and a server certificate with the given validity and names.
func pki(t *testing.T, names []string, ips []net.IP, notBefore, notAfter time.Time) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "server"}, DNSNames: names, IPAddresses: ips, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func serve(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "https://")
}

var _ = pem.Encode

func TestCheckDistinguishesTrustExpiryHostnameAndAvailability(t *testing.T) {
	ctx := context.Background()
	loop := []net.IP{net.ParseIP("127.0.0.1")}
	now := time.Now()

	good, pool := pki(t, nil, loop, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	addr := serve(t, good)
	if f := Check(ctx, addr, "127.0.0.1", pool, false); f.Verdict != domain.VerdictPass || f.Reason != "CERTIFICATE_VALID" || f.Facts["hostMatch"] != "true" || f.Facts["daysRemaining"] == "" {
		t.Fatalf("valid certificate: %+v", f)
	}
	if f := Check(ctx, addr, "127.0.0.1", nil, false); f.Verdict != domain.VerdictFail || f.Reason != "TLS_UNTRUSTED_CA" {
		t.Fatalf("untrusted CA must fail, not pass: %+v", f)
	}
	if f := Check(ctx, addr, "other.example", pool, false); f.Verdict != domain.VerdictFail || f.Reason != "TLS_HOSTNAME_MISMATCH" || f.Facts["hostMatch"] != "false" {
		t.Fatalf("hostname mismatch: %+v", f)
	}
	expired, expiredPool := pki(t, nil, loop, now.Add(-72*time.Hour), now.Add(-time.Hour))
	if f := Check(ctx, serve(t, expired), "127.0.0.1", expiredPool, false); f.Verdict != domain.VerdictFail || f.Reason != "TLS_CERTIFICATE_EXPIRED" {
		t.Fatalf("expired certificate: %+v", f)
	}

	// A target that accepts no connection yet is UNKNOWN when it may not be
	// deployed, and an observed failure when it must exist.
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := listener.Addr().String()
	listener.Close()
	if f := Check(ctx, dead, "127.0.0.1", nil, true); f.Verdict != domain.VerdictUnknown || f.Reason != "TARGET_NOT_AVAILABLE" {
		t.Fatalf("not deployed yet: %+v", f)
	}
	if f := Check(ctx, dead, "127.0.0.1", nil, false); f.Verdict != domain.VerdictFail || f.Reason != "ENDPOINT_REFUSED" {
		t.Fatalf("required endpoint refused: %+v", f)
	}
	for _, f := range []domain.Finding{Check(ctx, addr, "127.0.0.1", nil, false)} {
		for k, v := range f.Facts {
			if !domain.ValidFact(k, v) {
				t.Fatalf("unregistered fact %s=%s", k, v)
			}
		}
	}
}
