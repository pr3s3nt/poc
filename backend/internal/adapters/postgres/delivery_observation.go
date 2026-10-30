package postgres

import (
	"context"
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
	"time"
)

// Lock Environment before unit, in the same order as mutating executors. The
// transaction ends before any provider read; the save rechecks all scope pins.
func (s *Store) pollExecution(ctx context.Context, l persistence.DeliveryPollLease) (persistence.DeliveryExecution, error) {
	var envID string
	if err := s.q(ctx).QueryRow(ctx, `SELECT environment_id::text FROM delivery_units WHERE id=$1 AND organization_id=$2`, l.UnitID, l.OrganizationID).Scan(&envID); err != nil {
		return persistence.DeliveryExecution{}, translate(err)
	}
	var idle bool
	if err := s.q(ctx).QueryRow(ctx, `SELECT active_operation_id IS NULL FROM environments WHERE id=$1 FOR UPDATE`, envID).Scan(&idle); err != nil {
		return persistence.DeliveryExecution{}, translate(err)
	}
	if !idle {
		return persistence.DeliveryExecution{}, persistence.ErrVersionConflict
	}
	e, err := scanExecution(s.q(ctx).QueryRow(ctx, executionSelect+` WHERE u.id=$1 AND u.organization_id=$2 AND i.id=$3 FOR UPDATE OF u`, l.UnitID, l.OrganizationID, l.IntentID))
	if err != nil {
		return e, err
	}
	var valid bool
	err = s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_units u JOIN environments e ON e.id=u.environment_id JOIN delivery_intents i ON i.id=$3 WHERE u.id=$1 AND u.organization_id=$2 AND u.target_generation=e.target_generation AND u.delivery_epoch=e.delivery_epoch AND i.unit_id=u.id AND i.status='ACKNOWLEDGED' AND i.action IN ('SUBMIT','ADOPT') AND NOT EXISTS(SELECT 1 FROM delivery_intents newer WHERE newer.unit_id=u.id AND (newer.status IN ('RUNNING','OUTCOME_UNKNOWN') OR (newer.status='ACKNOWLEDGED' AND newer.created_at>i.created_at))))`, l.UnitID, l.OrganizationID, l.IntentID).Scan(&valid)
	if err != nil {
		return e, translate(err)
	}
	if !valid {
		return e, persistence.ErrVersionConflict
	}
	return e, nil
}
func (s *Store) ClaimDeliveryPoll(ctx context.Context, l persistence.DeliveryPollLease, now time.Time) (out persistence.DeliveryPollLease, err error) {
	if !validUUID(l.OrganizationID) || !validUUID(l.UnitID) || !validUUID(l.IntentID) || l.Owner == "" || !l.LeaseUntil.After(now) || l.LeaseUntil.Sub(now) > 5*time.Minute || !l.NextPollAt.After(now) {
		return out, domain.ErrInvalid
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.pollExecution(ctx, l); err != nil {
			return err
		}
		out = l
		err := s.q(ctx).QueryRow(ctx, `INSERT INTO delivery_poll_leases(unit_id,intent_id,owner,fence,lease_until,next_poll_at,last_sequence) VALUES($1,$2,$3,1,$4,$5,0) ON CONFLICT(unit_id) DO UPDATE SET intent_id=EXCLUDED.intent_id,owner=EXCLUDED.owner,fence=delivery_poll_leases.fence+1,lease_until=EXCLUDED.lease_until,next_poll_at=EXCLUDED.next_poll_at WHERE delivery_poll_leases.lease_until<=$6 AND delivery_poll_leases.next_poll_at<=$6 RETURNING fence,last_sequence`, l.UnitID, l.IntentID, l.Owner, l.LeaseUntil, l.NextPollAt, now).Scan(&out.Fence, &out.Sequence)
		return translate(err)
	})
	return
}
func (s *Store) SaveDeliveryObservation(ctx context.Context, l persistence.DeliveryPollLease, o persistence.DeliveryObservation, now time.Time) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		e, err := s.pollExecution(ctx, l)
		if err != nil {
			return err
		}
		if !validUUID(o.ID) || o.UnitID != l.UnitID || o.Sequence != l.Sequence+1 || o.Observation.Validate(e.Revision) != nil || o.Observation.ObservedAt.After(now) || !o.StoredAt.Equal(now) {
			return domain.ErrInvalid
		}
		tag, err := s.q(ctx).Exec(ctx, `UPDATE delivery_poll_leases SET last_sequence=$6,lease_until=$7 WHERE unit_id=$1 AND intent_id=$2 AND owner=$3 AND fence=$4 AND last_sequence=$5 AND lease_until>$7`, l.UnitID, l.IntentID, l.Owner, l.Fence, l.Sequence, o.Sequence, now)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		raw, err := json.Marshal(o.Observation)
		if err != nil {
			return domain.ErrInvalid
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO delivery_observations(id,unit_id,revision_id,sequence,observation,reason_code,observed_at,stored_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.ID, l.UnitID, e.Revision.ID, o.Sequence, raw, o.ReasonCode, o.Observation.ObservedAt, now)
		return translate(err)
	})
}
func (s *Store) ListDeliveryObservations(ctx context.Context, org, env string, generation, epoch int64, after string, limit int) ([]persistence.DeliveryObservation, error) {
	if !validUUID(org) || !validUUID(env) || limit < 1 || limit > 200 || (after != "" && !validUUID(after)) {
		return nil, domain.ErrInvalid
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT o.id::text,u.id::text,u.workload_id,o.sequence,o.observation,o.reason_code,o.stored_at FROM delivery_units u JOIN LATERAL(SELECT revision_id,action FROM delivery_intents i WHERE i.unit_id=u.id AND i.status='ACKNOWLEDGED' ORDER BY i.created_at DESC LIMIT 1)m ON m.action IN ('SUBMIT','ADOPT') JOIN LATERAL(SELECT * FROM delivery_observations x WHERE x.unit_id=u.id AND x.revision_id=m.revision_id ORDER BY sequence DESC LIMIT 1)o ON true WHERE u.organization_id=$1 AND u.environment_id=$2 AND u.target_generation=$3 AND u.delivery_epoch=$4 AND ($5='' OR u.id>NULLIF($5,'')::uuid) ORDER BY u.id LIMIT $6`, org, env, generation, epoch, after, limit)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []persistence.DeliveryObservation{}
	for rows.Next() {
		var o persistence.DeliveryObservation
		var raw []byte
		if err := rows.Scan(&o.ID, &o.UnitID, &o.WorkloadID, &o.Sequence, &raw, &o.ReasonCode, &o.StoredAt); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &o.Observation) != nil {
			return nil, domain.ErrInvalid
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
