package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

var _ terraformstate.BaselineRepository = (*StateStore)(nil)

func (s *StateStore) AppendExecutionBaseline(ctx context.Context, b terraformstate.ExecutionBaseline, a persistence.CipherArtifact) (out terraformstate.ExecutionBaseline, err error) {
	if !validUUID(b.ID) || !validUUID(b.InstanceID) || !validUUID(b.AttemptID) || !validUUID(a.ID) || b.ArtifactID != a.ID || a.OwnerID != b.InstanceID || a.OrganizationID != b.OrganizationID || a.Purpose != "TERRAFORM_BASELINE" || a.AADVersion != 1 || a.KeyID == "" || len(a.Nonce) != 12 || len(a.Ciphertext) < 16 {
		return out, persistence.ErrImmutable
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		if scope.organizationID != b.OrganizationID || scope.environmentID != b.EnvironmentID {
			return persistence.ErrNotFound
		}
		instance, err := s.GetInstance(ctx, b.InstanceID)
		if err != nil {
			return err
		}
		if instance.Identity.EnvironmentID != scope.environmentID || instance.Version != b.InstanceVersion || (instance.Status != tfresource.InstanceReady && instance.Status != tfresource.InstanceUnreferenced) || instance.ModuleBundleID != b.ModuleBundleID {
			return persistence.ErrVersionConflict
		}
		var valid bool
		if err := s.q(ctx).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terraform_resource_attempts WHERE id=$1 AND resource_instance_id=$2 AND environment_operation_id=$3 AND status='SUCCEEDED')`, b.AttemptID, b.InstanceID, scope.owner.OperationID).Scan(&valid); err != nil {
			return translate(err)
		}
		if !valid {
			return persistence.ErrImmutable
		}
		state, err := s.StateForUpdate(ctx, b.InstanceID)
		if err != nil {
			return err
		}
		// A module with no acknowledged state has no managed baseline to scan.
		if state == nil {
			return nil
		}
		b.StateVersion, b.Lineage, b.Serial = state.Version, state.Lineage, state.Serial
		rows, err := s.q(ctx).Query(ctx, `SELECT p.id::text,p.version,COALESCE(st.version,0) FROM terraform_resource_dependencies d JOIN terraform_resource_instances p ON p.id=d.provider_instance_id LEFT JOIN terraform_states st ON st.resource_instance_id=p.id WHERE d.consumer_instance_id=$1 ORDER BY p.id`, b.InstanceID)
		if err != nil {
			return translate(err)
		}
		b.Dependencies = []terraformstate.BaselineDependency{}
		for rows.Next() {
			var d terraformstate.BaselineDependency
			if err := rows.Scan(&d.InstanceID, &d.Version, &d.StateVersion); err != nil {
				rows.Close()
				return err
			}
			b.Dependencies = append(b.Dependencies, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		dependencies, err := json.Marshal(b.Dependencies)
		if err != nil {
			return persistence.ErrImmutable
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO reconciliation_private_artifacts(id,organization_id,owner_id,purpose,key_id,nonce,ciphertext,aad_version,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.ID, a.OrganizationID, a.OwnerID, a.Purpose, a.KeyID, a.Nonce, a.Ciphertext, a.AADVersion, a.CreatedAt)
		if err != nil {
			return translate(err)
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO terraform_execution_baselines(id,organization_id,environment_id,resource_instance_id,execution_attempt_id,module_bundle_id,instance_version,state_version,lineage,serial,private_artifact_ref,dependencies,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, b.ID, b.OrganizationID, b.EnvironmentID, b.InstanceID, b.AttemptID, b.ModuleBundleID, b.InstanceVersion, b.StateVersion, b.Lineage, b.Serial, b.ArtifactID, dependencies, b.CreatedAt)
		if err != nil {
			return translate(err)
		}
		out = b
		return nil
	})
	return
}

func scanExecutionBaseline(row pgx.Row) (b terraformstate.ExecutionBaseline, err error) {
	var dependencies []byte
	err = row.Scan(&b.ID, &b.OrganizationID, &b.EnvironmentID, &b.InstanceID, &b.AttemptID, &b.ModuleBundleID, &b.InstanceVersion, &b.StateVersion, &b.Lineage, &b.Serial, &b.ArtifactID, &dependencies, &b.CreatedAt)
	if err != nil {
		return b, translate(err)
	}
	if json.Unmarshal(dependencies, &b.Dependencies) != nil {
		return b, persistence.ErrImmutable
	}
	return
}
func (s *StateStore) LatestExecutionBaseline(ctx context.Context, org, instance string) (terraformstate.ExecutionBaseline, error) {
	if !validUUID(org) || !validUUID(instance) {
		return terraformstate.ExecutionBaseline{}, persistence.ErrNotFound
	}
	return scanExecutionBaseline(s.q(ctx).QueryRow(ctx, `SELECT id::text,organization_id::text,environment_id::text,resource_instance_id::text,execution_attempt_id::text,module_bundle_id::text,instance_version,state_version,lineage,serial,private_artifact_ref::text,dependencies,created_at FROM terraform_execution_baselines WHERE organization_id=$1 AND resource_instance_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, org, instance))
}
func (s *StateStore) ReadBaselineArtifact(ctx context.Context, org, id string) (a persistence.CipherArtifact, err error) {
	if !validUUID(org) || !validUUID(id) {
		return a, persistence.ErrNotFound
	}
	err = s.q(ctx).QueryRow(ctx, `SELECT a.id::text,a.organization_id::text,a.owner_id::text,a.purpose,a.key_id,a.nonce,a.ciphertext,a.aad_version,a.created_at FROM reconciliation_private_artifacts a JOIN terraform_execution_baselines b ON b.private_artifact_ref=a.id AND b.organization_id=a.organization_id AND b.resource_instance_id=a.owner_id WHERE a.organization_id=$1 AND a.id=$2 AND a.purpose='TERRAFORM_BASELINE'`, org, id).Scan(&a.ID, &a.OrganizationID, &a.OwnerID, &a.Purpose, &a.KeyID, &a.Nonce, &a.Ciphertext, &a.AADVersion, &a.CreatedAt)
	return a, translate(err)
}

func (s *StateStore) FindExecutionBaselineVersion(ctx context.Context, org, instance string, instanceVersion, stateVersion int64) (terraformstate.ExecutionBaseline, error) {
	if !validUUID(org) || !validUUID(instance) || instanceVersion < 1 || stateVersion < 1 {
		return terraformstate.ExecutionBaseline{}, persistence.ErrNotFound
	}
	return scanExecutionBaseline(s.q(ctx).QueryRow(ctx, `SELECT id::text,organization_id::text,environment_id::text,resource_instance_id::text,execution_attempt_id::text,module_bundle_id::text,instance_version,state_version,lineage,serial,private_artifact_ref::text,dependencies,created_at FROM terraform_execution_baselines WHERE organization_id=$1 AND resource_instance_id=$2 AND instance_version=$3 AND state_version=$4 ORDER BY created_at DESC,id DESC LIMIT 1`, org, instance, instanceVersion, stateVersion))
}
