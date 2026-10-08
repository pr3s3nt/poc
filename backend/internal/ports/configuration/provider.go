// Package configuration defines the provider boundary for Application values.
package configuration

import (
	"context"
	"errors"

	"orchestrator/internal/domain/secretstore"
)

// ErrStoreUnavailable means a referenced store is missing, foreign, not READY
// or lacks usable credentials. It never carries remote text or credentials.
var ErrStoreUnavailable = errors.New("configuration: the secret store is unavailable")

// ErrNoStore means no store is selected where one is required.
var ErrNoStore = errors.New("configuration: no secret store is selected")

// Provider stores values outside orchestrator state and returns opaque refs.
// ReadValue is an internal-only operation; delivery must never expose secrets.
// One Provider is bound to exactly one registered store.
type Provider interface {
	WriteValue(ctx context.Context, applicationKey, environmentKey, value string) (string, error)
	ReadValue(ctx context.Context, ref string) (string, error)
}

// StoreRef names a value together with the store that owns it.
type StoreRef struct {
	StoreKey string
	ValueRef string
}

// WorkloadAccess is the immutable, least-privilege Vault identity for one
// workload/revision. It contains no value or token bytes.
type WorkloadAccess struct {
	Role           string
	Address        string
	ServiceAccount string
}

// WorkloadBundle is the immutable store source and Kubernetes destination for
// one applied workload revision. Keys map container/environment names to Secret
// data keys; no value bytes cross this port. The store identity, in-cluster
// address and Kubernetes auth mount are pinned here, never read from flags.
type WorkloadBundle struct {
	StoreKey, Address, Mount, AuthMount, Path, Role, ServiceAccount, SecretName string
	CAPEM                                                                       string
	Keys                                                                        map[string]map[string]string
}

// BundleRequest asks the delivery store to host one workload's bundle. Each
// ref is read through its own owning store, so legacy and migrated refs mix.
type BundleRequest struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, WorkloadID string
	Namespace, RevisionID, DeploymentID                         string
	DeliveryStoreKey                                            string
	Refs                                                        map[string]map[string]StoreRef
}

// AccessRequest is the agent-delivery variant: all refs live in one store.
type AccessRequest struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, WorkloadID string
	Namespace, RevisionID, StoreKey                             string
	Refs                                                        []string
}

// Registry resolves Organization-scoped stores. Runtime lookup never falls
// back to another store or a host credential.
type Registry interface {
	Provider(ctx context.Context, organizationKey, storeKey string) (Provider, error)
	PrepareBundle(ctx context.Context, req BundleRequest) (WorkloadBundle, error)
	PrepareAccess(ctx context.Context, req AccessRequest) (WorkloadAccess, error)
	// CheckWorkloadAuth verifies, before a secret-dependent deploy, that the
	// store still verifies and its Kubernetes auth mount accepts role writes.
	CheckWorkloadAuth(ctx context.Context, organizationKey, storeKey string) error
}

// VerifyRequest is the private input of a registration verification.
type VerifyRequest struct {
	BackendAddress, Mount, AuthMount, CAPEM, Token string
}

// Verifier proves a store usable before it is persisted READY. Failures are
// typed categories with fixed text; token and remote bodies never appear.
type Verifier interface {
	Verify(ctx context.Context, req VerifyRequest) (map[string]any, error)
}

// Verification categories returned by a Verifier.
var (
	ErrUnreachable   = errors.New("the secret store could not be reached")
	ErrTokenRejected = errors.New("the secret store rejected the token")
	ErrNotKV2        = errors.New("the mount is not a Vault KV version 2 engine")
	ErrCapability    = errors.New("the token lacks a required capability")
	ErrProbe         = errors.New("the read/write probe failed")
	ErrProbeCleanup  = errors.New("the probe object could not be removed")
)

// Description projects a store into the nonsecret fields the resolver needs.
type Description = secretstore.Store
