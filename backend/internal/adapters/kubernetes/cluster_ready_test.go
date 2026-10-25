package kubernetes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/ports/execution"
)

type stubCredentials struct {
	calls int
	err   error
}

func (s *stubCredentials) ResolveKubeconfig(context.Context, execution.Target) ([]byte, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return []byte("apiVersion: v1\nkind: Config\n"), nil
}

const readyNodes = `{"items":[{"spec":{},"status":{"conditions":[{"type":"Ready","status":"True"}],"allocatable":{"cpu":"1930m","memory":"3Gi","pods":"17"}}}]}`

// kubectlStub writes a fake kubectl that answers from files in dir: version.exit, can-i.answer, nodes.json.
func kubectlStub(t *testing.T, version int, canI, nodes string) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/usr/bin/env bash
args=" $* "
case "$args" in
  *" version "*) [[ ` + itoa(version) + ` == 0 ]] && echo '{}' || { echo "Unable to connect to the server: dial tcp 10.0.0.1:443: i/o timeout" >&2; exit 1; } ;;
  *" auth can-i "*) echo '` + canI + `'; [[ '` + canI + `' == yes ]] || exit 1 ;;
  *" get nodes "*) cat '` + filepath.Join(dir, "nodes.json") + `' ;;
  *) echo "unexpected: $*" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "nodes.json"), []byte(nodes), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func itoa(n int) string { return string(rune('0' + n)) }

func eksTarget() execution.Target {
	return execution.Target{Kind: "kubernetes", Provider: "eks", ClusterName: "orch", Organization: "acme", Connection: "aws-a", Region: "us-east-1"}
}

func checker(path string, creds execution.KubeconfigSource) *ClusterReady {
	return &ClusterReady{KubectlPath: path, Credentials: creds, Interval: time.Millisecond, Sleep: func(context.Context, time.Duration) {}}
}

func TestClusterReadyAcceptsAuthenticatedAuthorisedClusterWithReadyNodeCapacity(t *testing.T) {
	creds := &stubCredentials{}
	if err := checker(kubectlStub(t, 0, "yes", readyNodes), creds).WaitClusterReady(context.Background(), eksTarget()); err != nil {
		t.Fatal(err)
	}
	if creds.calls != 1 {
		t.Fatalf("fresh credentials are derived for each attempt, got %d resolutions", creds.calls)
	}
}

func TestClusterReadyReportsEachFailureAsItsOwnFixedCategory(t *testing.T) {
	cases := map[string]struct {
		path  string
		creds execution.KubeconfigSource
		want  error
	}{
		"network: the API does not answer":    {kubectlStub(t, 1, "yes", readyNodes), &stubCredentials{}, execution.ErrClusterUnreachable},
		"auth: credentials cannot be derived": {kubectlStub(t, 0, "yes", readyNodes), &stubCredentials{err: errors.New("token endpoint said secret-detail")}, execution.ErrClusterAuth},
		"authorisation: cannot create":        {kubectlStub(t, 0, "no", readyNodes), &stubCredentials{}, execution.ErrClusterForbidden},
		"no node at all":                      {kubectlStub(t, 0, "yes", `{"items":[]}`), &stubCredentials{}, execution.ErrClusterNoCapacity},
		"node not Ready":                      {kubectlStub(t, 0, "yes", `{"items":[{"spec":{},"status":{"conditions":[{"type":"Ready","status":"False"}],"allocatable":{"cpu":"2","memory":"3Gi","pods":"17"}}}]}`), &stubCredentials{}, execution.ErrClusterNoCapacity},
		"node cordoned":                       {kubectlStub(t, 0, "yes", `{"items":[{"spec":{"unschedulable":true},"status":{"conditions":[{"type":"Ready","status":"True"}],"allocatable":{"cpu":"2","memory":"3Gi","pods":"17"}}}]}`), &stubCredentials{}, execution.ErrClusterNoCapacity},
		"node tainted NoSchedule":             {kubectlStub(t, 0, "yes", `{"items":[{"spec":{"taints":[{"effect":"NoSchedule"}]},"status":{"conditions":[{"type":"Ready","status":"True"}],"allocatable":{"cpu":"2","memory":"3Gi","pods":"17"}}}]}`), &stubCredentials{}, execution.ErrClusterNoCapacity},
		"no allocatable pods":                 {kubectlStub(t, 0, "yes", `{"items":[{"spec":{},"status":{"conditions":[{"type":"Ready","status":"True"}],"allocatable":{"cpu":"2","memory":"3Gi","pods":"0"}}}]}`), &stubCredentials{}, execution.ErrClusterNoCapacity},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// The first failed attempt is observed, then the wait is cancelled
			// (the Sleep hook runs only after an attempt that finished): the
			// category does not depend on how fast a shell stub starts. The
			// long deadline is only a safety net; the deadline-before-any-
			// observation case has its own test below.
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			c := checker(tc.path, tc.creds)
			c.Sleep = func(context.Context, time.Duration) { cancel() }
			err := c.WaitClusterReady(ctx, eksTarget())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret-detail") || strings.Contains(err.Error(), "10.0.0.1") {
				t.Fatalf("a raw message leaked: %v", err)
			}
		})
	}
}

// A deadline that passes before any attempt finishes says nothing about the
// cluster: the result is the generic not-ready category, never a guessed one.
func TestClusterReadyDeadlineBeforeAnyObservationIsGenericNotReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := checker(kubectlStub(t, 0, "yes", `{"items":[]}`), &stubCredentials{}).WaitClusterReady(ctx, eksTarget())
	if !errors.Is(err, execution.ErrClusterNotReady) || errors.Is(err, execution.ErrClusterNoCapacity) {
		t.Fatalf("err = %v, want the generic not-ready category", err)
	}
}

func TestClusterReadyRefusesATargetWithoutAnExplicitCredential(t *testing.T) {
	err := checker(kubectlStub(t, 0, "yes", readyNodes), &stubCredentials{}).WaitClusterReady(context.Background(), execution.Target{Kind: "kubernetes", Context: "host-default"})
	if err == nil {
		t.Fatal("a context-only target must never reach a host default")
	}
}

func TestClusterReadyWaitsUntilNodesBecomeReady(t *testing.T) {
	dir := t.TempDir()
	nodes := filepath.Join(dir, "nodes.json")
	_ = os.WriteFile(nodes, []byte(`{"items":[]}`), 0o600)
	script := `#!/usr/bin/env bash
args=" $* "
case "$args" in
  *" version "*) echo '{}' ;;
  *" auth can-i "*) echo yes ;;
  *" get nodes "*) cat '` + nodes + `' ;;
esac
`
	path := filepath.Join(dir, "kubectl")
	_ = os.WriteFile(path, []byte(script), 0o700)
	attempts := 0
	c := checker(path, &stubCredentials{})
	c.Sleep = func(context.Context, time.Duration) {
		attempts++
		if attempts == 2 {
			_ = os.WriteFile(nodes, []byte(readyNodes), 0o600) // the node group finally joins
		}
	}
	if err := c.WaitClusterReady(context.Background(), eksTarget()); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("the check must keep waiting for capacity, slept %d times", attempts)
	}
}
