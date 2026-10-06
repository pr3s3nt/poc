// Package credentialmemory is the explicit local/test Connection credential
// store. It is not durable: bootstrap selects it only when asked to, and never
// for real Kubernetes adapters or persistent state.
package credentialmemory

import (
	"context"
	"fmt"
	"sync"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/credentials"
)

const (
	scheme = "memory"
	mount  = "local"
)

// Store keeps scoped credential objects in process memory.
type Store struct {
	mu      sync.Mutex
	objects map[string][]byte
	// FailPut, when set, makes Put fail after choosing the reference. Tests
	// use it to exercise rollback.
	FailPut error
}

// New returns an empty in-memory credential store.
func New() *Store { return &Store{objects: map[string][]byte{}} }

func (s *Store) Durable() bool { return false }

func (s *Store) Put(_ context.Context, org, connection string, value []byte) (string, error) {
	if !credentials.ValidScope(org, connection) || len(value) == 0 {
		return "", credentials.ErrInvalidReference
	}
	ref := credentials.Reference(scheme, mount, org, connection, ids.New())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FailPut != nil {
		return ref, fmt.Errorf("%w: %v", credentials.ErrUnavailable, s.FailPut)
	}
	if _, exists := s.objects[ref]; exists {
		return ref, credentials.ErrUnavailable
	}
	s.objects[ref] = append([]byte(nil), value...)
	return ref, nil
}

func (s *Store) Get(_ context.Context, org, connection, ref string) ([]byte, error) {
	if _, err := credentials.ParseReference(ref, scheme, mount, org, connection); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.objects[ref]
	if !ok {
		return nil, credentials.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *Store) Delete(_ context.Context, org, connection, ref string) error {
	if _, err := credentials.ParseReference(ref, scheme, mount, org, connection); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, ref)
	return nil
}

// Len returns the number of stored objects (test observation only).
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}

var _ credentials.Store = (*Store)(nil)
