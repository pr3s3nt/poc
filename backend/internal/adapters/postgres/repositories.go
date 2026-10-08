package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetOrganization(ctx context.Context, key string) (application.Organization, error) {
	var v application.Organization
	err := s.q(ctx).QueryRow(ctx, `SELECT id::text,organization_key,name,default_connection_key FROM organizations WHERE organization_key=$1`, key).Scan(&v.ID, &v.Key, &v.Name, &v.DefaultConnectionKey)
	if err != nil {
		return v, fmt.Errorf("postgres: get organization: %w", translate(err))
	}
	return v, nil
}
func (s *Store) SaveOrganization(ctx context.Context, v application.Organization) error {
	if v.ID == "" {
		v.ID = ids.New()
	}
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO organizations(id,organization_key,name,default_connection_key,default_connection_id) VALUES($1::uuid,$2,$3,$4,(SELECT id FROM connections WHERE connection_key=$4 LIMIT 1)) ON CONFLICT(organization_key) DO UPDATE SET name=EXCLUDED.name,default_connection_key=EXCLUDED.default_connection_key,default_connection_id=COALESCE(EXCLUDED.default_connection_id,organizations.default_connection_id)`, v.ID, v.Key, v.Name, v.DefaultConnectionKey)
	if err != nil {
		return fmt.Errorf("postgres: save organization: %w", translate(err))
	}
	return nil
}
func (s *Store) GetUserAccountByUsername(ctx context.Context, username string) (identity.UserAccount, error) {
	return s.getUser(ctx, "username", username)
}
func (s *Store) GetUserAccount(ctx context.Context, id string) (identity.UserAccount, error) {
	return s.getUser(ctx, "u.id", id)
}
func (s *Store) getUser(ctx context.Context, column, value string) (identity.UserAccount, error) {
	var v identity.UserAccount
	q := `SELECT u.id,o.organization_key,u.username,u.password_hash,u.role,u.status FROM user_accounts u JOIN organizations o ON o.id=u.organization_id WHERE ` + column + `=$1`
	err := s.q(ctx).QueryRow(ctx, q, value).Scan(&v.ID, &v.OrganizationKey, &v.Username, &v.PasswordHash, &v.Role, &v.Status)
	if err != nil {
		return v, fmt.Errorf("postgres: get user: %w", translate(err))
	}
	return v, nil
}
func (s *Store) SaveUserAccount(ctx context.Context, v identity.UserAccount) error {
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO user_accounts(id,organization_id,username,password_hash,role,status) SELECT $1,o.id,$3,$4,$5,$6 FROM organizations o WHERE o.organization_key=$2 ON CONFLICT(id) DO UPDATE SET username=EXCLUDED.username,password_hash=EXCLUDED.password_hash,role=EXCLUDED.role,status=EXCLUDED.status,updated_at=now()`, v.ID, v.OrganizationKey, v.Username, v.PasswordHash, v.Role, v.Status)
	if err != nil {
		return fmt.Errorf("postgres: save user: %w", translate(err))
	}
	return nil
}
func (s *Store) SaveSession(ctx context.Context, v identity.Session) error {
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO sessions(id,user_account_id,token_hash,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO UPDATE SET token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at,revoked_at=EXCLUDED.revoked_at`, v.ID, v.UserAccountID, v.TokenHash, v.ExpiresAt, v.RevokedAt)
	if err != nil {
		return fmt.Errorf("postgres: save session: %w", translate(err))
	}
	return nil
}
func (s *Store) GetSessionByTokenHash(ctx context.Context, h string) (identity.Session, error) {
	var v identity.Session
	err := s.q(ctx).QueryRow(ctx, `SELECT id,user_account_id,token_hash,expires_at,revoked_at FROM sessions WHERE token_hash=$1`, h).Scan(&v.ID, &v.UserAccountID, &v.TokenHash, &v.ExpiresAt, &v.RevokedAt)
	if err != nil {
		return v, fmt.Errorf("postgres: get session: %w", translate(err))
	}
	return v, nil
}

func (s *Store) SaveConnection(ctx context.Context, v application.Connection) error {
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
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO connections(id,organization_id,connection_key,kind,config,secret_ref,status,verification,name,authentication_type) SELECT $1::uuid,o.id,$3,$4,$5,$6,$7,$8,$9,$10 FROM organizations o WHERE o.organization_key=$2 ON CONFLICT(organization_id,connection_key) DO UPDATE SET kind=EXCLUDED.kind,config=EXCLUDED.config,secret_ref=EXCLUDED.secret_ref,status=EXCLUDED.status,verification=EXCLUDED.verification,name=EXCLUDED.name,authentication_type=EXCLUDED.authentication_type,updated_at=now()`, v.ID, v.OrganizationKey, v.Key, v.Kind, config, v.SecretRef, v.Status, verification, v.Name, v.AuthenticationType)
	if err != nil {
		return fmt.Errorf("postgres: save connection: %w", translate(err))
	}
	_, err = s.q(ctx).Exec(ctx, `UPDATE organizations SET default_connection_id=c.id FROM connections c WHERE organizations.id=c.organization_id AND organizations.default_connection_key=c.connection_key AND c.connection_key=$1`, v.Key)
	return translate(err)
}
func (s *Store) GetConnection(ctx context.Context, org, key string) (application.Connection, error) {
	var v application.Connection
	var a, b []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT c.id::text,c.connection_key,o.organization_key,c.kind,c.config,c.secret_ref,c.status,c.verification,c.name,c.authentication_type FROM connections c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.connection_key=$2`, org, key).Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Kind, &a, &v.SecretRef, &v.Status, &b, &v.Name, &v.AuthenticationType)
	if err != nil {
		return v, fmt.Errorf("postgres: get connection: %w", translate(err))
	}
	if err = unmarshalJSON(a, &v.Config); err != nil {
		return v, err
	}
	err = unmarshalJSON(b, &v.Verification)
	return v, err
}
func (s *Store) ListConnections(ctx context.Context, org string) ([]application.Connection, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT c.id::text,c.connection_key,o.organization_key,c.kind,c.config,c.secret_ref,c.status,c.verification,c.name,c.authentication_type FROM connections c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 ORDER BY c.connection_key`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.Connection
	for rows.Next() {
		var v application.Connection
		var a, b []byte
		if err = rows.Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Kind, &a, &v.SecretRef, &v.Status, &b, &v.Name, &v.AuthenticationType); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(a, &v.Config); err != nil {
			return nil, err
		}
		if err = unmarshalJSON(b, &v.Verification); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveApplication(ctx context.Context, v application.Application) error {
	if v.ID == "" {
		v.ID = v.Key
	}
	if v.Version == 0 {
		v.Version = 1
	}
	// Legacy target columns are never written by a save (ADR-011): a new
	// Application is stored unbound and an existing one keeps its legacy data.
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO applications(id,application_key,organization_id,name,subdomain,execution_profile,connection_id,region,runtime_status,version,configuration_provider) SELECT $1::uuid,$2,o.id,$4,$5,'',NULL,'','UNCONFIGURED',$6,$7 FROM organizations o WHERE o.organization_key=$3 ON CONFLICT(id) DO UPDATE SET application_key=EXCLUDED.application_key,name=EXCLUDED.name,subdomain=EXCLUDED.subdomain,version=EXCLUDED.version,configuration_provider=EXCLUDED.configuration_provider`, v.ID, v.Key, v.OrganizationKey, v.Name, v.Subdomain, v.Version, v.ConfigurationProvider)
	if err != nil {
		return fmt.Errorf("postgres: save application: %w", translate(err))
	}
	return nil
}
func scanApp(row pgx.Row) (application.Application, error) {
	var v application.Application
	err := row.Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Name, &v.Subdomain, &v.Profile, &v.ConnectionKey, &v.Region, &v.RuntimeStatus, &v.Version, &v.ConfigurationProvider)
	return v, err
}

const appSelect = `SELECT a.id::text,a.application_key,o.organization_key,a.name,a.subdomain,a.execution_profile,COALESCE(c.connection_key,''),a.region,a.runtime_status,a.version,a.configuration_provider FROM applications a JOIN organizations o ON o.id=a.organization_id LEFT JOIN connections c ON c.id=a.connection_id`

func (s *Store) GetApplication(ctx context.Context, key string) (application.Application, error) {
	v, err := scanApp(s.q(ctx).QueryRow(ctx, appSelect+` WHERE a.application_key=$1`, key))
	if err != nil {
		return v, fmt.Errorf("postgres: get application: %w", translate(err))
	}
	return v, nil
}
func (s *Store) ListApplications(ctx context.Context) ([]application.Application, error) {
	rows, err := s.q(ctx).Query(ctx, appSelect+` ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.Application
	for rows.Next() {
		var v application.Application
		if err = rows.Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Name, &v.Subdomain, &v.Profile, &v.ConnectionKey, &v.Region, &v.RuntimeStatus, &v.Version, &v.ConfigurationProvider); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveEnvironment(ctx context.Context, v environment.Environment) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSaveEnvironment(ctx, v) })
}

func (s *Store) fencedSaveEnvironment(ctx context.Context, v environment.Environment) error {
	if v.ID == "" {
		v.ID = ids.New()
	}
	if v.Version == 0 {
		v.Version = 1
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,current_deployment_set_id,version,draft_version,public_routes_pending) SELECT $1::uuid,a.id,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,$9,$10 FROM applications a WHERE a.application_key=$2 ON CONFLICT(application_id,environment_key) DO UPDATE SET name=EXCLUDED.name,environment_type=EXCLUDED.environment_type,namespace_identity=EXCLUDED.namespace_identity,current_deployment_set_id=EXCLUDED.current_deployment_set_id,version=EXCLUDED.version,draft_version=EXCLUDED.draft_version,public_routes_pending=EXCLUDED.public_routes_pending WHERE environments.active_operation_id IS NULL OR environments.active_operation_id=NULLIF($11,'')::uuid`, v.ID, v.ApplicationKey, v.Key, v.Name, v.Type, v.NamespaceIdentity, v.CurrentDeploymentSetID, v.Version, v.DraftVersion, v.PublicRoutesPending, persistence.OperationFrom(ctx))
	if err != nil {
		return fmt.Errorf("postgres: save environment: %w", translate(err))
	}
	if tag.RowsAffected() == 0 {
		return persistence.Busy(v.ApplicationKey, v.Key)
	}
	return nil
}

const envSelect = `SELECT e.id::text,e.environment_key,a.id::text,a.application_key,e.name,e.environment_type,e.namespace_identity,COALESCE(e.current_deployment_set_id::text,''),e.version,e.draft_version,e.public_routes_pending,COALESCE(ec.connection_key,''),e.execution_profile,e.region,e.runtime_status,e.infrastructure_scope,e.target_generation,e.generation_high,COALESCE(ss.store_key,''),COALESCE(e.active_operation_id::text,'') FROM environments e JOIN applications a ON a.id=e.application_id LEFT JOIN connections ec ON ec.id=e.connection_id LEFT JOIN secret_store_connections ss ON ss.id=e.secret_store_id`

func scanEnv(row pgx.Row) (environment.Environment, error) {
	var v environment.Environment
	err := row.Scan(&v.ID, &v.Key, &v.ApplicationID, &v.ApplicationKey, &v.Name, &v.Type, &v.NamespaceIdentity, &v.CurrentDeploymentSetID, &v.Version, &v.DraftVersion, &v.PublicRoutesPending, &v.ConnectionKey, &v.Profile, &v.Region, &v.RuntimeStatus, &v.InfrastructureScope, &v.TargetGeneration, &v.GenerationHigh, &v.SecretStoreKey, &v.ActiveOperationID)
	return v, err
}
func (s *Store) GetEnvironment(ctx context.Context, app, key string) (environment.Environment, error) {
	v, err := scanEnv(s.q(ctx).QueryRow(ctx, envSelect+` WHERE a.application_key=$1 AND e.environment_key=$2`, app, key))
	if err != nil {
		return v, fmt.Errorf("postgres: get environment: %w", translate(err))
	}
	return v, nil
}
func (s *Store) ListEnvironments(ctx context.Context, app string) ([]environment.Environment, error) {
	rows, err := s.q(ctx).Query(ctx, envSelect+` WHERE a.application_key=$1 ORDER BY e.environment_key`, app)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []environment.Environment
	for rows.Next() {
		v, err := scanEnv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ownerArg is the SQL operation-owner argument; empty means no owner.
func ownerArg(ctx context.Context) string { return persistence.OperationFrom(ctx) }

// classifyEnvWrite explains why a guarded environment UPDATE matched no row.
func (s *Store) classifyEnvWrite(ctx context.Context, app, env string, expected int64, checkVersion bool, fallback ...error) error {
	current, err := s.GetEnvironment(ctx, app, env)
	switch {
	case err != nil:
		return err
	case current.ActiveOperationID != "" && current.ActiveOperationID != ownerArg(ctx):
		return persistence.Busy(app, env)
	case checkVersion && current.Version != expected:
		return persistence.ErrVersionConflict
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return nil
}

// BindEnvironment replaces the binding with one guarded UPDATE: expected
// version, Organization/Connection scope and claim owner in a single statement.
func (s *Store) BindEnvironment(ctx context.Context, b persistence.EnvironmentBinding) (environment.Environment, error) {
	var out0 environment.Environment
	err := s.fenced(ctx, func(ctx context.Context) error {
		var err error
		out0, err = s.fencedBindEnvironment(ctx, b)
		return err
	})
	return out0, err
}

func (s *Store) fencedBindEnvironment(ctx context.Context, b persistence.EnvironmentBinding) (environment.Environment, error) {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET connection_id=c.id,execution_profile=$4,region=$5,runtime_status=$6,infrastructure_scope=$7,target_generation=$8,generation_high=GREATEST(e.generation_high,$8),version=e.version+1
FROM applications a, connections c WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND c.organization_id=a.organization_id AND c.connection_key=$3
AND e.version=$9 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($10,'')::uuid)
AND $8>=0 AND ($8<=e.generation_high OR $8=e.target_generation)`, b.ApplicationKey, b.EnvironmentKey, b.ConnectionKey, b.Profile, b.Region, b.RuntimeStatus, b.Scope, b.Generation, b.ExpectedVersion, ownerArg(ctx))
	if err != nil {
		return environment.Environment{}, fmt.Errorf("postgres: bind environment: %w", translate(err))
	}
	if tag.RowsAffected() == 0 {
		if err := s.classifyEnvWrite(ctx, b.ApplicationKey, b.EnvironmentKey, b.ExpectedVersion, true); err != nil {
			return environment.Environment{}, err
		}
		return environment.Environment{}, fmt.Errorf("%w: connection %q", persistence.ErrNotFound, b.ConnectionKey)
	}
	return s.GetEnvironment(ctx, b.ApplicationKey, b.EnvironmentKey)
}

// SelectSecretStore replaces the secret-store selection with one guarded UPDATE.
func (s *Store) SelectSecretStore(ctx context.Context, sel persistence.SecretStoreSelection) (environment.Environment, error) {
	var out0 environment.Environment
	err := s.fenced(ctx, func(ctx context.Context) error {
		var err error
		out0, err = s.fencedSelectSecretStore(ctx, sel)
		return err
	})
	return out0, err
}

func (s *Store) fencedSelectSecretStore(ctx context.Context, sel persistence.SecretStoreSelection) (environment.Environment, error) {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET secret_store_id=ss.id,version=e.version+1
FROM applications a, secret_store_connections ss WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND ss.organization_id=a.organization_id AND ss.store_key=$3
AND e.version=$4 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($5,'')::uuid)`, sel.ApplicationKey, sel.EnvironmentKey, sel.StoreKey, sel.ExpectedVersion, ownerArg(ctx))
	if err != nil {
		return environment.Environment{}, fmt.Errorf("postgres: select secret store: %w", translate(err))
	}
	if tag.RowsAffected() == 0 {
		if err := s.classifyEnvWrite(ctx, sel.ApplicationKey, sel.EnvironmentKey, sel.ExpectedVersion, true); err != nil {
			return environment.Environment{}, err
		}
		return environment.Environment{}, fmt.Errorf("%w: secret store %q", persistence.ErrNotFound, sel.StoreKey)
	}
	return s.GetEnvironment(ctx, sel.ApplicationKey, sel.EnvironmentKey)
}

// UpdateRuntimeStatus advances the runtime status of a configured Environment.
func (s *Store) UpdateRuntimeStatus(ctx context.Context, app, key string, status application.RuntimeStatus) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedUpdateRuntimeStatus(ctx, app, key, status) })
}

func (s *Store) fencedUpdateRuntimeStatus(ctx context.Context, app, key string, status application.RuntimeStatus) error {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET runtime_status=$3 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.connection_id IS NOT NULL AND $3 IN ('PENDING','READY') AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($4,'')::uuid)`, app, key, status, ownerArg(ctx))
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		if err := s.classifyEnvWrite(ctx, app, key, 0, false); err != nil {
			return err
		}
		return fmt.Errorf("%w: environment runtime status", persistence.ErrImmutable)
	}
	return nil
}

// SetPublicRoutesPending changes only the route flag.
func (s *Store) SetPublicRoutesPending(ctx context.Context, app, key string, pending bool) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSetPublicRoutesPending(ctx, app, key, pending) })
}

func (s *Store) fencedSetPublicRoutesPending(ctx context.Context, app, key string, pending bool) error {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET public_routes_pending=$3 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($4,'')::uuid)`, app, key, pending, ownerArg(ctx))
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		if err := s.classifyEnvWrite(ctx, app, key, 0, false); err != nil {
			return err
		}
		return fmt.Errorf("%w: environment %s/%s", persistence.ErrNotFound, app, key)
	}
	return nil
}

// AllocateGeneration reserves the next target generation for the owner.
func (s *Store) AllocateGeneration(ctx context.Context, app, key string) (int64, error) {
	var out0 int64
	err := s.fenced(ctx, func(ctx context.Context) error {
		var err error
		out0, err = s.fencedAllocateGeneration(ctx, app, key)
		return err
	})
	return out0, err
}

func (s *Store) fencedAllocateGeneration(ctx context.Context, app, key string) (int64, error) {
	var generation int64
	err := s.q(ctx).QueryRow(ctx, `UPDATE environments e SET generation_high=GREATEST(e.generation_high,e.target_generation)+1 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.active_operation_id=NULLIF($3,'')::uuid RETURNING e.generation_high`, app, key, ownerArg(ctx)).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: generation allocation needs the owning operation", persistence.ErrEnvironmentBusy)
	}
	return generation, translate(err)
}

func (s *Store) CompareVersionAndSetCurrent(ctx context.Context, app, key string, expected int64, setID string) error {
	return s.fenced(ctx, func(ctx context.Context) error {
		return s.fencedCompareVersionAndSetCurrent(ctx, app, key, expected, setID)
	})
}

func (s *Store) fencedCompareVersionAndSetCurrent(ctx context.Context, app, key string, expected int64, setID string) error {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET current_deployment_set_id=$4::uuid,version=e.version+1 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.version=$3 AND (e.active_operation_id IS NULL OR e.active_operation_id=NULLIF($5,'')::uuid)`, app, key, expected, setID, ownerArg(ctx))
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		if err := s.classifyEnvWrite(ctx, app, key, expected, true); err != nil {
			return err
		}
		return persistence.ErrVersionConflict
	}
	return nil
}

func (s *Store) SaveDeploymentSet(ctx context.Context, v environment.DeploymentSet) error {
	return s.fenced(ctx, func(ctx context.Context) error { return s.fencedSaveDeploymentSet(ctx, v) })
}

func (s *Store) fencedSaveDeploymentSet(ctx context.Context, v environment.DeploymentSet) error {
	doc, err := jsonBytes(v.Document)
	if err != nil {
		return err
	}
	envID := v.EnvironmentID
	if envID == "" {
		parts := strings.SplitN(v.EnvironmentKey, "/", 2)
		if len(parts) == 2 {
			err = s.q(ctx).QueryRow(ctx, `SELECT e.id::text FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key=$1 AND e.environment_key=$2`, parts[0], parts[1]).Scan(&envID)
		}
	}
	if err != nil {
		return translate(err)
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO deployment_sets(id,environment_id,created_by_deployment_id,document,document_hash,created_at) VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,$4,$5,$6) ON CONFLICT(id) DO NOTHING`, v.ID, envID, v.CreatedByDeploymentID, doc, v.DocumentHash, v.CreatedAt)
	return translate(err)
}
func (s *Store) GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error) {
	var v environment.DeploymentSet
	var doc []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT ds.id::text,ds.environment_id::text,a.application_key||'/'||e.environment_key,COALESCE(ds.created_by_deployment_id::text,''),ds.document,ds.document_hash,ds.created_at FROM deployment_sets ds JOIN environments e ON e.id=ds.environment_id JOIN applications a ON a.id=e.application_id WHERE ds.id=$1::uuid`, id).Scan(&v.ID, &v.EnvironmentID, &v.EnvironmentKey, &v.CreatedByDeploymentID, &doc, &v.DocumentHash, &v.CreatedAt)
	if err != nil {
		return v, translate(err)
	}
	err = unmarshalJSON(doc, &v.Document)
	return v, err
}

// keep imports used by the remainder of this file's repository sections.
var _ = sort.Slice
var _ = time.Now
var _ = configuration.Scope{}
var _ = deployment.Deployment{}
var _ = resource.Type{}
