package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

var _ persistence.DriftScanRepository = (*Store)(nil)

// Preserve private worker/CAS fields omitted from the public domain JSON.
type driftScanRecord struct {
	Scan                                                                 drift.Scan
	OrganizationID, ApplicationKey, EnvironmentKey, RequestDigest, Owner string
	Fence, Version                                                       int64
}

func packDriftScan(s drift.Scan) driftScanRecord {
	raw, _ := json.Marshal(s)
	var public drift.Scan
	_ = json.Unmarshal(raw, &public)
	return driftScanRecord{public, s.OrganizationID, s.ApplicationKey, s.EnvironmentKey, s.RequestDigest, s.Owner, s.Fence, s.Version}
}
func (r driftScanRecord) unpack() drift.Scan {
	s := r.Scan
	raw, _ := json.Marshal(s)
	_ = json.Unmarshal(raw, &s)
	s.OrganizationID, s.ApplicationKey, s.EnvironmentKey, s.RequestDigest, s.Owner, s.Fence, s.Version = r.OrganizationID, r.ApplicationKey, r.EnvironmentKey, r.RequestDigest, r.Owner, r.Fence, r.Version
	return s
}
func (s *Store) QueueDriftScan(ctx context.Context, in drift.Scan) (out drift.Scan, err error) {
	if in.ValidateQueued() != nil {
		return out, drift.ErrScan
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		env, ok := s.state.Environments[envKey(in.ApplicationKey, in.EnvironmentKey)]
		app, exists := s.state.Applications[in.ApplicationKey]
		org := s.state.Organizations[app.OrganizationKey]
		if !ok || !exists || app.Lifecycle() != "ACTIVE" || org.ID != in.OrganizationID || env.ID != in.EnvironmentID {
			return persistence.ErrNotFound
		}
		for _, record := range s.state.DriftScans {
			prior := record.unpack()
			if prior.EnvironmentID != in.EnvironmentID {
				continue
			}
			if prior.RequestID == in.RequestID {
				if prior.RequestDigest != in.RequestDigest {
					return persistence.ErrVersionConflict
				}
				out = prior
				return nil
			}
		}
		if env.Version != in.EnvironmentVersion {
			return persistence.ErrVersionConflict
		}
		if env.Busy() {
			return persistence.ErrEnvironmentBusy
		}
		for _, record := range s.state.DriftScans {
			if record.Scan.EnvironmentID == in.EnvironmentID && (record.Scan.Status == drift.Queued || record.Scan.Status == drift.Running) {
				return persistence.ErrEnvironmentBusy
			}
		}
		if _, exists := s.state.DriftScans[in.ID]; exists {
			return persistence.ErrDuplicate
		}
		if s.state.DriftScans == nil {
			s.state.DriftScans = map[string]driftScanRecord{}
		}
		if in.Scope == nil {
			in.Scope = []drift.InstancePin{}
		}
		if in.Findings == nil {
			in.Findings = []drift.InstanceResult{}
		}
		in.Version = 1
		s.state.DriftScans[in.ID] = packDriftScan(in)
		out = s.state.DriftScans[in.ID].unpack()
		return nil
	})
	return
}
func (s *Store) GetDriftScan(ctx context.Context, org, env, id string) (drift.Scan, error) {
	defer s.rlock(ctx)()
	r, ok := s.state.DriftScans[id]
	if !ok || r.OrganizationID != org || r.Scan.EnvironmentID != env {
		return drift.Scan{}, persistence.ErrNotFound
	}
	return r.unpack(), nil
}
func (s *Store) ListDriftScans(ctx context.Context, org, env, after string, limit int) ([]drift.Scan, error) {
	defer s.rlock(ctx)()
	if limit < 1 || limit > 200 {
		return nil, drift.ErrScan
	}
	var cursor *drift.Scan
	if after != "" {
		r, ok := s.state.DriftScans[after]
		if !ok || r.OrganizationID != org || r.Scan.EnvironmentID != env {
			return []drift.Scan{}, nil
		}
		row := r.unpack()
		cursor = &row
	}
	rows := []drift.Scan{}
	for _, record := range s.state.DriftScans {
		row := record.unpack()
		if row.OrganizationID != org || row.EnvironmentID != env {
			continue
		}
		if cursor != nil && (row.CreatedAt.After(cursor.CreatedAt) || (row.CreatedAt.Equal(cursor.CreatedAt) && row.ID >= cursor.ID)) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}
func (s *Store) ListQueuedDriftScans(ctx context.Context, limit int) ([]drift.Scan, error) {
	defer s.rlock(ctx)()
	if limit < 1 || limit > 200 {
		return nil, drift.ErrScan
	}
	rows := []drift.Scan{}
	for _, record := range s.state.DriftScans {
		if record.Scan.Status == drift.Queued {
			rows = append(rows, record.unpack())
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}
func (s *Store) driftScanOwner(ctx context.Context, row drift.Scan) (persistence.Owner, environment.Operation, error) {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return owner, environment.Operation{}, persistence.ErrOperationLost
	}
	op, ok := s.state.Operations[owner.OperationID]
	if !ok || op.Owner != owner.Owner || op.Fence != owner.Fence || !op.Status.Holds() || op.Status == environment.OpInterrupted || op.Kind != environment.OpDriftScan {
		return owner, op, persistence.ErrOperationLost
	}
	env, ok := s.state.Environments[envKey(op.ApplicationKey, op.EnvironmentKey)]
	if !ok || env.ID != row.EnvironmentID || env.ActiveOperationID != op.ID {
		return owner, op, persistence.ErrOperationLost
	}
	return owner, op, nil
}
func (s *Store) StartDriftScan(ctx context.Context, org, id string, version int64) (out drift.Scan, err error) {
	err = s.Transact(ctx, func(ctx context.Context) error {
		r, ok := s.state.DriftScans[id]
		if !ok || r.OrganizationID != org {
			return persistence.ErrNotFound
		}
		row := r.unpack()
		owner, _, err := s.driftScanOwner(ctx, row)
		if err != nil {
			return err
		}
		if row.Status != drift.Queued || row.Version != version {
			return persistence.ErrVersionConflict
		}
		now := time.Now().UTC()
		row.Status, row.OperationID, row.Owner, row.Fence, row.StartedAt = drift.Running, owner.OperationID, owner.Owner, owner.Fence, &now
		row.Version++
		s.state.DriftScans[id] = packDriftScan(row)
		out = s.state.DriftScans[id].unpack()
		return nil
	})
	return
}
func (s *Store) FinishDriftScan(ctx context.Context, in drift.Scan, version int64) error {
	if in.ValidateTerminal() != nil || in.Status == drift.Skipped {
		return drift.ErrScan
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		r, ok := s.state.DriftScans[in.ID]
		if !ok || r.OrganizationID != in.OrganizationID || r.Scan.EnvironmentID != in.EnvironmentID {
			return persistence.ErrNotFound
		}
		row := r.unpack()
		owner, op, err := s.driftScanOwner(ctx, row)
		if err != nil {
			return err
		}
		if in.OperationID != owner.OperationID || row.OperationID != owner.OperationID || row.Status != drift.Running || row.Version != version || !((row.Owner == owner.Owner && row.Fence == owner.Fence) || (op.Status == environment.OpRecovering && row.Fence < owner.Fence)) {
			return persistence.ErrOperationLost
		}
		in.Scope = row.Scope
		if in.ValidateTerminal() != nil {
			return drift.ErrScan
		}
		row.Status, row.Result, row.Findings, row.ReasonCode, row.FinishedAt = in.Status, in.Result, in.Findings, in.ReasonCode, in.FinishedAt
		row.Owner, row.Fence, row.Version = owner.Owner, owner.Fence, row.Version+1
		s.state.DriftScans[in.ID] = packDriftScan(row)
		return nil
	})
}
func (s *Store) SkipQueuedDriftScan(ctx context.Context, org, id string, version int64, reason string) error {
	if reason == "" {
		return drift.ErrScan
	}
	return s.Transact(ctx, func(context.Context) error {
		r, ok := s.state.DriftScans[id]
		if !ok || r.OrganizationID != org {
			return persistence.ErrNotFound
		}
		row := r.unpack()
		if row.Status != drift.Queued || row.Version != version {
			return persistence.ErrVersionConflict
		}
		now := time.Now().UTC()
		row.Status, row.Result, row.ReasonCode, row.FinishedAt = drift.Skipped, drift.Unknown, reason, &now
		row.Version++
		s.state.DriftScans[id] = packDriftScan(row)
		return nil
	})
}
