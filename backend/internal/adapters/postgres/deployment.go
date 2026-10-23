package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) SaveDeployment(ctx context.Context, v deployment.Deployment) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSaveDeployment(ctx, v) })
}

func (s *Store) fencedSaveDeployment(ctx context.Context, v deployment.Deployment) error {
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO deployments(id,environment_id,organization_key,application_key,environment_key,action,workload_id,actor_ref,status,base_environment_version,base_deployment_set_id,candidate_deployment_set_id,failure_reason,started_at,finished_at,target_generation,review_request_id) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid,NULLIF($12,'')::uuid,$13,$14,$15,$16,NULLIF($17,'')::uuid) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status,candidate_deployment_set_id=EXCLUDED.candidate_deployment_set_id,failure_reason=EXCLUDED.failure_reason,finished_at=EXCLUDED.finished_at WHERE deployments.review_request_id IS NOT DISTINCT FROM EXCLUDED.review_request_id`, v.ID, v.EnvironmentID, v.OrganizationKey, v.ApplicationKey, v.EnvironmentKey, v.Action, v.WorkloadID, v.ActorRef, v.Status, v.BaseEnvironmentVersion, v.BaseDeploymentSetID, v.CandidateDeploymentSet, v.FailureReason, v.StartedAt, v.FinishedAt, v.TargetGeneration, v.ReviewRequestID)
	if err != nil {
		return fmt.Errorf("postgres: save deployment: %w", translate(err))
	}
	if tag.RowsAffected() == 0 {
		return persistence.ErrImmutable
	}
	return nil
}

const deploymentSelect = `SELECT d.id::text,d.environment_id::text,d.organization_key,d.application_key,d.environment_key,d.action,d.workload_id,d.actor_ref,d.status,d.base_environment_version,COALESCE(d.base_deployment_set_id::text,''),COALESCE(d.candidate_deployment_set_id::text,''),COALESCE(s.id::text,''),d.failure_reason,d.started_at,d.finished_at,d.target_generation,COALESCE(d.review_request_id::text,'') FROM deployments d LEFT JOIN deployment_delta_snapshots s ON s.deployment_id=d.id`

func scanDeployment(row pgx.Row) (deployment.Deployment, error) {
	var v deployment.Deployment
	err := row.Scan(&v.ID, &v.EnvironmentID, &v.OrganizationKey, &v.ApplicationKey, &v.EnvironmentKey, &v.Action, &v.WorkloadID, &v.ActorRef, &v.Status, &v.BaseEnvironmentVersion, &v.BaseDeploymentSetID, &v.CandidateDeploymentSet, &v.DeltaSnapshotID, &v.FailureReason, &v.StartedAt, &v.FinishedAt, &v.TargetGeneration, &v.ReviewRequestID)
	return v, err
}
func (s *Store) GetDeployment(ctx context.Context, id string) (deployment.Deployment, error) {
	// Deployment IDs arrive from request paths. A value that is not a UUID
	// cannot identify a Deployment, so it is missing rather than a SQL error.
	var parsed pgtype.UUID
	if err := parsed.Scan(id); err != nil {
		return deployment.Deployment{}, fmt.Errorf("%w: deployment %q", persistence.ErrNotFound, id)
	}
	v, err := scanDeployment(s.q(ctx).QueryRow(ctx, deploymentSelect+` WHERE d.id=$1::uuid`, id))
	if err != nil {
		return v, fmt.Errorf("postgres: get deployment: %w", translate(err))
	}
	return v, nil
}
func (s *Store) ListDeployments(ctx context.Context, app, env string) ([]deployment.Deployment, error) {
	rows, err := s.q(ctx).Query(ctx, deploymentSelect+` WHERE ($1='' OR d.application_key=$1) AND ($2='' OR d.environment_key=$2) ORDER BY d.started_at DESC,d.id`, app, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deployment.Deployment
	for rows.Next() {
		var v deployment.Deployment
		if err = rows.Scan(&v.ID, &v.EnvironmentID, &v.OrganizationKey, &v.ApplicationKey, &v.EnvironmentKey, &v.Action, &v.WorkloadID, &v.ActorRef, &v.Status, &v.BaseEnvironmentVersion, &v.BaseDeploymentSetID, &v.CandidateDeploymentSet, &v.DeltaSnapshotID, &v.FailureReason, &v.StartedAt, &v.FinishedAt, &v.TargetGeneration, &v.ReviewRequestID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveDeltaSnapshot(ctx context.Context, v deployment.DeploymentDeltaSnapshot) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSaveDeltaSnapshot(ctx, v) })
}

func (s *Store) fencedSaveDeltaSnapshot(ctx context.Context, v deployment.DeploymentDeltaSnapshot) error {
	if err := v.Validate(); err != nil {
		return err
	}
	doc, err := jsonBytes(v.Document)
	if err != nil {
		return err
	}
	meta, err := jsonBytes(v.Metadata)
	if err != nil {
		return err
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO deployment_delta_snapshots(id,deployment_id,document,document_hash,metadata,created_at) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6)`, v.ID, v.DeploymentID, doc, v.DocumentHash, meta, v.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: save delta snapshot: %w", translate(err))
	}
	return nil
}
func (s *Store) GetDeltaSnapshot(ctx context.Context, id string) (deployment.DeploymentDeltaSnapshot, error) {
	var v deployment.DeploymentDeltaSnapshot
	var doc, meta []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT id::text,deployment_id::text,document,document_hash,metadata,created_at FROM deployment_delta_snapshots WHERE id=$1::uuid`, id).Scan(&v.ID, &v.DeploymentID, &doc, &v.DocumentHash, &meta, &v.CreatedAt)
	if err != nil {
		return v, translate(err)
	}
	if err = unmarshalJSON(doc, &v.Document); err != nil {
		return v, err
	}
	err = unmarshalJSON(meta, &v.Metadata)
	return v, err
}
func (s *Store) SavePlan(ctx context.Context, deploymentID string, plan map[string]any) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSavePlan(ctx, deploymentID, plan) })
}

func (s *Store) fencedSavePlan(ctx context.Context, deploymentID string, plan map[string]any) error {
	doc, err := jsonBytes(plan)
	if err != nil {
		return err
	}
	hash, _ := plan["planHash"].(string)
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO deployment_plans(deployment_id,document,plan_hash) VALUES($1::uuid,$2,$3) ON CONFLICT(deployment_id) DO UPDATE SET document=EXCLUDED.document,plan_hash=EXCLUDED.plan_hash WHERE EXISTS(SELECT 1 FROM deployments d WHERE d.id=$1::uuid AND d.status='PLANNING')`, deploymentID, doc, hash)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return persistence.ErrImmutable
	}
	return nil
}
func (s *Store) GetPlan(ctx context.Context, id string) (map[string]any, error) {
	var b []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT document FROM deployment_plans WHERE deployment_id=$1::uuid`, id).Scan(&b)
	if err != nil {
		return nil, translate(err)
	}
	var v map[string]any
	err = unmarshalJSON(b, &v)
	return v, err
}
func (s *Store) SaveDeploymentResource(ctx context.Context, v deployment.Resource) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSaveDeploymentResource(ctx, v) })
}

func (s *Store) fencedSaveDeploymentResource(ctx context.Context, v deployment.Resource) error {
	a, b := []byte("{}"), []byte("{}")
	var err error
	if v.ResolvedInputs != nil {
		a, err = jsonBytes(v.ResolvedInputs)
		if err != nil {
			return err
		}
	}
	if v.OutputSnapshot != nil {
		b, err = jsonBytes(v.OutputSnapshot)
		if err != nil {
			return err
		}
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO deployment_resources(deployment_id,node_descriptor,active_resource_id,definition_key,resource_type_key,status,resolved_inputs,output_snapshot,batch_index,started_at,finished_at) VALUES($1::uuid,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(deployment_id,node_descriptor) DO UPDATE SET active_resource_id=EXCLUDED.active_resource_id,status=EXCLUDED.status,resolved_inputs=EXCLUDED.resolved_inputs,output_snapshot=EXCLUDED.output_snapshot,started_at=EXCLUDED.started_at,finished_at=EXCLUDED.finished_at`, v.DeploymentID, v.NodeDescriptor, v.ActiveResourceID, v.DefinitionKey, v.ResourceTypeKey, v.Status, a, b, v.BatchIndex, v.StartedAt, v.FinishedAt)
	return translate(err)
}
func (s *Store) ListDeploymentResources(ctx context.Context, id string) ([]deployment.Resource, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT deployment_id::text,node_descriptor,COALESCE(active_resource_id::text,''),COALESCE(definition_key,''),COALESCE(resource_type_key,''),status,resolved_inputs,output_snapshot,batch_index,started_at,finished_at FROM deployment_resources WHERE deployment_id=$1::uuid ORDER BY batch_index,node_descriptor`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deployment.Resource
	for rows.Next() {
		var v deployment.Resource
		var a, b []byte
		if err = rows.Scan(&v.DeploymentID, &v.NodeDescriptor, &v.ActiveResourceID, &v.DefinitionKey, &v.ResourceTypeKey, &v.Status, &a, &b, &v.BatchIndex, &v.StartedAt, &v.FinishedAt); err != nil {
			return nil, err
		}
		_ = unmarshalJSON(a, &v.ResolvedInputs)
		_ = unmarshalJSON(b, &v.OutputSnapshot)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListDeploymentAttempts reads the safe projection of the Terraform attempts
// linked to a Deployment. The Console login has SELECT only on the safe
// attempt/instance columns, so this query cannot reach snapshots, recovery
// handles, target references or state even by mistake. The instance must
// belong to the Deployment's own Environment.
func (s *Store) ListDeploymentAttempts(ctx context.Context, id string) ([]deployment.ResourceAttempt, error) {
	if !validUUID(id) {
		return []deployment.ResourceAttempt{}, nil
	}
	rows, err := s.q(ctx).Query(ctx, `SELECT i.id::text,i.descriptor,i.scope_type,a.attempt_number,a.action,a.ownership,a.stage,a.status,a.safe_error_code,b.module_id,b.revision,a.created_at,a.updated_at
FROM terraform_resource_attempts a
JOIN deployments d ON d.id=a.deployment_id
JOIN terraform_resource_instances i ON i.id=a.resource_instance_id AND i.environment_id=d.environment_id
JOIN terraform_module_bundles b ON b.id=i.module_bundle_id
WHERE a.deployment_id=$1::uuid ORDER BY a.created_at,i.descriptor,a.attempt_number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []deployment.ResourceAttempt{}
	for rows.Next() {
		var v deployment.ResourceAttempt
		if err := rows.Scan(&v.ResourceID, &v.Descriptor, &v.ScopeType, &v.Number, &v.Action, &v.Ownership, &v.Stage, &v.Status, &v.SafeErrorCode, &v.ModuleID, &v.ModuleRevision, &v.StartedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpsertWorkloadInstance(ctx context.Context, v deployment.WorkloadInstance) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedUpsertWorkloadInstance(ctx, v) })
}

func (s *Store) fencedUpsertWorkloadInstance(ctx context.Context, v deployment.WorkloadInstance) error {
	if v.ID == "" {
		v.ID = ids.New()
	}
	target, err := jsonBytes(v.TargetRef)
	if err != nil {
		return err
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO workload_instances(id,environment_id,workload_id,last_deployment_id,applied_config_revision_id,target_ref,manifest_digest,status,observed_at,generation) SELECT $1::uuid,e.id,$3,$4::uuid,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10 FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key||'/'||e.environment_key=$2 ON CONFLICT(environment_id,generation,workload_id) DO UPDATE SET last_deployment_id=EXCLUDED.last_deployment_id,applied_config_revision_id=EXCLUDED.applied_config_revision_id,target_ref=EXCLUDED.target_ref,manifest_digest=EXCLUDED.manifest_digest,status=EXCLUDED.status,observed_at=EXCLUDED.observed_at`, v.ID, v.EnvironmentKey, v.WorkloadID, v.LastDeploymentID, v.AppliedConfigRevisionID, target, v.ManifestDigest, v.Status, v.ObservedAt, v.Generation)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: environment %q", persistence.ErrNotFound, v.EnvironmentKey)
	}
	return nil
}

// UpsertWorkloadProgress records mutable current state and the owning
// Deployment snapshot in one transaction. The owning Deployment row is locked
// FOR UPDATE first, which serializes this write with the terminal status
// transition: once SUCCEEDED/FAILED commits, no later progress can land.
func (s *Store) UpsertWorkloadProgress(ctx context.Context, v deployment.WorkloadInstance) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedUpsertWorkloadProgress(ctx, v) })
}

func (s *Store) fencedUpsertWorkloadProgress(ctx context.Context, v deployment.WorkloadInstance) error {
	return s.Transact(ctx, func(ctx context.Context) error {
		var status, environmentKey string
		err := s.q(ctx).QueryRow(ctx, `SELECT d.status,a.application_key||'/'||e.environment_key FROM deployments d JOIN environments e ON e.id=d.environment_id JOIN applications a ON a.id=e.application_id WHERE d.id=$1::uuid FOR UPDATE OF d`, v.LastDeploymentID).Scan(&status, &environmentKey)
		if err != nil {
			return fmt.Errorf("postgres: lock deployment %q: %w", v.LastDeploymentID, translate(err))
		}
		if environmentKey != v.EnvironmentKey {
			return fmt.Errorf("%w: deployment %q in environment %q", persistence.ErrNotFound, v.LastDeploymentID, v.EnvironmentKey)
		}
		if deployment.Status(status) == deployment.StatusSucceeded || deployment.Status(status) == deployment.StatusFailed {
			return fmt.Errorf("%w: workload snapshot for terminal deployment %q", persistence.ErrImmutable, v.LastDeploymentID)
		}
		if err := s.UpsertWorkloadInstance(ctx, v); err != nil {
			return err
		}
		target, err := jsonBytes(v.TargetRef)
		if err != nil {
			return err
		}
		var review any
		if v.ManifestReview != nil {
			review, err = jsonBytes(v.ManifestReview)
			if err != nil {
				return err
			}
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO deployment_workloads(deployment_id,workload_id,status,target_ref,manifest_digest,applied_config_revision_id,observed_at,manifest_review) VALUES($1::uuid,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8) ON CONFLICT(deployment_id,workload_id) DO UPDATE SET status=EXCLUDED.status,target_ref=EXCLUDED.target_ref,manifest_digest=EXCLUDED.manifest_digest,applied_config_revision_id=EXCLUDED.applied_config_revision_id,observed_at=EXCLUDED.observed_at,manifest_review=COALESCE(deployment_workloads.manifest_review,EXCLUDED.manifest_review)`, v.LastDeploymentID, v.WorkloadID, v.Status, target, v.ManifestDigest, v.AppliedConfigRevisionID, v.ObservedAt, review)
		return translate(err)
	})
}

func (s *Store) ListWorkloadInstances(ctx context.Context, envKey string) ([]deployment.WorkloadInstance, error) {
	return s.listInstances(ctx, envKey, -1)
}

// ListWorkloadInstancesFor returns the instances of one target generation.
func (s *Store) ListWorkloadInstancesFor(ctx context.Context, envKey string, generation int64) ([]deployment.WorkloadInstance, error) {
	return s.listInstances(ctx, envKey, generation)
}

// listInstances reads the current generation when generation is negative.
func (s *Store) listInstances(ctx context.Context, envKey string, generation int64) ([]deployment.WorkloadInstance, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT w.id::text,a.application_key||'/'||e.environment_key,w.workload_id,w.last_deployment_id::text,COALESCE(w.applied_config_revision_id::text,''),w.target_ref,w.manifest_digest,w.status,w.observed_at,w.generation FROM workload_instances w JOIN environments e ON e.id=w.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key||'/'||e.environment_key=$1 AND w.generation=CASE WHEN $2<0 THEN e.target_generation ELSE $2 END ORDER BY w.workload_id`, envKey, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deployment.WorkloadInstance
	for rows.Next() {
		var v deployment.WorkloadInstance
		var b []byte
		if err = rows.Scan(&v.ID, &v.EnvironmentKey, &v.WorkloadID, &v.LastDeploymentID, &v.AppliedConfigRevisionID, &b, &v.ManifestDigest, &v.Status, &v.ObservedAt, &v.Generation); err != nil {
			return nil, err
		}
		_ = unmarshalJSON(b, &v.TargetRef)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListDeploymentWorkloads(ctx context.Context, deploymentID string) ([]deployment.WorkloadSnapshot, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT deployment_id::text,workload_id,COALESCE(applied_config_revision_id::text,''),target_ref,manifest_digest,status,observed_at,manifest_review FROM deployment_workloads WHERE deployment_id=$1::uuid ORDER BY workload_id`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deployment.WorkloadSnapshot
	for rows.Next() {
		var v deployment.WorkloadSnapshot
		var target, review []byte
		if err = rows.Scan(&v.DeploymentID, &v.WorkloadID, &v.AppliedConfigRevisionID, &target, &v.ManifestDigest, &v.Status, &v.ObservedAt, &review); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(target, &v.TargetRef); err != nil {
			return nil, err
		}
		if len(review) > 0 {
			if err = unmarshalJSON(review, &v.ManifestReview); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ = sort.Slice
