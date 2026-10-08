package vault

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"orchestrator/internal/domain/secretstore"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
)

// LegacyConfig is the flag-configured Vault of earlier releases. It is the
// credential source of the explicit per-Organization legacy store only; it is
// never the fallback of any other store or of an unselected Environment.
type LegacyConfig struct {
	Address, AgentAddress, Mount, AuthMount, Token string
}

// Registry resolves Organization-scoped Vault KV v2 stores from persisted
// metadata plus the platform credential store (ADR-012).
type Registry struct {
	stores persistence.SecretStoreRepository
	creds  credentials.Store
	legacy *LegacyConfig

	mu    sync.Mutex
	cache map[string]*Provider
}

// NewRegistry wires the store repository, the platform credential store (may
// be nil: only the legacy store then resolves) and the optional legacy Vault.
func NewRegistry(stores persistence.SecretStoreRepository, creds credentials.Store, legacy *LegacyConfig) *Registry {
	return &Registry{stores: stores, creds: creds, legacy: legacy, cache: map[string]*Provider{}}
}

// LegacyStoreKey is the generated key of the explicit platform store.
const LegacyStoreKey = "platform-vault"

func (r *Registry) token(ctx context.Context, store secretstore.Store) (string, error) {
	if store.Legacy {
		if r.legacy == nil || r.legacy.Token == "" || store.BackendAddress != strings.TrimRight(r.legacy.Address, "/") || store.Mount != r.legacy.Mount {
			return "", fmt.Errorf("%w: the platform Vault is not configured on this backend", configport.ErrStoreUnavailable)
		}
		return r.legacy.Token, nil
	}
	if r.creds == nil {
		return "", configport.ErrStoreUnavailable
	}
	raw, err := r.creds.Get(ctx, store.OrganizationKey, store.CredentialScope(), store.CredentialRef)
	if err != nil {
		return "", configport.ErrStoreUnavailable
	}
	return strings.TrimSpace(string(raw)), nil
}

// Describe returns the persisted store when it is READY in the Organization.
func (r *Registry) describe(ctx context.Context, org, key string) (secretstore.Store, error) {
	store, err := r.stores.GetSecretStore(ctx, org, key)
	if err != nil || store.OrganizationKey != org || store.Status != secretstore.StatusReady || store.Provider != secretstore.ProviderVaultKV2 {
		return secretstore.Store{}, configport.ErrStoreUnavailable
	}
	return store, nil
}

func (r *Registry) provider(ctx context.Context, org, key string) (*Provider, secretstore.Store, error) {
	store, err := r.describe(ctx, org, key)
	if err != nil {
		return nil, store, err
	}
	cacheKey := store.ID + "|" + store.CredentialRef
	r.mu.Lock()
	cached := r.cache[cacheKey]
	r.mu.Unlock()
	if cached != nil {
		return cached, store, nil
	}
	token, err := r.token(ctx, store)
	if err != nil {
		return nil, store, err
	}
	p, err := NewWithCA(store.BackendAddress, token, store.Mount, store.TLSCAPEM)
	if err != nil {
		return nil, store, configport.ErrStoreUnavailable
	}
	if err := p.SetAgentAddress(store.WorkloadAddress); err != nil {
		return nil, store, configport.ErrStoreUnavailable
	}
	if err := p.SetAuthMount(store.AuthMount); err != nil {
		return nil, store, configport.ErrStoreUnavailable
	}
	r.mu.Lock()
	r.cache[cacheKey] = p
	r.mu.Unlock()
	return p, store, nil
}

// Provider returns the scoped provider of one store.
func (r *Registry) Provider(ctx context.Context, org, key string) (configport.Provider, error) {
	p, _, err := r.provider(ctx, org, key)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// PrepareBundle reads every ref through its owning store and writes the
// immutable bundle, policy and role in the delivery store.
func (r *Registry) PrepareBundle(ctx context.Context, req configport.BundleRequest) (configport.WorkloadBundle, error) {
	delivery, store, err := r.provider(ctx, req.OrganizationKey, req.DeliveryStoreKey)
	if err != nil {
		return configport.WorkloadBundle{}, err
	}
	values := map[string]map[string]string{}
	for container, entries := range req.Refs {
		values[container] = map[string]string{}
		for name, ref := range entries {
			owner := delivery
			if ref.StoreKey != req.DeliveryStoreKey {
				if ref.StoreKey == "" {
					return configport.WorkloadBundle{}, fmt.Errorf("%w: a configuration value has no store identity", configport.ErrStoreUnavailable)
				}
				owner, _, err = r.provider(ctx, req.OrganizationKey, ref.StoreKey)
				if err != nil {
					return configport.WorkloadBundle{}, err
				}
			}
			if !owner.OwnsRef(ref.ValueRef, req.ApplicationKey, req.EnvironmentKey) {
				return configport.WorkloadBundle{}, fmt.Errorf("vault: value reference is outside workload scope")
			}
			value, err := owner.ReadValue(ctx, ref.ValueRef)
			if err != nil {
				return configport.WorkloadBundle{}, err
			}
			values[container][name] = value
		}
	}
	bundle, err := delivery.WriteBundle(ctx, req.ApplicationKey, req.EnvironmentKey, req.WorkloadID, req.Namespace, req.RevisionID, req.DeploymentID, values)
	if err != nil {
		return configport.WorkloadBundle{}, err
	}
	bundle.StoreKey = store.Key
	return bundle, nil
}

// PrepareAccess is the agent-delivery variant for refs of one store.
func (r *Registry) PrepareAccess(ctx context.Context, req configport.AccessRequest) (configport.WorkloadAccess, error) {
	p, _, err := r.provider(ctx, req.OrganizationKey, req.StoreKey)
	if err != nil {
		return configport.WorkloadAccess{}, err
	}
	return p.PrepareWorkloadAccess(ctx, req.ApplicationKey, req.EnvironmentKey, req.WorkloadID, req.Namespace, req.RevisionID, req.Refs)
}

// CheckWorkloadAuth verifies the store and its Kubernetes auth mount.
func (r *Registry) CheckWorkloadAuth(ctx context.Context, org, key string) error {
	p, _, err := r.provider(ctx, org, key)
	if err != nil {
		return err
	}
	return p.CheckWorkloadAuth(ctx, "check")
}

var _ configport.Registry = (*Registry)(nil)
