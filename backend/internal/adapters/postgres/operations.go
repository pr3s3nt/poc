package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

const secretStoreSelect = `SELECT ss.id::text,ss.store_key,o.organization_key,ss.name,ss.provider,ss.backend_address,ss.workload_address,ss.kv_mount,ss.auth_mount,ss.tls_ca_pem,ss.credential_ref,ss.status,ss.verification,ss.created_at FROM secret_store_connections ss JOIN organizations o ON o.id=ss.organization_id`

func scanSecretStore(row pgx.Row) (secretstore.Store, error) {
	var v secretstore.Store
	var verification []byte
	if err := row.Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Name, &v.Provider, &v.BackendAddress, &v.WorkloadAddress, &v.Mount, &v.AuthMount, &v.TLSCAPEM, &v.CredentialRef, &v.Status, &verification, &v.CreatedAt); err != nil {
		return v, err
	}
	return v, unmarshalJSON(verification, &v.Verification)
}

// CreateSecretStore is insert-only registration.
func (s *Store) CreateSecretStore(ctx context.Context, v secretstore.Store) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ID == "" {
		v.ID = ids.New()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	verification, err := jsonBytes(v.Verification)
	if err != nil {
		return err
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO secret_store_connections(id,organization_id,store_key,name,provider,backend_address,workload_address,kv_mount,auth_mount,tls_ca_pem,credential_ref,status,verification,created_at) SELECT $1::uuid,o.id,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14 FROM organizations o WHERE o.organization_key=$2`,
		v.ID, v.OrganizationKey, v.Key, v.Name, v.Provider, v.BackendAddress, v.WorkloadAddress, v.Mount, v.AuthMount, v.TLSCAPEM, v.CredentialRef, v.Status, verification, v.CreatedAt)
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return fmt.Errorf("%w: secret store %q", persistence.ErrDuplicate, v.Key)
		}
		return fmt.Errorf("postgres: create secret store: %w", translate(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: organization %q", persistence.ErrNotFound, v.OrganizationKey)
	}
	return nil
}

func (s *Store) GetSecretStore(ctx context.Context, org, key string) (secretstore.Store, error) {
	v, err := scanSecretStore(s.q(ctx).QueryRow(ctx, secretStoreSelect+` WHERE o.organization_key=$1 AND ss.store_key=$2`, org, key))
	if err != nil {
		return v, fmt.Errorf("postgres: get secret store: %w", translate(err))
	}
	return v, nil
}

func (s *Store) ListSecretStores(ctx context.Context, org string) ([]secretstore.Store, error) {
	rows, err := s.q(ctx).Query(ctx, secretStoreSelect+` WHERE o.organization_key=$1 ORDER BY ss.store_key`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []secretstore.Store{}
	for rows.Next() {
		v, err := scanSecretStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AdmitManagedSecretStore creates the bootstrap store or refreshes the credential
// of a Compose-managed record; an unmarked user record is never rewritten.
// The prior row is locked so concurrent startups serialize; a concurrent insert
// is stopped by the unique (organization, key) index.
func (s *Store) AdmitManagedSecretStore(ctx context.Context, adm persistence.ManagedStoreAdmission) (secretstore.Store, error) {
	next := adm.Store
	if err := next.Validate(); err != nil {
		return secretstore.Store{}, err
	}
	var out secretstore.Store
	err := s.Transact(ctx, func(ctx context.Context) error {
		if adm.ExpectedID == "" {
			if err := s.CreateSecretStore(ctx, next); err != nil {
				return err
			}
			var err error
			out, err = s.GetSecretStore(ctx, next.OrganizationKey, next.Key)
			return err
		}
		var lockedID string
		err := s.q(ctx).QueryRow(ctx, `SELECT ss.id::text FROM secret_store_connections ss JOIN organizations o ON o.id=ss.organization_id WHERE o.organization_key=$1 AND ss.store_key=$2 FOR UPDATE OF ss`, next.OrganizationKey, next.Key).Scan(&lockedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: secret store %q changed", persistence.ErrVersionConflict, next.Key)
		}
		if err != nil {
			return translate(err)
		}
		current, err := s.GetSecretStore(ctx, next.OrganizationKey, next.Key)
		if err != nil {
			return err
		}
		if current.ID != adm.ExpectedID || current.CredentialRef != adm.ExpectedCredentialRef {
			return fmt.Errorf("%w: secret store %q changed", persistence.ErrVersionConflict, next.Key)
		}
		if !current.ComposeManaged() || !next.ComposeManaged() {
			return fmt.Errorf("%w: secret store %q is not Compose-managed", persistence.ErrImmutable, next.Key)
		}
		if !current.SameEndpoint(next) {
			return fmt.Errorf("%w: secret store %q endpoint identity", persistence.ErrImmutable, next.Key)
		}
		verification, err := jsonBytes(next.Verification)
		if err != nil {
			return err
		}
		if _, err := s.q(ctx).Exec(ctx, `UPDATE secret_store_connections SET name=$2, credential_ref=$3, status=$4, verification=$5 WHERE id=$1::uuid AND credential_ref=$6`,
			current.ID, next.Name, next.CredentialRef, next.Status, verification, adm.ExpectedCredentialRef); err != nil {
			return translate(err)
		}
		out, err = s.GetSecretStore(ctx, next.OrganizationKey, next.Key)
		return err
	})
	return out, err
}

const operationSelect = `SELECT op.id::text,a.application_key,e.environment_key,COALESCE(op.review_request_id::text,''),op.kind,op.owner_instance,op.fence,op.status,op.stage,op.pins,op.detail,op.failure,op.started_at,op.updated_at,op.heartbeat_at,op.deadline,op.finished_at FROM environment_operations op JOIN environments e ON e.id=op.environment_id JOIN applications a ON a.id=e.application_id`

func scanOperation(row pgx.Row) (environment.Operation, error) {
	var v environment.Operation
	var pins, detail []byte
	if err := row.Scan(&v.ID, &v.ApplicationKey, &v.EnvironmentKey, &v.ReviewRequestID, &v.Kind, &v.Owner, &v.Fence, &v.Status, &v.Stage, &pins, &detail, &v.Failure, &v.StartedAt, &v.UpdatedAt, &v.HeartbeatAt, &v.Deadline, &v.FinishedAt); err != nil {
		return v, err
	}
	if err := unmarshalJSON(pins, &v.Pins); err != nil {
		return v, err
	}
	return v, unmarshalJSON(detail, &v.Detail)
}

// ClaimEnvironment locks the Environment row, verifies every pin and inserts
// the claim in one local transaction. The partial unique index is the final
// guard against two claimants across backend processes.
func (s *Store) ClaimEnvironment(ctx context.Context, claim persistence.OperationClaim) (environment.Operation, error) {
	if claim.Kind == environment.OpResourceCleanup {
		return environment.Operation{}, fmt.Errorf("postgres: a resource cleanup is claimed only with its confirmed scope")
	}
	return s.claimEnvironment(ctx, claim, nil)
}

// cleanupColumns are the RESOURCE_CLEANUP authorization columns inserted with
// the claim row, so the admitted scope exists from the claim's first commit.
type cleanupColumns struct {
	mode, actor, root, requestHash, confirmationName, runID any
	confirmedScope, scopeSnapshot                           []byte
}

func (s *Store) claimEnvironment(ctx context.Context, claim persistence.OperationClaim, cleanup *cleanupColumns) (environment.Operation, error) {
	if cleanup == nil {
		cleanup = &cleanupColumns{}
	}
	var out environment.Operation
	err := s.Transact(ctx, func(ctx context.Context) error {
		var envID string
		if err := s.q(ctx).QueryRow(ctx, `SELECT e.id::text FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2 FOR UPDATE OF e`, claim.ApplicationKey, claim.EnvironmentKey).Scan(&envID); err != nil {
			return translate(err)
		}
		env, err := s.GetEnvironment(ctx, claim.ApplicationKey, claim.EnvironmentKey)
		if err != nil {
			return err
		}
		if env.ActiveOperationID != "" {
			return persistence.Busy(claim.ApplicationKey, claim.EnvironmentKey)
		}
		scope, err := s.GetConfigurationScope(ctx, claim.ApplicationKey, claim.EnvironmentKey)
		if err != nil {
			return err
		}
		pins := claim.Pins
		if (pins.CheckEnvVersion && env.Version != pins.EnvVersion) ||
			(pins.CheckDraftVersion && env.DraftVersion != pins.DraftVersion) ||
			(pins.CheckConfigVersion && scope.Version != pins.ConfigVersion) ||
			(pins.CheckSet && env.CurrentDeploymentSetID != pins.CurrentSetID) ||
			(pins.CheckRevision && scope.DesiredRevisionID != pins.DesiredRevisionID) ||
			(pins.CheckBinding && env.Binding() != pins.Binding) ||
			(pins.CheckStore && env.SecretStoreKey != pins.SecretStoreKey) {
			return fmt.Errorf("%w: environment %s/%s changed since it was read", persistence.ErrVersionConflict, claim.ApplicationKey, claim.EnvironmentKey)
		}
		id := claim.ID
		if id == "" {
			id = ids.New()
		}
		now := time.Now().UTC()
		pinJSON, err := jsonBytes(pins)
		if err != nil {
			return err
		}
		detail := claim.Detail
		if detail == nil {
			detail = map[string]any{}
		}
		detailJSON, err := jsonBytes(detail)
		if err != nil {
			return err
		}
		if _, err = s.q(ctx).Exec(ctx, `INSERT INTO environment_operations(id,environment_id,kind,owner_instance,status,stage,pins,detail,started_at,updated_at,heartbeat_at,deadline,authorization_mode,actor_ref,root_resource_instance_id,confirmed_scope,deletion_request_hash,confirmation_name,scope_snapshot,run_id,review_request_id) VALUES($1::uuid,$2::uuid,$3,$4,'ACTIVE',$17,$5,$6,$7,$7,$7,$8,$9,$10,$11::uuid,$12::jsonb,$13,$14,$15::jsonb,$16,NULLIF($18,'')::uuid)`, id, envID, claim.Kind, claim.Owner, pinJSON, detailJSON, now, claim.Deadline,
			cleanup.mode, cleanup.actor, cleanup.root, cleanup.confirmedScope, cleanup.requestHash, cleanup.confirmationName, cleanup.scopeSnapshot, cleanup.runID, initialStage(claim.Kind), claim.ReviewRequestID); err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" {
				if pg.ConstraintName == "environment_operations_pkey" {
					return fmt.Errorf("%w: operation id %q is already used", persistence.ErrDuplicate, id)
				}
				if pg.ConstraintName == "environment_operations_review_request" {
					return persistence.ErrDuplicate
				}
				return persistence.Busy(claim.ApplicationKey, claim.EnvironmentKey)
			}
			return translate(err)
		}
		if _, err = s.q(ctx).Exec(ctx, `UPDATE environments SET active_operation_id=$1::uuid WHERE id=$2::uuid`, id, envID); err != nil {
			return translate(err)
		}
		out, err = s.GetOperation(ctx, id)
		return err
	})
	return out, err
}

func (s *Store) GetOperation(ctx context.Context, id string) (environment.Operation, error) {
	var parsed pgtype.UUID
	if err := parsed.Scan(id); err != nil {
		return environment.Operation{}, fmt.Errorf("%w: operation %q", persistence.ErrNotFound, id)
	}
	v, err := scanOperation(s.q(ctx).QueryRow(ctx, operationSelect+` WHERE op.id=$1::uuid`, id))
	if err != nil {
		return v, fmt.Errorf("postgres: get operation: %w", translate(err))
	}
	return v, nil
}

func (s *Store) ActiveOperation(ctx context.Context, app, env string) (environment.Operation, bool, error) {
	v, err := scanOperation(s.q(ctx).QueryRow(ctx, operationSelect+` WHERE a.application_key=$1 AND e.environment_key=$2 AND op.status IN ('ACTIVE','INTERRUPTED','RECOVERING')`, app, env))
	if errors.Is(err, pgx.ErrNoRows) {
		return environment.Operation{}, false, nil
	}
	if err != nil {
		return v, false, err
	}
	return v, true, nil
}

func (s *Store) ListOperations(ctx context.Context, app, env string, limit int) ([]environment.Operation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q(ctx).Query(ctx, operationSelect+` WHERE a.application_key=$1 AND e.environment_key=$2 ORDER BY op.started_at DESC LIMIT $3`, app, env, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []environment.Operation
	for rows.Next() {
		v, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) HeartbeatOperation(ctx context.Context, owner persistence.Owner, stage string, detail map[string]any) error {
	now := time.Now().UTC()
	var detailJSON any
	if detail != nil {
		b, err := jsonBytes(detail)
		if err != nil {
			return err
		}
		detailJSON = b
	}
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environment_operations SET heartbeat_at=$4,updated_at=$4,stage=CASE WHEN $5='' THEN stage ELSE $5 END,detail=COALESCE($6::jsonb,detail) WHERE id=$1::uuid AND owner_instance=$2 AND fence=$3 AND status IN ('ACTIVE','RECOVERING')`, owner.OperationID, owner.Owner, owner.Fence, now, stage, detailJSON)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return persistence.ErrOperationLost
	}
	return nil
}

// ReleaseOperation ends a claim; only the current (owner, fence) may release.
func (s *Store) ReleaseOperation(ctx context.Context, owner persistence.Owner, status environment.OperationStatus, failure string) error {
	if status.Holds() {
		return fmt.Errorf("postgres: release needs a terminal status")
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		now := time.Now().UTC()
		// Lock the operation row first: state-lock and capability admission
		// share-lock the same row, so once this lock is held no admission is
		// in flight and every later admission sees the released status.
		var current, currentStatus string
		var fence int64
		if err := s.q(ctx).QueryRow(ctx, `SELECT owner_instance,fence,status FROM environment_operations WHERE id=$1::uuid FOR UPDATE`, owner.OperationID).Scan(&current, &fence, &currentStatus); err != nil {
			return translate(err)
		}
		if !environment.OperationStatus(currentStatus).Holds() {
			return nil
		}
		if current != owner.Owner || fence != owner.Fence {
			return persistence.ErrOperationLost
		}
		var held bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terraform_state_locks WHERE environment_operation_id=$1::uuid)`, owner.OperationID).Scan(&held); err != nil {
			return translate(err)
		}
		if held {
			return persistence.ErrStateLocksHeld
		}
		if _, err := s.q(ctx).Exec(ctx, `UPDATE environment_operations SET status=$2,failure=$3,updated_at=$4,finished_at=$4 WHERE id=$1::uuid`, owner.OperationID, status, failure, now); err != nil {
			return translate(err)
		}
		var err error
		if _, err = s.q(ctx).Exec(ctx, `DELETE FROM terraform_executor_capabilities WHERE environment_operation_id=$1::uuid`, owner.OperationID); err != nil {
			return translate(err)
		}
		_, err = s.q(ctx).Exec(ctx, `UPDATE environments SET active_operation_id=NULL WHERE active_operation_id=$1::uuid`, owner.OperationID)
		return translate(err)
	})
}

// fenced runs fn in a transaction that holds a share lock on the caller's
// operation row after verifying its (owner, fence). BeginRecovery needs the
// row exclusively, so a recovery that already committed makes the stale owner's
// write fail and one that has not yet committed waits for the write to finish.
// A context without an owner is not fenced.
func (s *Store) fenced(ctx context.Context, fn func(ctx context.Context) error) error {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return fn(ctx)
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		var current string
		var fence int64
		var status string
		err := s.q(ctx).QueryRow(ctx, `SELECT owner_instance,fence,status FROM environment_operations WHERE id=$1::uuid FOR SHARE`, owner.OperationID).Scan(&current, &fence, &status)
		if err != nil {
			return persistence.ErrOperationLost
		}
		if current != owner.Owner || fence != owner.Fence || (status != "ACTIVE" && status != "RECOVERING") {
			return persistence.ErrOperationLost
		}
		return fn(ctx)
	})
}

// SuspendOperation returns a held claim to INTERRUPTED without releasing it.
func (s *Store) SuspendOperation(ctx context.Context, owner persistence.Owner, failure string) error {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environment_operations SET status='INTERRUPTED',failure=$4,updated_at=$5 WHERE id=$1::uuid AND owner_instance=$2 AND fence=$3 AND status IN ('ACTIVE','RECOVERING')`, owner.OperationID, owner.Owner, owner.Fence, failure, time.Now().UTC())
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return persistence.ErrOperationLost
	}
	return nil
}

func (s *Store) MarkInterrupted(ctx context.Context, staleBefore time.Time) (int, error) {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environment_operations SET status='INTERRUPTED',updated_at=$2 WHERE status IN ('ACTIVE','RECOVERING') AND heartbeat_at<$1`, staleBefore, time.Now().UTC())
	if err != nil {
		return 0, translate(err)
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) BeginRecovery(ctx context.Context, id, newOwner, confirmedBy string) (environment.Operation, error) {
	if strings.TrimSpace(confirmedBy) == "" {
		return environment.Operation{}, persistence.ErrRecoveryUnconfirmed
	}
	now := time.Now().UTC()
	marker, _ := jsonBytes(map[string]any{"stoppedConfirmedBy": confirmedBy})
	var out environment.Operation
	err := s.Transact(ctx, func(ctx context.Context) error {
		tag, err := s.q(ctx).Exec(ctx, `UPDATE environment_operations SET status='RECOVERING',owner_instance=$2,fence=fence+1,updated_at=$3,heartbeat_at=$3,detail=detail||jsonb_build_object('originalDeadline',to_char(deadline AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'))||$4::jsonb,deadline=$5 WHERE id=$1::uuid AND status='INTERRUPTED'`, id, newOwner, now, marker, now.Add(persistence.RecoveryWindow))
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			if _, getErr := s.GetOperation(ctx, id); getErr != nil {
				return getErr
			}
			return persistence.ErrVersionConflict
		}
		// The fence changed: every executor capability of the old owner is revoked.
		if _, err = s.q(ctx).Exec(ctx, `DELETE FROM terraform_executor_capabilities WHERE environment_operation_id=$1::uuid`, id); err != nil {
			return translate(err)
		}
		out, err = s.GetOperation(ctx, id)
		return err
	})
	return out, err
}

// initialStage is the durable stage an operation is admitted with. A resource
// cleanup is REQUESTED the moment its claim and confirmed scope commit, so the
// persisted operation, the 202 and a restart all agree before the engine runs.
func initialStage(kind environment.OperationKind) string {
	if kind == environment.OpResourceCleanup {
		return "REQUESTED"
	}
	return ""
}

func (s *Store) GetReviewOperation(ctx context.Context, app, env, requestID string) (environment.Operation, error) {
	op, err := scanOperation(s.q(ctx).QueryRow(ctx, operationSelect+` WHERE a.application_key=$1 AND e.environment_key=$2 AND op.review_request_id=$3::uuid`, app, env, requestID))
	return op, translate(err)
}
