CREATE TABLE IF NOT EXISTS organizations (
  id uuid PRIMARY KEY,
  organization_key text UNIQUE NOT NULL,
  name text NOT NULL,
  default_connection_key text NOT NULL,
  default_connection_id uuid
);

CREATE TABLE IF NOT EXISTS user_accounts (
  id text PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  username text UNIQUE NOT NULL,
  password_hash text NOT NULL,
  role text NOT NULL CHECK (role IN ('ADMIN','PLATFORM_ENGINEER','DEVELOPER')),
  status text NOT NULL CHECK (status IN ('ACTIVE','DISABLED')),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions (
  id text PRIMARY KEY, user_account_id text NOT NULL REFERENCES user_accounts(id),
  token_hash text UNIQUE NOT NULL, expires_at timestamptz NOT NULL,
  revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS connections (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
  connection_key text NOT NULL, kind text NOT NULL CHECK (kind IN ('AWS','KUBERNETES')),
  config jsonb NOT NULL, secret_ref text NOT NULL, status text NOT NULL,
  verification jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, connection_key)
);
DO $$ BEGIN ALTER TABLE organizations ADD CONSTRAINT organizations_default_connection_fk
  FOREIGN KEY (default_connection_id) REFERENCES connections(id) DEFERRABLE INITIALLY DEFERRED;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
CREATE TABLE IF NOT EXISTS applications (
  id uuid PRIMARY KEY, application_key text UNIQUE NOT NULL, organization_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL, subdomain text NOT NULL UNIQUE, execution_profile text NOT NULL,
  connection_id uuid NOT NULL REFERENCES connections(id), region text NOT NULL DEFAULT '',
  runtime_status text NOT NULL, version bigint NOT NULL,
  configuration_provider text NOT NULL DEFAULT '', UNIQUE (organization_id, name)
);
CREATE TABLE IF NOT EXISTS environments (
  id uuid PRIMARY KEY, application_id uuid NOT NULL REFERENCES applications(id),
  environment_key text NOT NULL, name text NOT NULL, environment_type text NOT NULL,
  namespace_identity text NOT NULL, current_deployment_set_id uuid,
  version bigint NOT NULL, draft_version bigint NOT NULL DEFAULT 0,
  public_routes_pending boolean NOT NULL DEFAULT false,
  desired_config_revision_id uuid,
  UNIQUE(application_id, environment_key), UNIQUE(application_id, namespace_identity)
);
CREATE TABLE IF NOT EXISTS deployments (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id),
  organization_key text NOT NULL, application_key text NOT NULL, environment_key text NOT NULL,
  execution_profile text NOT NULL, action text NOT NULL, workload_id text NOT NULL,
  actor_ref text NOT NULL, status text NOT NULL, base_environment_version bigint NOT NULL,
  base_deployment_set_id uuid, candidate_deployment_set_id uuid,
  failure_reason text NOT NULL DEFAULT '', started_at timestamptz NOT NULL,
  finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS deployments_environment_started_idx ON deployments(environment_id, started_at DESC);
CREATE TABLE IF NOT EXISTS deployment_sets (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id) DEFERRABLE INITIALLY DEFERRED,
  created_by_deployment_id uuid REFERENCES deployments(id) DEFERRABLE INITIALLY DEFERRED,
  document jsonb NOT NULL, document_hash text NOT NULL, created_at timestamptz NOT NULL
);
DO $$ BEGIN ALTER TABLE environments ADD CONSTRAINT environments_current_set_fk
  FOREIGN KEY(current_deployment_set_id) REFERENCES deployment_sets(id) DEFERRABLE INITIALLY DEFERRED;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
ALTER TABLE deployments DROP CONSTRAINT IF EXISTS deployments_base_set_fk;
ALTER TABLE deployments ADD CONSTRAINT deployments_base_set_fk FOREIGN KEY(base_deployment_set_id) REFERENCES deployment_sets(id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE deployments DROP CONSTRAINT IF EXISTS deployments_candidate_set_fk;
ALTER TABLE deployments ADD CONSTRAINT deployments_candidate_set_fk FOREIGN KEY(candidate_deployment_set_id) REFERENCES deployment_sets(id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE IF NOT EXISTS deployment_delta_snapshots (
  id uuid PRIMARY KEY, deployment_id uuid UNIQUE NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  document jsonb NOT NULL, document_hash text NOT NULL, metadata jsonb NOT NULL, created_at timestamptz NOT NULL
);
CREATE OR REPLACE FUNCTION require_deployment_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('PROVISIONING','DEPLOYING','SUCCEEDED') AND
     NOT EXISTS (SELECT 1 FROM deployment_delta_snapshots s WHERE s.deployment_id=NEW.id) THEN
    RAISE EXCEPTION 'deployment % requires one delta snapshot', NEW.id USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS deployments_require_snapshot ON deployments;
CREATE CONSTRAINT TRIGGER deployments_require_snapshot AFTER INSERT OR UPDATE ON deployments
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_deployment_snapshot();
CREATE TABLE IF NOT EXISTS deployment_plans (
  deployment_id uuid PRIMARY KEY REFERENCES deployments(id) ON DELETE CASCADE,
  document jsonb NOT NULL, plan_hash text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS resource_types (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), resource_type_key text NOT NULL,
  input_schema jsonb NOT NULL, output_schema jsonb NOT NULL, UNIQUE(organization_id, resource_type_key)
);
CREATE TABLE IF NOT EXISTS resource_definitions (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), definition_key text NOT NULL,
  resource_type_id uuid NOT NULL REFERENCES resource_types(id), execution_profile text,
  driver_type text NOT NULL, connection_id uuid REFERENCES connections(id), driver_inputs jsonb NOT NULL,
  provision_rules jsonb NOT NULL, source_fingerprint text, UNIQUE(organization_id, definition_key)
);
CREATE TABLE IF NOT EXISTS matching_criteria (
  id uuid PRIMARY KEY, resource_definition_id uuid NOT NULL REFERENCES resource_definitions(id) ON DELETE CASCADE,
  ordinal integer NOT NULL, env_type text, app_id text, env_id text, res_id text, class text,
  specificity_score integer NOT NULL, UNIQUE(resource_definition_id, ordinal)
);
CREATE TABLE IF NOT EXISTS active_resources (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), descriptor text NOT NULL,
  scope_type text NOT NULL, scope_id text NOT NULL, resource_definition_id uuid NOT NULL REFERENCES resource_definitions(id),
  connection_id uuid REFERENCES connections(id), status text NOT NULL, executor_state_ref jsonb NOT NULL,
  outputs jsonb NOT NULL, input_fingerprint text, last_deployment_id uuid REFERENCES deployments(id),
  version bigint NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  UNIQUE(organization_id, descriptor, scope_type, scope_id)
);
CREATE TABLE IF NOT EXISTS deployment_resources (
  deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE, node_descriptor text NOT NULL,
  active_resource_id uuid REFERENCES active_resources(id), definition_key text, resource_type_key text,
  status text NOT NULL, resolved_inputs jsonb NOT NULL, output_snapshot jsonb NOT NULL,
  batch_index integer NOT NULL, started_at timestamptz NOT NULL, finished_at timestamptz,
  PRIMARY KEY(deployment_id,node_descriptor)
);
CREATE TABLE IF NOT EXISTS configuration_revisions (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), version bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(environment_id,version)
);
CREATE TABLE IF NOT EXISTS configuration_revision_entries (
  revision_id uuid NOT NULL REFERENCES configuration_revisions(id) ON DELETE CASCADE,
  key_name text NOT NULL, kind text NOT NULL, value_ref text NOT NULL, PRIMARY KEY(revision_id,key_name)
);
DO $$ BEGIN ALTER TABLE environments ADD CONSTRAINT environments_desired_revision_fk
  FOREIGN KEY(desired_config_revision_id) REFERENCES configuration_revisions(id) DEFERRABLE INITIALLY DEFERRED;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
CREATE TABLE IF NOT EXISTS workload_drafts (
  environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE, workload_id text NOT NULL,
  score_document jsonb, state text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(environment_id,workload_id)
);
CREATE TABLE IF NOT EXISTS workload_instances (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), workload_id text NOT NULL,
  last_deployment_id uuid NOT NULL REFERENCES deployments(id), applied_config_revision_id uuid REFERENCES configuration_revisions(id),
  target_ref jsonb NOT NULL, manifest_digest text NOT NULL, status text NOT NULL, observed_at timestamptz NOT NULL,
  UNIQUE(environment_id,workload_id)
);
