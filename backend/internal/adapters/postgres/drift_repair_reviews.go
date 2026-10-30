package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

var _ terraformstate.RepairReviewRepository = (*StateStore)(nil)

const repairSelect = `SELECT safe_review,organization_id::text,actor_id::text,request_id::text,request_digest,private_refs,token_cipher,token_hash,version,COALESCE(consumed_operation_id::text,''),COALESCE(confirmation_request_id::text,'') FROM drift_repair_reviews`

func scanRepairReview(row pgx.Row) (out terraformstate.PrivateRepairReview, err error) {
	var safe, refs, token []byte
	var org, actor, request, digest, consumed, confirmation string
	var version int64
	err = row.Scan(&safe, &org, &actor, &request, &digest, &refs, &token, &out.TokenHash, &version, &consumed, &confirmation)
	if err != nil {
		return out, translate(err)
	}
	var artifacts map[string]string
	if json.Unmarshal(safe, &out.Review) != nil || json.Unmarshal(refs, &artifacts) != nil || json.Unmarshal(token, &out.Token) != nil {
		return out, persistence.ErrImmutable
	}
	r := &out.Review
	r.OrganizationID, r.ActorID, r.RequestID, r.RequestDigest, r.Version, r.ConsumedOperationID, r.ConfirmationRequestID = org, actor, request, digest, version, consumed, confirmation
	for n := range r.Instances {
		r.Instances[n].ArtifactID = artifacts[r.Instances[n].InstanceID]
	}
	return
}
func repairJSON(row terraformstate.PrivateRepairReview) ([]byte, []byte, []byte, error) {
	safe, err := json.Marshal(row.Review)
	if err != nil {
		return nil, nil, nil, err
	}
	refs := map[string]string{}
	for _, i := range row.Review.Instances {
		if i.ArtifactID != "" {
			refs[i.InstanceID] = i.ArtifactID
		}
	}
	private, err := json.Marshal(refs)
	if err != nil {
		return nil, nil, nil, err
	}
	token, err := json.Marshal(row.Token)
	return safe, private, token, err
}
func (s *StateStore) CreateRepairReview(ctx context.Context, row terraformstate.PrivateRepairReview) error {
	r := row.Review
	if !validUUID(r.ID) || !validUUID(r.ScanID) || !validUUID(r.ActorID) || !validUUID(r.RequestID) || r.Status != drift.Preparing || r.Version != 1 || r.ExpiresAt.Sub(r.CreatedAt) > 10*time.Minute || !r.ExpiresAt.After(r.CreatedAt) || row.Token.OwnerID != r.ID || row.Token.OrganizationID != r.OrganizationID || row.Token.Purpose != "TERRAFORM_REVIEW_TOKEN" || len(row.TokenHash) != 64 {
		return drift.ErrScan
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		if scope.organizationID != r.OrganizationID || scope.environmentID != r.EnvironmentID || scope.owner.OperationID != r.OperationID {
			return persistence.ErrOperationLost
		}
		safe, refs, token, err := repairJSON(row)
		if err != nil {
			return err
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO drift_repair_reviews(id,organization_id,environment_id,scan_id,actor_id,request_id,request_digest,operation_id,status,safe_review,private_refs,token_hash,token_cipher,environment_version,version,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PREPARING',$9,$10,$11,$12,$13,1,$14,$15)`, r.ID, r.OrganizationID, r.EnvironmentID, r.ScanID, r.ActorID, r.RequestID, r.RequestDigest, r.OperationID, safe, refs, row.TokenHash, token, r.EnvironmentVersion, r.ExpiresAt, r.CreatedAt)
		return translate(err)
	})
}
func (s *StateStore) UpdateRepairReview(ctx context.Context, row terraformstate.PrivateRepairReview, version int64) error {
	r := row.Review
	if r.Status != drift.Ready && r.Status != drift.Blocked && r.Status != drift.RepairFailed {
		return drift.ErrScan
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		if scope.organizationID != r.OrganizationID || scope.environmentID != r.EnvironmentID || scope.owner.OperationID != r.OperationID {
			return persistence.ErrOperationLost
		}
		current, err := s.GetRepairReview(ctx, r.OrganizationID, r.EnvironmentID, r.ID)
		if err != nil {
			return err
		}
		prior := current.Review
		if prior.ScanID != r.ScanID || prior.ActorID != r.ActorID || prior.OperationID != r.OperationID || prior.EnvironmentVersion != r.EnvironmentVersion || len(prior.Instances) != len(r.Instances) {
			return persistence.ErrImmutable
		}
		for n, instance := range r.Instances {
			pin := prior.Instances[n]
			if pin.InstanceID != instance.InstanceID || pin.BaselineID != instance.BaselineID || pin.InstanceVersion != instance.InstanceVersion {
				return persistence.ErrImmutable
			}
			if r.Status == drift.Ready && (len(r.Blockers) > 0 || len(instance.Blockers) > 0 || instance.ArtifactID == "" || !instance.Report.InPlaceOnly()) {
				return persistence.ErrImmutable
			}
			if instance.ArtifactID != "" {
				var valid bool
				if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reconciliation_private_artifacts WHERE id=$1 AND organization_id=$2 AND owner_id=$3 AND purpose='TERRAFORM_REVIEW')`, instance.ArtifactID, r.OrganizationID, instance.InstanceID).Scan(&valid); err != nil {
					return translate(err)
				}
				if !valid {
					return persistence.ErrImmutable
				}
			}
		}
		safe, refs, _, err := repairJSON(row)
		if err != nil {
			return err
		}
		tag, err := s.q(ctx).Exec(ctx, `UPDATE drift_repair_reviews SET status=$5,safe_review=$6,private_refs=$7,version=version+1 WHERE id=$1 AND organization_id=$2 AND environment_id=$3 AND version=$4 AND status='PREPARING' AND operation_id=$8`, r.ID, r.OrganizationID, r.EnvironmentID, version, r.Status, safe, refs, r.OperationID)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		return nil
	})
}
func (s *StateStore) GetRepairReview(ctx context.Context, org, env, id string) (terraformstate.PrivateRepairReview, error) {
	if !validUUID(org) || !validUUID(env) || !validUUID(id) {
		return terraformstate.PrivateRepairReview{}, persistence.ErrNotFound
	}
	return scanRepairReview(s.q(ctx).QueryRow(ctx, repairSelect+` WHERE organization_id=$1 AND environment_id=$2 AND id=$3`, org, env, id))
}
func (s *StateStore) FindRepairReviewRequest(ctx context.Context, org, env, actor, request string) (terraformstate.PrivateRepairReview, error) {
	if !validUUID(org) || !validUUID(env) || !validUUID(actor) || !validUUID(request) {
		return terraformstate.PrivateRepairReview{}, persistence.ErrNotFound
	}
	return scanRepairReview(s.q(ctx).QueryRow(ctx, repairSelect+` WHERE organization_id=$1 AND environment_id=$2 AND actor_id=$3 AND request_id=$4`, org, env, actor, request))
}
func (s *StateStore) ConsumeRepairReview(ctx context.Context, org, env, id, actor, request, tokenHash string, version int64) (out terraformstate.PrivateRepairReview, err error) {
	if !validUUID(actor) || !validUUID(request) || len(tokenHash) != 64 {
		return out, drift.ErrScan
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		if scope.organizationID != org || scope.environmentID != env {
			return persistence.ErrOperationLost
		}
		current, err := s.GetRepairReview(ctx, org, env, id)
		if err != nil {
			return err
		}
		var environmentVersion int64
		if err := s.q(ctx).QueryRow(ctx, `SELECT version FROM environments WHERE id=$1 AND active_operation_id=$2`, env, scope.owner.OperationID).Scan(&environmentVersion); err != nil {
			return persistence.ErrOperationLost
		}
		if environmentVersion != current.Review.EnvironmentVersion {
			return persistence.ErrVersionConflict
		}
		tag, err := s.q(ctx).Exec(ctx, `UPDATE drift_repair_reviews SET status='CONSUMED',safe_review=jsonb_set(safe_review,'{status}','"CONSUMED"'),consumed_operation_id=$7,confirmation_request_id=$8,version=version+1 WHERE id=$1 AND organization_id=$2 AND environment_id=$3 AND actor_id=$4 AND version=$5 AND token_hash=$6 AND status='READY' AND expires_at>$9`, id, org, env, actor, version, tokenHash, scope.owner.OperationID, request, time.Now().UTC())
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		out, err = s.GetRepairReview(ctx, org, env, id)
		if err != nil {
			return err
		}
		if current.Review.Status != drift.Ready {
			return errors.New("repair review not ready")
		}
		return nil
	})
	return
}
