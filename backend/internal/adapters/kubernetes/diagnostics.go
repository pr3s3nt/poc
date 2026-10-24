package kubernetes

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"syscall"
	"time"

	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/ports/credentials"
	probes "orchestrator/internal/ports/diagnostics"
)

// Diagnostics implements the Infrastructure Account diagnostic provider for
// Internal (Kubernetes) accounts. It reads the saved scoped kubeconfig from the
// credential store, never the host's, and reports only fixed reason codes and
// registered facts: raw kubectl or TLS text never leaves this file.
type Diagnostics struct {
	KubectlPath string
	Credentials credentials.Store
	// ProbeImage is the platform-owned image of execution probes. Empty selects
	// the sandbox image a node already holds, so no registry access is needed.
	ProbeImage string
}

var _ probes.Provider = Diagnostics{}

const (
	groupAccess  = "access"
	groupCluster = "cluster"
	groupStorage = "storage"
	groupPlatf   = "platform"
	groupExec    = "execution"

	managedBy = "orchestrator-diagnostics"
)

var allUseCases = []string{"preview", "deploy", "database", "cache", "secrets", "https", "lifecycle"}

// Plan implements probes.Provider. A probe is planned only when the selected
// work needs it (or, for a GENERAL check, as an inventory), and execution
// probes only for capabilities that can be exercised: nothing is created for
// an unselected capability.
func (Diagnostics) Plan(scope domain.Scope, req *probes.Requirements) []probes.Check {
	affect := scope.Affects
	general := scope.Purpose == domain.PurposeGeneral
	selected := map[string]bool{}
	for _, u := range scope.UseCases {
		selected[u] = true
	}
	any := func(uses ...string) bool {
		for _, u := range uses {
			if selected[u] {
				return true
			}
		}
		return false
	}
	storage := any("database", "cache")
	if req != nil {
		for _, r := range req.Resources {
			if r.Storage.Required {
				storage = storage || any("deploy")
			}
		}
	}
	checks := []probes.Check{
		{ID: "k8s.credential", Group: groupAccess, AffectedUseCases: affect(allUseCases...)},
		{ID: "k8s.reachable", Group: groupAccess, DependsOn: []string{"k8s.credential"}, AffectedUseCases: affect(allUseCases...)},
		{ID: "k8s.tls", Group: groupAccess, DependsOn: []string{"k8s.reachable"}, AffectedUseCases: affect(allUseCases...)},
		{ID: "k8s.api", Group: groupAccess, DependsOn: []string{"k8s.credential", "k8s.reachable", "k8s.tls"}, AffectedUseCases: affect(allUseCases...)},
		{ID: "k8s.auth", Group: groupAccess, DependsOn: []string{"k8s.api"}, AffectedUseCases: affect(allUseCases...)},
	}
	if general || any("deploy", "database", "cache", "secrets", "https", "lifecycle") {
		checks = append(checks, probes.Check{ID: "k8s.rbac.platform", Group: groupAccess, DependsOn: []string{"k8s.auth"}, AffectedUseCases: affect("deploy", "database", "cache", "secrets", "https", "lifecycle")})
	}
	if general || any("deploy", "database", "cache", "lifecycle") {
		checks = append(checks, probes.Check{ID: "k8s.nodes", Group: groupCluster, DependsOn: []string{"k8s.auth"}, AffectedUseCases: affect("deploy", "database", "cache", "lifecycle")})
	}
	if general || storage {
		checks = append(checks,
			probes.Check{ID: "k8s.storage.classes", Group: groupStorage, DependsOn: []string{"k8s.auth"}, AffectedUseCases: affect("database", "cache")},
			probes.Check{ID: "k8s.storage.provisioner", Group: groupStorage, DependsOn: []string{"k8s.storage.classes"}, AffectedUseCases: affect("database", "cache")})
	}
	if general {
		checks = append(checks, probes.Check{ID: "k8s.controllers", Group: groupPlatf, DependsOn: []string{"k8s.auth"}})
	}
	if selected["lifecycle"] {
		checks = append(checks, probes.Check{ID: "k8s.rbac.lifecycle", Group: groupAccess, DependsOn: []string{"k8s.auth"}, AffectedUseCases: []string{"lifecycle"}})
	}
	if selected["secrets"] {
		checks = append(checks,
			probes.Check{ID: "k8s.vso.crd", Group: groupPlatf, DependsOn: []string{"k8s.auth"}, AffectedUseCases: []string{"secrets"}},
			probes.Check{ID: "k8s.vso.controller", Group: groupPlatf, DependsOn: []string{"k8s.vso.crd"}, AffectedUseCases: []string{"secrets"}})
	}
	if selected["https"] {
		checks = append(checks, probes.Check{ID: "k8s.route.controller", Group: groupPlatf, DependsOn: []string{"k8s.auth"}, AffectedUseCases: []string{"https"}})
	}
	if scope.Mode == domain.ModeExecution {
		switch {
		case !any("deploy", "lifecycle", "database", "cache"):
			// Secrets and HTTPS have no run-owned probe object: say so instead of
			// creating an unrelated workload and reporting it as proof.
			checks = append(checks, probes.Check{ID: "k8s.exec.none", Group: groupExec, AffectedUseCases: scope.UseCases})
		default:
			// A namespace is created only when something executable will run in
			// it: a workload for deploy/lifecycle, or a volume whose storage
			// requirement is resolved. An unresolved storage requirement is
			// reported UNKNOWN without creating anything.
			_, _, volumeReason := volumeTargets(req, scope, scope.AccountKey)
			volume := any("database", "cache")
			createsVolume := volume && volumeReason == ""
			if any("deploy", "lifecycle") || createsVolume {
				checks = append(checks, probes.Check{ID: "k8s.exec.namespace", Group: groupExec, DependsOn: []string{"k8s.rbac.platform", "k8s.nodes"}, AffectedUseCases: affect("deploy", "database", "cache", "lifecycle"), Execution: true})
			}
			if any("deploy", "lifecycle") {
				checks = append(checks, probes.Check{ID: "k8s.exec.workload", Group: groupExec, DependsOn: []string{"k8s.exec.namespace"}, AffectedUseCases: affect("deploy", "lifecycle"), Execution: true})
			}
			if volume {
				checks = append(checks, probes.Check{ID: "k8s.exec.volume", Group: groupExec, DependsOn: []string{"k8s.exec.namespace", "k8s.storage.provisioner"}, AffectedUseCases: affect("database", "cache"), Execution: true})
			}
			if any("deploy", "lifecycle") || createsVolume {
				checks = append(checks, probes.Check{ID: "k8s.exec.cleanup", Group: groupExec, AffectedUseCases: affect("deploy", "database", "cache", "lifecycle"), Always: true})
			}
		}
	}
	return checks
}

type tlsTarget struct {
	server, host, port, serverName string
	roots                          *x509.CertPool
	insecure                       bool
	mode                           string
}

type kubeSession struct {
	d       Diagnostics
	cli     CLI
	cleanup func()
	reason  string // set when the saved credential cannot be used
	cluster string
	tls     tlsTarget
	scope   domain.Scope
	org     string
	account string

	nsName, nsUID, pvcName string
	image                  string
}

// Open implements probes.Provider.
func (d Diagnostics) Open(ctx context.Context, acct application.Connection, scope domain.Scope) probes.Session {
	s := &kubeSession{d: d, cleanup: func() {}, scope: scope, org: acct.OrganizationKey, account: acct.Key}
	if d.Credentials == nil || acct.SecretRef == "" || acct.Kind != application.ConnectionKubernetes {
		s.reason = "CREDENTIAL_UNAVAILABLE"
		return s
	}
	value, err := d.Credentials.Get(ctx, acct.OrganizationKey, acct.Key, acct.SecretRef)
	if err != nil || len(value) == 0 {
		s.reason = "CREDENTIAL_UNAVAILABLE"
		return s
	}
	selected, err := connection.NormalizeKubeconfig(string(value), acct.ConfigString("kubeContext"))
	if err != nil {
		s.reason = "CREDENTIAL_INVALID"
		return s
	}
	// The pinned target is the account's saved cluster and endpoint: a
	// credential that now points elsewhere is never probed.
	endpoint, perr := url.Parse(selected.Context.Endpoint)
	if perr != nil || selected.Context.Cluster != acct.ConfigString("cluster") || selected.Context.Endpoint != acct.ConfigString("endpoint") ||
		(scope.Identity["endpoint"] != "" && scope.Identity["endpoint"] != endpoint.Host) {
		s.reason = "TARGET_CHANGED"
		return s
	}
	target, err := tlsTargetOf(selected.Document, endpoint)
	if err != nil {
		s.reason = "CREDENTIAL_INVALID"
		return s
	}
	cli, cleanup, err := materialize(d.KubectlPath, selected.Context.Name, selected.Document)
	if err != nil {
		s.reason = "CREDENTIAL_UNAVAILABLE"
		return s
	}
	s.cli, s.cleanup, s.cluster, s.tls = cli, cleanup, selected.Context.Cluster, target
	return s
}

func tlsTargetOf(document []byte, endpoint *url.URL) (tlsTarget, error) {
	var doc struct {
		Clusters []struct {
			Cluster struct {
				CA       string `json:"certificate-authority-data"`
				Insecure bool   `json:"insecure-skip-tls-verify"`
				Name     string `json:"tls-server-name"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	if json.Unmarshal(document, &doc) != nil || len(doc.Clusters) != 1 {
		return tlsTarget{}, errors.New("kubeconfig")
	}
	c := doc.Clusters[0].Cluster
	host, port := endpoint.Hostname(), endpoint.Port()
	if port == "" {
		port = "443"
	}
	t := tlsTarget{server: endpoint.Host, host: host, port: port, serverName: host, insecure: c.Insecure, mode: "SYSTEM_ROOTS"}
	if c.Name != "" {
		t.serverName = c.Name
	}
	switch {
	case c.Insecure:
		t.mode = "INSECURE_SKIP_VERIFY"
	case c.CA != "":
		t.mode = "CUSTOM_CA"
		pem, err := base64.StdEncoding.DecodeString(c.CA)
		if err != nil {
			return tlsTarget{}, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return tlsTarget{}, errors.New("ca")
		}
		t.roots = pool
	}
	return t, nil
}

func (s *kubeSession) Close() { s.cleanup() }

func fact(kv ...string) map[string]string {
	out := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func finding(v domain.Verdict, reason, remediation string, facts map[string]string) domain.Finding {
	return domain.Finding{Verdict: v, Reason: reason, Remediation: remediation, Facts: facts}
}

// Run implements probes.Session.
func (s *kubeSession) Run(ctx context.Context, c probes.Check, prior map[string]domain.Finding, req *probes.Requirements, rec probes.Recorder) domain.Finding {
	if c.ID == "k8s.credential" {
		if s.reason != "" {
			return finding(domain.VerdictFail, s.reason, "restore-credential", nil)
		}
		return finding(domain.VerdictPass, "CREDENTIAL_RESOLVED", "", fact("cluster", s.cluster, "endpoint", s.tls.server, "tlsMode", s.tls.mode))
	}
	if s.reason != "" {
		return finding(domain.VerdictBlocked, "PREREQUISITE_FAILED", "", nil)
	}
	switch c.ID {
	case "k8s.reachable":
		return s.reachable(ctx)
	case "k8s.tls":
		return s.checkTLS(ctx)
	case "k8s.api":
		return s.api(ctx)
	case "k8s.auth":
		return s.auth(ctx)
	case "k8s.rbac.platform":
		return s.rbac(ctx, "ALL_ALLOWED", "PERMISSIONS_DENIED", [][3]string{{"create", "namespaces", ""}, {"create", "deployments", "--all-namespaces"}, {"create", "statefulsets", "--all-namespaces"}, {"create", "services", "--all-namespaces"}, {"create", "secrets", "--all-namespaces"}})
	case "k8s.rbac.lifecycle":
		return s.rbac(ctx, "ALL_ALLOWED", "PERMISSIONS_DENIED", [][3]string{{"patch", "deployments", "--all-namespaces"}, {"delete", "deployments", "--all-namespaces"}, {"delete", "namespaces", ""}, {"list", "pods", "--all-namespaces"}})
	case "k8s.nodes":
		return s.nodes(ctx)
	case "k8s.storage.classes":
		return s.storageClasses(ctx)
	case "k8s.storage.provisioner":
		return s.provisioner(ctx, prior)
	case "k8s.controllers":
		return s.controllers(ctx)
	case "k8s.vso.crd":
		return s.vsoCRD(ctx, req)
	case "k8s.vso.controller":
		return s.deploymentReady(ctx, "app.kubernetes.io/name=vault-secrets-operator", "VSO_CONTROLLER_READY", "VSO_CONTROLLER_NOT_READY", "install-vso")
	case "k8s.route.controller":
		return s.routeController(ctx, req)
	case "k8s.exec.namespace":
		return s.execNamespace(ctx, rec)
	case "k8s.exec.workload":
		return s.execWorkload(ctx, rec)
	case "k8s.exec.volume":
		return s.execVolume(ctx, rec, req)
	case "k8s.exec.none":
		return finding(domain.VerdictUnknown, "EXECUTION_NOT_APPLICABLE", "", fact("evidence", "NONE"))
	}
	return finding(domain.VerdictUnknown, "PROBE_ERROR", "", nil)
}

func (s *kubeSession) reachable(ctx context.Context) domain.Finding {
	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.tls.host, s.tls.port))
	if err == nil {
		_ = conn.Close()
		return finding(domain.VerdictPass, "ENDPOINT_REACHABLE", "", fact("host", s.tls.server))
	}
	var dns *net.DNSError
	var netErr net.Error
	switch {
	case errors.As(err, &dns):
		return finding(domain.VerdictFail, "ENDPOINT_DNS_FAILED", "check-endpoint", fact("host", s.tls.server))
	case errors.Is(err, syscall.ECONNREFUSED):
		return finding(domain.VerdictFail, "ENDPOINT_REFUSED", "check-endpoint", fact("host", s.tls.server))
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return finding(domain.VerdictUnknown, "ENDPOINT_TIMEOUT", "check-network", fact("host", s.tls.server))
	}
	return finding(domain.VerdictFail, "ENDPOINT_UNREACHABLE", "check-network", fact("host", s.tls.server))
}

func (s *kubeSession) checkTLS(ctx context.Context) domain.Finding {
	t := s.tls
	base := fact("host", t.server, "tlsMode", t.mode)
	if t.insecure {
		return finding(domain.VerdictUnknown, "TLS_VERIFICATION_DISABLED", "provide-ca", base)
	}
	cfg := &tls.Config{RootCAs: t.roots, ServerName: t.serverName, MinVersion: tls.VersionTLS12}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: cfg}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(t.host, t.port))
	if err == nil {
		defer conn.Close()
		if tc, ok := conn.(*tls.Conn); ok {
			if certs := tc.ConnectionState().PeerCertificates; len(certs) > 0 {
				base["notAfter"] = certs[0].NotAfter.UTC().Format(time.RFC3339)
				base["daysRemaining"] = domain.Itoa(int(time.Until(certs[0].NotAfter).Hours() / 24))
			}
		}
		base["hostMatch"] = "true"
		return finding(domain.VerdictPass, "TLS_VERIFIED", "", base)
	}
	var unknownAuthority x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var netErr net.Error
	switch {
	case errors.As(err, &unknownAuthority):
		return finding(domain.VerdictFail, "TLS_UNTRUSTED_CA", "provide-ca", base)
	case errors.As(err, &invalid) && (invalid.Reason == x509.Expired):
		return finding(domain.VerdictFail, "TLS_CERTIFICATE_EXPIRED", "renew-certificate", base)
	case errors.As(err, &hostname):
		base["hostMatch"] = "false"
		return finding(domain.VerdictFail, "TLS_HOSTNAME_MISMATCH", "fix-server-name", base)
	case errors.As(err, &invalid):
		return finding(domain.VerdictFail, "TLS_CERTIFICATE_INVALID", "renew-certificate", base)
	case errors.As(err, &netErr) && netErr.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return finding(domain.VerdictUnknown, "ENDPOINT_TIMEOUT", "check-network", base)
	}
	return finding(domain.VerdictFail, "TLS_HANDSHAKE_FAILED", "check-endpoint", base)
}

// readFailure maps a failed kubectl read to a verdict that never claims more
// than was observed: missing permission or a transient failure is UNKNOWN.
func readFailure(err error) domain.Finding {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return finding(domain.VerdictFail, "AUTH_REJECTED", "refresh-credential", nil)
	case errors.Is(err, ErrForbidden):
		return finding(domain.VerdictUnknown, "READ_FORBIDDEN", "grant-read", nil)
	case errors.Is(err, context.DeadlineExceeded):
		return finding(domain.VerdictUnknown, "CHECK_TIMEOUT", "rerun", nil)
	case errors.Is(err, ErrUnreachable):
		return finding(domain.VerdictUnknown, "READ_UNREACHABLE", "check-network", nil)
	}
	return finding(domain.VerdictUnknown, "READ_FAILED", "rerun", nil)
}

func (s *kubeSession) api(ctx context.Context) domain.Finding {
	out, err := s.cli.Run(ctx, nil, "get", "--raw=/version")
	if err != nil {
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
			// The endpoint answered: authentication is judged by k8s.auth.
			return finding(domain.VerdictPass, "API_ANSWERED", "", nil)
		}
		if errors.Is(err, ErrUnreachable) {
			return finding(domain.VerdictFail, "API_UNREACHABLE", "check-network", nil)
		}
		return readFailure(err)
	}
	var v struct {
		GitVersion string `json:"gitVersion"`
	}
	if json.Unmarshal(out, &v) != nil || v.GitVersion == "" {
		return finding(domain.VerdictUnknown, "API_RESPONSE_UNREADABLE", "rerun", nil)
	}
	return finding(domain.VerdictPass, "API_ANSWERED", "", fact("serverVersion", v.GitVersion))
}

func (s *kubeSession) auth(ctx context.Context) domain.Finding {
	out, err := s.cli.Run(ctx, nil, "auth", "can-i", "get", "namespaces")
	answer := strings.TrimSpace(string(out))
	if err != nil && answer != "no" {
		if errors.Is(err, ErrUnauthorized) {
			return finding(domain.VerdictFail, "AUTH_REJECTED", "refresh-credential", nil)
		}
		return readFailure(err)
	}
	return finding(domain.VerdictPass, "AUTH_ACCEPTED", "", nil)
}

func (s *kubeSession) rbac(ctx context.Context, okReason, denyReason string, perms [][3]string) domain.Finding {
	var denied []string
	for _, p := range perms {
		args := []string{"auth", "can-i", p[0], p[1]}
		if p[2] != "" {
			args = append(args, p[2])
		}
		out, err := s.cli.Run(ctx, nil, args...)
		answer := strings.TrimSpace(string(out))
		if err != nil && answer != "no" {
			return readFailure(err)
		}
		if answer != "yes" {
			denied = append(denied, p[0]+"/"+p[1])
		}
	}
	facts := fact("allowed", domain.Itoa(len(perms)-len(denied)), "denied", domain.Itoa(len(denied)))
	if len(denied) > 0 {
		facts["deniedPermissions"] = domain.JoinList(denied)
		return finding(domain.VerdictFail, denyReason, "grant-rbac", facts)
	}
	return finding(domain.VerdictPass, okReason, "", facts)
}

type list struct {
	Items []map[string]any `json:"items"`
}

func (s *kubeSession) list(ctx context.Context, args ...string) (list, error) {
	out, err := s.cli.Run(ctx, nil, append(args, "-o", "json")...)
	if err != nil {
		return list{}, err
	}
	var l list
	if json.Unmarshal(out, &l) != nil {
		return list{}, errors.New("decode")
	}
	return l, nil
}

func str(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		next, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = next[p]
	}
	text, _ := cur.(string)
	return text
}

func (s *kubeSession) nodes(ctx context.Context) domain.Finding {
	l, err := s.list(ctx, "get", "nodes")
	if err != nil {
		return readFailure(err)
	}
	ready, versions := 0, map[string]bool{}
	for _, n := range l.Items {
		if v := str(n, "status", "nodeInfo", "kubeletVersion"); v != "" {
			versions[v] = true
		}
		unschedulable, _ := mapAt(n, "spec")["unschedulable"].(bool)
		conds, _ := mapAt(n, "status")["conditions"].([]any)
		for _, c := range conds {
			cm, _ := c.(map[string]any)
			if cm["type"] == "Ready" && cm["status"] == "True" && !unschedulable {
				ready++
			}
		}
	}
	facts := fact("nodes", domain.Itoa(len(l.Items)), "nodesReady", domain.Itoa(ready))
	if len(versions) > 0 {
		facts["kubeletVersions"] = domain.JoinList(sortedSet(versions))
	}
	if ready == 0 {
		return finding(domain.VerdictFail, "NO_READY_NODE", "restore-nodes", facts)
	}
	return finding(domain.VerdictPass, "NODES_READY", "", facts)
}

func mapAt(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func (s *kubeSession) storageClassList(ctx context.Context) ([]map[string]any, error) {
	l, err := s.list(ctx, "get", "storageclass")
	return l.Items, err
}

func isDefaultClass(sc map[string]any) bool {
	ann := mapAt(mapAt(sc, "metadata"), "annotations")
	return ann["storageclass.kubernetes.io/is-default-class"] == "true" || ann["storageclass.beta.kubernetes.io/is-default-class"] == "true"
}

func (s *kubeSession) storageClasses(ctx context.Context) domain.Finding {
	items, err := s.storageClassList(ctx)
	if err != nil {
		return readFailure(err)
	}
	defaults, names := 0, []string{}
	for _, sc := range items {
		names = append(names, str(sc, "metadata", "name"))
		if isDefaultClass(sc) {
			defaults++
		}
	}
	facts := fact("storageClasses", domain.Itoa(len(items)), "defaultClasses", domain.Itoa(defaults))
	if len(names) > 0 {
		facts["classes"] = domain.JoinList(sortedList(names))
	}
	// An inventory: zero classes is a valid fact for a stateless account. A
	// selected path that needs storage turns a missing class into a failure.
	return finding(domain.VerdictPass, "STORAGE_CLASSES_LISTED", "", facts)
}

// provisioner reports what the cluster really shows about the provisioner of
// the default class (or, without one, of every class). A ready local-path
// controller is observed; a registered CSI node plugin proves node
// registration only, never controller readiness, so it stays UNKNOWN; any
// provisioner it cannot observe is UNKNOWN, not broken.
func (s *kubeSession) provisioner(ctx context.Context, _ map[string]domain.Finding) domain.Finding {
	items, err := s.storageClassList(ctx)
	if err != nil {
		return readFailure(err)
	}
	if len(items) == 0 {
		return finding(domain.VerdictNA, "NO_STORAGE_CLASS", "", fact("storageClasses", "0"))
	}
	var scoped []map[string]any
	for _, sc := range items {
		if isDefaultClass(sc) {
			scoped = append(scoped, sc)
		}
	}
	if len(scoped) == 0 {
		scoped = items
	}
	registered := map[string]bool{}
	nodes, nerr := s.list(ctx, "get", "csinodes")
	if nerr == nil {
		for _, n := range nodes.Items {
			drivers, _ := mapAt(n, "spec")["drivers"].([]any)
			for _, d := range drivers {
				if name := str(asMap(d), "name"); name != "" {
					registered[name] = true
				}
			}
		}
	}
	failed, unobserved, csiOnly, ready, static := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	localChecked, localReady := false, false
	for _, sc := range scoped {
		p := str(sc, "provisioner")
		switch {
		case p == "kubernetes.io/no-provisioner":
			static[p] = true
		case p == "rancher.io/local-path":
			if !localChecked {
				localChecked = true
				// The stock manifest names the controller and carries no label.
				ds, err := s.list(ctx, "get", "deployments", "--all-namespaces", "--field-selector", "metadata.name=local-path-provisioner")
				if err != nil || len(ds.Items) == 0 {
					// Not found by its stock name is not proof it is absent.
					unobserved[p] = true
					localChecked = false
					continue
				}
				localReady = anyReady(ds.Items)
			}
			if localReady {
				ready[p] = true
			} else if localChecked {
				failed[p] = true
			}
		case registered[p]:
			csiOnly[p] = true
		default:
			unobserved[p] = true
		}
	}
	facts := fact("storageClasses", domain.Itoa(len(scoped)), "csiDrivers", domain.Itoa(len(registered)))
	switch {
	case len(failed) > 0:
		facts["drivers"] = domain.JoinList(sortedSet(failed))
		return finding(domain.VerdictFail, "PROVISIONER_NOT_READY", "repair-provisioner", facts)
	case len(csiOnly) > 0:
		facts["drivers"] = domain.JoinList(sortedSet(csiOnly))
		return finding(domain.VerdictUnknown, "CSI_CONTROLLER_UNOBSERVED", "run-execution-check", facts)
	case len(unobserved) > 0:
		return finding(domain.VerdictUnknown, "PROVISIONER_UNOBSERVABLE", "run-execution-check", facts)
	case len(ready) == 0:
		return finding(domain.VerdictNA, "STATIC_PROVISIONING", "", facts)
	}
	facts["drivers"] = domain.JoinList(sortedSet(ready))
	return finding(domain.VerdictPass, "PROVISIONER_OBSERVED", "", facts)
}

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }

func anyReady(items []map[string]any) bool {
	for _, d := range items {
		if n, _ := mapAt(d, "status")["readyReplicas"].(float64); n > 0 {
			return true
		}
	}
	return false
}

func (s *kubeSession) crdNames(ctx context.Context) (map[string]bool, error) {
	out, err := s.cli.Run(ctx, nil, "get", "crd", "-o", "name")
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if _, name, ok := strings.Cut(strings.TrimSpace(line), "/"); ok {
			names[name] = true
		}
	}
	return names, nil
}

func (s *kubeSession) controllers(ctx context.Context) domain.Finding {
	crds, err := s.crdNames(ctx)
	if err != nil {
		return readFailure(err)
	}
	installed := []string{}
	if crds["vaultstaticsecrets.secrets.hashicorp.com"] {
		installed = append(installed, "vault-secrets-operator")
	}
	if crds["certificates.cert-manager.io"] {
		installed = append(installed, "cert-manager")
	}
	classes := []string{}
	if out, err := s.cli.Run(ctx, nil, "get", "ingressclass", "-o", "name"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if _, name, ok := strings.Cut(strings.TrimSpace(line), "/"); ok {
				classes = append(classes, name)
			}
		}
	}
	facts := fact("found", domain.Itoa(len(installed)+len(classes)))
	if len(installed) > 0 {
		facts["installed"] = domain.JoinList(sortedList(installed))
	}
	if len(classes) > 0 {
		facts["ingressClasses"] = domain.JoinList(sortedList(classes))
	}
	// An inventory only: no add-on is required by a general check.
	return finding(domain.VerdictPass, "INVENTORY_OBSERVED", "", facts)
}

func (s *kubeSession) vsoCRD(ctx context.Context, req *probes.Requirements) domain.Finding {
	if req != nil && req.SecretDelivery != "" && req.SecretDelivery != "vso" {
		return finding(domain.VerdictNA, "NOT_REQUIRED_BY_DELIVERY", "", fact("role", "AGENT"))
	}
	if req == nil || req.SecretDelivery == "" {
		return finding(domain.VerdictNA, "NOT_REQUIRED_BY_DELIVERY", "", nil)
	}
	crds, err := s.crdNames(ctx)
	if err != nil {
		return readFailure(err)
	}
	var missing []string
	for _, name := range []string{"vaultconnections.secrets.hashicorp.com", "vaultauths.secrets.hashicorp.com", "vaultstaticsecrets.secrets.hashicorp.com"} {
		if !crds[name] {
			missing = append(missing, strings.SplitN(name, ".", 2)[0])
		}
	}
	if len(missing) > 0 {
		return finding(domain.VerdictFail, "VSO_CRD_MISSING", "install-vso", fact("missingCrds", domain.JoinList(missing)))
	}
	return finding(domain.VerdictPass, "VSO_CRD_PRESENT", "", fact("found", "3"))
}

func (s *kubeSession) deploymentReady(ctx context.Context, selector, okReason, badReason, remediation string) domain.Finding {
	ds, err := s.list(ctx, "get", "deployments", "--all-namespaces", "-l", selector)
	if err != nil {
		return readFailure(err)
	}
	if len(ds.Items) == 0 {
		// Not found under the chart's standard label is not proof of absence.
		return finding(domain.VerdictUnknown, "CONTROLLER_UNOBSERVED", remediation, fact("found", "0"))
	}
	if !anyReady(ds.Items) {
		return finding(domain.VerdictFail, badReason, remediation, fact("found", domain.Itoa(len(ds.Items)), "replicasReady", "0"))
	}
	return finding(domain.VerdictPass, okReason, "", fact("found", domain.Itoa(len(ds.Items))))
}

func (s *kubeSession) routeController(ctx context.Context, req *probes.Requirements) domain.Finding {
	if req != nil && req.Resolved && req.PublicRoutes == 0 {
		return finding(domain.VerdictNA, "NO_PUBLIC_ROUTES", "", nil)
	}
	// The platform's public Ingress always uses the "traefik" class.
	if _, err := s.cli.Run(ctx, nil, "get", "ingressclass", "traefik", "-o", "name"); err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return finding(domain.VerdictFail, "INGRESS_CLASS_MISSING", "install-ingress", fact("ingressClasses", "none"))
		}
		return readFailure(err)
	}
	return s.deploymentReady(ctx, "app.kubernetes.io/name=traefik", "ROUTE_CONTROLLER_READY", "ROUTE_CONTROLLER_NOT_READY", "install-ingress")
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return sortedList(out)
}

func sortedList(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("kubernetes: entropy unavailable")
	}
	return hex.EncodeToString(b)
}
