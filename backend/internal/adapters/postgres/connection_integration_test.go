package postgres

import (
	"context"
	"errors"
	"orchestrator/internal/ports/persistence"
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
	} {
		if _, err := st.pool.Exec(ctx, update); err == nil {
			t.Fatalf("%s authentication type accepted", name)
		}
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

// V01: seeded records start at version 1, the SQL constraint refuses a lower
// version on a real row, a repeated seed (restart) does not bump the version,
// and a CAS-updated version survives closing and reopening the Console store.
func TestConnectionVersionConstraintSeedIdempotenceAndReopen(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seed.Defaults()
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{opts.ConnectionKey, opts.CloudConnectionKey} {
		got, err := st.GetConnection(ctx, opts.OrganizationKey, key)
		if err != nil || got.Version != 1 {
			t.Fatalf("seeded %s = %+v, %v", key, got, err)
		}
	}
	if err := seed.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetConnection(ctx, opts.OrganizationKey, opts.CloudConnectionKey); got.Version != 1 {
		t.Fatalf("an idempotent re-seed changed the version: %d", got.Version)
	}
	for name, sql := range map[string]string{
		"zero":     `UPDATE connections SET version=0 WHERE connection_key='` + opts.CloudConnectionKey + `'`,
		"negative": `UPDATE connections SET version=-1 WHERE connection_key='` + opts.CloudConnectionKey + `'`,
		"null":     `UPDATE connections SET version=NULL WHERE connection_key='` + opts.CloudConnectionKey + `'`,
	} {
		if _, err := st.pool.Exec(ctx, sql); err == nil {
			t.Fatalf("%s version accepted by the Console login", name)
		}
	}
	cloud, _ := st.GetConnection(ctx, opts.OrganizationKey, opts.CloudConnectionKey)
	updated, err := st.UpdateConnection(ctx, persistence.ConnectionUpdate{OrganizationKey: cloud.OrganizationKey, Key: cloud.Key, ExpectedVersion: 1,
		Name: "Renamed", Config: cloud.Config, SecretRef: cloud.SecretRef, Status: cloud.Status, Verification: cloud.Verification})
	if err != nil || updated.Version != 2 || updated.Name != "Renamed" {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	st.Close()
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetConnection(ctx, opts.OrganizationKey, opts.CloudConnectionKey)
	if err != nil || got.Version != 2 || got.Name != "Renamed" {
		t.Fatalf("after reopen: %+v, %v", got, err)
	}
	if _, err := reopened.UpdateConnection(ctx, persistence.ConnectionUpdate{OrganizationKey: got.OrganizationKey, Key: got.Key, ExpectedVersion: 1,
		Name: "Stale", Config: got.Config, SecretRef: got.SecretRef, Status: got.Status, Verification: got.Verification}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale update after reopen: %v", err)
	}
	// A real transaction rolls an update back with the surrounding work.
	boom := errors.New("rollback")
	err = reopened.Transact(ctx, func(ctx context.Context) error {
		if _, err := reopened.UpdateConnection(ctx, persistence.ConnectionUpdate{OrganizationKey: got.OrganizationKey, Key: got.Key, ExpectedVersion: 2,
			Name: "InTx", Config: got.Config, SecretRef: got.SecretRef, Status: got.Status, Verification: got.Verification}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("transaction: %v", err)
	}
	if again, _ := reopened.GetConnection(ctx, opts.OrganizationKey, opts.CloudConnectionKey); again.Version != 2 || again.Name != "Renamed" {
		t.Fatalf("rolled-back update persisted: %+v", again)
	}
}
