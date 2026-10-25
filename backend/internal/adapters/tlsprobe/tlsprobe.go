// Package tlsprobe reads real TLS evidence of an endpoint: reachability,
// certificate trust, expiry and hostname. It exposes only fixed reason codes
// and registered facts; handshake text never leaves the package.
package tlsprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"syscall"
	"time"

	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
)

// Check dials hostPort and verifies its certificate for serverName against
// roots (nil: the system roots). notReady maps "nothing is listening / cannot
// be resolved" to UNKNOWN, as for a target that is not deployed yet.
func Check(ctx context.Context, hostPort, serverName string, roots *x509.CertPool, notReady bool) domain.Finding {
	host, _, _ := net.SplitHostPort(hostPort)
	facts := map[string]string{"host": hostPort}
	if host == "" {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "TARGET_INVALID", Facts: facts}
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: &tls.Config{RootCAs: roots, ServerName: serverName, MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err == nil {
		defer conn.Close()
		if tc, ok := conn.(*tls.Conn); ok {
			if certs := tc.ConnectionState().PeerCertificates; len(certs) > 0 {
				facts["notAfter"] = certs[0].NotAfter.UTC().Format(time.RFC3339)
				facts["daysRemaining"] = domain.Itoa(int(time.Until(certs[0].NotAfter).Hours() / 24))
			}
		}
		facts["hostMatch"] = "true"
		return domain.Finding{Verdict: domain.VerdictPass, Reason: "CERTIFICATE_VALID", Facts: facts}
	}
	var authority x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var dns *net.DNSError
	var netErr net.Error
	switch {
	case errors.As(err, &authority):
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "TLS_UNTRUSTED_CA", Remediation: "provide-ca", Facts: facts}
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "TLS_CERTIFICATE_EXPIRED", Remediation: "renew-certificate", Facts: facts}
	case errors.As(err, &hostname):
		facts["hostMatch"] = "false"
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "TLS_HOSTNAME_MISMATCH", Remediation: "fix-server-name", Facts: facts}
	case errors.As(err, &invalid):
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "TLS_CERTIFICATE_INVALID", Remediation: "renew-certificate", Facts: facts}
	case errors.As(err, &dns):
		return unreachable(notReady, "TARGET_NOT_RESOLVABLE", "ENDPOINT_DNS_FAILED", facts)
	case errors.Is(err, syscall.ECONNREFUSED):
		return unreachable(notReady, "TARGET_NOT_AVAILABLE", "ENDPOINT_REFUSED", facts)
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "ENDPOINT_TIMEOUT", Remediation: "check-network", Facts: facts}
	}
	return domain.Finding{Verdict: domain.VerdictFail, Reason: "TLS_HANDSHAKE_FAILED", Remediation: "check-endpoint", Facts: facts}
}

func unreachable(notReady bool, notReadyReason, failReason string, facts map[string]string) domain.Finding {
	if notReady {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: notReadyReason, Remediation: "deploy-first", Facts: facts}
	}
	return domain.Finding{Verdict: domain.VerdictFail, Reason: failReason, Remediation: "check-endpoint", Facts: facts}
}

// Routes inspects the public HTTPS host of an Application.
type Routes struct{}

var _ probes.RouteInspector = Routes{}

// Inspect implements probes.RouteInspector: the certificate the host really
// serves on 443. A host that does not resolve or accepts no connection is
// UNKNOWN (not deployed yet), never PASS.
func (Routes) Inspect(ctx context.Context, host string) domain.Finding {
	return Check(ctx, net.JoinHostPort(host, "443"), host, nil, true)
}
