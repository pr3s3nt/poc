package configmemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"orchestrator/internal/domain/secretstore"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

// Registry is the in-process store registry of fake/test mode. It enforces the
// same Organization scope and READY rules as the Vault registry and exposes
// failure injection for copy and delivery tests.
type Registry struct {
	stores persistence.SecretStoreRepository

	mu        sync.Mutex
	providers map[string]*Provider
	// writeBudget limits successful writes per store key: a missing entry is
	// unlimited, zero fails every write. Used to test copy failure.
	writeBudget map[string]int
	// readFail makes ReadValue fail for the store key.
	readFail map[string]bool
	// deleteFail makes DeleteValue fail for the store key.
	deleteFail map[string]bool
	// authFail makes CheckWorkloadAuth fail for the store key.
	authFail map[string]bool
	// Bundles records bundle requests per delivery store (no values).
	Bundles []configport.BundleRequest
	// Access records agent-delivery requests per store (no values).
	Access []configport.AccessRequest
}

// NewRegistry returns a registry backed by the persisted store metadata.
func NewRegistry(stores persistence.SecretStoreRepository) *Registry {
	return &Registry{stores: stores, providers: map[string]*Provider{}, writeBudget: map[string]int{}, readFail: map[string]bool{}, deleteFail: map[string]bool{}, authFail: map[string]bool{}}
}

func (r *Registry) describe(ctx context.Context, org, key string) (secretstore.Store, error) {
	store, err := r.stores.GetSecretStore(ctx, org, key)
	if err != nil || store.OrganizationKey != org || store.Status != secretstore.StatusReady {
		return secretstore.Store{}, configport.ErrStoreUnavailable
	}
	return store, nil
}

// LimitWrites lets only n more writes to the store succeed.
func (r *Registry) LimitWrites(storeKey string, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeBudget[storeKey] = n
}

// FailReads makes every read through the store fail.
func (r *Registry) FailReads(storeKey string, fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readFail[storeKey] = fail
}

// FailDeletes makes every value removal through the store fail.
func (r *Registry) FailDeletes(storeKey string, fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteFail[storeKey] = fail
}

// FailWorkloadAuth makes the pre-deploy auth check fail for the store.
func (r *Registry) FailWorkloadAuth(storeKey string, fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authFail[storeKey] = fail
}

// Values returns how many values the store currently holds.
func (r *Registry) Values(org, storeKey string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.providers[org+"/"+storeKey]; p != nil {
		p.mu.RLock()
		defer p.mu.RUnlock()
		return len(p.values)
	}
	return 0
}

type scoped struct {
	r        *Registry
	key      string
	provider *Provider
}

func (s scoped) WriteValue(ctx context.Context, app, env, value string) (string, error) {
	s.r.mu.Lock()
	budget, limited := s.r.writeBudget[s.key]
	if limited {
		if budget <= 0 {
			s.r.mu.Unlock()
			return "", fmt.Errorf("configmemory: injected write failure")
		}
		s.r.writeBudget[s.key] = budget - 1
	}
	s.r.mu.Unlock()
	return s.provider.WriteValue(ctx, app, env, value)
}

func (s scoped) ReadValue(ctx context.Context, ref string) (string, error) {
	s.r.mu.Lock()
	fail := s.r.readFail[s.key]
	s.r.mu.Unlock()
	if fail {
		return "", fmt.Errorf("configmemory: injected read failure")
	}
	return s.provider.ReadValue(ctx, ref)
}

func (s scoped) DeleteValue(_ context.Context, ref string) error {
	s.r.mu.Lock()
	fail := s.r.deleteFail[s.key]
	s.r.mu.Unlock()
	if fail {
		return fmt.Errorf("configmemory: injected delete failure")
	}
	s.provider.mu.Lock()
	defer s.provider.mu.Unlock()
	delete(s.provider.values, ref)
	return nil
}

func (r *Registry) provider(ctx context.Context, org, key string) (scoped, secretstore.Store, error) {
	store, err := r.describe(ctx, org, key)
	if err != nil {
		return scoped{}, store, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := org + "/" + key
	p := r.providers[id]
	if p == nil {
		p = New()
		r.providers[id] = p
	}
	return scoped{r: r, key: key, provider: p}, store, nil
}

// Provider returns the scoped in-memory provider of one store.
func (r *Registry) Provider(ctx context.Context, org, key string) (configport.Provider, error) {
	p, _, err := r.provider(ctx, org, key)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// PrepareBundle validates every ref through its owning store and returns a
// deterministic fake bundle pinned to the delivery store's metadata.
func (r *Registry) PrepareBundle(ctx context.Context, req configport.BundleRequest) (configport.WorkloadBundle, error) {
	_, store, err := r.provider(ctx, req.OrganizationKey, req.DeliveryStoreKey)
	if err != nil {
		return configport.WorkloadBundle{}, err
	}
	keys := map[string]map[string]string{}
	for container, entries := range req.Refs {
		keys[container] = map[string]string{}
		for name, ref := range entries {
			owner, _, err := r.provider(ctx, req.OrganizationKey, ref.StoreKey)
			if err != nil {
				return configport.WorkloadBundle{}, err
			}
			if _, err := owner.ReadValue(ctx, ref.ValueRef); err != nil {
				return configport.WorkloadBundle{}, err
			}
			keys[container][name] = container + "_" + name
		}
	}
	r.mu.Lock()
	r.Bundles = append(r.Bundles, req)
	r.mu.Unlock()
	sum := sha256.Sum256([]byte(req.ApplicationKey + "/" + req.EnvironmentKey + "/" + req.WorkloadID + "/" + req.DeploymentID))
	return configport.WorkloadBundle{
		StoreKey: store.Key, Address: store.WorkloadAddress, Mount: store.Mount, AuthMount: store.AuthMount,
		Path: "orchestrator/apps/" + req.ApplicationKey + "/envs/" + req.EnvironmentKey + "/values/" + req.DeploymentID,
		Role: "fake", ServiceAccount: req.WorkloadID + "-vault", SecretName: "orch-" + hex.EncodeToString(sum[:10]), CAPEM: store.TLSCAPEM, Keys: keys,
	}, nil
}

// PrepareAccess returns the fake agent-delivery access of a store.
func (r *Registry) PrepareAccess(ctx context.Context, req configport.AccessRequest) (configport.WorkloadAccess, error) {
	_, store, err := r.provider(ctx, req.OrganizationKey, req.StoreKey)
	if err != nil {
		return configport.WorkloadAccess{}, err
	}
	r.mu.Lock()
	r.Access = append(r.Access, req)
	r.mu.Unlock()
	return configport.WorkloadAccess{Role: "fake", Address: store.WorkloadAddress, ServiceAccount: req.WorkloadID + "-vault"}, nil
}

// CheckWorkloadAuth succeeds unless failure injection says otherwise.
func (r *Registry) CheckWorkloadAuth(ctx context.Context, org, key string) error {
	if _, _, err := r.provider(ctx, org, key); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.authFail[key] {
		return fmt.Errorf("%w: workload Kubernetes auth is not available", configport.ErrStoreUnavailable)
	}
	return nil
}

// Verifier accepts any token except the documented failure tokens.
type Verifier struct{}

// Verify implements configport.Verifier for fake mode.
func (Verifier) Verify(_ context.Context, req configport.VerifyRequest) (map[string]any, error) {
	switch req.Token {
	case "unreachable-token":
		return nil, configport.ErrUnreachable
	case "bad-token":
		return nil, configport.ErrTokenRejected
	case "no-kv2-token":
		return nil, configport.ErrNotKV2
	case "no-caps-token":
		return nil, fmt.Errorf("%w: create on Kubernetes auth roles", configport.ErrCapability)
	case "probe-token":
		return nil, configport.ErrProbe
	case "cleanup-token":
		return nil, configport.ErrProbeCleanup
	}
	return map[string]any{"verified": true, "kvVersion": float64(2), "capabilities": "VERIFIED", "probe": "REMOVED", "kubernetesAuth": "CONFIGURED"}, nil
}

var (
	_ configport.Registry = (*Registry)(nil)
	_ configport.Verifier = Verifier{}
)
