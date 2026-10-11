package connection_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/execution"
)

// PrepareReady is fake-only setup, but with the injected store its reference
// resolves through the real resolver; it keeps an existing record's ID and does
// not overwrite a real READY upload.
func TestPrepareReadyKeepsIDResolvesAndSparesUploads(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	creds := credentialmemory.New()
	if err := st.SaveConnection(ctx, application.Connection{ID: "seeded-id", Key: "internal-cluster", OrganizationKey: "acme", Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionVerifying}); err != nil {
		t.Fatal(err)
	}
	if err := ct.PrepareReady(ctx, st, creds, "acme", "internal-cluster", "kind-idp-internal", "idp-internal"); err != nil {
		t.Fatal(err)
	}
	conn, _ := st.GetConnection(ctx, "acme", "internal-cluster")
	if conn.ID != "seeded-id" || conn.Status != application.ConnectionReady || conn.SecretRef == "" {
		t.Fatalf("prepared record: %+v", conn)
	}
	resolver := connection.NewCredentialResolver(st, creds)
	value, err := resolver.ResolveKubeconfig(ctx, execution.Target{Organization: "acme", Connection: "internal-cluster", Context: "kind-idp-internal"})
	if err != nil || !strings.Contains(string(value), "kind-idp-internal") {
		t.Fatalf("resolve prepared credential: %v", err)
	}
	// A real upload (READY with a reference) is left untouched.
	before := conn
	if err := ct.PrepareReady(ctx, st, creds, "acme", "internal-cluster", "other", "other"); err != nil {
		t.Fatal(err)
	}
	if after, _ := st.GetConnection(ctx, "acme", "internal-cluster"); after.SecretRef != before.SecretRef || after.ConfigString("kubeContext") != "kind-idp-internal" {
		t.Fatalf("existing READY record overwritten: %+v", after)
	}
}
