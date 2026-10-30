package postgres

import (
	"context"
	"encoding/json"
	"errors"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

const executionSelect = `SELECT u.id::text,u.environment_id::text,u.organization_id::text,u.workload_id,u.target_generation,u.delivery_epoch,u.unit_key,u.phase,u.target_ref,u.version,i.created_at,i.updated_at,
 r.id::text,r.artifact_id::text,r.payload_digest,r.selection,
 i.id::text,i.environment_operation_id::text,i.action,i.status,i.request_id,i.request_digest,i.owner,i.fence,i.version,i.receipt,i.inventory
 FROM delivery_units u JOIN delivery_revisions r ON r.unit_id=u.id JOIN delivery_intents i ON i.unit_id=u.id AND i.revision_id=r.id`

func scanExecution(row interface{ Scan(...any) error }) (out persistence.DeliveryExecution, err error) {
	var target, selection, receipt, inventory []byte
	err = row.Scan(&out.UnitID, &out.EnvironmentID, &out.OrganizationID, &out.WorkloadID, &out.Revision.Scope.TargetGeneration, &out.Revision.Scope.Epoch, &out.Revision.Scope.UnitKey, &out.Revision.Phase, &target, &out.Version, &out.CreatedAt, &out.UpdatedAt,
		&out.Revision.ID, &out.Revision.ArtifactRef, &out.Revision.PayloadDigest, &selection,
		&out.Mutation.ID, &out.Mutation.OperationID, &out.Mutation.Action, &out.Mutation.Status, &out.Mutation.RequestID, &out.Mutation.RequestDigest, &out.Mutation.Owner, &out.Mutation.Fence, &out.Mutation.Version, &receipt, &inventory)
	if err != nil {
		return out, translate(err)
	}
	out.Revision.Scope.OrganizationID = out.OrganizationID
	out.Revision.Scope.EnvironmentID = out.EnvironmentID
	out.Mutation.Scope = out.Revision.Scope
	out.Mutation.RevisionID = out.Revision.ID
	if json.Unmarshal(target, &out.Target) != nil || json.Unmarshal(selection, &out.Revision.Selection) != nil || json.Unmarshal(receipt, &out.Receipt) != nil || json.Unmarshal(inventory, &out.Inventory) != nil {
		return out, domain.ErrInvalid
	}
	if out.Revision.Validate() != nil || out.Mutation.Validate() != nil {
		return out, domain.ErrInvalid
	}
	return
}
func (s *Store) validateDeliveryOwner(ctx context.Context, e persistence.DeliveryExecution) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok || owner.OperationID != e.Mutation.OperationID || owner.Owner != e.Mutation.Owner || owner.Fence != e.Mutation.Fence {
		return persistence.ErrOperationLost
	}
	var matches bool
	err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments e JOIN applications a ON a.id=e.application_id JOIN environment_operations op ON op.id=e.active_operation_id WHERE e.id=$1 AND a.organization_id=$2 AND e.target_generation=$3 AND op.id=$4 AND op.owner_instance=$5 AND op.fence=$6 AND op.status IN ('ACTIVE','RECOVERING'))`, e.EnvironmentID, e.OrganizationID, e.Revision.Scope.TargetGeneration, owner.OperationID, owner.Owner, owner.Fence).Scan(&matches)
	if err != nil {
		return translate(err)
	}
	if !matches {
		return persistence.ErrOperationLost
	}
	return nil
}
func validExecution(e persistence.DeliveryExecution) bool {
	return validUUID(e.UnitID) && validUUID(e.EnvironmentID) && validUUID(e.OrganizationID) && validUUID(e.Revision.ID) && validUUID(e.Revision.ArtifactRef) && validUUID(e.Mutation.ID) && validUUID(e.Mutation.OperationID) && e.Revision.Validate() == nil && e.Mutation.Validate() == nil && e.Revision.Scope == e.Mutation.Scope && e.Revision.Scope.EnvironmentID == e.EnvironmentID && e.Revision.Scope.OrganizationID == e.OrganizationID && e.Mutation.RevisionID == e.Revision.ID && e.Target.Explicit()
}
func (s *Store) PrepareDelivery(ctx context.Context, e persistence.DeliveryExecution) (out persistence.DeliveryExecution, err error) {
	if !validExecution(e) || e.Mutation.Status != domain.IntentPending || e.Version != 1 {
		return out, domain.ErrInvalid
	}
	err = s.fenced(ctx, func(ctx context.Context) error {
		if err := s.validateDeliveryOwner(ctx, e); err != nil {
			return err
		}
		// Deduplicate exact original intent, never trust only its request ID.
		prior, err := scanExecution(s.q(ctx).QueryRow(ctx, executionSelect+` WHERE u.organization_id=$1 AND i.id=$2`, e.OrganizationID, e.Mutation.ID))
		if err == nil {
			if !prior.Mutation.SameRequest(e.Mutation) || prior.Revision.PayloadDigest != e.Revision.PayloadDigest {
				return persistence.ErrImmutable
			}
			out = prior
			return nil
		}
		if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		selection, _ := json.Marshal(e.Revision.Selection)
		target, _ := json.Marshal(e.Target)
		var binding any
		var version any
		if e.Revision.Selection.Effective().Mode == domain.CD {
			binding = e.Revision.Selection.BindingID
			version = e.Revision.Selection.BindingVersion
			var valid bool
			if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_bindings WHERE organization_id=$1 AND id=$2 AND version=$3 AND (status='READY' OR ($4 AND EXISTS(SELECT 1 FROM delivery_intents i JOIN delivery_revisions r ON r.id=i.revision_id WHERE i.unit_id=$5 AND i.status='ACKNOWLEDGED' AND i.action IN ('SUBMIT','ADOPT') AND r.id=NULLIF($6,'')::uuid AND r.selection=$7::jsonb))))`, e.OrganizationID, binding, version, e.Mutation.Action == domain.Withdraw, e.UnitID, e.Receipt.RevisionID, selection).Scan(&valid); err != nil {
				return translate(err)
			}
			if !valid {
				return persistence.ErrNotFound
			}
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO delivery_units(id,organization_id,environment_id,target_generation,delivery_epoch,unit_key,workload_id,phase,target_ref,binding_id,binding_version,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,$12,$13) ON CONFLICT(id) DO NOTHING`, e.UnitID, e.OrganizationID, e.EnvironmentID, e.Revision.Scope.TargetGeneration, e.Revision.Scope.Epoch, e.Revision.Scope.UnitKey, e.WorkloadID, e.Revision.Phase, target, binding, version, e.CreatedAt, e.UpdatedAt)
		if err != nil {
			return translate(err)
		}
		var active bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_intents WHERE unit_id=$1 AND status IN ('RUNNING','OUTCOME_UNKNOWN'))`, e.UnitID).Scan(&active); err != nil {
			return translate(err)
		}
		if active {
			return domain.ErrOutcomeUnknown
		}
		var same bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT organization_id=$2 AND environment_id=$3 AND target_generation=$4 AND delivery_epoch=$5 AND unit_key=$6 AND target_ref=$7::jsonb FROM delivery_units WHERE id=$1 FOR UPDATE`, e.UnitID, e.OrganizationID, e.EnvironmentID, e.Revision.Scope.TargetGeneration, e.Revision.Scope.Epoch, e.Revision.Scope.UnitKey, target).Scan(&same); err != nil {
			return translate(err)
		}
		if !same {
			return persistence.ErrImmutable
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO delivery_revisions(id,unit_id,environment_operation_id,artifact_id,payload_digest,selection,created_at,organization_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Revision.ID, e.UnitID, e.Mutation.OperationID, e.Revision.ArtifactRef, e.Revision.PayloadDigest, selection, e.CreatedAt, e.OrganizationID)
		if err != nil {
			return translate(err)
		}
		receipt, _ := json.Marshal(e.Receipt)
		inventory, _ := json.Marshal(e.Inventory)
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO delivery_intents(id,unit_id,revision_id,environment_operation_id,action,status,request_id,request_digest,owner,fence,version,created_at,updated_at,receipt,inventory) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,$11,$12,$13,$14)`, e.Mutation.ID, e.UnitID, e.Revision.ID, e.Mutation.OperationID, e.Mutation.Action, e.Mutation.Status, e.Mutation.RequestID, e.Mutation.RequestDigest, e.Mutation.Owner, e.Mutation.Fence, e.CreatedAt, e.UpdatedAt, receipt, inventory)
		if err != nil {
			return translate(err)
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE delivery_units SET pending_revision_id=$2,version=version+1,updated_at=$3 WHERE id=$1`, e.UnitID, e.Revision.ID, e.UpdatedAt)
		if err != nil {
			return translate(err)
		}
		out, err = s.GetDeliveryExecution(ctx, e.OrganizationID, e.Mutation.ID)
		return err
	})
	return
}
func (s *Store) GetDeliveryExecution(ctx context.Context, org, intent string) (persistence.DeliveryExecution, error) {
	if !validUUID(org) || !validUUID(intent) {
		return persistence.DeliveryExecution{}, persistence.ErrNotFound
	}
	return scanExecution(s.q(ctx).QueryRow(ctx, executionSelect+` WHERE u.organization_id=$1 AND i.id=$2`, org, intent))
}
func (s *Store) ListDeliveryExecutions(ctx context.Context, org, env string) ([]persistence.DeliveryExecution, error) {
	if !validUUID(org) || !validUUID(env) {
		return nil, persistence.ErrNotFound
	}
	rows, err := s.q(ctx).Query(ctx, executionSelect+` WHERE u.organization_id=$1 AND u.environment_id=$2 ORDER BY u.unit_key,i.created_at DESC`, org, env)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []persistence.DeliveryExecution{}
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) UpdateDeliveryExecution(ctx context.Context, e persistence.DeliveryExecution, expected int64) (out persistence.DeliveryExecution, err error) {
	if !validExecution(e) {
		return out, domain.ErrInvalid
	}
	err = s.fenced(ctx, func(ctx context.Context) error {
		if err := s.validateDeliveryOwner(ctx, e); err != nil {
			return err
		}
		old, err := scanExecution(s.q(ctx).QueryRow(ctx, executionSelect+` WHERE u.organization_id=$1 AND i.id=$2 FOR UPDATE OF i,u`, e.OrganizationID, e.Mutation.ID))
		if err != nil {
			return err
		}
		if old.Mutation.Version != expected {
			return persistence.ErrVersionConflict
		}
		if !old.Mutation.SameRequest(e.Mutation) || old.UnitID != e.UnitID || old.Revision != e.Revision || old.WorkloadID != e.WorkloadID || old.OrganizationID != e.OrganizationID || old.EnvironmentID != e.EnvironmentID {
			return persistence.ErrImmutable
		}
		// Only state transitions already validated by the application may be stored.
		next, err := old.Mutation.Advance(e.Mutation.Status, true)
		if err != nil {
			return err
		}
		if next.Version != e.Mutation.Version {
			return persistence.ErrVersionConflict
		}
		receipt, _ := json.Marshal(e.Receipt)
		inventory, _ := json.Marshal(e.Inventory)
		_, err = s.q(ctx).Exec(ctx, `UPDATE delivery_intents SET status=$2,version=$3,receipt=$4,inventory=$5,updated_at=$6 WHERE id=$1`, e.Mutation.ID, e.Mutation.Status, e.Mutation.Version, receipt, inventory, e.UpdatedAt)
		if err != nil {
			return translate(err)
		}
		if e.Mutation.Status == domain.IntentAcknowledged {
			_, err = s.q(ctx).Exec(ctx, `UPDATE delivery_units SET successful_revision_id=$2,updated_at=$3,version=version+1 WHERE id=$1`, e.UnitID, e.Revision.ID, e.UpdatedAt)
			if err != nil {
				return translate(err)
			}
		}
		out, err = s.GetDeliveryExecution(ctx, e.OrganizationID, e.Mutation.ID)
		return err
	})
	return
}

var _ persistence.DeliveryExecutionRepository = (*Store)(nil)
