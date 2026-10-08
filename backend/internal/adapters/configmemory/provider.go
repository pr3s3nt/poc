// Package configmemory is an in-process UC-12 provider for fake/test mode only.
package configmemory

import (
	"context"
	"fmt"
	"sync"

	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
)

type Provider struct {
	mu     sync.RWMutex
	values map[string]string
}

func New() *Provider { return &Provider{values: map[string]string{}} }

func (p *Provider) WriteValue(_ context.Context, app, env, value string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Keep the fake ref shape compatible with the renderer while values remain
	// in process memory; fake mode must never be used for a real cluster apply.
	ref := "kv2://kv/orchestrator/apps/" + app + "/envs/" + env + "/values/" + ids.New()
	p.values[ref] = value
	return ref, nil
}

func (p *Provider) ReadValue(_ context.Context, ref string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	value, ok := p.values[ref]
	if !ok {
		return "", fmt.Errorf("configmemory: value not found")
	}
	return value, nil
}

var _ configport.Provider = (*Provider)(nil)
