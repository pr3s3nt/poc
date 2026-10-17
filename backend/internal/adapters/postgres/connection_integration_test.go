package postgres

import (
	"context"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/seed"
)

// The fresh schema accepts only KUBECONFIG for Kubernetes and AWS_ACCESS_KEY
// for AWS. The product seed is VERIFYING KUBECONFIG metadata without a
// credential reference and still satisfies the Organization default link.
// Registration never changes the default link.
func TestFreshSchemaSeedsVerifyingKubeconfigAndRejectsHostContext(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	var defaultID, connectionID string
	if err := st.pool.QueryRow(ctx, `SELECT default_connection_id::text FROM organizations WHERE organization_key=$1`, opts.OrganizationKey).Scan(&defaultID); err != nil {
		t.Fatal(err)
	}
	seeded, err := st.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
	if err != nil || seeded.AuthenticationType != application.AuthKubeconfig || seeded.Status != application.ConnectionVerifying ||
		seeded.SecretRef != "" || seeded.Verification["verified"] != false || seeded.Name != opts.ConnectionKey {
		t.Fatalf("seeded connection = %+v, %v", seeded, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT id::text FROM connections WHERE connection_key=$1`, opts.ConnectionKey).Scan(&connectionID); err != nil || connectionID != defaultID {
		t.Fatalf("default link %q does not reference the seeded record %q (%v)", defaultID, connectionID, err)
	}
	cloud, err := st.GetConnection(ctx, opts.OrganizationKey, opts.CloudConnectionKey)
	if err != nil || cloud.AuthenticationType != application.AuthAWSAccessKey {
		t.Fatalf("cloud connection = %+v, %v", cloud, err)
	}
	for name, update := range map[string]string{
		"kind-incompatible": `UPDATE connections SET authentication_type='KUBECONFIG' WHERE connection_key='` + opts.CloudConnectionKey + `'`,
		"host context":      `UPDATE connections SET authentication_type='HOST_CONTEXT' WHERE connection_key='` + opts.ConnectionKey + `'`,
		"zero version":      `UPDATE connections SET version=0 WHERE connection_key='` + opts.ConnectionKey + `'`,
	} {
		if _, err := st.pool.Exec(ctx, update); err == nil {
			t.Fatalf("%s authentication type accepted", name)
		}
	}
	if cloud.Version != 1 {
		t.Fatalf("seeded connection version = %d, want 1", cloud.Version)
	}
	if err := st.CreateConnection(ctx, application.Connection{Key: "uploaded", Name: "Uploaded", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady, Config: map[string]any{"kubeContext": "lab"},
		SecretRef: "kv2://kv/orchestrator/connections/acme/uploaded/credentials/00000000-0000-4000-8000-000000000002", Verification: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := st.pool.QueryRow(ctx, `SELECT default_connection_id::text FROM organizations WHERE organization_key=$1`, opts.OrganizationKey).Scan(&after); err != nil || after != defaultID {
		t.Fatalf("registration changed the default connection: %s -> %s (%v)", defaultID, after, err)
	}
}
