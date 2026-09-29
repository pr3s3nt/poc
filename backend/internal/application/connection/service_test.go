package connection_test

import (
	"context"
	"errors"
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
