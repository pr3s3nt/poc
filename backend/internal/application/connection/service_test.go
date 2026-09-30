package connection_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
)

type verifier struct {
	result connection.KubernetesVerification
	err    error
	calls  int
}

func (v *verifier) Verify(_ context.Context, _ string) (connection.KubernetesVerification, error) {
	v.calls++
	return v.result, v.err
}

func TestRegisterKubernetesCluster_StoresOnlyHostContextReference(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	for _, key := range []string{"acme", "other"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: key, Name: key}); err != nil {
			t.Fatal(err)
		}
	}
	v := &verifier{result: connection.KubernetesVerification{Endpoint: "https://cluster.example", Version: "v1.34"}}
	svc := connection.NewService(st, v)
	cmd := connection.RegisterKubernetesCommand{Key: "internal", ClusterID: "kind-internal", KubeContext: "kind-idp-internal"}
	created, err := svc.RegisterKubernetesCluster(ctx, "acme", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != application.ConnectionReady || created.SecretRef != "host-kube-context://kind-idp-internal" || created.ConfigString("endpoint") != v.result.Endpoint {
		t.Fatalf("connection: %#v", created)
	}
	if _, err := st.GetConnection(ctx, "other", "internal"); err == nil {
		t.Fatal("cross-organization connection leaked")
	}
	if _, err := svc.RegisterKubernetesCluster(ctx, "acme", cmd); !errors.Is(err, connection.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := svc.RegisterKubernetesCluster(ctx, "other", cmd); err != nil {
		t.Fatalf("same ID in another Organization: %v", err)
	}
	if _, err := svc.RegisterKubernetesCluster(ctx, "acme", connection.RegisterKubernetesCommand{Key: "invalid/key", ClusterID: "kind-internal", KubeContext: "kind-idp-internal"}); !errors.Is(err, connection.ErrInvalid) {
		t.Fatalf("invalid ID: %v", err)
	}
	if v.calls != 3 {
		t.Fatalf("verifier calls: %d", v.calls)
	}
}

func TestRegisterKubernetesCluster_RejectsUnverifiedContext(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	if err := st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	v := &verifier{err: errors.New("RBAC denied")}
	svc := connection.NewService(st, v)
	_, err := svc.RegisterKubernetesCluster(ctx, "acme", connection.RegisterKubernetesCommand{Key: "internal", ClusterID: "kind-internal", KubeContext: "bad-context"})
	if !errors.Is(err, connection.ErrVerification) {
		t.Fatalf("verification: %v", err)
	}
	connections, err := st.ListConnections(ctx, "acme")
	if err != nil || len(connections) != 0 {
		t.Fatalf("rejected connection persisted: %#v, %v", connections, err)
	}
}

// TestRegisterKubernetesCluster_VerificationGuidanceIsFixed: raw verifier or
// kubectl output never reaches the caller, only category guidance.
func TestRegisterKubernetesCluster_VerificationGuidanceIsFixed(t *testing.T) {
	ctx := context.Background()
	const sentinel = "sentinel-kubeconfig-token-abc"
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"missing context": {fmt.Errorf("%w: %s", connection.ErrContextMissing, sentinel), "not configured on the backend host"},
		"unreachable":     {fmt.Errorf("%w: dial tcp %s", connection.ErrClusterUnreachable, sentinel), "could not be reached"},
		"permission":      {fmt.Errorf("%w: %s", connection.ErrPermissionDenied, sentinel), "required permission"},
		"unknown":         {errors.New("kubectl panic " + sentinel), "check that the context exists"},
	} {
		st := store.New()
		if err := st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "Acme"}); err != nil {
			t.Fatal(err)
		}
		_, err := connection.NewService(st, &verifier{err: tc.err}).RegisterKubernetesCluster(ctx, "acme", connection.RegisterKubernetesCommand{Key: "internal", ClusterID: "kind-internal", KubeContext: "ctx"})
		if !errors.Is(err, connection.ErrVerification) || strings.Contains(err.Error(), sentinel) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRegisterKubernetesCluster_DuplicateKeepsFirstRecord(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	if err := st.SaveOrganization(ctx, application.Organization{Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	svc := connection.NewService(st, &verifier{result: connection.KubernetesVerification{Endpoint: "https://first"}})
	if _, err := svc.RegisterKubernetesCluster(ctx, "acme", connection.RegisterKubernetesCommand{Key: "internal", ClusterID: "one", KubeContext: "ctx-one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterKubernetesCluster(ctx, "acme", connection.RegisterKubernetesCommand{Key: "internal", ClusterID: "two", KubeContext: "ctx-two"}); !errors.Is(err, connection.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	got, _ := st.GetConnection(ctx, "acme", "internal")
	if got.Config["cluster"] != "one" || got.SecretRef != "host-kube-context://ctx-one" {
		t.Fatalf("duplicate replaced the first record: %+v", got)
	}
}
