package postgres

import (
	"context"
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
	config, err := jsonBytes(v.Config)
	if err != nil {
		return err
	}
	verification, err := jsonBytes(v.Verification)
	if err != nil {
		return err
	}
	_, err = s.q(ctx).Exec(ctx, `INSERT INTO connections(id,organization_id,connection_key,kind,config,secret_ref,status,verification) SELECT $1::uuid,o.id,$3,$4,$5,$6,$7,$8 FROM organizations o WHERE o.organization_key=$2 ON CONFLICT(organization_id,connection_key) DO UPDATE SET kind=EXCLUDED.kind,config=EXCLUDED.config,secret_ref=EXCLUDED.secret_ref,status=EXCLUDED.status,verification=EXCLUDED.verification,updated_at=now()`, v.ID, v.OrganizationKey, v.Key, v.Kind, config, v.SecretRef, v.Status, verification)
	if err != nil {
		return fmt.Errorf("postgres: save connection: %w", translate(err))
	}
	_, err = s.q(ctx).Exec(ctx, `UPDATE organizations SET default_connection_id=c.id FROM connections c WHERE organizations.id=c.organization_id AND organizations.default_connection_key=c.connection_key AND c.connection_key=$1`, v.Key)
	return translate(err)
}
func (s *Store) GetConnection(ctx context.Context, org, key string) (application.Connection, error) {
	var v application.Connection
	var a, b []byte
	err := s.q(ctx).QueryRow(ctx, `SELECT c.id::text,c.connection_key,o.organization_key,c.kind,c.config,c.secret_ref,c.status,c.verification FROM connections c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 AND c.connection_key=$2`, org, key).Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Kind, &a, &v.SecretRef, &v.Status, &b)
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
	rows, err := s.q(ctx).Query(ctx, `SELECT c.id::text,c.connection_key,o.organization_key,c.kind,c.config,c.secret_ref,c.status,c.verification FROM connections c JOIN organizations o ON o.id=c.organization_id WHERE o.organization_key=$1 ORDER BY c.connection_key`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []application.Connection
	for rows.Next() {
		var v application.Connection
		var a, b []byte
		if err = rows.Scan(&v.ID, &v.Key, &v.OrganizationKey, &v.Kind, &a, &v.SecretRef, &v.Status, &b); err != nil {
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
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO applications(id,application_key,organization_id,name,subdomain,execution_profile,connection_id,region,runtime_status,version,configuration_provider) SELECT $1::uuid,$2,o.id,$4,$5,$6,c.id,$8,$9,$10,$11 FROM organizations o JOIN connections c ON c.organization_id=o.id AND c.connection_key=$7 WHERE o.organization_key=$3 ON CONFLICT(id) DO UPDATE SET application_key=EXCLUDED.application_key,name=EXCLUDED.name,subdomain=EXCLUDED.subdomain,runtime_status=EXCLUDED.runtime_status,version=EXCLUDED.version,configuration_provider=EXCLUDED.configuration_provider`, v.ID, v.Key, v.OrganizationKey, v.Name, v.Subdomain, v.Profile, v.ConnectionKey, v.Region, v.RuntimeStatus, v.Version, v.ConfigurationProvider)
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

const appSelect = `SELECT a.id::text,a.application_key,o.organization_key,a.name,a.subdomain,a.execution_profile,c.connection_key,a.region,a.runtime_status,a.version,a.configuration_provider FROM applications a JOIN organizations o ON o.id=a.organization_id JOIN connections c ON c.id=a.connection_id`

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
	if v.ID == "" {
		v.ID = ids.New()
	}
	if v.Version == 0 {
		v.Version = 1
	}
	_, err := s.q(ctx).Exec(ctx, `INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,current_deployment_set_id,version,draft_version,public_routes_pending) SELECT $1::uuid,a.id,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,$9,$10 FROM applications a WHERE a.application_key=$2 ON CONFLICT(application_id,environment_key) DO UPDATE SET name=EXCLUDED.name,environment_type=EXCLUDED.environment_type,namespace_identity=EXCLUDED.namespace_identity,current_deployment_set_id=EXCLUDED.current_deployment_set_id,version=EXCLUDED.version,draft_version=EXCLUDED.draft_version,public_routes_pending=EXCLUDED.public_routes_pending`, v.ID, v.ApplicationKey, v.Key, v.Name, v.Type, v.NamespaceIdentity, v.CurrentDeploymentSetID, v.Version, v.DraftVersion, v.PublicRoutesPending)
	if err != nil {
		return fmt.Errorf("postgres: save environment: %w", translate(err))
	}
	return nil
}

const envSelect = `SELECT e.id::text,e.environment_key,a.id::text,a.application_key,e.name,e.environment_type,e.namespace_identity,COALESCE(e.current_deployment_set_id::text,''),e.version,e.draft_version,e.public_routes_pending FROM environments e JOIN applications a ON a.id=e.application_id`

func scanEnv(row pgx.Row) (environment.Environment, error) {
	var v environment.Environment
	err := row.Scan(&v.ID, &v.Key, &v.ApplicationID, &v.ApplicationKey, &v.Name, &v.Type, &v.NamespaceIdentity, &v.CurrentDeploymentSetID, &v.Version, &v.DraftVersion, &v.PublicRoutesPending)
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
		var v environment.Environment
		if err = rows.Scan(&v.ID, &v.Key, &v.ApplicationID, &v.ApplicationKey, &v.Name, &v.Type, &v.NamespaceIdentity, &v.CurrentDeploymentSetID, &v.Version, &v.DraftVersion, &v.PublicRoutesPending); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CompareVersionAndSetCurrent(ctx context.Context, app, key string, expected int64, setID string) error {
	tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET current_deployment_set_id=$4::uuid,version=version+1 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.version=$3`, app, key, expected, setID)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() == 0 {
		return persistence.ErrVersionConflict
	}
	return nil
}

func (s *Store) SaveDeploymentSet(ctx context.Context, v environment.DeploymentSet) error {
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
