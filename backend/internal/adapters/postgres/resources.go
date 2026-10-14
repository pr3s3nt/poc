package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// The Console's safe resource reads (UC-09 RES-01..03). They run on the
// Console login, which may SELECT only the safe instance and attempt columns,
// so these queries cannot reach target references, physical intent,
// snapshots, state or recovery handles even by mistake. Every query joins the
// Application/Environment keys: an ID outside that scope is simply absent.

// ListResourceRecords returns the safe projection of every instance of the
// Environment, ordered by generation then descriptor.
func (s *Store) ListResourceRecords(ctx context.Context, applicationKey, environmentKey string) ([]tfresource.ResourceRecord, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT i.id::text,i.organization_id::text,i.application_id::text,i.environment_id::text,i.target_generation,i.descriptor,i.scope_type,i.scope_id,i.state_key,COALESCE(i.active_resource_id::text,''),i.module_bundle_id::text,i.status,i.version,i.created_at,i.updated_at,b.module_id,b.revision,
  (SELECT count(*) FROM terraform_resource_attempts t WHERE t.resource_instance_id=i.id),
  la.attempt_number,la.action,la.ownership,la.stage,la.status,la.safe_error_code,la.updated_at
FROM terraform_resource_instances i
JOIN environments e ON e.id=i.environment_id
JOIN applications a ON a.id=e.application_id AND a.id=i.application_id
JOIN terraform_module_bundles b ON b.id=i.module_bundle_id
LEFT JOIN LATERAL (SELECT t.attempt_number,t.action,t.ownership,t.stage,t.status,t.safe_error_code,t.updated_at FROM terraform_resource_attempts t WHERE t.resource_instance_id=i.id ORDER BY t.created_at DESC,t.attempt_number DESC LIMIT 1) la ON true
WHERE a.application_key=$1 AND e.environment_key=$2
ORDER BY i.target_generation,i.descriptor,i.scope_type,i.scope_id`, applicationKey, environmentKey)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []tfresource.ResourceRecord{}
	for rows.Next() {
		var rec tfresource.ResourceRecord
		var scopeType string
		var number *int
		var action, ownership, stage, status, code *string
		var updated *time.Time
		in := &rec.Instance
		if err := rows.Scan(&in.ID, &in.Identity.OrganizationID, &in.Identity.ApplicationID, &in.Identity.EnvironmentID, &in.Identity.TargetGeneration, &in.Identity.Descriptor, &scopeType, &in.Identity.Scope.ID, &in.StateKey, &in.ActiveResourceID, &in.ModuleBundleID, &in.Status, &in.Version, &in.CreatedAt, &in.UpdatedAt, &rec.ModuleID, &rec.ModuleRevision,
			&rec.Attempts, &number, &action, &ownership, &stage, &status, &code, &updated); err != nil {
			return nil, err
		}
		in.Identity.Scope.Type = resource.ScopeType(scopeType)
		if number != nil {
			rec.LastAttempt = &tfresource.AttemptSummary{Number: *number, Action: tfresource.Action(deref(action)), Ownership: tfresource.Ownership(deref(ownership)), Stage: deref(stage), Status: tfresource.AttemptStatus(deref(status)), SafeErrorCode: deref(code)}
			if updated != nil {
				rec.LastAttempt.UpdatedAt = *updated
			}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ListResourceEdges returns every consumer -> provider edge of the Environment
// with the consumer's current status.
func (s *Store) ListResourceEdges(ctx context.Context, applicationKey, environmentKey string) ([]tfresource.Dependency, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT d.consumer_instance_id::text,d.provider_instance_id::text,c.status,d.version
FROM terraform_resource_dependencies d
JOIN terraform_resource_instances c ON c.id=d.consumer_instance_id
JOIN environments e ON e.id=d.environment_id
JOIN applications a ON a.id=e.application_id
WHERE a.application_key=$1 AND e.environment_key=$2 ORDER BY 1,2`, applicationKey, environmentKey)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []tfresource.Dependency{}
	for rows.Next() {
		var d tfresource.Dependency
		if err := rows.Scan(&d.ConsumerID, &d.ProviderID, &d.ConsumerStatus, &d.Version); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const cleanupRecordSelect = `SELECT op.id::text,op.authorization_mode,op.actor_ref,op.root_resource_instance_id::text,
  COALESCE(op.confirmed_scope->'selectedResourceIds','[]'::jsonb),op.status,op.stage,op.failure,COALESCE(op.confirmation_name,''),op.detail,op.started_at,op.updated_at,op.finished_at
FROM environment_operations op
JOIN environments e ON e.id=op.environment_id
JOIN applications a ON a.id=e.application_id`

func scanCleanupRecord(row interface{ Scan(...any) error }) (tfresource.CleanupRecord, error) {
	var r tfresource.CleanupRecord
	var mode string
	var selected, detail []byte
	if err := row.Scan(&r.OperationID, &mode, &r.ActorRef, &r.RootID, &selected, &r.Status, &r.Stage, &r.Failure, &r.ConfirmationName, &detail, &r.StartedAt, &r.UpdatedAt, &r.FinishedAt); err != nil {
		return r, err
	}
	r.Mode = tfresource.CleanupMode(mode)
	if err := unmarshalJSON(selected, &r.SelectedIDs); err != nil {
		return r, err
	}
	return r, unmarshalJSON(detail, &r.Detail)
}

// GetCleanupRecord reads one cleanup operation of the Environment. The pinned
// items (which carry original target references) are never selected: only the
// selected IDs are projected out of the confirmed scope.
func (s *Store) GetCleanupRecord(ctx context.Context, applicationKey, environmentKey, operationID string) (tfresource.CleanupRecord, error) {
	if !validUUID(operationID) {
		return tfresource.CleanupRecord{}, persistence.ErrNotFound
	}
	rec, err := scanCleanupRecord(s.q(ctx).QueryRow(ctx, cleanupRecordSelect+` WHERE op.id=$1::uuid AND op.kind='RESOURCE_CLEANUP' AND a.application_key=$2 AND e.environment_key=$3`, operationID, applicationKey, environmentKey))
	if err != nil {
		return tfresource.CleanupRecord{}, translate(err)
	}
	return rec, nil
}

// ListCleanupRecords returns the Environment's cleanup operations, newest first.
func (s *Store) ListCleanupRecords(ctx context.Context, applicationKey, environmentKey string, limit int) ([]tfresource.CleanupRecord, error) {
	if limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("%w: cleanup list limit", persistence.ErrNotFound)
	}
	rows, err := s.q(ctx).Query(ctx, cleanupRecordSelect+` WHERE op.kind='RESOURCE_CLEANUP' AND a.application_key=$1 AND e.environment_key=$2 ORDER BY op.started_at DESC,op.id DESC LIMIT $3`, applicationKey, environmentKey, limit)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []tfresource.CleanupRecord{}
	for rows.Next() {
		rec, err := scanCleanupRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// FindCleanupByRequest returns the cleanup operation admitted by this actor
// with this request hash in the Environment.
func (s *Store) FindCleanupByRequest(ctx context.Context, applicationKey, environmentKey, actorRef, requestHash string) (tfresource.CleanupRecord, bool, error) {
	rec, err := scanCleanupRecord(s.q(ctx).QueryRow(ctx, cleanupRecordSelect+` WHERE op.kind='RESOURCE_CLEANUP' AND op.actor_ref=$1 AND op.deletion_request_hash=$2 AND a.application_key=$3 AND e.environment_key=$4`, actorRef, requestHash, applicationKey, environmentKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return tfresource.CleanupRecord{}, false, nil
	}
	if err != nil {
		return tfresource.CleanupRecord{}, false, translate(err)
	}
	return rec, true, nil
}

// ListOperationAttempts returns the safe attempt summaries bound to one
// cleanup operation of the Environment (the only attempt columns the Console
// may read).
func (s *Store) ListOperationAttempts(ctx context.Context, applicationKey, environmentKey, operationID string) ([]tfresource.OperationAttempt, error) {
	if !validUUID(operationID) {
		return []tfresource.OperationAttempt{}, nil
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT t.resource_instance_id::text,t.action,t.stage,t.status,t.safe_error_code
FROM terraform_resource_attempts t
JOIN terraform_resource_instances i ON i.id=t.resource_instance_id
JOIN environments e ON e.id=i.environment_id
JOIN applications a ON a.id=e.application_id
WHERE t.environment_operation_id=$1::uuid AND a.application_key=$2 AND e.environment_key=$3
ORDER BY t.created_at,t.attempt_number`, operationID, applicationKey, environmentKey)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []tfresource.OperationAttempt{}
	for rows.Next() {
		var a tfresource.OperationAttempt
		if err := rows.Scan(&a.InstanceID, &a.Action, &a.Stage, &a.Status, &a.SafeErrorCode); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var _ terraformstate.ResourceReader = (*Store)(nil)
