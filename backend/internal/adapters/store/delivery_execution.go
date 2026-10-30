package store

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"

	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

func cloneDelivery(e persistence.DeliveryExecution) persistence.DeliveryExecution {
	ref := e.Revision.ArtifactRef
	raw, _ := json.Marshal(e)
	var out persistence.DeliveryExecution
	_ = json.Unmarshal(raw, &out)
	out.Revision.ArtifactRef = ref
	return out
}
func (s *Store) deliveryOwner(ctx context.Context, e persistence.DeliveryExecution) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || owner.OperationID != e.Mutation.OperationID || owner.Owner != e.Mutation.Owner || owner.Fence != e.Mutation.Fence {
		return persistence.ErrOperationLost
	}
	for _, env := range s.state.Environments {
		if env.ID == e.EnvironmentID && env.TargetGeneration == e.Revision.Scope.TargetGeneration {
			app, ok := s.state.Applications[env.ApplicationKey]
			if !ok {
				return persistence.ErrNotFound
			}
			org, ok := s.state.Organizations[app.OrganizationKey]
			if !ok || org.ID != e.OrganizationID {
				return persistence.ErrNotFound
			}
			op, ok := s.state.Operations[owner.OperationID]
			if !ok || env.ActiveOperationID != owner.OperationID || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() {
				return persistence.ErrOperationLost
			}
			return s.ownerCheck(ctx, env)
		}
	}
	return persistence.ErrNotFound
}
func (s *Store) PrepareDelivery(ctx context.Context, e persistence.DeliveryExecution) (out persistence.DeliveryExecution, err error) {
	if e.Revision.Validate() != nil || e.Mutation.Validate() != nil || e.Version != 1 || e.Mutation.Status != delivery.IntentPending || e.Mutation.Scope != e.Revision.Scope || e.OrganizationID != e.Revision.Scope.OrganizationID || e.EnvironmentID != e.Revision.Scope.EnvironmentID {
		return out, delivery.ErrInvalid
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := s.deliveryOwner(ctx, e); err != nil {
			return err
		}
		if old, ok := s.state.DeliveryExecutions[e.Mutation.ID]; ok {
			prior := cloneDelivery(old.Execution)
			prior.Revision.ArtifactRef = old.ArtifactRef
			if !prior.Mutation.SameRequest(e.Mutation) || prior.Revision.PayloadDigest != e.Revision.PayloadDigest {
				return persistence.ErrImmutable
			}
			out = prior
			return nil
		}
		artifact, ok := s.state.DeliveryArtifacts[e.Revision.ArtifactRef]
		if !ok || artifact.OrganizationID != e.OrganizationID || artifact.OwnerID != e.UnitID {
			return persistence.ErrNotFound
		}
		for _, record := range s.state.DeliveryExecutions {
			prior := record.Execution
			if prior.UnitID == e.UnitID {
				if prior.Revision.Scope != e.Revision.Scope || !reflect.DeepEqual(prior.Target, e.Target) {
					return persistence.ErrImmutable
				}
				if prior.Mutation.Status == delivery.IntentRunning || prior.Mutation.Status == delivery.IntentUnknown {
					return delivery.ErrOutcomeUnknown
				}
			}
		}
		if e.Revision.Selection.Effective().Mode == delivery.CD {
			b, ok := s.state.DeliveryBindings[deliveryBindingKey(e.OrganizationID, e.Revision.Selection.BindingID, e.Revision.Selection.BindingVersion)]
			historicalWithdrawal := false
			if e.Mutation.Action == delivery.Withdraw {
				for _, prior := range s.state.DeliveryExecutions {
					p := prior.Execution
					if p.UnitID == e.UnitID && p.OrganizationID == e.OrganizationID && p.Mutation.Status == delivery.IntentAcknowledged && (p.Mutation.Action == delivery.Submit || p.Mutation.Action == delivery.Adopt) && p.Revision.ID == e.Receipt.RevisionID && p.Revision.Selection.Effective() == e.Revision.Selection.Effective() {
						historicalWithdrawal = true
					}
				}
			}
			if !ok || (b.Binding.Status != delivery.BindingReady && !historicalWithdrawal) {
				return persistence.ErrNotFound
			}
		}
		s.state.DeliveryExecutions[e.Mutation.ID] = deliveryExecutionRecord{Execution: cloneDelivery(e), ArtifactRef: e.Revision.ArtifactRef}
		out = cloneDelivery(e)
		return nil
	})
	return
}
func (s *Store) GetDeliveryExecution(ctx context.Context, org, id string) (persistence.DeliveryExecution, error) {
	defer s.rlock(ctx)()
	r, ok := s.state.DeliveryExecutions[id]
	if !ok || r.Execution.OrganizationID != org {
		return persistence.DeliveryExecution{}, persistence.ErrNotFound
	}
	out := cloneDelivery(r.Execution)
	out.Revision.ArtifactRef = r.ArtifactRef
	return out, nil
}
func (s *Store) ListDeliveryExecutions(ctx context.Context, org, env string) ([]persistence.DeliveryExecution, error) {
	defer s.rlock(ctx)()
	out := []persistence.DeliveryExecution{}
	for _, r := range s.state.DeliveryExecutions {
		if r.Execution.OrganizationID == org && r.Execution.EnvironmentID == env {
			e := cloneDelivery(r.Execution)
			e.Revision.ArtifactRef = r.ArtifactRef
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mutation.ID < out[j].Mutation.ID })
	return out, nil
}
func (s *Store) UpdateDeliveryExecution(ctx context.Context, e persistence.DeliveryExecution, expected int64) (out persistence.DeliveryExecution, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := s.deliveryOwner(ctx, e); err != nil {
			return err
		}
		r, ok := s.state.DeliveryExecutions[e.Mutation.ID]
		if !ok || r.Execution.OrganizationID != e.OrganizationID {
			return persistence.ErrNotFound
		}
		old := r.Execution
		old.Revision.ArtifactRef = r.ArtifactRef
		if old.Mutation.Version != expected {
			return persistence.ErrVersionConflict
		}
		if !old.Mutation.SameRequest(e.Mutation) || !reflect.DeepEqual(old.Revision, e.Revision) || old.UnitID != e.UnitID || old.WorkloadID != e.WorkloadID || old.OrganizationID != e.OrganizationID || old.EnvironmentID != e.EnvironmentID {
			return persistence.ErrImmutable
		}
		next, err := old.Mutation.Advance(e.Mutation.Status, true)
		if err != nil {
			return err
		}
		if next.Version != e.Mutation.Version {
			return persistence.ErrVersionConflict
		}
		s.state.DeliveryExecutions[e.Mutation.ID] = deliveryExecutionRecord{Execution: cloneDelivery(e), ArtifactRef: e.Revision.ArtifactRef}
		out = cloneDelivery(e)
		return nil
	})
	return
}
func (s *Store) PutDeliveryArtifact(ctx context.Context, a persistence.CipherArtifact) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		owner, ok := persistence.OwnerFrom(ctx)
		if !ok {
			return persistence.ErrOperationLost
		}
		op, ok := s.state.Operations[owner.OperationID]
		if !ok || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() {
			return persistence.ErrOperationLost
		}
		env, ok := s.state.Environments[envKey(op.ApplicationKey, op.EnvironmentKey)]
		if !ok {
			return persistence.ErrNotFound
		}
		app := s.state.Applications[env.ApplicationKey]
		org := s.state.Organizations[app.OrganizationKey]
		if a.OrganizationID != org.ID {
			return persistence.ErrNotFound
		}
		if a.Purpose != "DELIVERY_PAYLOAD" || a.AADVersion != 1 || len(a.Nonce) != 12 || len(a.Ciphertext) < 16 {
			return persistence.ErrImmutable
		}
		if _, exists := s.state.DeliveryArtifacts[a.ID]; exists {
			return persistence.ErrImmutable
		}
		a.Nonce = append([]byte(nil), a.Nonce...)
		a.Ciphertext = append([]byte(nil), a.Ciphertext...)
		s.state.DeliveryArtifacts[a.ID] = a
		return nil
	})
}
func (s *Store) GetDeliveryArtifact(ctx context.Context, org, id string) (persistence.CipherArtifact, error) {
	defer s.rlock(ctx)()
	a, ok := s.state.DeliveryArtifacts[id]
	if !ok || a.OrganizationID != org || a.Purpose != "DELIVERY_PAYLOAD" {
		return a, persistence.ErrNotFound
	}
	a.Nonce = append([]byte(nil), a.Nonce...)
	a.Ciphertext = append([]byte(nil), a.Ciphertext...)
	return a, nil
}
