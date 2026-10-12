-- Current complete PostgreSQL baseline (T28). Applied only to an empty schema
-- inside one transaction; there is no upgrade path from any earlier layout.
CREATE TABLE schema_baseline (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  baseline text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organizations (
  id uuid PRIMARY KEY,
  organization_key text UNIQUE NOT NULL,
  name text NOT NULL,
  default_connection_key text NOT NULL,
  default_connection_id uuid
);

CREATE TABLE user_accounts (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  username text UNIQUE NOT NULL,
  password_hash text NOT NULL,
  role text NOT NULL CHECK (role IN ('ADMIN','PLATFORM_ENGINEER','DEVELOPER')),
  status text NOT NULL CHECK (status IN ('ACTIVE','DISABLED')),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
  id uuid PRIMARY KEY, user_account_id uuid NOT NULL REFERENCES user_accounts(id),
  token_hash text UNIQUE NOT NULL, expires_at timestamptz NOT NULL,
  revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE connections (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
  connection_key text NOT NULL, name text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('AWS','KUBERNETES')),
  authentication_type text NOT NULL,
  config jsonb NOT NULL, secret_ref text NOT NULL, status text NOT NULL,
  verification jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, connection_key),
  CONSTRAINT connections_authentication_type_check CHECK (
    (kind='KUBERNETES' AND authentication_type='KUBECONFIG') OR
    (kind='AWS' AND authentication_type='AWS_ACCESS_KEY'))
);
ALTER TABLE organizations ADD CONSTRAINT organizations_default_connection_fk
  FOREIGN KEY (default_connection_id) REFERENCES connections(id) DEFERRABLE INITIALLY DEFERRED;

-- Application is identity and current metadata only; execution and store
-- binding belong to the Environment.
CREATE TABLE applications (
  id uuid PRIMARY KEY, application_key text UNIQUE NOT NULL, organization_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL, subdomain text NOT NULL UNIQUE, version bigint NOT NULL,
  UNIQUE (organization_id, name)
);
CREATE UNIQUE INDEX applications_organization_lower_name_key ON applications(organization_id, lower(name));

CREATE TABLE secret_store_connections (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
  store_key text NOT NULL, name text NOT NULL, provider text NOT NULL CHECK (provider IN ('VAULT_KV_V2')),
  backend_address text NOT NULL, workload_address text NOT NULL, kv_mount text NOT NULL, auth_mount text NOT NULL,
  tls_ca_pem text NOT NULL DEFAULT '', credential_ref text NOT NULL DEFAULT '', status text NOT NULL CHECK (status IN ('READY','REJECTED')),
  verification jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id, store_key)
);

-- No permanent binding immutability trigger: binding writes go through the
-- dedicated versioned commands and the operation claim owner.
CREATE TABLE environments (
  id uuid PRIMARY KEY, application_id uuid NOT NULL REFERENCES applications(id),
  environment_key text NOT NULL, name text NOT NULL, environment_type text NOT NULL,
  namespace_identity text NOT NULL, current_deployment_set_id uuid,
  version bigint NOT NULL, draft_version bigint NOT NULL DEFAULT 0,
  public_routes_pending boolean NOT NULL DEFAULT false,
  desired_config_revision_id uuid,
  connection_id uuid REFERENCES connections(id),
  execution_profile text NOT NULL DEFAULT '',
  region text NOT NULL DEFAULT '',
  runtime_status text NOT NULL DEFAULT 'UNCONFIGURED',
  infrastructure_scope text NOT NULL DEFAULT 'ENVIRONMENT',
  secret_store_id uuid REFERENCES secret_store_connections(id),
  target_generation bigint NOT NULL DEFAULT 0,
  generation_high bigint NOT NULL DEFAULT 0,
  active_operation_id uuid,
  UNIQUE(application_id, environment_key), UNIQUE(application_id, namespace_identity),
  CONSTRAINT environments_binding_consistent CHECK (
    (connection_id IS NULL AND runtime_status='UNCONFIGURED' AND execution_profile='' AND region='') OR
    (connection_id IS NOT NULL AND execution_profile IN ('aws-eks','internal-k8s') AND runtime_status IN ('PENDING','READY')
      AND (execution_profile<>'aws-eks' OR region<>''))),
  CONSTRAINT environments_infrastructure_scope_check CHECK (infrastructure_scope = 'ENVIRONMENT')
);

CREATE TABLE environment_operations (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id),
  kind text NOT NULL, owner_instance text NOT NULL, fence bigint NOT NULL DEFAULT 1, status text NOT NULL, stage text NOT NULL DEFAULT '',
  pins jsonb NOT NULL, detail jsonb NOT NULL DEFAULT '{}'::jsonb, failure text NOT NULL DEFAULT '',
  started_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, heartbeat_at timestamptz NOT NULL,
  deadline timestamptz NOT NULL, finished_at timestamptz
);
CREATE UNIQUE INDEX environment_operations_one_holder ON environment_operations(environment_id)
  WHERE status IN ('ACTIVE','INTERRUPTED','RECOVERING');
ALTER TABLE environments ADD CONSTRAINT environments_active_operation_fk
  FOREIGN KEY(active_operation_id) REFERENCES environment_operations(id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE environment_transitions (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), operation_id uuid NOT NULL,
  status text NOT NULL, stage text NOT NULL, document jsonb NOT NULL,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE INDEX environment_transitions_env_idx ON environment_transitions(environment_id, created_at DESC);

CREATE TABLE deployments (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id),
  organization_key text NOT NULL, application_key text NOT NULL, environment_key text NOT NULL,
  execution_profile text NOT NULL, action text NOT NULL, workload_id text NOT NULL,
  actor_ref text NOT NULL, status text NOT NULL, base_environment_version bigint NOT NULL,
  base_deployment_set_id uuid, candidate_deployment_set_id uuid,
  failure_reason text NOT NULL DEFAULT '', started_at timestamptz NOT NULL,
  finished_at timestamptz,
  connection_key text NOT NULL DEFAULT '', target_generation bigint NOT NULL DEFAULT 0
);
CREATE INDEX deployments_environment_started_idx ON deployments(environment_id, started_at DESC);
CREATE TABLE deployment_sets (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id) DEFERRABLE INITIALLY DEFERRED,
  created_by_deployment_id uuid REFERENCES deployments(id) DEFERRABLE INITIALLY DEFERRED,
  document jsonb NOT NULL, document_hash text NOT NULL, created_at timestamptz NOT NULL
);
ALTER TABLE environments ADD CONSTRAINT environments_current_set_fk
  FOREIGN KEY(current_deployment_set_id) REFERENCES deployment_sets(id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE deployments ADD CONSTRAINT deployments_base_set_fk FOREIGN KEY(base_deployment_set_id) REFERENCES deployment_sets(id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE deployments ADD CONSTRAINT deployments_candidate_set_fk FOREIGN KEY(candidate_deployment_set_id) REFERENCES deployment_sets(id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE deployment_delta_snapshots (
  id uuid PRIMARY KEY, deployment_id uuid UNIQUE NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  document jsonb NOT NULL, document_hash text NOT NULL, metadata jsonb NOT NULL, created_at timestamptz NOT NULL
);
CREATE FUNCTION require_deployment_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('PROVISIONING','DEPLOYING','SUCCEEDED') AND
     NOT EXISTS (SELECT 1 FROM deployment_delta_snapshots s WHERE s.deployment_id=NEW.id) THEN
    RAISE EXCEPTION 'deployment % requires one delta snapshot', NEW.id USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER deployments_require_snapshot AFTER INSERT OR UPDATE ON deployments
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_deployment_snapshot();
CREATE TABLE deployment_plans (
  deployment_id uuid PRIMARY KEY REFERENCES deployments(id) ON DELETE CASCADE,
  document jsonb NOT NULL, plan_hash text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE resource_types (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), resource_type_key text NOT NULL,
  input_schema jsonb NOT NULL, output_schema jsonb NOT NULL, UNIQUE(organization_id, resource_type_key)
);
CREATE TABLE resource_definitions (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), definition_key text NOT NULL,
  resource_type_id uuid NOT NULL REFERENCES resource_types(id), execution_profile text,
  driver_type text NOT NULL, connection_id uuid REFERENCES connections(id), driver_inputs jsonb NOT NULL,
  provision_rules jsonb NOT NULL, source_fingerprint text, UNIQUE(organization_id, definition_key)
);
CREATE TABLE matching_criteria (
  id uuid PRIMARY KEY, resource_definition_id uuid NOT NULL REFERENCES resource_definitions(id) ON DELETE CASCADE,
  ordinal integer NOT NULL, env_type text, app_id text, env_id text, res_id text, class text,
  specificity_score integer NOT NULL, UNIQUE(resource_definition_id, ordinal)
);
CREATE TABLE active_resources (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), descriptor text NOT NULL,
  scope_type text NOT NULL, scope_id text NOT NULL, resource_definition_id uuid NOT NULL REFERENCES resource_definitions(id),
  connection_id uuid REFERENCES connections(id), status text NOT NULL, executor_state_ref jsonb NOT NULL,
  outputs jsonb NOT NULL, input_fingerprint text, last_deployment_id uuid REFERENCES deployments(id),
  version bigint NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  UNIQUE(organization_id, descriptor, scope_type, scope_id)
);
CREATE TABLE deployment_resources (
  deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE, node_descriptor text NOT NULL,
  active_resource_id uuid REFERENCES active_resources(id), definition_key text, resource_type_key text,
  status text NOT NULL, resolved_inputs jsonb NOT NULL, output_snapshot jsonb NOT NULL,
  batch_index integer NOT NULL, started_at timestamptz NOT NULL, finished_at timestamptz,
  PRIMARY KEY(deployment_id,node_descriptor)
);

CREATE TABLE configuration_revisions (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), version bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(environment_id,version)
);
-- A Variable is metadata only; a Secret is an owning store and an opaque ref only.
CREATE TABLE configuration_revision_entries (
  revision_id uuid NOT NULL REFERENCES configuration_revisions(id) ON DELETE CASCADE,
  key_name text NOT NULL, kind text NOT NULL, value_ref text,
  store_key text NOT NULL DEFAULT '', variable_value text,
  PRIMARY KEY(revision_id,key_name),
  CONSTRAINT configuration_revision_entries_kind_check CHECK (
    (kind='VARIABLE' AND variable_value IS NOT NULL AND variable_value<>'' AND value_ref IS NULL AND store_key='') OR
    (kind='SECRET' AND value_ref IS NOT NULL AND value_ref<>'' AND store_key<>'' AND variable_value IS NULL))
);
ALTER TABLE environments ADD CONSTRAINT environments_desired_revision_fk
  FOREIGN KEY(desired_config_revision_id) REFERENCES configuration_revisions(id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE workload_drafts (
  environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE, workload_id text NOT NULL,
  score_document jsonb, state text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(environment_id,workload_id)
);
CREATE TABLE workload_instances (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), workload_id text NOT NULL,
  last_deployment_id uuid NOT NULL REFERENCES deployments(id), applied_config_revision_id uuid REFERENCES configuration_revisions(id),
  target_ref jsonb NOT NULL, manifest_digest text NOT NULL, status text NOT NULL, observed_at timestamptz NOT NULL,
  generation bigint NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX workload_instances_env_generation_workload_key ON workload_instances(environment_id, generation, workload_id);
-- Immutable per-Deployment workload history; never reconstructed at startup.
CREATE TABLE deployment_workloads (
  deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  workload_id text NOT NULL,
  status text NOT NULL,
  target_ref jsonb NOT NULL,
  manifest_digest text NOT NULL,
  applied_config_revision_id uuid REFERENCES configuration_revisions(id),
  observed_at timestamptz NOT NULL,
  PRIMARY KEY(deployment_id, workload_id)
);
