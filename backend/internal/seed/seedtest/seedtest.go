// Package seedtest holds test and conformance harness helpers around the
// product seed. The product seed creates the internal Kubernetes record as
// VERIFYING KUBECONFIG metadata without a credential. Fake and local test
// workflows that execute against it opt in here to explicit READY metadata
// with a synthetic scoped credential. No product path uses this package.
package seedtest

import (
	"context"

	"orchestrator/internal/adapters/credentialmemory"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

// Ready replaces the seeded Kubernetes record (keeping its ID) with READY
// KUBECONFIG metadata that references a synthetic credential in a throwaway
// in-memory store. This is fake-only setup: fake executors never resolve the
// credential. Harnesses with an injected credential store use ReadyIn.
func Ready(ctx context.Context, st persistence.Store, o seed.Options) error {
	return ReadyIn(ctx, st, credentialmemory.New(), o)
}

// ReadyIn is Ready with the caller's (injected) credential store, so the
// reference resolves through the real resolver.
func ReadyIn(ctx context.Context, st persistence.Store, creds credentials.Store, o seed.Options) error {
	return ct.PrepareReady(ctx, st, creds, o.OrganizationKey, o.ConnectionKey, o.KubeContext, o.ClusterName)
}

// Apply runs the product seed and then marks the seeded cluster READY.
func Apply(ctx context.Context, st persistence.Store, o seed.Options) error {
	if err := seed.Apply(ctx, st, o); err != nil {
		return err
	}
	return Ready(ctx, st, o)
}

// Defaults is seed.Defaults plus an RFC 5737 documentation address as the EKS
// public API allowlist. The product seed has no allowlist (a PE/Admin must
// supply one); this fixture exists only so tests can plan the AWS profile. It
// is never a real executor address.
func Defaults() seed.Options {
	o := seed.Defaults()
	o.Cloud.EKSPublicAccessCIDRs = []string{"203.0.113.10/32"}
	return o
}
