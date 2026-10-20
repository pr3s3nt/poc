package postgres

import (
	"context"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/seed"
)

// The fresh schema accepts only KUBECONFIG for Kubernetes and AWS_ACCESS_KEY
// for AWS. The product seed is VERIFYING KUBECONFIG metadata without a
// credential reference. No Organization deployment default exists.
// Registration preserves the entire Organization metadata and scoped ownership.
func TestFreshSchemaSeedsVerifyingKubeconfigAndRejectsHostContext(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	beforeOrg, err := st.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	var connectionID string
	seeded, err := st.GetConnection(ctx, opts.OrganizationKey, opts.ConnectionKey)
	if err != nil || seeded.AuthenticationType != application.AuthKubeconfig || seeded.Status != application.ConnectionVerifying ||
		seeded.SecretRef != "" || seeded.Verification["verified"] != false || seeded.Name != opts.ConnectionKey {
		t.Fatalf("seeded connection = %+v, %v", seeded, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT c.id::text FROM connections c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.connection_key=$2`, opts.OrganizationKey, opts.ConnectionKey).Scan(&connectionID); err != nil || connectionID != seeded.ID {
		t.Fatalf("scoped seeded identity mismatch: %q vs %q (%v)", connectionID, seeded.ID, err)
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
	after, err := st.GetOrganization(ctx, opts.OrganizationKey)
	if err != nil || after != beforeOrg {
		t.Fatalf("registration changed Organization metadata: %+v -> %+v (%v)", beforeOrg, after, err)
	}
}
