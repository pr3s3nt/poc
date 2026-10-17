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
		err := s.q(ctx).QueryRow(ctx, `INSERT INTO resource_definitions(id,organization_id,definition_key,resource_type_id,definition_name,driver_type,connection_id,driver_inputs,provision_rules,source_fingerprint,version) SELECT $1::uuid,o.id,$3,rt.id,$4,$5,c.id,$7,$8,NULLIF($9,''),$11 FROM organizations o JOIN resource_types rt ON rt.organization_id=o.id AND rt.resource_type_key=$6 LEFT JOIN connections c ON c.organization_id=o.id AND c.connection_key=NULLIF($10,'') WHERE o.organization_key=$2 AND ($10='' OR c.id IS NOT NULL) RETURNING id::text`, ids.New(), org, v.Key, v.Name, v.DriverType, v.ResourceTypeKey, driver, rules, v.SourceFingerpr, v.DriverAccount, v.Version).Scan(&id)
		if err != nil {
			return registrationError(err, "resource definition "+v.Key)
		}
		return s.insertCriteria(ctx, id, v.Criteria)
	})
}

func (s *Store) insertCriteria(ctx context.Context, definitionID string, criteria []resource.Criterion) error {
	for i, c := range criteria {
		if _, err := s.q(ctx).Exec(ctx, `INSERT INTO matching_criteria(id,resource_definition_id,ordinal,env_type,app_id,env_id,res_id,class,specificity_score) VALUES($1::uuid,$2::uuid,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9)`, ids.New(), definitionID, i, c.EnvironmentType, c.ApplicationID, c.EnvironmentID, c.ResourceID, c.Class, c.Specificity()); err != nil {
			return err
		}
	}
	return nil
}

// UpdateResourceDefinition is the UC-03 V02-06 compare-and-set. One UPDATE
// guarded by the stored version replaces the editable Definition columns and
// bumps the version; the criteria are replaced in the same transaction, so a
// failure on any part rolls the whole edit back and a lost race leaves the
// winner untouched. Key and Resource Type are immutable by construction: the
// UPDATE never sets them and a different Resource Type matches no row.
func (s *Store) UpdateResourceDefinition(ctx context.Context, org string, expected int64, v resource.Definition) (resource.Definition, error) {
	next := v
	next.Version = expected + 1
	if err := next.Validate(); err != nil {
		return resource.Definition{}, err
	}
	driver, err := jsonBytes(v.DriverInputs)
	if err != nil {
		return resource.Definition{}, err
	}
	rules, err := jsonBytes(v.Provision)
	if err != nil {
		return resource.Definition{}, err
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		var id string
		err := s.q(ctx).QueryRow(ctx, `UPDATE resource_definitions rd SET definition_name=$4,driver_type=$5,connection_id=(SELECT c.id FROM connections c WHERE c.organization_id=rd.organization_id AND c.connection_key=NULLIF($10,'')),driver_inputs=$6,provision_rules=$7,source_fingerprint=NULLIF($8,''),version=rd.version+1 FROM organizations o, resource_types rt WHERE o.id=rd.organization_id AND rt.id=rd.resource_type_id AND o.organization_key=$1 AND rd.definition_key=$2 AND rd.version=$3 AND rt.resource_type_key=$9 AND ($10='' OR EXISTS(SELECT 1 FROM connections c WHERE c.organization_id=rd.organization_id AND c.connection_key=$10)) RETURNING rd.id::text`, org, v.Key, expected, v.Name, v.DriverType, driver, rules, v.SourceFingerpr, v.ResourceTypeKey, v.DriverAccount).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.classifyDefinitionMiss(ctx, org, v, expected)
		}
		if err != nil {
			return registrationError(err, "resource definition "+v.Key)
		}
		if _, err = s.q(ctx).Exec(ctx, `DELETE FROM matching_criteria WHERE resource_definition_id=$1::uuid`, id); err != nil {
			return err
		}
		return s.insertCriteria(ctx, id, v.Criteria)
	})
	if err != nil {
		return resource.Definition{}, err
	}
	return next, nil
}

// classifyDefinitionMiss explains why the guarded UPDATE touched no row.
func (s *Store) classifyDefinitionMiss(ctx context.Context, org string, v resource.Definition, expected int64) error {
	var version int64
	var typeKey string
	err := s.q(ctx).QueryRow(ctx, `SELECT rd.version,rt.resource_type_key FROM resource_definitions rd JOIN organizations o ON o.id=rd.organization_id JOIN resource_types rt ON rt.id=rd.resource_type_id WHERE o.organization_key=$1 AND rd.definition_key=$2`, org, v.Key).Scan(&version, &typeKey)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%w: resource definition %s", persistence.ErrNotFound, v.Key)
	case err != nil:
		return err
	case typeKey != v.ResourceTypeKey:
		return fmt.Errorf("%w: the resource type of definition %s is immutable", persistence.ErrNotFound, v.Key)
	case version != expected:
		return fmt.Errorf("%w: resource definition %s", persistence.ErrVersionConflict, v.Key)
	}
	return fmt.Errorf("%w: driver account of definition %s", persistence.ErrNotFound, v.Key)
}

func (s *Store) CreateConnection(ctx context.Context, v application.Connection) error {
	if v.ID == "" {
		v.ID = ids.New()
	}
	if v.Version < 1 {
		v.Version = 1
	}
	if err := v.Validate(); err != nil {
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
	err = s.q(ctx).QueryRow(ctx, `INSERT INTO connections(id,organization_id,connection_key,kind,config,secret_ref,status,verification,name,authentication_type,version) SELECT $1::uuid,o.id,$3,$4,$5,$6,$7,$8,$9,$10,$11 FROM organizations o WHERE o.organization_key=$2 RETURNING id::text`, v.ID, v.OrganizationKey, v.Key, v.Kind, config, v.SecretRef, v.Status, verification, v.Name, v.AuthenticationType, v.Version).Scan(&id)
	if err != nil {
		return registrationError(err, "connection "+v.Key)
	}
	return nil
}

// UpdateConnection is the V01 account compare-and-set: one UPDATE guarded by
// the stored version. A stale expected version leaves the winner untouched.
func (s *Store) UpdateConnection(ctx context.Context, org, key string, expected int64, next application.Connection) (application.Connection, error) {
	if err := next.Validate(); err != nil {
		return application.Connection{}, err
	}
	config, err := jsonBytes(next.Config)
	if err != nil {
		return application.Connection{}, err
	}
	verification, err := jsonBytes(next.Verification)
	if err != nil {
		return application.Connection{}, err
	}
	tag, err := s.q(ctx).Exec(ctx, `UPDATE connections c SET name=$4,config=$5,secret_ref=$6,status=$7,verification=$8,version=c.version+1,updated_at=now() FROM organizations o WHERE o.id=c.organization_id AND o.organization_key=$1 AND c.connection_key=$2 AND c.version=$3`,
		org, key, expected, next.Name, config, next.SecretRef, next.Status, verification)
	if err != nil {
		return application.Connection{}, fmt.Errorf("postgres: update connection: %w", translate(err))
	}
	if tag.RowsAffected() == 0 {
		// Distinguish a missing/foreign key from a lost race.
		if _, err := s.GetConnection(ctx, org, key); err != nil {
			return application.Connection{}, err
		}
		return application.Connection{}, fmt.Errorf("%w: connection %s", persistence.ErrVersionConflict, key)
	}
	return s.GetConnection(ctx, org, key)
}
