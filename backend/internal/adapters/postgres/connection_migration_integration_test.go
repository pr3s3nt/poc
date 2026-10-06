package postgres

import (
	"context"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/seed"
)

// Migration 5 backfills name/authentication type of pre-UC-04-upload rows and
// enforces kind compatibility. Registration never changes the default link.
func TestMigration5BackfillsConnectionNameAndAuthentication(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	var defaultID string
	if err := st.pool.QueryRow(ctx, `SELECT default_connection_id::text FROM organizations WHERE organization_key=$1`, opts.OrganizationKey).Scan(&defaultID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `ALTER TABLE connections DROP CONSTRAINT connections_authentication_type_check; ALTER TABLE connections DROP COLUMN name; ALTER TABLE connections DROP COLUMN authentication_type; DELETE FROM schema_migrations WHERE version=5`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for key, want := range map[string]application.AuthenticationType{opts.ConnectionKey: application.AuthHostContext, opts.CloudConnectionKey: application.AuthAWSAccessKey} {
		conn, err := reopened.GetConnection(ctx, opts.OrganizationKey, key)
		if err != nil || conn.Name != key || conn.AuthenticationType != want {
			t.Fatalf("%s backfill = %+v, %v", key, conn, err)
		}
	}
	if _, err := reopened.pool.Exec(ctx, `UPDATE connections SET authentication_type='KUBECONFIG' WHERE connection_key=$1`, opts.CloudConnectionKey); err == nil {
		t.Fatal("kind-incompatible authentication type accepted")
	}
	if err := reopened.CreateConnection(ctx, application.Connection{Key: "uploaded", Name: "Uploaded", OrganizationKey: opts.OrganizationKey, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady, Config: map[string]any{"kubeContext": "lab"},
		SecretRef: "kv2://kv/orchestrator/connections/acme/uploaded/credentials/00000000-0000-4000-8000-000000000002", Verification: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := reopened.pool.QueryRow(ctx, `SELECT default_connection_id::text FROM organizations WHERE organization_key=$1`, opts.OrganizationKey).Scan(&after); err != nil || after != defaultID {
		t.Fatalf("registration changed the default connection: %s -> %s (%v)", defaultID, after, err)
	}
}
