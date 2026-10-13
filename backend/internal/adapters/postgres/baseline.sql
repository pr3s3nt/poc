-- Current complete PostgreSQL baseline (T28). Applied only to an empty schema
-- by the owner setup inside one transaction; there is no upgrade path from any
-- earlier layout. Runtime logins only validate it (option A, ADR-014).
-- The marker records digests of this baseline and of the created catalog, the
-- installation ID shared by the Console and State connections and the schema.
CREATE TABLE schema_baseline (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  baseline text NOT NULL,
  baseline_sha256 text NOT NULL CHECK (baseline_sha256 ~ '^[0-9a-f]{64}$'),
  catalog_sha256 text NOT NULL CHECK (catalog_sha256 ~ '^[0-9a-f]{64}$'),
  installation_id uuid NOT NULL,
  schema_name text NOT NULL,
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
  deadline timestamptz NOT NULL, finished_at timestamptz,
  -- RESOURCE_CLEANUP only: the admitted authorization and the exact selected set.
  authorization_mode text, actor_ref text, root_resource_instance_id uuid, confirmed_scope jsonb,
  deletion_request_hash text, confirmation_name text, scope_snapshot jsonb, run_id text,
  CONSTRAINT environment_operations_cleanup_check CHECK (
    (kind<>'RESOURCE_CLEANUP' AND num_nulls(authorization_mode,actor_ref,root_resource_instance_id,confirmed_scope,deletion_request_hash,confirmation_name,scope_snapshot,run_id)=8) OR
    (kind='RESOURCE_CLEANUP' AND authorization_mode IN ('USER_CONFIRMED','RUN_CLEANUP')
      AND actor_ref IS NOT NULL AND actor_ref<>'' AND root_resource_instance_id IS NOT NULL AND scope_snapshot IS NOT NULL
      AND confirmed_scope IS NOT NULL AND jsonb_typeof(confirmed_scope->'selectedResourceIds')='array'
      AND confirmed_scope->'selectedResourceIds' ? root_resource_instance_id::text
      AND (authorization_mode<>'USER_CONFIRMED' OR (deletion_request_hash IS NOT NULL AND deletion_request_hash<>'' AND confirmation_name IS NOT NULL AND confirmation_name<>''))
      AND (authorization_mode<>'RUN_CLEANUP' OR (run_id IS NOT NULL AND run_id<>''))))
);
CREATE UNIQUE INDEX environment_operations_deletion_request ON environment_operations(actor_ref, deletion_request_hash)
  WHERE deletion_request_hash IS NOT NULL;
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

-- Terraform v2 resource identity and state (ADR-014). Instances, attempts,
-- dependency edges and state blobs are retained audit records: they are never
-- deleted, and their identity and original snapshots never change.
CREATE FUNCTION terraform_retained() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% rows are retained', TG_TABLE_NAME USING ERRCODE='23514';
END $$;

CREATE TABLE terraform_module_bundles (
  id uuid PRIMARY KEY, source_id text NOT NULL, module_id text NOT NULL, revision text NOT NULL,
  subdirectory text NOT NULL, content_sha256 text NOT NULL, terraform_version text NOT NULL,
  terraform_binary_sha256 text NOT NULL, provider_lock_digest text NOT NULL,
  contract jsonb NOT NULL, provider_requirements jsonb NOT NULL, status text NOT NULL,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  UNIQUE(source_id, module_id, revision, subdirectory, content_sha256),
  -- One verified digest per revision path: same-revision drift cannot commit.
  UNIQUE(source_id, module_id, revision, subdirectory),
  CHECK (source_id ~ '^[a-z0-9][a-z0-9-]{0,62}$' AND module_id ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  CHECK (revision ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'),
  CHECK (subdirectory = '.' OR (subdirectory ~ '^[A-Za-z0-9_-][A-Za-z0-9._-]*(/[A-Za-z0-9_-][A-Za-z0-9._-]*)*$'
    AND subdirectory !~ '(^|/)\.\.?(/|$)' AND length(subdirectory) <= 512)),
  CHECK (content_sha256 ~ '^[0-9a-f]{64}$' AND terraform_binary_sha256 ~ '^[0-9a-f]{64}$' AND provider_lock_digest ~ '^[0-9a-f]{64}$'),
  CHECK (terraform_version ~ '^[0-9]+\.[0-9]+\.[0-9]+$'),
  CHECK (status = 'VERIFIED'),
  CHECK (jsonb_typeof(contract) = 'object' AND jsonb_typeof(provider_requirements) = 'object')
);

-- Verified bundles are immutable and retained: no update, no delete.
CREATE FUNCTION terraform_module_bundle_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'terraform_module_bundles rows are immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER terraform_module_bundles_immutable BEFORE UPDATE OR DELETE ON terraform_module_bundles
  FOR EACH ROW EXECUTE FUNCTION terraform_module_bundle_immutable();

CREATE TABLE terraform_resource_instances (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  application_id uuid NOT NULL REFERENCES applications(id),
  environment_id uuid NOT NULL REFERENCES environments(id),
  target_generation bigint NOT NULL CHECK (target_generation >= 0),
  descriptor text NOT NULL CHECK (descriptor<>''),
  scope_type text NOT NULL CHECK (scope_type IN ('APPLICATION','ENVIRONMENT','WORKLOAD','SHARED')),
  scope_id text NOT NULL CHECK (scope_id<>''),
  state_key text NOT NULL UNIQUE CHECK (state_key ~ '^[0-9a-f]{64}$'),
  active_resource_id uuid REFERENCES active_resources(id),
  module_bundle_id uuid NOT NULL REFERENCES terraform_module_bundles(id),
  target_ref jsonb NOT NULL, physical_intent jsonb NOT NULL,
  created_by_run_id text NOT NULL CHECK (created_by_run_id<>''),
  status text NOT NULL CHECK (status IN ('PENDING','PROVISIONING','READY','FAILED','UNREFERENCED','INTERRUPTED','NEEDS_ATTENTION','DESTROYING','DESTROYED')),
  version bigint NOT NULL CHECK (version >= 1),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  UNIQUE(organization_id, environment_id, target_generation, descriptor, scope_type, scope_id),
  UNIQUE(id, organization_id),
  UNIQUE(id, organization_id, environment_id, target_generation)
);
CREATE FUNCTION terraform_instance_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' AND (NEW.id, NEW.organization_id, NEW.application_id, NEW.environment_id, NEW.target_generation,
      NEW.descriptor, NEW.scope_type, NEW.scope_id, NEW.state_key, NEW.target_ref, NEW.physical_intent,
      NEW.created_by_run_id, NEW.created_at) IS DISTINCT FROM (OLD.id, OLD.organization_id, OLD.application_id,
      OLD.environment_id, OLD.target_generation, OLD.descriptor, OLD.scope_type, OLD.scope_id, OLD.state_key,
      OLD.target_ref, OLD.physical_intent, OLD.created_by_run_id, OLD.created_at) THEN
    RAISE EXCEPTION 'terraform resource instance identity is immutable' USING ERRCODE='23514';
  END IF;
  IF NOT (EXISTS (SELECT 1 FROM environments e JOIN applications a ON a.id=e.application_id
      WHERE e.id=NEW.environment_id AND a.id=NEW.application_id AND a.organization_id=NEW.organization_id)) THEN
    RAISE EXCEPTION 'terraform resource instance scope is inconsistent' USING ERRCODE='23514';
  END IF;
  IF NEW.active_resource_id IS NOT NULL AND NOT (EXISTS (SELECT 1 FROM active_resources r
      WHERE r.id=NEW.active_resource_id AND r.organization_id=NEW.organization_id)) THEN
    RAISE EXCEPTION 'terraform resource instance active resource is outside the Organization' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER terraform_resource_instances_guard BEFORE INSERT OR UPDATE ON terraform_resource_instances
  FOR EACH ROW EXECUTE FUNCTION terraform_instance_guard();
CREATE TRIGGER terraform_resource_instances_retained BEFORE DELETE ON terraform_resource_instances
  FOR EACH ROW EXECUTE FUNCTION terraform_retained();
ALTER TABLE environment_operations ADD CONSTRAINT environment_operations_cleanup_root_fk
  FOREIGN KEY(root_resource_instance_id) REFERENCES terraform_resource_instances(id);
CREATE FUNCTION environment_operation_cleanup_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.kind, NEW.authorization_mode, NEW.actor_ref, NEW.root_resource_instance_id, NEW.confirmed_scope,
      NEW.deletion_request_hash, NEW.confirmation_name, NEW.scope_snapshot, NEW.run_id) IS DISTINCT FROM
     (OLD.kind, OLD.authorization_mode, OLD.actor_ref, OLD.root_resource_instance_id, OLD.confirmed_scope,
      OLD.deletion_request_hash, OLD.confirmation_name, OLD.scope_snapshot, OLD.run_id) THEN
    RAISE EXCEPTION 'operation kind and admitted cleanup scope are immutable' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER environment_operations_cleanup_guard BEFORE UPDATE ON environment_operations
  FOR EACH ROW EXECUTE FUNCTION environment_operation_cleanup_guard();

-- Edges are recorded under the Environment owner before execution and kept
-- until absence proof; both ends share Organization, Environment and generation.
CREATE TABLE terraform_resource_dependencies (
  consumer_instance_id uuid NOT NULL, provider_instance_id uuid NOT NULL,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  environment_id uuid NOT NULL REFERENCES environments(id),
  target_generation bigint NOT NULL, version bigint NOT NULL DEFAULT 1,
  PRIMARY KEY(consumer_instance_id, provider_instance_id),
  CONSTRAINT terraform_resource_dependencies_distinct CHECK (consumer_instance_id <> provider_instance_id),
  CONSTRAINT terraform_resource_dependencies_consumer_scope FOREIGN KEY(consumer_instance_id, organization_id, environment_id, target_generation)
    REFERENCES terraform_resource_instances(id, organization_id, environment_id, target_generation),
  CONSTRAINT terraform_resource_dependencies_provider_scope FOREIGN KEY(provider_instance_id, organization_id, environment_id, target_generation)
    REFERENCES terraform_resource_instances(id, organization_id, environment_id, target_generation)
);
CREATE INDEX terraform_resource_dependencies_provider_idx ON terraform_resource_dependencies(provider_instance_id);
-- An edge's identity never changes and its version only advances. An edge is
-- removed only once its consumer is READY without it (an applied update) or
-- DESTROYED (absence proven); a failed, interrupted or unknown consumer keeps
-- its edges as evidence that it still holds the provider.
CREATE FUNCTION terraform_dependency_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' THEN
    IF (NEW.consumer_instance_id, NEW.provider_instance_id, NEW.organization_id, NEW.environment_id, NEW.target_generation)
       IS DISTINCT FROM (OLD.consumer_instance_id, OLD.provider_instance_id, OLD.organization_id, OLD.environment_id, OLD.target_generation)
       OR NEW.version <= OLD.version THEN
      RAISE EXCEPTION 'terraform dependency identity is immutable and its version only advances' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  END IF;
  IF NOT (EXISTS (SELECT 1 FROM terraform_resource_instances c WHERE c.id=OLD.consumer_instance_id AND c.status IN ('READY','DESTROYED'))) THEN
    RAISE EXCEPTION 'terraform dependency is retained while its consumer may still hold the provider' USING ERRCODE='23514';
  END IF;
  RETURN OLD;
END $$;
CREATE TRIGGER terraform_resource_dependencies_guard BEFORE DELETE OR UPDATE ON terraform_resource_dependencies
  FOR EACH ROW EXECUTE FUNCTION terraform_dependency_guard();

-- Capabilities, locks and attempts bind an instance to an Environment
-- operation; both must belong to the same Environment (and Organization).
CREATE FUNCTION terraform_operation_scope_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT (EXISTS (SELECT 1 FROM terraform_resource_instances i JOIN environment_operations op ON op.environment_id=i.environment_id
      WHERE i.id=NEW.resource_instance_id AND op.id=NEW.environment_operation_id)) THEN
    RAISE EXCEPTION '% instance and operation belong to different Environments', TG_TABLE_NAME USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;

-- Restricted encrypted state: AES-256-GCM ciphertext whose AAD binds the
-- Organization, state key, key ID and revision (= version). Never purged.
CREATE TABLE terraform_states (
  resource_instance_id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  ciphertext bytea NOT NULL, nonce bytea NOT NULL CHECK (length(nonce)=12),
  key_id text NOT NULL CHECK (key_id<>''), aad_revision bigint NOT NULL,
  lineage text NOT NULL CHECK (lineage<>''), serial bigint NOT NULL CHECK (serial >= 0),
  version bigint NOT NULL CHECK (version >= 1), updated_at timestamptz NOT NULL,
  CONSTRAINT terraform_states_revision CHECK (aad_revision = version),
  CONSTRAINT terraform_states_instance_fk FOREIGN KEY(resource_instance_id, organization_id)
    REFERENCES terraform_resource_instances(id, organization_id)
);
CREATE TRIGGER terraform_states_retained BEFORE DELETE ON terraform_states
  FOR EACH ROW EXECUTE FUNCTION terraform_retained();

CREATE TABLE terraform_state_locks (
  resource_instance_id uuid PRIMARY KEY REFERENCES terraform_resource_instances(id),
  lock_id text NOT NULL CHECK (lock_id<>''),
  environment_operation_id uuid NOT NULL REFERENCES environment_operations(id),
  owner text NOT NULL, fencing_generation bigint NOT NULL, lock_metadata jsonb NOT NULL,
  acquired_at timestamptz NOT NULL, heartbeat_at timestamptz NOT NULL, deadline timestamptz NOT NULL
);
CREATE INDEX terraform_state_locks_operation_idx ON terraform_state_locks(environment_operation_id);
CREATE TRIGGER terraform_state_locks_scope BEFORE INSERT OR UPDATE ON terraform_state_locks
  FOR EACH ROW EXECUTE FUNCTION terraform_operation_scope_guard();

-- Hash-only short-lived executor capabilities; never readable through an API.
CREATE TABLE terraform_executor_capabilities (
  token_hash text PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  resource_instance_id uuid NOT NULL,
  environment_operation_id uuid NOT NULL REFERENCES environment_operations(id),
  fencing_generation bigint NOT NULL,
  action text NOT NULL CHECK (action IN ('PLAN','CREATE','UPDATE','DESTROY')),
  expires_at timestamptz NOT NULL,
  CONSTRAINT terraform_executor_capabilities_instance_fk FOREIGN KEY(resource_instance_id, organization_id)
    REFERENCES terraform_resource_instances(id, organization_id)
);
CREATE INDEX terraform_executor_capabilities_operation_idx ON terraform_executor_capabilities(environment_operation_id);
CREATE TRIGGER terraform_executor_capabilities_scope BEFORE INSERT OR UPDATE ON terraform_executor_capabilities
  FOR EACH ROW EXECUTE FUNCTION terraform_operation_scope_guard();

CREATE TABLE terraform_resource_attempts (
  id uuid PRIMARY KEY,
  resource_instance_id uuid NOT NULL REFERENCES terraform_resource_instances(id),
  environment_operation_id uuid NOT NULL REFERENCES environment_operations(id),
  deployment_id uuid REFERENCES deployments(id),
  run_id text NOT NULL CHECK (run_id<>''), attempt_number integer NOT NULL CHECK (attempt_number >= 1),
  action text NOT NULL CHECK (action IN ('CREATE','UPDATE','DESTROY')),
  ownership text NOT NULL CHECK (ownership IN ('CREATED_BY_RUN','PREEXISTING','UPDATED_BY_RUN')),
  stage text NOT NULL,
  status text NOT NULL CHECK (status IN ('RUNNING','SUCCEEDED','FAILED','INTERRUPTED','NEEDS_ATTENTION')),
  module_snapshot jsonb NOT NULL, target_snapshot jsonb NOT NULL,
  observed_physical_ids jsonb NOT NULL DEFAULT '[]'::jsonb, state_serial bigint,
  safe_error_code text NOT NULL DEFAULT '', recovery_handle text,
  deadline timestamptz NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  UNIQUE(environment_operation_id, resource_instance_id, attempt_number)
);
CREATE INDEX terraform_resource_attempts_instance_idx ON terraform_resource_attempts(resource_instance_id, created_at);
CREATE FUNCTION terraform_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.resource_instance_id, NEW.environment_operation_id, NEW.deployment_id, NEW.run_id,
      NEW.attempt_number, NEW.action, NEW.ownership, NEW.module_snapshot, NEW.target_snapshot,
      NEW.deadline, NEW.created_at) IS DISTINCT FROM (OLD.id, OLD.resource_instance_id,
      OLD.environment_operation_id, OLD.deployment_id, OLD.run_id, OLD.attempt_number, OLD.action,
      OLD.ownership, OLD.module_snapshot, OLD.target_snapshot, OLD.deadline, OLD.created_at) THEN
    RAISE EXCEPTION 'terraform attempt provenance and snapshots are immutable' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER terraform_resource_attempts_guard BEFORE UPDATE ON terraform_resource_attempts
  FOR EACH ROW EXECUTE FUNCTION terraform_attempt_guard();
CREATE TRIGGER terraform_resource_attempts_scope BEFORE INSERT ON terraform_resource_attempts
  FOR EACH ROW EXECUTE FUNCTION terraform_operation_scope_guard();
CREATE TRIGGER terraform_resource_attempts_retained BEFORE DELETE ON terraform_resource_attempts
  FOR EACH ROW EXECUTE FUNCTION terraform_retained();
