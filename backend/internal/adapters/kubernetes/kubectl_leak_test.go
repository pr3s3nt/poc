package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/ports/execution"
)

// credentialFiles returns every private kubeconfig path the stub saw.
func credentialFiles(log string) []string {
	var files []string
	for _, line := range strings.Split(log, "\n") {
		if fields := strings.Fields(line); len(fields) == 4 && fields[0] == "FILE" {
			files = append(files, fields[1])
		}
	}
	return files
}

// TestCredentialTarget_KubectlFailuresNeverEchoOutput makes the stub kubectl
// behave like an API server or proxy that echoes the submitted credential
// file, its path and its directory on stderr. Credential-backed failures must
// keep a typed category but carry none of those bytes, and the private file
// must still be removed.
func TestCredentialTarget_KubectlFailuresNeverEchoOutput(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	ca, cert, key := ct.TLSMaterial()
	documents := map[string]struct {
		doc     string
		secrets []string
	}{
		"token":       {ct.Single("lab", "https://lab.example", "stub-token-value"), []string{"stub-token-value"}},
		"certificate": {ct.Document(ct.CertificateContext("lab", "https://lab.example")), []string{ca, cert, key}},
	}
	for docName, d := range documents {
		selected, err := connection.NormalizeKubeconfig(d.doc, "lab")
		if err != nil {
			t.Fatal(err)
		}
		forbidden := append([]string{"orch-kube-", `"clusters"`, "client-key-data", "token"}, d.secrets...)
		cases := []struct {
			name, match, stderr string
			want                error
			run                 func(ctx context.Context, kubectl string, source execution.KubeconfigSource) error
		}{
			{"version", "version -o json", "error: You must be logged in to the server (Unauthorized)", ErrUnauthorized,
				func(ctx context.Context, kubectl string, source execution.KubeconfigSource) error {
					cli, cleanup, err := OpenCLI(ctx, kubectl, source, credentialTarget())
					if err != nil {
						return err
					}
					defer cleanup()
					_, err = cli.Run(ctx, nil, "version", "-o", "json")
					return err
				}},
			{"apply", "apply -f -", "Error from server (Forbidden): apply denied", ErrForbidden,
				func(ctx context.Context, kubectl string, source execution.KubeconfigSource) error {
					return (&Deployer{KubectlPath: kubectl, Credentials: source}).Apply(ctx, credentialTarget(), []execution.Manifest{{Object: map[string]any{"kind": "ConfigMap"}}})
				}},
			{"get", " get ", "Error from server (InternalError): echo", ErrRequestRejected,
				func(ctx context.Context, kubectl string, source execution.KubeconfigSource) error {
					return (&PublicRoutes{KubectlPath: kubectl, Credentials: source}).Reconcile(ctx, credentialTarget(), execution.PublicRoute{ApplicationID: "a", EnvironmentID: "staging"})
				}},
			{"delete", " delete ", "Unable to connect to the server: dial tcp 10.0.0.1:443: connect: connection refused", ErrUnreachable,
				func(ctx context.Context, kubectl string, source execution.KubeconfigSource) error {
					return (&Deployer{KubectlPath: kubectl, Credentials: source}).Remove(ctx, credentialTarget(), "api")
				}},
		}
		for _, tc := range cases {
			t.Run(docName+"/"+tc.name, func(t *testing.T) {
				kubectl, logPath := installStub(t)
				t.Setenv("STUB_LEAK", tc.match)
				t.Setenv("STUB_LEAK_MSG", tc.stderr)
				err := tc.run(context.Background(), kubectl, &stubResolver{document: selected.Document})
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v, want category %v", err, tc.want)
				}
				var failure *CommandError
				if !errors.As(err, &failure) || failure.Operation[0] != strings.Fields(tc.match)[0] {
					t.Fatalf("error lost its operation identity: %v", err)
				}
				stubLog := readLog(t, logPath)
				files := credentialFiles(stubLog)
				if len(files) == 0 {
					t.Fatalf("stub never received a credential file: %s", stubLog)
				}
				for _, file := range files {
					forbidden = append(forbidden, file)
					if _, statErr := os.Stat(file); !os.IsNotExist(statErr) {
						t.Fatalf("private credential file not removed: %s", file)
					}
				}
				for _, leaked := range append(forbidden, tc.stderr) {
					if strings.Contains(err.Error(), leaked) {
						t.Fatalf("error carries kubectl output %q: %v", leaked, err)
					}
					if strings.Contains(logs.String(), leaked) {
						t.Fatalf("log carries kubectl output %q: %s", leaked, logs.String())
					}
				}
			})
		}
	}
}

// TestCredentialTarget_TypedCategoriesKeepBehavior checks the decisions that
// used to parse kubectl text still work from the fixed categories.
func TestCredentialTarget_TypedCategoriesKeepBehavior(t *testing.T) {
	ctx := context.Background()
	kubectl, _ := installStub(t)
	source := &stubResolver{document: normalized(t)}

	// CLI.Get still treats a NotFound answer as a missing object.
	cli, cleanup, err := OpenCLI(ctx, kubectl, source, credentialTarget())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_LEAK", " get ")
	t.Setenv("STUB_LEAK_MSG", `Error from server (NotFound): ingresses "x" not found`)
	if obj, exists, err := cli.Get(ctx, "team-a", "ingress", "x"); err != nil || exists || obj != nil {
		t.Fatalf("NotFound: %v %v %v", obj, exists, err)
	}
	cleanup()

	// The verifier still distinguishes authentication from reachability.
	selected, err := connection.NormalizeKubeconfig(ct.Single("lab", "https://lab.example", "stub-token-value"), "lab")
	if err != nil {
		t.Fatal(err)
	}
	verifier := ConnectionVerifier{KubectlPath: kubectl}
	for stderr, want := range map[string]error{
		"error: You must be logged in to the server (Unauthorized)":                   connection.ErrAuthentication,
		"Error from server (Forbidden): forbidden":                                    connection.ErrAuthentication,
		"Unable to connect to the server: dial tcp: lookup lab.example: no such host": connection.ErrClusterUnreachable,
	} {
		t.Setenv("STUB_LEAK", "version -o json")
		t.Setenv("STUB_LEAK_MSG", stderr)
		_, err := verifier.VerifyKubeconfig(ctx, selected)
		if !errors.Is(err, want) || strings.Contains(err.Error(), "stub-token-value") {
			t.Fatalf("%q: got %v, want %v", stderr, err, want)
		}
	}

	// A resolver failure keeps its identity but never its text.
	secretErr := errors.New("vault answered token=stub-token-value at kv2://kv/orchestrator/connections/acme/lab")
	_, _, err = OpenCLI(ctx, kubectl, &stubResolver{err: secretErr}, credentialTarget())
	if !errors.Is(err, ErrCredentialTarget) || !errors.Is(err, secretErr) || err.Error() != ErrCredentialTarget.Error() {
		t.Fatalf("resolver failure: %v", err)
	}

	// A cancelled operation reports a fixed status, not output.
	cancelled, cancel := context.WithTimeout(ctx, time.Nanosecond)
	defer cancel()
	<-cancelled.Done()
	t.Setenv("STUB_LEAK", "")
	_, err = (&Executor{KubectlPath: kubectl, Credentials: source}).Provision(cancelled, execution.ProvisionRequest{ResourceType: "k8s-namespace", Descriptor: "ns", Inputs: map[string]any{"name": "team-a"}, Target: credentialTarget()})
	if err == nil || strings.Contains(err.Error(), "stub-token-value") || strings.Contains(err.Error(), "orch-kube-") {
		t.Fatalf("cancelled operation: %v", err)
	}
}

// TestLegacyTarget_KeepsKubectlDiagnostics checks host-context targets still
// report kubectl stderr, with the same typed category.
func TestLegacyTarget_KeepsKubectlDiagnostics(t *testing.T) {
	kubectl, _ := installStub(t)
	t.Setenv("STUB_LEAK", " delete ")
	t.Setenv("STUB_LEAK_MSG", "Error from server (Forbidden): deployments.apps is forbidden for host user")
	err := (&Deployer{KubectlPath: kubectl}).Remove(context.Background(), execution.Target{Context: "kind-idp-internal", Namespace: "ns"}, "api")
	if !errors.Is(err, ErrForbidden) || !strings.Contains(err.Error(), "forbidden for host user") {
		t.Fatalf("legacy diagnostics lost: %v", err)
	}
}
