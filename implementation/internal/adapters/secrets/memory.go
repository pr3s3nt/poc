// Package secrets implements the SecretStore port. Values stay in process memory
// and are never written to the state snapshot or to logs.
package secrets

import (
	"context"
	"sync"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
)

// Memory is an in-process Secret Store.
type Memory struct {
	mu     sync.RWMutex
	values map[string]string
}

// NewMemory returns an empty Secret Store.
func NewMemory() *Memory { return &Memory{values: map[string]string{}} }

// Put stores a secret value and returns its opaque reference.
func (m *Memory) Put(_ context.Context, name string, value string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := "secret://" + name + "/" + ids.New()
	m.values[ref] = value
	return ref, nil
}

// Get resolves a secret reference.
func (m *Memory) Get(_ context.Context, ref string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.values[ref]
	return v, ok
}

var _ execution.SecretStore = (*Memory)(nil)
