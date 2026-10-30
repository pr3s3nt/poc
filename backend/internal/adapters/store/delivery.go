package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

var _ persistence.DeliveryBindingRepository = (*Store)(nil)

func deliveryBindingKey(org, id string, version int64) string {
	return fmt.Sprintf("%s/%s/%d", org, id, version)
}
func (s *Store) deliveryBindingScope(b delivery.BindingVersion) bool {
	found := false
	for _, o := range s.state.Organizations {
		if o.ID == b.OrganizationID {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	for _, c := range s.state.Connections {
		if c.ID == b.ManagementAccountID && c.Version == b.ManagementAccountVersion {
			for _, o := range s.state.Organizations {
				if o.ID == b.OrganizationID && c.OrganizationKey == o.Key {
					return true
				}
			}
		}
	}
	return false
}
func (s *Store) CreateDeliveryBinding(ctx context.Context, b delivery.BindingVersion) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		if err := b.Validate(); err != nil {
			return err
		}
		if b.Status != delivery.Unverified || b.Revision != 1 || b.VerifiedAt != nil {
			return delivery.ErrInvalid
		}
		if !s.deliveryBindingScope(b) {
			return persistence.ErrNotFound
		}
		key := deliveryBindingKey(b.OrganizationID, b.ID, b.Version)
		if _, exists := s.state.DeliveryBindings[key]; exists {
			return persistence.ErrDuplicate
		}
		if b.Version > 1 {
			if _, exists := s.state.DeliveryBindings[deliveryBindingKey(b.OrganizationID, b.ID, b.Version-1)]; !exists {
				return persistence.ErrVersionConflict
			}
		}
		s.state.DeliveryBindings[key] = deliveryBindingRecord{Binding: b.Clone(), WriterRef: b.WriterCredentialRef}
		return nil
	})
}
func (s *Store) GetDeliveryBinding(ctx context.Context, org, id string, version int64) (delivery.BindingVersion, error) {
	defer s.rlock(ctx)()
	r, ok := s.state.DeliveryBindings[deliveryBindingKey(org, id, version)]
	if !ok {
		return delivery.BindingVersion{}, persistence.ErrNotFound
	}
	b := r.Binding.Clone()
	b.WriterCredentialRef = r.WriterRef
	return b, nil
}
func (s *Store) ListDeliveryBindings(ctx context.Context, org string) ([]delivery.BindingVersion, error) {
	defer s.rlock(ctx)()
	out := []delivery.BindingVersion{}
	for _, r := range s.state.DeliveryBindings {
		if r.Binding.OrganizationID == org {
			b := r.Binding.Clone()
			b.WriterCredentialRef = r.WriterRef
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == out[j].ID {
			return out[i].Version < out[j].Version
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func (s *Store) VerifyDeliveryBinding(ctx context.Context, org, id string, version, expected int64, status delivery.BindingStatus, caps delivery.Capabilities, at time.Time, reason string) (out delivery.BindingVersion, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		key := deliveryBindingKey(org, id, version)
		r, ok := s.state.DeliveryBindings[key]
		if !ok {
			return persistence.ErrNotFound
		}
		if r.Binding.Revision != expected {
			return persistence.ErrVersionConflict
		}
		if status != delivery.BindingReady && status != delivery.Rejected && status != delivery.Disabled {
			return delivery.ErrInvalid
		}
		b := r.Binding.Clone()
		b.WriterCredentialRef = r.WriterRef
		b.Status = status
		b.Capabilities = caps
		b.VerifiedAt = &at
		b.ReasonCode = reason
		b.Revision++
		if err := b.Validate(); err != nil {
			return err
		}
		s.state.DeliveryBindings[key] = deliveryBindingRecord{Binding: b.Clone(), WriterRef: r.WriterRef}
		out = b
		return nil
	})
	return
}

func (s *Store) SelectDelivery(ctx context.Context, cmd persistence.DeliverySelectionCommand) (out environment.Environment, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := cmd.Selection.Validate(); err != nil {
			return err
		}
		app, ok := s.state.Applications[cmd.ApplicationKey]
		if !ok {
			return persistence.ErrNotFound
		}
		org, ok := s.state.Organizations[app.OrganizationKey]
		if !ok || org.ID != cmd.OrganizationID {
			return persistence.ErrNotFound
		}
		key := envKey(cmd.ApplicationKey, cmd.EnvironmentKey)
		env, ok := s.state.Environments[key]
		if !ok {
			return persistence.ErrNotFound
		}
		if env.Busy() {
			return persistence.ErrEnvironmentBusy
		}
		if env.Version != cmd.ExpectedEnvironmentVersion || env.DeliveryConfigRevision != cmd.ExpectedConfigurationRevision {
			return persistence.ErrVersionConflict
		}
		selection := cmd.Selection.Effective()
		if selection.Mode == delivery.CD {
			b, ok := s.state.DeliveryBindings[deliveryBindingKey(cmd.OrganizationID, selection.BindingID, selection.BindingVersion)]
			if !ok || b.Binding.Status != delivery.BindingReady {
				return persistence.ErrNotFound
			}
			if err := b.Binding.Capabilities.ValidateSelection(selection); err != nil {
				return err
			}
		}
		if env.DesiredDelivery.Effective() != selection {
			env.DesiredDelivery = selection
			env.Version++
			env.DeliveryConfigRevision++
			s.state.Environments[key] = env
		}
		out = env
		return nil
	})
	return
}

func (s *Store) GetOrganizationByID(ctx context.Context, id string) (application.Organization, error) {
	defer s.rlock(ctx)()
	for _, org := range s.state.Organizations {
		if org.ID == id {
			return org, nil
		}
	}
	return application.Organization{}, persistence.ErrNotFound
}

func (s *Store) ActivateDelivery(ctx context.Context, cmd persistence.DeliveryActivation) error {
	if cmd.CheckEpoch && (cmd.ExpectedEpoch < 0 || cmd.NextEpoch != cmd.ExpectedEpoch+1) {
		return delivery.ErrInvalid
	}
	activate := func(ctx context.Context) error {
		if _, ok := persistence.OwnerFrom(ctx); !ok {
			return persistence.ErrOperationLost
		}
		key := envKey(cmd.ApplicationKey, cmd.EnvironmentKey)
		env, ok := s.state.Environments[key]
		if !ok {
			return persistence.ErrNotFound
		}
		if err := s.ownerCheck(ctx, env); err != nil {
			return err
		}
		if env.DeliveryConfigRevision != cmd.ConfigurationRevision || env.DesiredDelivery.Effective() != cmd.Selection.Effective() {
			return persistence.ErrVersionConflict
		}
		if cmd.CheckEpoch {
			if env.DeliveryEpoch != cmd.ExpectedEpoch {
				return persistence.ErrVersionConflict
			}
			env.DeliveryEpoch = cmd.NextEpoch
			for key, instance := range s.state.WorkloadInstances {
				if instance.EnvironmentKey == cmd.ApplicationKey+"/"+cmd.EnvironmentKey && instance.Generation == env.TargetGeneration && string(instance.Status) != "REMOVED" {
					ref := map[string]any{}
					for k, v := range instance.TargetRef {
						ref[k] = v
					}
					ref["deliveryMode"] = string(cmd.Selection.Effective().Mode)
					instance.TargetRef = ref
					s.state.WorkloadInstances[key] = instance
				}
			}
		}
		env.AppliedDelivery = cmd.Selection.Effective()
		s.state.Environments[key] = env
		return nil
	}
	if s.inTx(ctx) {
		return activate(ctx)
	}
	return s.Transact(ctx, activate)
}
