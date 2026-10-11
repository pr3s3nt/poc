package seed_test

import (
	"context"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/seed"
)

// T22: the seeded Kubernetes record is explicit KUBECONFIG metadata without a
// credential reference. It is VERIFYING and unverified, never READY, and a
// re-seed never replaces an existing record (for example a READY upload).
func TestSeedKubernetesRecordIsVerifyingWithoutCredential(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	conn, err := st.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
	if err != nil {
		t.Fatal(err)
	}
	if conn.AuthenticationType != application.AuthKubeconfig || conn.Status != application.ConnectionVerifying || conn.SecretRef != "" || conn.Verification["verified"] != false {
		t.Fatalf("seeded record = %+v", conn)
	}
	org, err := st.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil || org.DefaultConnectionKey != opts.ConnectionKey {
		t.Fatalf("default marker = %+v, %v", org, err)
	}
	ready := conn
	ready.Status, ready.SecretRef = application.ConnectionReady, "fake-credential://kept"
	if err := st.SaveConnection(ctx, ready); err != nil {
		t.Fatal(err)
	}
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey); again.Status != application.ConnectionReady || again.SecretRef != "fake-credential://kept" {
		t.Fatalf("re-seed replaced an existing record: %+v", again)
	}
}
