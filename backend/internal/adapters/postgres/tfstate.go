package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// The Terraform v2 repository (ADR-014, OC-18). Owner-only writes verify the
// claim owner of ctx against the operation row under a share lock in the same
// transaction, exactly like fenced(): a recovery that already took the claim
// makes the write fail, one that has not committed waits for it.

// StateStore is the private Terraform state repository. It runs only on the
// State runtime login, a connection separate from the Console Store: that
// login alone may read ciphertext and capability hashes, and it has no
// product, owner or DDL privilege (see grants.go).
type StateStore struct {
	db
	identity Identity
}

// OpenState connects the State runtime login and validates, without DDL,
// the owner's baseline and that the login holds exactly the State grants.
// It never accepts the Console, owner or a superuser login.
func OpenState(ctx context.Context, databaseURL string) (*StateStore, error) {
	d, identity, err := openRuntime(ctx, databaseURL, StateRuntime)
	if err != nil {
		return nil, err
	}
	return &StateStore{db: d, identity: identity}, nil
}

// Identity is the validated installation the State connection serves.
func (s *StateStore) Identity() Identity { return s.identity }

// ownerScope is the verified Environment scope of the claim owner in ctx.
type ownerScope struct {
	owner          persistence.Owner
	status         string
	environmentID  string
	applicationID  string
	organizationID string
	generationHigh int64
}

func (s *StateStore) requireOwner(ctx context.Context) (ownerScope, error) {
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return ownerScope{}, terraformstate.ErrOwnerRequired
	}
	if !validUUID(owner.OperationID) {
		return ownerScope{}, persistence.ErrOperationLost
	}
	v := ownerScope{owner: owner}
	var current, active string
	var fence int64
	err := s.q(ctx).QueryRow(ctx, `SELECT op.owner_instance,op.fence,op.status,op.environment_id::text,a.id::text,a.organization_id::text,COALESCE(e.active_operation_id::text,''),e.generation_high FROM environment_operations op JOIN environments e ON e.id=op.environment_id JOIN applications a ON a.id=e.application_id WHERE op.id=$1::uuid FOR SHARE OF op`, owner.OperationID).
		Scan(&current, &fence, &v.status, &v.environmentID, &v.applicationID, &v.organizationID, &active, &v.generationHigh)
	if err != nil {
		return ownerScope{}, persistence.ErrOperationLost
	}
	if current != owner.Owner || fence != owner.Fence || (v.status != string(environment.OpActive) && v.status != string(environment.OpRecovering)) || active != owner.OperationID {
		return ownerScope{}, persistence.ErrOperationLost
	}
	return v, nil
}

func validUUID(id string) bool {
	var parsed pgtype.UUID
	return parsed.Scan(id) == nil
}

func translateTF(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514":
			return fmt.Errorf("%w: %s", persistence.ErrImmutable, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: reference outside the instance scope", tfresource.ErrInvalid)
		}
	}
	return translate(err)
}

const instanceColumns = `i.id::text,i.organization_id::text,i.application_id::text,i.environment_id::text,i.target_generation,i.descriptor,i.scope_type,i.scope_id,i.state_key,COALESCE(i.active_resource_id::text,''),i.module_bundle_id::text,i.target_ref,i.physical_intent,i.created_by_run_id,i.status,i.version,i.created_at,i.updated_at`

func scanInstance(row pgx.Row) (tfresource.Instance, error) {
	var v tfresource.Instance
	var scopeType string
	var target, intent []byte
	if err := row.Scan(&v.ID, &v.Identity.OrganizationID, &v.Identity.ApplicationID, &v.Identity.EnvironmentID, &v.Identity.TargetGeneration, &v.Identity.Descriptor, &scopeType, &v.Identity.Scope.ID, &v.StateKey, &v.ActiveResourceID, &v.ModuleBundleID, &target, &intent, &v.CreatedByRunID, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return v, err
	}
	v.Identity.Scope.Type = resource.ScopeType(scopeType)
	if err := unmarshalJSON(target, &v.TargetRef); err != nil {
		return v, err
	}
	return v, unmarshalJSON(intent, &v.PhysicalIntent)
}

// RegisterInstance writes the instance identity before any provider call.
func (s *StateStore) RegisterInstance(ctx context.Context, in terraformstate.NewInstance) (tfresource.Instance, error) {
	if in.ModuleBundleID == "" || !validUUID(in.ModuleBundleID) || in.TargetRef == nil || in.PhysicalIntent == nil || in.CreatedByRunID == "" {
		return tfresource.Instance{}, fmt.Errorf("%w: instance needs module, target, physical intent and run", tfresource.ErrInvalid)
	}
	if in.ActiveResourceID != "" && !validUUID(in.ActiveResourceID) {
		return tfresource.Instance{}, fmt.Errorf("%w: active resource id", tfresource.ErrInvalid)
	}
	target, err := jsonBytes(in.TargetRef)
	if err != nil {
		return tfresource.Instance{}, err
	}
	intent, err := jsonBytes(in.PhysicalIntent)
	if err != nil {
		return tfresource.Instance{}, err
	}
	var out tfresource.Instance
	err = s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		if in.TargetGeneration < 0 || in.TargetGeneration > scope.generationHigh {
			return fmt.Errorf("%w: target generation is not allocated in the Environment", tfresource.ErrInvalid)
		}
		identity := tfresource.Identity{OrganizationID: scope.organizationID, ApplicationID: scope.applicationID, EnvironmentID: scope.environmentID, TargetGeneration: in.TargetGeneration, Descriptor: in.Descriptor, Scope: resource.Scope{Type: resource.ScopeType(in.ScopeType), ID: in.ScopeID}}
		if err := identity.Validate(); err != nil {
			return err
		}
		var active any
		if in.ActiveResourceID != "" {
			active = in.ActiveResourceID
		}
		now := time.Now().UTC()
		key := identity.StateKey()
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO terraform_resource_instances(id,organization_id,application_id,environment_id,target_generation,descriptor,scope_type,scope_id,state_key,active_resource_id,module_bundle_id,target_ref,physical_intent,created_by_run_id,status,version,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10::uuid,$11::uuid,$12,$13,$14,'PENDING',1,$15,$15) ON CONFLICT(organization_id,environment_id,target_generation,descriptor,scope_type,scope_id) DO NOTHING`,
			ids.New(), identity.OrganizationID, identity.ApplicationID, identity.EnvironmentID, identity.TargetGeneration, identity.Descriptor, string(identity.Scope.Type), identity.Scope.ID, key, active, in.ModuleBundleID, target, intent, in.CreatedByRunID, now)
		if err != nil {
			return translateTF(err)
		}
		out, err = scanInstance(s.q(ctx).QueryRow(ctx, `SELECT `+instanceColumns+` FROM terraform_resource_instances i WHERE i.state_key=$1`, key))
		return translate(err)
	})
	return out, err
}

// FindInstance looks an instance up by identity inside the owner's Environment.
func (s *StateStore) FindInstance(ctx context.Context, generation int64, descriptor, scopeType, scopeID string) (tfresource.Instance, bool, error) {
	var out tfresource.Instance
	found := false
	err := s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		v, err := scanInstance(s.q(ctx).QueryRow(ctx, `SELECT `+instanceColumns+` FROM terraform_resource_instances i WHERE i.environment_id=$1::uuid AND i.target_generation=$2 AND i.descriptor=$3 AND i.scope_type=$4 AND i.scope_id=$5`, scope.environmentID, generation, descriptor, scopeType, scopeID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return translate(err)
		}
		out, found = v, true
		return nil
	})
	return out, found, err
}

func (s *StateStore) GetInstance(ctx context.Context, id string) (tfresource.Instance, error) {
	if !validUUID(id) {
		return tfresource.Instance{}, persistence.ErrNotFound
	}
	v, err := scanInstance(s.q(ctx).QueryRow(ctx, `SELECT `+instanceColumns+` FROM terraform_resource_instances i WHERE i.id=$1::uuid`, id))
	if err != nil {
		return v, translate(err)
	}
	return v, nil
}

func (s *StateStore) ListInstances(ctx context.Context, environmentID string) ([]tfresource.Instance, error) {
	if !validUUID(environmentID) {
		return nil, persistence.ErrNotFound
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT `+instanceColumns+` FROM terraform_resource_instances i WHERE i.environment_id=$1::uuid ORDER BY i.target_generation,i.descriptor,i.scope_type,i.scope_id`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tfresource.Instance{}
	for rows.Next() {
		v, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *StateStore) SetInstanceStatus(ctx context.Context, id string, expectedVersion int64, status tfresource.InstanceStatus) (tfresource.Instance, error) {
	if !status.Valid() || !validUUID(id) {
		return tfresource.Instance{}, fmt.Errorf("%w: instance status", tfresource.ErrInvalid)
	}
	var out tfresource.Instance
	err := s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		out, err = scanInstance(s.q(ctx).QueryRow(ctx, `UPDATE terraform_resource_instances i SET status=$3,version=version+1,updated_at=$4 WHERE i.id=$1::uuid AND i.environment_id=$2::uuid AND i.version=$5 RETURNING `+instanceColumns, id, scope.environmentID, status, time.Now().UTC(), expectedVersion))
		if errors.Is(err, pgx.ErrNoRows) {
			var env string
			if getErr := s.q(ctx).QueryRow(ctx, `SELECT environment_id::text FROM terraform_resource_instances WHERE id=$1::uuid`, id).Scan(&env); getErr != nil || env != scope.environmentID {
				return persistence.ErrNotFound
			}
			return persistence.ErrVersionConflict
		}
		return translateTF(err)
	})
	return out, err
}

// instanceInScope returns the organization and generation of an instance of
// the owner's Environment; ErrNotFound for any other instance.
func (s *StateStore) instanceInScope(ctx context.Context, scope ownerScope, id string, forUpdate bool) (tfresource.Instance, error) {
	if !validUUID(id) {
		return tfresource.Instance{}, persistence.ErrNotFound
	}
	query := `SELECT ` + instanceColumns + ` FROM terraform_resource_instances i WHERE i.id=$1::uuid AND i.environment_id=$2::uuid`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	v, err := scanInstance(s.q(ctx).QueryRow(ctx, query, id, scope.environmentID))
	if err != nil {
		return v, translate(err)
	}
	return v, nil
}

// advanceConsumer is the CAS on the consumer version that makes a graph
// change visible to pinned previews.
func (s *StateStore) advanceConsumer(ctx context.Context, consumer tfresource.Instance, expected int64) (tfresource.Instance, error) {
	if consumer.Version != expected {
		return tfresource.Instance{}, persistence.ErrVersionConflict
	}
	v, err := scanInstance(s.q(ctx).QueryRow(ctx, `UPDATE terraform_resource_instances i SET version=version+1,updated_at=$3 WHERE i.id=$1::uuid AND i.version=$2 RETURNING `+instanceColumns, consumer.ID, expected, time.Now().UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return tfresource.Instance{}, persistence.ErrVersionConflict
	}
	return v, translateTF(err)
}

func (s *StateStore) RecordDependencies(ctx context.Context, consumerID string, expectedConsumerVersion int64, providerIDs []string) (tfresource.Instance, error) {
	var out tfresource.Instance
	err := s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		consumer, err := s.instanceInScope(ctx, scope, consumerID, true)
		if err != nil {
			return err
		}
		if consumer.Version != expectedConsumerVersion {
			return persistence.ErrVersionConflict
		}
		out = consumer
		var added []string
		for _, provider := range providerIDs {
			if !validUUID(provider) || provider == consumer.ID {
				return fmt.Errorf("%w: provider id", tfresource.ErrInvalid)
			}
			var exists bool
			if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terraform_resource_dependencies WHERE consumer_instance_id=$1::uuid AND provider_instance_id=$2::uuid)`, consumer.ID, provider).Scan(&exists); err != nil {
				return translate(err)
			}
			if !exists {
				added = append(added, provider)
			}
		}
		if len(added) == 0 {
			return nil
		}
		if out, err = s.advanceConsumer(ctx, consumer, expectedConsumerVersion); err != nil {
			return err
		}
		for _, provider := range added {
			// The composite foreign keys reject a provider outside the
			// consumer's Organization, Environment and generation.
			if _, err := s.q(ctx).Exec(ctx, `INSERT INTO terraform_resource_dependencies(consumer_instance_id,provider_instance_id,organization_id,environment_id,target_generation,version) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6)`,
				consumer.ID, provider, consumer.Identity.OrganizationID, consumer.Identity.EnvironmentID, consumer.Identity.TargetGeneration, out.Version); err != nil {
				return translateTF(err)
			}
		}
		return nil
	})
	return out, err
}

func (s *StateStore) RetireDependencies(ctx context.Context, consumerID string, expectedConsumerVersion int64, providerIDs []string) (tfresource.Instance, error) {
	var out tfresource.Instance
	err := s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		consumer, err := s.instanceInScope(ctx, scope, consumerID, true)
		if err != nil {
			return err
		}
		if consumer.Version != expectedConsumerVersion {
			return persistence.ErrVersionConflict
		}
		if consumer.Status != tfresource.InstanceReady && consumer.Status != tfresource.InstanceDestroyed {
			return fmt.Errorf("%w: consumer may still hold its providers", persistence.ErrImmutable)
		}
		if out, err = s.advanceConsumer(ctx, consumer, expectedConsumerVersion); err != nil {
			return err
		}
		for _, provider := range providerIDs {
			if !validUUID(provider) {
				return fmt.Errorf("%w: provider id", tfresource.ErrInvalid)
			}
			tag, err := s.q(ctx).Exec(ctx, `DELETE FROM terraform_resource_dependencies WHERE consumer_instance_id=$1::uuid AND provider_instance_id=$2::uuid`, consumer.ID, provider)
			if err != nil {
				return translateTF(err)
			}
			if tag.RowsAffected() == 0 {
				return persistence.ErrNotFound
			}
		}
		return nil
	})
	return out, err
}

func (s *StateStore) Dependencies(ctx context.Context, instanceID string) ([]tfresource.Dependency, error) {
	if !validUUID(instanceID) {
		return nil, persistence.ErrNotFound
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT d.consumer_instance_id::text,d.provider_instance_id::text,c.status,d.version FROM terraform_resource_dependencies d JOIN terraform_resource_instances c ON c.id=d.consumer_instance_id WHERE d.consumer_instance_id=$1::uuid OR d.provider_instance_id=$1::uuid ORDER BY 1,2`, instanceID)
	if err != nil {
		return nil, err
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

const attemptColumns = `id::text,resource_instance_id::text,environment_operation_id::text,COALESCE(deployment_id::text,''),run_id,attempt_number,action,ownership,stage,status,module_snapshot,target_snapshot,observed_physical_ids,state_serial,safe_error_code,COALESCE(recovery_handle,''),deadline,created_at,updated_at`

func scanAttempt(row pgx.Row) (tfresource.Attempt, error) {
	var a tfresource.Attempt
	var module, target, observed []byte
	if err := row.Scan(&a.ID, &a.InstanceID, &a.OperationID, &a.DeploymentID, &a.RunID, &a.Number, &a.Action, &a.Ownership, &a.Stage, &a.Status, &module, &target, &observed, &a.StateSerial, &a.SafeErrorCode, &a.RecoveryHandle, &a.Deadline, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return a, err
	}
	if err := unmarshalJSON(module, &a.ModuleSnapshot); err != nil {
		return a, err
	}
	if err := unmarshalJSON(target, &a.TargetSnapshot); err != nil {
		return a, err
	}
	return a, unmarshalJSON(observed, &a.ObservedPhysicalIDs)
}

// BeginAttempt assigns the next attempt number under the instance row lock,
// refuses a second RUNNING attempt and classifies ownership atomically.
func (s *StateStore) BeginAttempt(ctx context.Context, a tfresource.Attempt) (tfresource.Attempt, error) {
	a.Status = tfresource.AttemptRunning
	if err := a.ValidateNew(); err != nil {
		return tfresource.Attempt{}, err
	}
	if a.DeploymentID != "" && !validUUID(a.DeploymentID) {
		return tfresource.Attempt{}, fmt.Errorf("%w: deployment id", tfresource.ErrInvalid)
	}
	module, err := jsonBytes(a.ModuleSnapshot)
	if err != nil {
		return tfresource.Attempt{}, err
	}
	target, err := jsonBytes(a.TargetSnapshot)
	if err != nil {
		return tfresource.Attempt{}, err
	}
	var out tfresource.Attempt
	err = s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		inst, err := s.instanceInScope(ctx, scope, a.InstanceID, true)
		if err != nil {
			return err
		}
		var running bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terraform_resource_attempts WHERE resource_instance_id=$1::uuid AND status='RUNNING')`, inst.ID).Scan(&running); err != nil {
			return translate(err)
		}
		if running {
			return fmt.Errorf("%w: instance already has a running attempt", persistence.ErrVersionConflict)
		}
		var previous *tfresource.Ownership
		var first tfresource.Ownership
		err = s.q(ctx).QueryRow(ctx, `SELECT ownership FROM terraform_resource_attempts WHERE resource_instance_id=$1::uuid AND run_id=$2 ORDER BY created_at,attempt_number LIMIT 1`, inst.ID, a.RunID).Scan(&first)
		if err == nil {
			previous = &first
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return translate(err)
		}
		var priorState bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terraform_states WHERE resource_instance_id=$1::uuid)`, inst.ID).Scan(&priorState); err != nil {
			return translate(err)
		}
		if err := tfresource.Classify(a.Ownership, previous, inst.CreatedByRunID == a.RunID, priorState); err != nil {
			return err
		}
		var number int
		if err := s.q(ctx).QueryRow(ctx, `SELECT COALESCE(max(attempt_number),0)+1 FROM terraform_resource_attempts WHERE environment_operation_id=$1::uuid AND resource_instance_id=$2::uuid`, scope.owner.OperationID, inst.ID).Scan(&number); err != nil {
			return translate(err)
		}
		var deployment any
		if a.DeploymentID != "" {
			deployment = a.DeploymentID
		}
		now := time.Now().UTC()
		out, err = scanAttempt(s.q(ctx).QueryRow(ctx, `INSERT INTO terraform_resource_attempts(id,resource_instance_id,environment_operation_id,deployment_id,run_id,attempt_number,action,ownership,stage,status,module_snapshot,target_snapshot,observed_physical_ids,deadline,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,'RUNNING',$10,$11,'[]'::jsonb,$12,$13,$13) RETURNING `+attemptColumns,
			ids.New(), inst.ID, scope.owner.OperationID, deployment, a.RunID, number, a.Action, a.Ownership, a.Stage, module, target, a.Deadline, now))
		return translateTF(err)
	})
	return out, err
}

// UpdateAttempt records safe progress of the owner's attempt. SUCCEEDED and
// FAILED attempts are final.
func (s *StateStore) UpdateAttempt(ctx context.Context, id string, p tfresource.AttemptProgress) (tfresource.Attempt, error) {
	if err := p.Validate(); err != nil {
		return tfresource.Attempt{}, err
	}
	if !validUUID(id) {
		return tfresource.Attempt{}, persistence.ErrNotFound
	}
	observed := p.ObservedPhysicalIDs
	if observed == nil {
		observed = []string{}
	}
	observedJSON, err := jsonBytes(observed)
	if err != nil {
		return tfresource.Attempt{}, err
	}
	var handle any
	if p.RecoveryHandle != "" {
		handle = p.RecoveryHandle
	}
	var out tfresource.Attempt
	err = s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		out, err = scanAttempt(s.q(ctx).QueryRow(ctx, `UPDATE terraform_resource_attempts SET stage=$3,status=$4,observed_physical_ids=$5,safe_error_code=$6,recovery_handle=COALESCE($7,recovery_handle),updated_at=$8 WHERE id=$1::uuid AND environment_operation_id=$2::uuid AND status IN ('RUNNING','INTERRUPTED','NEEDS_ATTENTION') RETURNING `+attemptColumns,
			id, scope.owner.OperationID, p.Stage, p.Status, observedJSON, p.SafeErrorCode, handle, time.Now().UTC()))
		if errors.Is(err, pgx.ErrNoRows) {
			var op, status string
			if getErr := s.q(ctx).QueryRow(ctx, `SELECT environment_operation_id::text,status FROM terraform_resource_attempts WHERE id=$1::uuid`, id).Scan(&op, &status); getErr != nil || op != scope.owner.OperationID {
				return persistence.ErrNotFound
			}
			return fmt.Errorf("%w: attempt is final", persistence.ErrImmutable)
		}
		return translateTF(err)
	})
	return out, err
}

func (s *StateStore) ListAttempts(ctx context.Context, instanceID string) ([]tfresource.Attempt, error) {
	if !validUUID(instanceID) {
		return nil, persistence.ErrNotFound
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT `+attemptColumns+` FROM terraform_resource_attempts WHERE resource_instance_id=$1::uuid ORDER BY created_at,attempt_number`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tfresource.Attempt{}
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *StateStore) InsertCapability(ctx context.Context, c tfresource.Capability) error {
	if !c.Action.Valid() || c.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: capability action or expiry", tfresource.ErrInvalid)
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		inst, err := s.instanceInScope(ctx, scope, c.InstanceID, false)
		if err != nil {
			return err
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO terraform_executor_capabilities(token_hash,organization_id,resource_instance_id,environment_operation_id,fencing_generation,action,expires_at) VALUES($1,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7)`,
			c.TokenHash, inst.Identity.OrganizationID, inst.ID, scope.owner.OperationID, scope.owner.Fence, c.Action, c.ExpiresAt)
		return translateTF(err)
	})
}

// AuthorizeForUpdate verifies a capability against the current claim. The
// operation row stays share-locked until the caller's transaction ends.
func (s *StateStore) AuthorizeForUpdate(ctx context.Context, tokenHash, stateKey string, now time.Time) (terraformstate.Invocation, error) {
	var v terraformstate.Invocation
	var opFence int64
	var status, active, instanceEnv, opEnv, instanceOrg string
	err := s.q(ctx).QueryRow(ctx, `SELECT c.resource_instance_id::text,c.organization_id::text,i.state_key,c.environment_operation_id::text,op.owner_instance,c.fencing_generation,c.action,c.expires_at,op.fence,op.status,COALESCE(e.active_operation_id::text,''),i.environment_id::text,op.environment_id::text,i.organization_id::text
		FROM terraform_executor_capabilities c
		JOIN terraform_resource_instances i ON i.id=c.resource_instance_id
		JOIN environment_operations op ON op.id=c.environment_operation_id
		JOIN environments e ON e.id=op.environment_id
		WHERE c.token_hash=$1 FOR SHARE OF op`, tokenHash).
		Scan(&v.InstanceID, &v.OrganizationID, &v.StateKey, &v.OperationID, &v.Owner, &v.Fence, &v.Action, &v.ExpiresAt, &opFence, &status, &active, &instanceEnv, &opEnv, &instanceOrg)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, terraformstate.ErrUnauthorized
	}
	if err != nil {
		return v, translate(err)
	}
	switch {
	case v.StateKey != stateKey, instanceEnv != opEnv, instanceOrg != v.OrganizationID:
		return v, fmt.Errorf("%w: state scope", terraformstate.ErrForbidden)
	case !now.Before(v.ExpiresAt):
		return v, fmt.Errorf("%w: capability expired", terraformstate.ErrForbidden)
	case opFence != v.Fence, status != string(environment.OpActive) && status != string(environment.OpRecovering), active != v.OperationID:
		return v, fmt.Errorf("%w: operation owner is not current", terraformstate.ErrForbidden)
	}
	return v, nil
}

const lockColumns = `resource_instance_id::text,lock_id,environment_operation_id::text,owner,fencing_generation,lock_metadata,acquired_at,heartbeat_at,deadline`

func scanLock(row pgx.Row) (*tfresource.Lock, error) {
	var l tfresource.Lock
	var meta []byte
	err := row.Scan(&l.InstanceID, &l.LockID, &l.OperationID, &l.Owner, &l.Fence, &meta, &l.AcquiredAt, &l.HeartbeatAt, &l.Deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, translate(err)
	}
	return &l, unmarshalJSON(meta, &l.Metadata)
}

func (s *StateStore) LockForUpdate(ctx context.Context, instanceID string) (*tfresource.Lock, error) {
	return scanLock(s.q(ctx).QueryRow(ctx, `SELECT `+lockColumns+` FROM terraform_state_locks WHERE resource_instance_id=$1::uuid FOR UPDATE`, instanceID))
}

func (s *StateStore) Lock(ctx context.Context, instanceID string) (*tfresource.Lock, error) {
	if !validUUID(instanceID) {
		return nil, persistence.ErrNotFound
	}
	return scanLock(s.q(ctx).QueryRow(ctx, `SELECT `+lockColumns+` FROM terraform_state_locks WHERE resource_instance_id=$1::uuid`, instanceID))
}

func (s *StateStore) InsertLock(ctx context.Context, l tfresource.Lock) (bool, error) {
	meta, err := jsonBytes(l.Metadata)
	if err != nil {
		return false, err
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO terraform_state_locks(resource_instance_id,lock_id,environment_operation_id,owner,fencing_generation,lock_metadata,acquired_at,heartbeat_at,deadline) VALUES($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$7,$8) ON CONFLICT(resource_instance_id) DO NOTHING`,
		l.InstanceID, l.LockID, l.OperationID, l.Owner, l.Fence, meta, l.AcquiredAt, l.Deadline)
	if err != nil {
		return false, translateTF(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *StateStore) DeleteLock(ctx context.Context, instanceID, lockID, operationID string, fence int64) error {
	tag, err := s.q(ctx).Exec(ctx, `DELETE FROM terraform_state_locks WHERE resource_instance_id=$1::uuid AND lock_id=$2 AND environment_operation_id=$3::uuid AND fencing_generation=$4`, instanceID, lockID, operationID, fence)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return terraformstate.ErrLockMismatch
	}
	return nil
}

// ReleaseRecoveredLock removes a state lock left by an earlier fence of the
// owner's own operation. It needs a RECOVERING claim, which BeginRecovery only
// grants after confirmation that the prior execution stopped; it is never
// triggered by expiry.
func (s *StateStore) ReleaseRecoveredLock(ctx context.Context, instanceID string) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		if scope.status != string(environment.OpRecovering) {
			return fmt.Errorf("%w: only a recovering owner may release a prior lock", terraformstate.ErrForbidden)
		}
		if _, err := s.instanceInScope(ctx, scope, instanceID, false); err != nil {
			return err
		}
		tag, err := s.q(ctx).Exec(ctx, `DELETE FROM terraform_state_locks WHERE resource_instance_id=$1::uuid AND environment_operation_id=$2::uuid AND fencing_generation<$3`, instanceID, scope.owner.OperationID, scope.owner.Fence)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() == 0 {
			return terraformstate.ErrLockMismatch
		}
		return nil
	})
}

const stateColumns = `resource_instance_id::text,organization_id::text,ciphertext,nonce,key_id,aad_revision,lineage,serial,version,updated_at`

func (s *StateStore) StateForUpdate(ctx context.Context, instanceID string) (*tfresource.StateBlob, error) {
	var b tfresource.StateBlob
	err := s.q(ctx).QueryRow(ctx, `SELECT `+stateColumns+` FROM terraform_states WHERE resource_instance_id=$1::uuid FOR UPDATE`, instanceID).
		Scan(&b.InstanceID, &b.OrganizationID, &b.Ciphertext, &b.Nonce, &b.KeyID, &b.AADRevision, &b.Lineage, &b.Serial, &b.Version, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, translate(err)
	}
	return &b, nil
}

func (s *StateStore) PutState(ctx context.Context, b tfresource.StateBlob, expectedVersion int64) error {
	var tag pgconn.CommandTag
	var err error
	if expectedVersion == 0 {
		tag, err = s.q(ctx).Exec(ctx, `INSERT INTO terraform_states(resource_instance_id,organization_id,ciphertext,nonce,key_id,aad_revision,lineage,serial,version,updated_at) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(resource_instance_id) DO NOTHING`,
			b.InstanceID, b.OrganizationID, b.Ciphertext, b.Nonce, b.KeyID, b.AADRevision, b.Lineage, b.Serial, b.Version, b.UpdatedAt)
	} else {
		tag, err = s.q(ctx).Exec(ctx, `UPDATE terraform_states SET ciphertext=$3,nonce=$4,key_id=$5,aad_revision=$6,lineage=$7,serial=$8,version=$9,updated_at=$10 WHERE resource_instance_id=$1::uuid AND organization_id=$2::uuid AND version=$11`,
			b.InstanceID, b.OrganizationID, b.Ciphertext, b.Nonce, b.KeyID, b.AADRevision, b.Lineage, b.Serial, b.Version, b.UpdatedAt, expectedVersion)
	}
	if err != nil {
		return translateTF(err)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}

func (s *StateStore) RecordStateSerial(ctx context.Context, operationID, instanceID string, serial int64) error {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE terraform_resource_attempts SET state_serial=$3,updated_at=$4 WHERE id=(SELECT id FROM terraform_resource_attempts WHERE environment_operation_id=$1::uuid AND resource_instance_id=$2::uuid AND status='RUNNING' ORDER BY attempt_number DESC LIMIT 1)`, operationID, instanceID, serial, time.Now().UTC())
	if err != nil {
		return translateTF(err)
	}
	if tag.RowsAffected() == 0 {
		return terraformstate.ErrNoAttempt
	}
	return nil
}

var _ terraformstate.Repository = (*StateStore)(nil)

// ClaimResourceCleanup stores the claim and the exact selected set atomically.
func (s *Store) ClaimResourceCleanup(ctx context.Context, claim persistence.OperationClaim, req tfresource.CleanupRequest) (environment.Operation, bool, error) {
	if err := req.Validate(); err != nil {
		return environment.Operation{}, false, err
	}
	claim.Kind = environment.OpResourceCleanup
	scopeJSON, err := jsonBytes(req.Scope)
	if err != nil {
		return environment.Operation{}, false, err
	}
	snapshot := req.ScopeSnapshot
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	snapshotJSON, err := jsonBytes(snapshot)
	if err != nil {
		return environment.Operation{}, false, err
	}
	cols := &cleanupColumns{mode: string(req.Mode), actor: req.ActorRef, root: req.RootInstanceID, confirmedScope: scopeJSON, scopeSnapshot: snapshotJSON}
	if req.RequestHash != "" {
		cols.requestHash = req.RequestHash
	}
	if req.ConfirmationName != "" {
		cols.confirmationName = req.ConfirmationName
	}
	if req.RunID != "" {
		cols.runID = req.RunID
	}
	var out environment.Operation
	var replayed bool
	err = s.Transact(ctx, func(ctx context.Context) error {
		var envID string
		if err := s.q(ctx).QueryRow(ctx, `SELECT e.id::text FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2 FOR UPDATE OF e`, claim.ApplicationKey, claim.EnvironmentKey).Scan(&envID); err != nil {
			return translate(err)
		}
		if req.RequestHash != "" {
			var opID, priorEnv, priorRoot, priorMode string
			var stored []byte
			err := s.q(ctx).QueryRow(ctx, `SELECT id::text,environment_id::text,root_resource_instance_id::text,authorization_mode,confirmed_scope FROM environment_operations WHERE actor_ref=$1 AND deletion_request_hash=$2`, req.ActorRef, req.RequestHash).Scan(&opID, &priorEnv, &priorRoot, &priorMode, &stored)
			if err == nil {
				// A replay returns only the operation of the same Environment,
				// root, mode and selection; any other scope is refused, never
				// returned.
				var prior tfresource.ConfirmedScope
				if err := unmarshalJSON(stored, &prior); err != nil {
					return err
				}
				if priorEnv != envID || priorRoot != req.RootInstanceID || priorMode != string(req.Mode) || !tfresource.SameSelection(prior.SelectedResourceIDs, req.Scope.SelectedResourceIDs) {
					return terraformstate.ErrCleanupSelectionChanged
				}
				replayed = true
				out, err = s.GetOperation(ctx, opID)
				return err
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return translate(err)
			}
		}
		for _, item := range req.Scope.Items {
			if !validUUID(item.InstanceID) {
				return fmt.Errorf("%w: selected id", tfresource.ErrInvalid)
			}
			var version int64
			var stateKey, module string
			// No row lock (the Console login may not lock instance rows): the
			// State login changes an instance only under the Environment's
			// active claim, and the claim is released only after that write
			// committed. The Environment row locked above blocks any release
			// meanwhile, so a value read here cannot change before this claim
			// commits, and a still-active claim makes the claim below fail.
			err := s.q(ctx).QueryRow(ctx, `SELECT version,state_key,module_bundle_id::text FROM terraform_resource_instances WHERE id=$1::uuid AND environment_id=$2::uuid`, item.InstanceID, envID).Scan(&version, &stateKey, &module)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: selected instance is outside the Environment", tfresource.ErrInvalid)
			}
			if err != nil {
				return translate(err)
			}
			if version != item.Version || stateKey != item.StateKey || module != item.ModuleBundleID {
				return persistence.ErrVersionConflict
			}
		}
		var err error
		out, err = s.claimEnvironment(ctx, claim, cols)
		return err
	})
	return out, replayed, err
}

func (s *Store) CleanupRequest(ctx context.Context, operationID string) (tfresource.CleanupRequest, error) {
	var r tfresource.CleanupRequest
	if !validUUID(operationID) {
		return r, persistence.ErrNotFound
	}
	var scope, snapshot []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT authorization_mode,actor_ref,root_resource_instance_id::text,confirmed_scope,COALESCE(deletion_request_hash,''),COALESCE(confirmation_name,''),scope_snapshot,COALESCE(run_id,'') FROM environment_operations WHERE id=$1::uuid AND kind='RESOURCE_CLEANUP'`, operationID).
		Scan(&r.Mode, &r.ActorRef, &r.RootInstanceID, &scope, &r.RequestHash, &r.ConfirmationName, &snapshot, &r.RunID)
	if err != nil {
		return r, translate(err)
	}
	if err := unmarshalJSON(scope, &r.Scope); err != nil {
		return r, err
	}
	return r, unmarshalJSON(snapshot, &r.ScopeSnapshot)
}

var _ terraformstate.CleanupRepository = (*Store)(nil)
