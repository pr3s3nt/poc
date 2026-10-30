package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

var _ persistence.DriftScanRepository = (*Store)(nil)

const driftScanSelect = `SELECT s.id::text,s.organization_id::text,s.environment_id::text,a.application_key,e.environment_key,COALESCE(s.operation_id::text,''),s.request_id::text,s.request_digest,s.environment_version,s.status,s.result,s.scope,s.findings,s.owner,s.fence,s.version,s.started_at,s.finished_at,s.created_at,s.reason_code FROM drift_scans s JOIN environments e ON e.id=s.environment_id JOIN applications a ON a.id=e.application_id`

func scanDriftScan(row pgx.Row) (s drift.Scan, err error) {
	var scope, findings []byte
	err = row.Scan(&s.ID, &s.OrganizationID, &s.EnvironmentID, &s.ApplicationKey, &s.EnvironmentKey, &s.OperationID, &s.RequestID, &s.RequestDigest, &s.EnvironmentVersion, &s.Status, &s.Result, &scope, &findings, &s.Owner, &s.Fence, &s.Version, &s.StartedAt, &s.FinishedAt, &s.CreatedAt, &s.ReasonCode)
	if err != nil {
		return s, translate(err)
	}
	if json.Unmarshal(scope, &s.Scope) != nil || json.Unmarshal(findings, &s.Findings) != nil {
		return s, drift.ErrScan
	}
	return
}
func (s *Store) GetDriftScan(ctx context.Context, org, env, id string) (drift.Scan, error) {
	if !validUUID(org) || !validUUID(env) || !validUUID(id) {
		return drift.Scan{}, persistence.ErrNotFound
	}
	return scanDriftScan(s.q(ctx).QueryRow(ctx, driftScanSelect+` WHERE s.organization_id=$1 AND s.environment_id=$2 AND s.id=$3`, org, env, id))
}
func (s *Store) QueueDriftScan(ctx context.Context, in drift.Scan) (out drift.Scan, err error) {
	if in.ValidateQueued() != nil || !validUUID(in.ID) || !validUUID(in.OrganizationID) || !validUUID(in.EnvironmentID) || !validUUID(in.RequestID) {
		return out, drift.ErrScan
	}
	for _, p := range in.Scope {
		if !validUUID(p.InstanceID) || (p.BaselineID != "" && !validUUID(p.BaselineID)) {
			return out, drift.ErrScan
		}
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		var version int64
		var active *string
		if err := s.q(ctx).QueryRow(ctx, `SELECT e.version,e.active_operation_id::text FROM environments e JOIN applications a ON a.id=e.application_id WHERE e.id=$1 AND a.organization_id=$2 AND a.application_key=$3 AND e.environment_key=$4 AND a.status='ACTIVE' FOR UPDATE OF e`, in.EnvironmentID, in.OrganizationID, in.ApplicationKey, in.EnvironmentKey).Scan(&version, &active); err != nil {
			return translate(err)
		}
		prior, lookup := scanDriftScan(s.q(ctx).QueryRow(ctx, driftScanSelect+` WHERE s.environment_id=$1 AND s.request_id=$2`, in.EnvironmentID, in.RequestID))
		if lookup == nil {
			if prior.RequestDigest != in.RequestDigest {
				return persistence.ErrVersionConflict
			}
			out = prior
			return nil
		}
		if !errors.Is(lookup, persistence.ErrNotFound) {
			return lookup
		}
		if version != in.EnvironmentVersion {
			return persistence.ErrVersionConflict
		}
		if active != nil {
			return persistence.ErrEnvironmentBusy
		}
		var queued bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drift_scans WHERE environment_id=$1 AND status IN ('QUEUED','RUNNING'))`, in.EnvironmentID).Scan(&queued); err != nil {
			return translate(err)
		}
		if queued {
			return persistence.ErrEnvironmentBusy
		}
		for _, pin := range in.Scope {
			var valid bool
			if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terraform_resource_instances WHERE id=$1 AND organization_id=$2 AND environment_id=$3 AND version=$4)`, pin.InstanceID, in.OrganizationID, in.EnvironmentID, pin.InstanceVersion).Scan(&valid); err != nil {
				return translate(err)
			}
			if !valid {
				return persistence.ErrVersionConflict
			}
		}
		if in.Scope == nil {
			in.Scope = []drift.InstancePin{}
		}
		scope, err := json.Marshal(in.Scope)
		if err != nil {
			return drift.ErrScan
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO drift_scans(id,organization_id,environment_id,request_id,request_digest,environment_version,status,result,scope,findings,version,created_at) VALUES($1,$2,$3,$4,$5,$6,'QUEUED','UNKNOWN',$7,'[]',1,$8)`, in.ID, in.OrganizationID, in.EnvironmentID, in.RequestID, in.RequestDigest, in.EnvironmentVersion, scope, in.CreatedAt)
		if err != nil {
			return translate(err)
		}
		out, err = s.GetDriftScan(ctx, in.OrganizationID, in.EnvironmentID, in.ID)
		return err
	})
	return
}
func (s *Store) ListDriftScans(ctx context.Context, org, env, after string, limit int) ([]drift.Scan, error) {
	if !validUUID(org) || !validUUID(env) || limit < 1 || limit > 200 || (after != "" && !validUUID(after)) {
		return nil, drift.ErrScan
	}
	rows, err := s.q(ctx).Query(ctx, driftScanSelect+` WHERE s.organization_id=$1 AND s.environment_id=$2 AND ($3='' OR (s.created_at,s.id)<(SELECT created_at,id FROM drift_scans WHERE organization_id=$1 AND environment_id=$2 AND id=NULLIF($3,'')::uuid)) ORDER BY s.created_at DESC,s.id DESC LIMIT $4`, org, env, after, limit)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	result := []drift.Scan{}
	for rows.Next() {
		scan, err := scanDriftScan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, scan)
	}
	return result, translate(rows.Err())
}
func (s *Store) ListQueuedDriftScans(ctx context.Context, limit int) ([]drift.Scan, error) {
	if limit < 1 || limit > 200 {
		return nil, drift.ErrScan
	}
	rows, err := s.q(ctx).Query(ctx, driftScanSelect+` WHERE s.status='QUEUED' ORDER BY s.created_at,s.id LIMIT $1`, limit)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	result := []drift.Scan{}
	for rows.Next() {
		scan, err := scanDriftScan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, scan)
	}
	return result, translate(rows.Err())
}
func (s *Store) StartDriftScan(ctx context.Context, org, id string, version int64) (out drift.Scan, err error) {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return out, persistence.ErrOperationLost
	}
	err = s.fenced(ctx, func(ctx context.Context) error {
		now := time.Now().UTC()
		tag, err := s.q(ctx).Exec(ctx, `UPDATE drift_scans s SET status='RUNNING',operation_id=$3,owner=$4,fence=$5,version=version+1,started_at=$6 WHERE s.id=$1 AND s.organization_id=$2 AND s.status='QUEUED' AND s.version=$7 AND EXISTS(SELECT 1 FROM environment_operations op JOIN environments e ON e.id=op.environment_id WHERE op.id=$3 AND op.environment_id=s.environment_id AND op.kind='DRIFT_SCAN' AND e.active_operation_id=op.id)`, id, org, owner.OperationID, owner.Owner, owner.Fence, now, version)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		out, err = scanDriftScan(s.q(ctx).QueryRow(ctx, driftScanSelect+` WHERE s.id=$1 AND s.organization_id=$2`, id, org))
		return err
	})
	return
}
func (s *Store) FinishDriftScan(ctx context.Context, in drift.Scan, version int64) error {
	if in.ValidateTerminal() != nil || in.Status == drift.Skipped {
		return drift.ErrScan
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || in.OperationID != owner.OperationID {
		return persistence.ErrOperationLost
	}
	return s.fenced(ctx, func(ctx context.Context) error {
		op, err := s.GetOperation(ctx, owner.OperationID)
		if err != nil {
			return err
		}
		if op.Kind != environment.OpDriftScan {
			return persistence.ErrOperationLost
		}
		current, err := s.GetDriftScan(ctx, in.OrganizationID, in.EnvironmentID, in.ID)
		if err != nil {
			return err
		}
		if current.Status != drift.Running || current.Version != version || current.OperationID != owner.OperationID || (!((current.Owner == owner.Owner && current.Fence == owner.Fence) || (op.Status == environment.OpRecovering && current.Fence < owner.Fence))) {
			return persistence.ErrOperationLost
		}
		// Scope and immutable pins come from storage, not the worker's replacement.
		in.Scope = current.Scope
		if in.ValidateTerminal() != nil {
			return drift.ErrScan
		}
		if in.Findings == nil {
			in.Findings = []drift.InstanceResult{}
		}
		findings, err := json.Marshal(in.Findings)
		if err != nil {
			return drift.ErrScan
		}
		tag, err := s.q(ctx).Exec(ctx, `UPDATE drift_scans SET status=$4,result=$5,findings=$6,reason_code=$7,finished_at=$8,owner=$9,fence=$10,version=version+1 WHERE id=$1 AND organization_id=$2 AND version=$3 AND status='RUNNING'`, in.ID, in.OrganizationID, version, in.Status, in.Result, findings, in.ReasonCode, in.FinishedAt, owner.Owner, owner.Fence)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		return nil
	})
}
func (s *Store) SkipQueuedDriftScan(ctx context.Context, org, id string, version int64, reason string) error {
	if reason == "" {
		return drift.ErrScan
	}
	tag, err := s.q(ctx).Exec(ctx, `UPDATE drift_scans SET status='SKIPPED',result='UNKNOWN',reason_code=$4,finished_at=$5,version=version+1 WHERE id=$1 AND organization_id=$2 AND version=$3 AND status='QUEUED'`, id, org, version, reason, time.Now().UTC())
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
