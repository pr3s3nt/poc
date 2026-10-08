package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// CreateSecretStore inserts a store; an existing key is ErrDuplicate.
func (s *Store) CreateSecretStore(ctx context.Context, store secretstore.Store) error {
	defer s.lock(ctx)()
	if _, ok := s.state.Organizations[store.OrganizationKey]; !ok {
		return fmt.Errorf("%w: organization %q", persistence.ErrNotFound, store.OrganizationKey)
	}
	if err := store.Validate(); err != nil {
		return err
	}
	key := catalogKey(store.OrganizationKey, store.Key)
	if _, exists := s.state.SecretStores[key]; exists {
		return fmt.Errorf("%w: secret store %q", persistence.ErrDuplicate, store.Key)
	}
	if store.ID == "" {
		store.ID = ids.New()
	}
	if store.CreatedAt.IsZero() {
		store.CreatedAt = time.Now().UTC()
	}
	s.state.SecretStores[key] = store
	return nil
}

// GetSecretStore reads one Organization-scoped store.
func (s *Store) GetSecretStore(ctx context.Context, organizationKey, key string) (secretstore.Store, error) {
	defer s.rlock(ctx)()
	st, ok := s.state.SecretStores[catalogKey(organizationKey, key)]
	if !ok {
		return secretstore.Store{}, fmt.Errorf("%w: secret store %q", persistence.ErrNotFound, key)
	}
	return st, nil
}

// ListSecretStores lists the Organization's stores ordered by key.
func (s *Store) ListSecretStores(ctx context.Context, organizationKey string) ([]secretstore.Store, error) {
	defer s.rlock(ctx)()
	out := []secretstore.Store{}
	for key, st := range s.state.SecretStores {
		if strings.HasPrefix(key, organizationKey+"/") {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// BackfillLegacySecretStore stamps the explicit legacy identity, idempotently.
func (s *Store) BackfillLegacySecretStore(ctx context.Context, organizationKey, storeKey string) error {
	defer s.lock(ctx)()
	if _, ok := s.state.SecretStores[catalogKey(organizationKey, storeKey)]; !ok {
		return fmt.Errorf("%w: secret store %q", persistence.ErrNotFound, storeKey)
	}
	for id, revision := range s.state.ConfigRevisions {
		app, ok := s.state.Applications[revision.ApplicationKey]
		if !ok || app.OrganizationKey != organizationKey {
			continue
		}
		changed := false
		for name, entry := range revision.Entries {
			if entry.StoreKey == "" && entry.ValueRef != "" {
				entry.StoreKey = storeKey
				revision.Entries[name] = entry
				changed = true
			}
		}
		if changed {
			s.state.ConfigRevisions[id] = revision
		}
	}
	for key, env := range s.state.Environments {
		app, ok := s.state.Applications[env.ApplicationKey]
		if !ok || app.OrganizationKey != organizationKey || env.SecretStoreKey != "" {
			continue
		}
		scope := s.state.ConfigScopes[key]
		if scope.DesiredRevisionID == "" {
			continue
		}
		if revision, ok := s.state.ConfigRevisions[scope.DesiredRevisionID]; ok && hasStoreEntries(revision) {
			env.SecretStoreKey = storeKey
			s.state.Environments[key] = env
		}
	}
	return nil
}

func hasStoreEntries(revision configuration.Revision) bool {
	for _, entry := range revision.Entries {
		if entry.UsesStore() {
			return true
		}
	}
	return false
}

// ClaimEnvironment verifies pins and claims the Environment atomically.
func (s *Store) ClaimEnvironment(ctx context.Context, claim persistence.OperationClaim) (environment.Operation, error) {
	defer s.lock(ctx)()
	key := envKey(claim.ApplicationKey, claim.EnvironmentKey)
	env, ok := s.state.Environments[key]
	if !ok {
		return environment.Operation{}, fmt.Errorf("%w: environment %s", persistence.ErrNotFound, key)
	}
	if env.ActiveOperationID != "" {
		return environment.Operation{}, persistence.Busy(claim.ApplicationKey, claim.EnvironmentKey)
	}
	pins := claim.Pins
	scope := s.state.ConfigScopes[key]
	stale := (pins.CheckEnvVersion && env.Version != pins.EnvVersion) ||
		(pins.CheckDraftVersion && env.DraftVersion != pins.DraftVersion) ||
		(pins.CheckConfigVersion && scope.Version != pins.ConfigVersion) ||
		(pins.CheckSet && env.CurrentDeploymentSetID != pins.CurrentSetID) ||
		(pins.CheckRevision && scope.DesiredRevisionID != pins.DesiredRevisionID) ||
		(pins.CheckBinding && env.Binding() != pins.Binding) ||
		(pins.CheckStore && env.SecretStoreKey != pins.SecretStoreKey)
	if stale {
		return environment.Operation{}, fmt.Errorf("%w: environment %s changed since it was read", persistence.ErrVersionConflict, key)
	}
	now := time.Now().UTC()
	if claim.ID != "" {
		if _, exists := s.state.Operations[claim.ID]; exists {
			return environment.Operation{}, fmt.Errorf("%w: operation id %q is already used", persistence.ErrDuplicate, claim.ID)
		}
	}
	op := environment.Operation{
		ID: claim.ID, ApplicationKey: claim.ApplicationKey, EnvironmentKey: claim.EnvironmentKey,
		Kind: claim.Kind, Owner: claim.Owner, Fence: 1, Status: environment.OpActive, Pins: pins,
		Detail: copyMap(claim.Detail), StartedAt: now, UpdatedAt: now, HeartbeatAt: now, Deadline: claim.Deadline,
	}
	if op.ID == "" {
		op.ID = ids.New()
	}
	s.state.Operations[op.ID] = op
	env.ActiveOperationID = op.ID
	s.state.Environments[key] = env
	return op, nil
}

// GetOperation reads one operation record.
func (s *Store) GetOperation(ctx context.Context, id string) (environment.Operation, error) {
	defer s.rlock(ctx)()
	op, ok := s.state.Operations[id]
	if !ok {
		return environment.Operation{}, fmt.Errorf("%w: operation %q", persistence.ErrNotFound, id)
	}
	op.Detail = copyMap(op.Detail)
	return op, nil
}

// ActiveOperation returns the operation holding the Environment, if any.
func (s *Store) ActiveOperation(ctx context.Context, applicationKey, environmentKey string) (environment.Operation, bool, error) {
	defer s.rlock(ctx)()
	env, ok := s.state.Environments[envKey(applicationKey, environmentKey)]
	if !ok {
		return environment.Operation{}, false, fmt.Errorf("%w: environment", persistence.ErrNotFound)
	}
	if env.ActiveOperationID == "" {
		return environment.Operation{}, false, nil
	}
	op := s.state.Operations[env.ActiveOperationID]
	op.Detail = copyMap(op.Detail)
	return op, true, nil
}

// ListOperations returns recent operations of an Environment, newest first.
func (s *Store) ListOperations(ctx context.Context, applicationKey, environmentKey string, limit int) ([]environment.Operation, error) {
	defer s.rlock(ctx)()
	var out []environment.Operation
	for _, op := range s.state.Operations {
		if op.ApplicationKey == applicationKey && op.EnvironmentKey == environmentKey {
			op.Detail = copyMap(op.Detail)
			out = append(out, op)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// HeartbeatOperation extends a live claim of its fenced owner.
func (s *Store) HeartbeatOperation(ctx context.Context, owner persistence.Owner, stage string, detail map[string]any) error {
	defer s.lock(ctx)()
	op, ok := s.state.Operations[owner.OperationID]
	if !ok || op.Owner != owner.Owner || op.Fence != owner.Fence || (op.Status != environment.OpActive && op.Status != environment.OpRecovering) {
		return persistence.ErrOperationLost
	}
	now := time.Now().UTC()
	op.HeartbeatAt, op.UpdatedAt = now, now
	if stage != "" {
		op.Stage = stage
	}
	if detail != nil {
		op.Detail = copyMap(detail)
	}
	s.state.Operations[owner.OperationID] = op
	return nil
}

// ReleaseOperation ends a claim and frees the Environment; only the current
// fenced owner may release it.
func (s *Store) ReleaseOperation(ctx context.Context, owner persistence.Owner, status environment.OperationStatus, failure string) error {
	defer s.lock(ctx)()
	op, ok := s.state.Operations[owner.OperationID]
	if !ok {
		return fmt.Errorf("%w: operation %q", persistence.ErrNotFound, owner.OperationID)
	}
	if status.Holds() {
		return fmt.Errorf("store: release needs a terminal status")
	}
	if !op.Status.Holds() {
		return nil
	}
	if op.Owner != owner.Owner || op.Fence != owner.Fence {
		return persistence.ErrOperationLost
	}
	now := time.Now().UTC()
	op.Status, op.Failure, op.UpdatedAt, op.FinishedAt = status, failure, now, &now
	s.state.Operations[owner.OperationID] = op
	key := envKey(op.ApplicationKey, op.EnvironmentKey)
	if env, ok := s.state.Environments[key]; ok && env.ActiveOperationID == owner.OperationID {
		env.ActiveOperationID = ""
		s.state.Environments[key] = env
	}
	return nil
}

// SuspendOperation returns a held claim to INTERRUPTED without releasing it.
func (s *Store) SuspendOperation(ctx context.Context, owner persistence.Owner, failure string) error {
	defer s.lock(ctx)()
	op, ok := s.state.Operations[owner.OperationID]
	if !ok || op.Owner != owner.Owner || op.Fence != owner.Fence || (op.Status != environment.OpActive && op.Status != environment.OpRecovering) {
		return persistence.ErrOperationLost
	}
	op.Status, op.Failure, op.UpdatedAt = environment.OpInterrupted, failure, time.Now().UTC()
	s.state.Operations[owner.OperationID] = op
	return nil
}

// MarkInterrupted flags stale ACTIVE claims without releasing them.
func (s *Store) MarkInterrupted(ctx context.Context, staleBefore time.Time) (int, error) {
	defer s.lock(ctx)()
	count := 0
	for id, op := range s.state.Operations {
		if (op.Status == environment.OpActive || op.Status == environment.OpRecovering) && op.HeartbeatAt.Before(staleBefore) {
			op.Status, op.UpdatedAt = environment.OpInterrupted, time.Now().UTC()
			s.state.Operations[id] = op
			count++
		}
	}
	return count, nil
}

// BeginRecovery moves an INTERRUPTED claim to RECOVERING under a new owner and
// increments the fence, so the former owner can no longer write or release.
func (s *Store) BeginRecovery(ctx context.Context, id, newOwner, confirmedBy string) (environment.Operation, error) {
	defer s.lock(ctx)()
	if strings.TrimSpace(confirmedBy) == "" {
		return environment.Operation{}, persistence.ErrRecoveryUnconfirmed
	}
	op, ok := s.state.Operations[id]
	if !ok {
		return environment.Operation{}, fmt.Errorf("%w: operation %q", persistence.ErrNotFound, id)
	}
	if op.Status != environment.OpInterrupted {
		return environment.Operation{}, fmt.Errorf("%w: operation %q is %s", persistence.ErrVersionConflict, id, op.Status)
	}
	now := time.Now().UTC()
	op.Status, op.Owner, op.Fence, op.UpdatedAt, op.HeartbeatAt = environment.OpRecovering, newOwner, op.Fence+1, now, now
	// Recovery gets its own bounded window; the original deadline stays in the audit detail.
	op.Detail = copyMap(op.Detail)
	if op.Detail == nil {
		op.Detail = map[string]any{}
	}
	op.Detail["originalDeadline"] = op.Deadline.Format(time.RFC3339)
	op.Deadline = now.Add(persistence.RecoveryWindow)
	op.Detail = copyMap(op.Detail)
	if op.Detail == nil {
		op.Detail = map[string]any{}
	}
	op.Detail["stoppedConfirmedBy"] = confirmedBy
	s.state.Operations[id] = op
	return op, nil
}

// SaveTransition upserts a transition record.
func (s *Store) SaveTransition(ctx context.Context, t environment.Transition) error {
	defer s.lock(ctx)()
	if err := s.fence(ctx); err != nil {
		return err
	}
	if t.ID == "" {
		return fmt.Errorf("store: transition needs an id")
	}
	b, err := cloneJSON(t)
	if err != nil {
		return err
	}
	b.UpdatedAt = time.Now().UTC()
	s.state.Transitions[t.ID] = b
	return nil
}

// GetTransition reads one transition.
func (s *Store) GetTransition(ctx context.Context, id string) (environment.Transition, error) {
	defer s.rlock(ctx)()
	t, ok := s.state.Transitions[id]
	if !ok {
		return environment.Transition{}, fmt.Errorf("%w: transition %q", persistence.ErrNotFound, id)
	}
	return cloneJSON(t)
}

// ListTransitions lists the Environment's transitions, newest first.
func (s *Store) ListTransitions(ctx context.Context, applicationKey, environmentKey string) ([]environment.Transition, error) {
	defer s.rlock(ctx)()
	var out []environment.Transition
	for _, t := range s.state.Transitions {
		if t.ApplicationKey == applicationKey && t.EnvironmentKey == environmentKey {
			c, err := cloneJSON(t)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func cloneJSON[T any](in T) (T, error) {
	var out T
	b, err := jsonMarshal(in)
	if err != nil {
		return out, err
	}
	err = jsonUnmarshal(b, &out)
	return out, err
}
