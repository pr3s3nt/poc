package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// Insert-only registration (UC-02/03/04). Unlike the seed upserts, a second
// insert of the same (Organization, key) hits the unique constraint and
// returns persistence.ErrDuplicate without touching the stored record. The
// unique-violation mapping is local to these methods, so translate() and the
// optimistic-conflict semantics elsewhere are unchanged.

func registrationError(err error, what string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", persistence.ErrDuplicate, what)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s organization or referenced record", persistence.ErrNotFound, what)
	}
	return err
}

func (s *Store) CreateResourceType(ctx context.Context, org string, v resource.Type) error {
	if err := v.Validate(); err != nil {
		return err
	}
	inputs, err := jsonBytes(v.Inputs)
	if err != nil {
		return err
	}
	outputs, err := jsonBytes(v.Outputs)
	if err != nil {
		return err
	}
	var id string
	err = s.q(ctx).QueryRow(ctx, `INSERT INTO resource_types(id,organization_id,resource_type_key,input_schema,output_schema) SELECT $1::uuid,o.id,$3,$4,$5 FROM organizations o WHERE o.organization_key=$2 RETURNING id::text`, ids.New(), org, v.Key, inputs, outputs).Scan(&id)
	return registrationError(err, "resource type "+v.Key)
}

func (s *Store) CreateResourceDefinition(ctx context.Context, org string, v resource.Definition) error {
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
		err := s.q(ctx).QueryRow(ctx, `INSERT INTO resource_definitions(id,organization_id,definition_key,resource_type_id,execution_profile,driver_type,connection_id,driver_inputs,provision_rules,source_fingerprint) SELECT $1::uuid,o.id,$3,rt.id,NULLIF($4,''),$5,c.id,$7,$8,NULLIF($9,'') FROM organizations o JOIN resource_types rt ON rt.organization_id=o.id AND rt.resource_type_key=$6 LEFT JOIN connections c ON c.organization_id=o.id AND c.connection_key=NULLIF($10,'') WHERE o.organization_key=$2 AND ($10='' OR c.id IS NOT NULL) RETURNING id::text`, ids.New(), org, v.Key, v.ExecutionProfile, v.DriverType, v.ResourceTypeKey, driver, rules, v.SourceFingerpr, v.ConnectionKey).Scan(&id)
		if err != nil {
			return registrationError(err, "resource definition "+v.Key)
		}
		for i, c := range v.Criteria {
			if _, err := s.q(ctx).Exec(ctx, `INSERT INTO matching_criteria(id,resource_definition_id,ordinal,env_type,app_id,env_id,res_id,class,specificity_score) VALUES($1::uuid,$2::uuid,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9)`, ids.New(), id, i, c.EnvironmentType, c.ApplicationID, c.EnvironmentID, c.ResourceID, c.Class, c.Specificity()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) CreateConnection(ctx context.Context, v application.Connection) error {
	if v.ID == "" {
		v.ID = ids.New()
	}
	v, err := v.WithLegacyDefaults()
	if err != nil {
		return err
	}
	config, err := jsonBytes(v.Config)
	if err != nil {
		return err
	}
	verification, err := jsonBytes(v.Verification)
	if err != nil {
		return err
	}
	// Insert-only registration never changes the Organization default
	// connection (UC-04 BR-16); only the seed SaveConnection links it.
	var id string
	err = s.q(ctx).QueryRow(ctx, `INSERT INTO connections(id,organization_id,connection_key,kind,config,secret_ref,status,verification,name,authentication_type) SELECT $1::uuid,o.id,$3,$4,$5,$6,$7,$8,$9,$10 FROM organizations o WHERE o.organization_key=$2 RETURNING id::text`, v.ID, v.OrganizationKey, v.Key, v.Kind, config, v.SecretRef, v.Status, verification, v.Name, v.AuthenticationType).Scan(&id)
	if err != nil {
		return registrationError(err, "connection "+v.Key)
	}
	return nil
}
