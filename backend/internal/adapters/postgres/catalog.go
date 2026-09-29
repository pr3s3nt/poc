package postgres

import (
	"context"
	"fmt"
	"time"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
)

func (s *Store) SaveResourceType(ctx context.Context, org string, v resource.Type) error {
	if err := v.Validate(); err != nil {
		return err
	}
	a, err := jsonBytes(v.Inputs)
	if err != nil {
		return err
	}
	b, err := jsonBytes(v.Outputs)
	if err != nil {
		return err
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO resource_types(id,organization_id,resource_type_key,input_schema,output_schema) SELECT $1::uuid,o.id,$3,$4,$5 FROM organizations o WHERE o.organization_key=$2 ON CONFLICT(organization_id,resource_type_key) DO UPDATE SET input_schema=EXCLUDED.input_schema,output_schema=EXCLUDED.output_schema`, ids.New(), org, v.Key, a, b)
	return translate(err)
}
func (s *Store) ListResourceTypes(ctx context.Context, org string) ([]resource.Type, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT rt.resource_type_key,rt.input_schema,rt.output_schema FROM resource_types rt JOIN organizations o ON o.id=rt.organization_id WHERE o.organization_key=$1 ORDER BY rt.resource_type_key`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resource.Type
	for rows.Next() {
		var v resource.Type
		var a, b []byte
		if err = rows.Scan(&v.Key, &a, &b); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(a, &v.Inputs); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(b, &v.Outputs); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SaveResourceDefinition(ctx context.Context, org string, v resource.Definition) error {
	if err := v.Validate(); err != nil {
		return err
	}
	driver, err := jsonBytes(v.DriverInputs)
	if err != nil {
		return err
	}
	rules, err := jsonBytes(v.Provision)
	if err != nil {
		return err
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		var id string
		err := s.q(ctx).QueryRow(ctx, `INSERT INTO resource_definitions(id,organization_id,definition_key,resource_type_id,execution_profile,driver_type,connection_id,driver_inputs,provision_rules,source_fingerprint) SELECT $1::uuid,o.id,$3,rt.id,NULLIF($4,''),$5,c.id,$7,$8,NULLIF($9,'') FROM organizations o JOIN resource_types rt ON rt.organization_id=o.id AND rt.resource_type_key=$6 LEFT JOIN connections c ON c.organization_id=o.id AND c.connection_key=NULLIF($10,'') WHERE o.organization_key=$2 ON CONFLICT(organization_id,definition_key) DO UPDATE SET resource_type_id=EXCLUDED.resource_type_id,execution_profile=EXCLUDED.execution_profile,driver_type=EXCLUDED.driver_type,connection_id=EXCLUDED.connection_id,driver_inputs=EXCLUDED.driver_inputs,provision_rules=EXCLUDED.provision_rules,source_fingerprint=EXCLUDED.source_fingerprint RETURNING id::text`, ids.New(), org, v.Key, v.ExecutionProfile, v.DriverType, v.ResourceTypeKey, driver, rules, v.SourceFingerpr, v.ConnectionKey).Scan(&id)
		if err != nil {
			return translate(err)
		}
		if _, err = s.q(ctx).Exec(ctx, `DELETE FROM matching_criteria WHERE resource_definition_id=$1::uuid`, id); err != nil {
			return err
		}
		for i, c := range v.Criteria {
			_, err = s.q(ctx).Exec(ctx, `INSERT INTO matching_criteria(id,resource_definition_id,ordinal,env_type,app_id,env_id,res_id,class,specificity_score) VALUES($1::uuid,$2::uuid,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9)`, ids.New(), id, i, c.EnvironmentType, c.ApplicationID, c.EnvironmentID, c.ResourceID, c.Class, c.Specificity())
			if err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) ListResourceDefinitions(ctx context.Context, org string) ([]resource.Definition, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT rd.id::text,rd.definition_key,rt.resource_type_key,COALESCE(rd.execution_profile,''),rd.driver_type,COALESCE(c.connection_key,''),rd.driver_inputs,rd.provision_rules,COALESCE(rd.source_fingerprint,'') FROM resource_definitions rd JOIN organizations o ON o.id=rd.organization_id JOIN resource_types rt ON rt.id=rd.resource_type_id LEFT JOIN connections c ON c.id=rd.connection_id WHERE o.organization_key=$1 ORDER BY rd.definition_key`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		id string
		v  resource.Definition
	}
	var items []item
	for rows.Next() {
		var x item
		var a, b []byte
		if err = rows.Scan(&x.id, &x.v.Key, &x.v.ResourceTypeKey, &x.v.ExecutionProfile, &x.v.DriverType, &x.v.ConnectionKey, &a, &b, &x.v.SourceFingerpr); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(a, &x.v.DriverInputs); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(b, &x.v.Provision); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]resource.Definition, 0, len(items))
	for _, x := range items {
		cr, err := s.q(ctx).Query(ctx, `SELECT COALESCE(env_type,''),COALESCE(app_id,''),COALESCE(env_id,''),COALESCE(res_id,''),COALESCE(class,'') FROM matching_criteria WHERE resource_definition_id=$1::uuid ORDER BY ordinal`, x.id)
		if err != nil {
			return nil, err
		}
		for cr.Next() {
			var c resource.Criterion
			if err = cr.Scan(&c.EnvironmentType, &c.ApplicationID, &c.EnvironmentID, &c.ResourceID, &c.Class); err != nil {
				cr.Close()
				return nil, err
			}
			x.v.Criteria = append(x.v.Criteria, c)
		}
		cr.Close()
		out = append(out, x.v)
	}
	return out, nil
}

func (s *Store) UpsertActiveResource(ctx context.Context, v resource.ActiveResource) (resource.ActiveResource, error) {
	if v.ID == "" {
		v.ID = ids.New()
	}
	now := time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	a, err := jsonBytes(v.ExecutorState)
	if err != nil {
		return v, err
	}
	b, err := jsonBytes(v.Outputs)
	if err != nil {
		return v, err
	}
	err = s.q(ctx).QueryRow(ctx, `INSERT INTO active_resources(id,organization_id,descriptor,scope_type,scope_id,resource_definition_id,connection_id,status,executor_state_ref,outputs,input_fingerprint,last_deployment_id,version,created_at,updated_at) SELECT $1::uuid,o.id,$3,$4,$5,rd.id,c.id,$8,$9,$10,NULLIF($11,''),NULLIF($12,'')::uuid,1,$13,$14 FROM organizations o JOIN resource_definitions rd ON rd.organization_id=o.id AND rd.definition_key=$6 LEFT JOIN connections c ON c.organization_id=o.id AND c.connection_key=NULLIF($7,'') WHERE o.organization_key=$2 ON CONFLICT(organization_id,descriptor,scope_type,scope_id) DO UPDATE SET status=EXCLUDED.status,executor_state_ref=EXCLUDED.executor_state_ref,outputs=EXCLUDED.outputs,input_fingerprint=EXCLUDED.input_fingerprint,last_deployment_id=EXCLUDED.last_deployment_id,version=active_resources.version+1,updated_at=EXCLUDED.updated_at RETURNING id::text,version,created_at,updated_at`, v.ID, v.OrganizationKey, v.Descriptor.String(), v.Scope.Type, v.Scope.ID, v.DefinitionKey, v.ConnectionKey, v.Status, a, b, v.InputFingerprint, v.LastDeploymentID, v.CreatedAt, v.UpdatedAt).Scan(&v.ID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, fmt.Errorf("postgres: upsert active resource: %w", translate(err))
	}
	return v, nil
}
func scanActive(values interface{ Scan(...any) error }) (resource.ActiveResource, error) {
	var v resource.ActiveResource
	var descriptor string
	var a, b []byte
	err := values.Scan(&v.ID, &v.OrganizationKey, &descriptor, &v.Scope.Type, &v.Scope.ID, &v.DefinitionKey, &v.ConnectionKey, &v.Status, &a, &b, &v.InputFingerprint, &v.LastDeploymentID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	v.Descriptor, err = resource.ParseDescriptor(descriptor)
	if err != nil {
		return v, err
	}
	_ = unmarshalJSON(a, &v.ExecutorState)
	_ = unmarshalJSON(b, &v.Outputs)
	return v, nil
}

const activeSelect = `SELECT ar.id::text,o.organization_key,ar.descriptor,ar.scope_type,ar.scope_id,rd.definition_key,COALESCE(c.connection_key,''),ar.status,ar.executor_state_ref,ar.outputs,COALESCE(ar.input_fingerprint,''),COALESCE(ar.last_deployment_id::text,''),ar.version,ar.created_at,ar.updated_at FROM active_resources ar JOIN organizations o ON o.id=ar.organization_id JOIN resource_definitions rd ON rd.id=ar.resource_definition_id LEFT JOIN connections c ON c.id=ar.connection_id`

func (s *Store) FindByLogicalIdentity(ctx context.Context, org string, d resource.Descriptor, scope resource.Scope) (resource.ActiveResource, error) {
	v, err := scanActive(s.q(ctx).QueryRow(ctx, activeSelect+` WHERE o.organization_key=$1 AND ar.descriptor=$2 AND ar.scope_type=$3 AND ar.scope_id=$4`, org, d.String(), scope.Type, scope.ID))
	if err != nil {
		return v, translate(err)
	}
	return v, nil
}
func (s *Store) ListActiveResources(ctx context.Context, org string) ([]resource.ActiveResource, error) {
	rows, err := s.q(ctx).Query(ctx, activeSelect+` WHERE o.organization_key=$1 ORDER BY ar.descriptor,ar.scope_type,ar.scope_id`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resource.ActiveResource
	for rows.Next() {
		v, e := scanActive(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
