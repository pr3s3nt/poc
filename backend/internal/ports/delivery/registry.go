package delivery

import (
	"errors"
	"sort"
	"sync"
)

var ErrProviderUnavailable = errors.New("delivery: provider unavailable")

// Registry is composed at bootstrap. Lifecycle services depend only on Resolve;
// they never switch on concrete Fleet/Argo/Flux implementation types.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: make(map[string]Provider)} }
func (r *Registry) Register(kind string, provider Provider) error {
	if kind == "" || provider == nil {
		return ErrProviderUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[kind]; ok {
		return errors.New("delivery: provider already registered")
	}
	if r.providers == nil {
		r.providers = make(map[string]Provider)
	}
	r.providers[kind] = provider
	return nil
}
func (r *Registry) Resolve(kind string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[kind]
	if !ok {
		return nil, ErrProviderUnavailable
	}
	return provider, nil
}

// Kinds returns only adapters installed in this runtime, in stable order.
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for kind := range r.providers {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}
