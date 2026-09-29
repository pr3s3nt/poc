package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) SaveDeployment(ctx context.Context, v deployment.Deployment) error {
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO deployments(id,environment_id,organization_key,application_key,environment_key,execution_profile,action,workload_id,actor_ref,status,base_environment_version,base_deployment_set_id,candidate_deployment_set_id,failure_reason,started_at,finished_at) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,NULLIF($13,'')::uuid,$14,$15,$16) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status,candidate_deployment_set_id=EXCLUDED.candidate_deployment_set_id,failure_reason=EXCLUDED.failure_reason,finished_at=EXCLUDED.finished_at`, v.ID, v.EnvironmentID, v.OrganizationKey, v.ApplicationKey, v.EnvironmentKey, v.ExecutionProfile, v.Action, v.WorkloadID, v.ActorRef, v.Status, v.BaseEnvironmentVersion, v.BaseDeploymentSetID, v.CandidateDeploymentSet, v.FailureReason, v.StartedAt, v.FinishedAt)
	if err != nil {
		return fmt.Errorf("postgres: save deployment: %w", translate(err))
	}
	return nil
}

const deploymentSelect = `SELECT d.id::text,d.environment_id::text,d.organization_key,d.application_key,d.environment_key,d.execution_profile,d.action,d.workload_id,d.actor_ref,d.status,d.base_environment_version,COALESCE(d.base_deployment_set_id::text,''),COALESCE(d.candidate_deployment_set_id::text,''),COALESCE(s.id::text,''),d.failure_reason,d.started_at,d.finished_at FROM deployments d LEFT JOIN deployment_delta_snapshots s ON s.deployment_id=d.id`

func scanDeployment(row pgx.Row) (deployment.Deployment, error) {
	var v deployment.Deployment
	err := row.Scan(&v.ID, &v.EnvironmentID, &v.OrganizationKey, &v.ApplicationKey, &v.EnvironmentKey, &v.ExecutionProfile, &v.Action, &v.WorkloadID, &v.ActorRef, &v.Status, &v.BaseEnvironmentVersion, &v.BaseDeploymentSetID, &v.CandidateDeploymentSet, &v.DeltaSnapshotID, &v.FailureReason, &v.StartedAt, &v.FinishedAt)
	return v, err
}
func (s *Store) GetDeployment(ctx context.Context, id string) (deployment.Deployment, error) {
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
		if err = rows.Scan(&v.ID, &v.EnvironmentID, &v.OrganizationKey, &v.ApplicationKey, &v.EnvironmentKey, &v.ExecutionProfile, &v.Action, &v.WorkloadID, &v.ActorRef, &v.Status, &v.BaseEnvironmentVersion, &v.BaseDeploymentSetID, &v.CandidateDeploymentSet, &v.DeltaSnapshotID, &v.FailureReason, &v.StartedAt, &v.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveDeltaSnapshot(ctx context.Context, v deployment.DeploymentDeltaSnapshot) error {
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

func (s *Store) UpsertWorkloadInstance(ctx context.Context, v deployment.WorkloadInstance) error {
	if v.ID == "" {
		v.ID = ids.New()
	}
	target, err := jsonBytes(v.TargetRef)
	if err != nil {
		return err
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO workload_instances(id,environment_id,workload_id,last_deployment_id,applied_config_revision_id,target_ref,manifest_digest,status,observed_at) SELECT $1::uuid,e.id,$3,$4::uuid,NULLIF($5,'')::uuid,$6,$7,$8,$9 FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key||'/'||e.environment_key=$2 ON CONFLICT(environment_id,workload_id) DO UPDATE SET last_deployment_id=EXCLUDED.last_deployment_id,applied_config_revision_id=EXCLUDED.applied_config_revision_id,target_ref=EXCLUDED.target_ref,manifest_digest=EXCLUDED.manifest_digest,status=EXCLUDED.status,observed_at=EXCLUDED.observed_at`, v.ID, v.EnvironmentKey, v.WorkloadID, v.LastDeploymentID, v.AppliedConfigRevisionID, target, v.ManifestDigest, v.Status, v.ObservedAt)
	return translate(err)
}
func (s *Store) ListWorkloadInstances(ctx context.Context, envKey string) ([]deployment.WorkloadInstance, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT w.id::text,a.application_key||'/'||e.environment_key,w.workload_id,w.last_deployment_id::text,COALESCE(w.applied_config_revision_id::text,''),w.target_ref,w.manifest_digest,w.status,w.observed_at FROM workload_instances w JOIN environments e ON e.id=w.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key||'/'||e.environment_key=$1 ORDER BY w.workload_id`, envKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deployment.WorkloadInstance
	for rows.Next() {
		var v deployment.WorkloadInstance
		var b []byte
		if err = rows.Scan(&v.ID, &v.EnvironmentKey, &v.WorkloadID, &v.LastDeploymentID, &v.AppliedConfigRevisionID, &b, &v.ManifestDigest, &v.Status, &v.ObservedAt); err != nil {
			return nil, err
		}
		_ = unmarshalJSON(b, &v.TargetRef)
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ = sort.Slice
