package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/domain/application"
)

// Snapshots written before UC-04 upload have no Connection name or
// authentication type; known legacy shapes read with defaults and unknown
// shapes stay unresolved so execution fails closed.
func TestSnapshotConnectionsReadWithLegacyDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	snapshot := `{"connections":{
	  "acme/internal-cluster":{"key":"internal-cluster","organizationKey":"acme","kind":"KUBERNETES","config":{"kubeContext":"kind-idp-internal"},"secretRef":"secret://connections/internal-cluster","status":"READY","verification":{}},
	  "acme/aws-account":{"key":"aws-account","organizationKey":"acme","kind":"AWS","config":{},"secretRef":"secret://connections/aws-account","status":"READY","verification":{}},
	  "acme/odd":{"key":"odd","organizationKey":"acme","kind":"KUBERNETES","config":{},"secretRef":"vault://unknown","status":"READY","verification":{}}}}`
	if err := os.WriteFile(path, []byte(snapshot), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for key, want := range map[string]application.AuthenticationType{"internal-cluster": application.AuthHostContext, "aws-account": application.AuthAWSAccessKey, "odd": ""} {
		conn, err := st.GetConnection(ctx, "acme", key)
		if err != nil {
			t.Fatal(err)
		}
		if conn.Name != key || conn.AuthenticationType != want {
			t.Fatalf("%s: name %q authentication %q", key, conn.Name, conn.AuthenticationType)
		}
	}
}
