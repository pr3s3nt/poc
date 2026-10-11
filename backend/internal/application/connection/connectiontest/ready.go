package connectiontest

import (
	"context"
	"errors"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
)

// connectionSaver is the persistence subset PrepareReady needs.
type connectionSaver interface {
	GetConnection(ctx context.Context, organizationKey, key string) (application.Connection, error)
	SaveConnection(ctx context.Context, conn application.Connection) error
}

// PrepareReady makes a Kubernetes Connection explicitly executable in a test:
// it writes a synthetic selected-context kubeconfig into the scoped credential
// store and saves KUBECONFIG metadata in READY state referencing that
// credential. Only test and conformance harnesses call it; no product path
// derives READY without an uploaded credential.
//
// An existing record keeps its ID (SQL foreign keys reference it). An existing
// READY KUBECONFIG record that already has a credential reference (for example
// a real upload) is left untouched. Fake-only metadata setup: tests that prove
// the real resolver must pass the injected store and a real document path.
func PrepareReady(ctx context.Context, store connectionSaver, creds credentials.Store, org, key, kubeContext, cluster string) error {
	existing, err := store.GetConnection(ctx, org, key)
	switch {
	case err == nil:
		if existing.Status == application.ConnectionReady && existing.AuthenticationType == application.AuthKubeconfig && existing.SecretRef != "" {
			return nil
		}
	case !errors.Is(err, persistence.ErrNotFound):
		return err
	}
	document := Single(kubeContext, "https://"+cluster+".invalid", "synthetic-token-"+key+"-0123456789")
	ref, err := creds.Put(ctx, org, key, []byte(document))
	if err != nil {
		return err
	}
	return store.SaveConnection(ctx, application.Connection{
		ID: existing.ID, Key: key, Name: key, OrganizationKey: org, Kind: application.ConnectionKubernetes,
		AuthenticationType: application.AuthKubeconfig, Status: application.ConnectionReady, SecretRef: ref,
		Config:       map[string]any{"cluster": cluster, "kubeContext": kubeContext, "endpoint": "https://" + cluster + ".invalid"},
		Verification: map[string]any{"verified": true, "endpoint": "https://" + cluster + ".invalid"},
	})
}
