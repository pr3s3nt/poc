package store

import (
	"context"
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
	"sort"
	"time"
)

func (s *Store) pollExecution(l persistence.DeliveryPollLease) (persistence.DeliveryExecution, error) {
	r, ok := s.state.DeliveryExecutions[l.IntentID]
	e := r.Execution
	if !ok || e.OrganizationID != l.OrganizationID || e.UnitID != l.UnitID || e.Mutation.Status != domain.IntentAcknowledged || (e.Mutation.Action != domain.Submit && e.Mutation.Action != domain.Adopt) {
		return e, persistence.ErrNotFound
	}
	valid := false
	for _, env := range s.state.Environments {
		if env.ID == e.EnvironmentID && !env.Busy() && env.TargetGeneration == e.Revision.Scope.TargetGeneration && env.DeliveryEpoch == e.Revision.Scope.Epoch {
			valid = true
		}
	}
	if !valid {
		return e, persistence.ErrVersionConflict
	}
	for _, record := range s.state.DeliveryExecutions {
		p := record.Execution
		if p.UnitID != e.UnitID {
			continue
		}
		if p.Mutation.Status == domain.IntentRunning || p.Mutation.Status == domain.IntentUnknown || (p.Mutation.Status == domain.IntentAcknowledged && p.CreatedAt.After(e.CreatedAt)) {
			return e, persistence.ErrVersionConflict
		}
	}
	return e, nil
}
func (s *Store) ClaimDeliveryPoll(ctx context.Context, l persistence.DeliveryPollLease, now time.Time) (out persistence.DeliveryPollLease, err error) {
	if l.Owner == "" || !l.LeaseUntil.After(now) || l.LeaseUntil.Sub(now) > 5*time.Minute || !l.NextPollAt.After(now) {
		return out, domain.ErrInvalid
	}
	err = s.Transact(ctx, func(context.Context) error {
		if _, err := s.pollExecution(l); err != nil {
			return err
		}
		if s.state.DeliveryPollLeases == nil {
			s.state.DeliveryPollLeases = map[string]persistence.DeliveryPollLease{}
		}
		old, ok := s.state.DeliveryPollLeases[l.UnitID]
		if ok && (old.LeaseUntil.After(now) || old.NextPollAt.After(now)) {
			return persistence.ErrVersionConflict
		}
		l.Fence = old.Fence + 1
		l.Sequence = old.Sequence
		s.state.DeliveryPollLeases[l.UnitID] = l
		out = l
		return nil
	})
	return
}
func (s *Store) SaveDeliveryObservation(ctx context.Context, l persistence.DeliveryPollLease, o persistence.DeliveryObservation, now time.Time) error {
	return s.Transact(ctx, func(context.Context) error {
		e, err := s.pollExecution(l)
		if err != nil {
			return err
		}
		old, ok := s.state.DeliveryPollLeases[l.UnitID]
		if !ok || old.Owner != l.Owner || old.Fence != l.Fence || old.IntentID != l.IntentID || !old.LeaseUntil.After(now) {
			return persistence.ErrVersionConflict
		}
		if o.ID == "" || o.UnitID != l.UnitID || o.Sequence != old.Sequence+1 || o.Observation.Validate(e.Revision) != nil || o.Observation.ObservedAt.After(now) || o.StoredAt != now {
			return domain.ErrInvalid
		}
		if s.state.DeliveryObservations == nil {
			s.state.DeliveryObservations = map[string][]persistence.DeliveryObservation{}
		}
		o.WorkloadID = e.WorkloadID
		raw, _ := json.Marshal(o)
		var cloned persistence.DeliveryObservation
		_ = json.Unmarshal(raw, &cloned)
		s.state.DeliveryObservations[l.UnitID] = append(s.state.DeliveryObservations[l.UnitID], cloned)
		old.Sequence = o.Sequence
		old.LeaseUntil = now
		s.state.DeliveryPollLeases[l.UnitID] = old
		return nil
	})
}
func (s *Store) ListDeliveryObservations(ctx context.Context, org, env string, generation, epoch int64, after string, limit int) ([]persistence.DeliveryObservation, error) {
	defer s.rlock(ctx)()
	if limit < 1 || limit > 200 {
		return nil, domain.ErrInvalid
	}
	out := []persistence.DeliveryObservation{}
	for unit, rows := range s.state.DeliveryObservations {
		if unit <= after || len(rows) == 0 {
			continue
		}
		o := rows[len(rows)-1]
		var latest *persistence.DeliveryExecution
		for _, record := range s.state.DeliveryExecutions {
			e := record.Execution
			if e.UnitID == unit && e.Mutation.Status == domain.IntentAcknowledged && (latest == nil || e.CreatedAt.After(latest.CreatedAt)) {
				copy := e
				latest = &copy
			}
		}
		if latest == nil || (latest.Mutation.Action != domain.Submit && latest.Mutation.Action != domain.Adopt) || latest.Revision.ID != o.Observation.IntendedRevision {
			continue
		}
		sc := o.Observation.Scope
		if sc.OrganizationID == org && sc.EnvironmentID == env && sc.TargetGeneration == generation && sc.Epoch == epoch {
			raw, _ := json.Marshal(o)
			var cloned persistence.DeliveryObservation
			_ = json.Unmarshal(raw, &cloned)
			out = append(out, cloned)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UnitID < out[j].UnitID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
