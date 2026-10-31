-- Current schema; owner setup initializes or upgrades per ADR-027.
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
  name text NOT NULL
);

CREATE TABLE user_accounts (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  username text UNIQUE NOT NULL,
  password_hash text NOT NULL,
  role text NOT NULL CHECK (role IN ('ADMIN','PLATFORM_ENGINEER','DEVELOPER')),
  status text NOT NULL CHECK (status IN ('ACTIVE','DISABLED')),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT user_accounts_id_organization_key UNIQUE (id, organization_id)
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
  version bigint NOT NULL DEFAULT 1 CONSTRAINT connections_version_check CHECK (version >= 1),
  config jsonb NOT NULL, secret_ref text NOT NULL, status text NOT NULL,
  verification jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, connection_key),
  CONSTRAINT connections_id_organization_key UNIQUE (id, organization_id),
  CONSTRAINT connections_authentication_type_check CHECK (
    (kind='KUBERNETES' AND authentication_type='KUBECONFIG') OR
    (kind='AWS' AND authentication_type IN ('AWS_ACCESS_KEY','AWS_EC2_ROLE')))
);

-- Application is identity and current metadata only; execution and store
-- binding belong to the Environment.
-- UC-18/ADR-025: a DELETED Application is a retained tombstone that no longer
-- reserves its name or subdomain.
CREATE TABLE applications (
  id uuid PRIMARY KEY, application_key text UNIQUE NOT NULL, organization_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL, subdomain text NOT NULL, version bigint NOT NULL,
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DELETING','DELETED')),
  deletion jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(deletion)='object')
);
CREATE UNIQUE INDEX applications_organization_lower_name_key ON applications(organization_id, lower(name)) WHERE status <> 'DELETED';
CREATE UNIQUE INDEX applications_subdomain_key ON applications(subdomain) WHERE status <> 'DELETED';

CREATE TABLE secret_store_connections (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
  store_key text NOT NULL, name text NOT NULL, provider text NOT NULL CHECK (provider IN ('VAULT_KV_V2')),
  backend_address text NOT NULL, workload_address text NOT NULL, kv_mount text NOT NULL, auth_mount text NOT NULL,
  tls_ca_pem text NOT NULL DEFAULT '', credential_ref text NOT NULL DEFAULT '', status text NOT NULL CHECK (status IN ('READY','REJECTED')),
  verification jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id, store_key)
);

-- An Environment carries no deployment Connection, execution profile or region (ADR-017).
CREATE TABLE environments (
  id uuid PRIMARY KEY, application_id uuid NOT NULL REFERENCES applications(id),
  environment_key text NOT NULL, name text NOT NULL, environment_type text NOT NULL,
  namespace_identity text NOT NULL, current_deployment_set_id uuid,
  version bigint NOT NULL, draft_version bigint NOT NULL DEFAULT 0,
  public_routes_pending boolean NOT NULL DEFAULT false,
 desired_delivery jsonb NOT NULL DEFAULT '{"mode":"DIRECT","selfHeal":false}' CHECK(jsonb_typeof(desired_delivery)='object'),
 applied_delivery jsonb NOT NULL DEFAULT '{"mode":"DIRECT","selfHeal":false}' CHECK(jsonb_typeof(applied_delivery)='object'),
 delivery_config_revision bigint NOT NULL DEFAULT 0 CHECK(delivery_config_revision>=0),
 delivery_epoch bigint NOT NULL DEFAULT 0 CHECK(delivery_epoch>=0),
  desired_config_revision_id uuid,
  infrastructure_scope text NOT NULL DEFAULT 'ENVIRONMENT',
  secret_store_id uuid REFERENCES secret_store_connections(id),
  target_generation bigint NOT NULL DEFAULT 0,
  generation_high bigint NOT NULL DEFAULT 0,
  active_operation_id uuid,
  UNIQUE(application_id, environment_key), UNIQUE(application_id, namespace_identity),
  CONSTRAINT environments_infrastructure_scope_check CHECK (infrastructure_scope = 'ENVIRONMENT')
);

CREATE TABLE environment_operations (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), review_request_id uuid,
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
CREATE UNIQUE INDEX environment_operations_review_request ON environment_operations(environment_id, review_request_id)
  WHERE review_request_id IS NOT NULL;
ALTER TABLE environments ADD CONSTRAINT environments_active_operation_fk
  FOREIGN KEY(active_operation_id) REFERENCES environment_operations(id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE deployments (
  id uuid PRIMARY KEY, environment_id uuid NOT NULL REFERENCES environments(id), review_request_id uuid,
  organization_key text NOT NULL, application_key text NOT NULL, environment_key text NOT NULL,
  action text NOT NULL, workload_id text NOT NULL,
  actor_ref text NOT NULL, status text NOT NULL, base_environment_version bigint NOT NULL,
  base_deployment_set_id uuid, candidate_deployment_set_id uuid,
  failure_reason text NOT NULL DEFAULT '', started_at timestamptz NOT NULL,
  finished_at timestamptz,
  target_generation bigint NOT NULL DEFAULT 0
);
CREATE INDEX deployments_environment_started_idx ON deployments(environment_id, started_at DESC);
CREATE INDEX deployments_review_request_idx ON deployments(environment_id, review_request_id, started_at)
  WHERE review_request_id IS NOT NULL;
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
  resource_type_id uuid NOT NULL REFERENCES resource_types(id),
  definition_name text NOT NULL CONSTRAINT resource_definitions_name_check CHECK (
    btrim(definition_name) <> '' AND char_length(definition_name) <= 253 AND definition_name !~ '[[:cntrl:]]'),
  driver_type text NOT NULL, connection_id uuid REFERENCES connections(id), driver_inputs jsonb NOT NULL,
  provision_rules jsonb NOT NULL, source_fingerprint text,
  version bigint NOT NULL DEFAULT 1 CONSTRAINT resource_definitions_version_check CHECK (version >= 1),
  UNIQUE(organization_id, definition_key)
);
CREATE TABLE matching_criteria (
  id uuid PRIMARY KEY, resource_definition_id uuid NOT NULL REFERENCES resource_definitions(id) ON DELETE CASCADE,
  ordinal integer NOT NULL, env_type text, app_id text, env_id text, res_id text, class text,
  specificity_score integer NOT NULL, UNIQUE(resource_definition_id, ordinal)
);
CREATE TABLE active_resources (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), descriptor text NOT NULL,
  scope_type text NOT NULL, scope_id text NOT NULL, resource_definition_id uuid REFERENCES resource_definitions(id),
  catalog_source text, catalog_tag text, catalog_definition_key text,
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
  manifest_review jsonb,
  PRIMARY KEY(deployment_id, workload_id)
);

-- UC-17 baseline versions: append-only (the Console has SELECT/INSERT only).
-- NULL application_id is the Organization scope.
CREATE TABLE effectiveness_baselines (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  application_id uuid REFERENCES applications(id),
  metric_id text NOT NULL CHECK (metric_id IN ('work_duration')),
  version bigint NOT NULL CHECK (version >= 1),
  retired boolean NOT NULL DEFAULT false,
  statistic text CHECK (statistic IN ('MEDIAN')),
  value_seconds bigint CHECK (value_seconds > 0),
  source text NOT NULL CHECK (btrim(source) <> '' AND char_length(source) <= 2000),
  sample_size integer CHECK (sample_size > 0),
  measured_from date, measured_to date,
  effective_from date NOT NULL,
  created_by uuid NOT NULL REFERENCES user_accounts(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((retired AND statistic IS NULL AND value_seconds IS NULL) OR (NOT retired AND statistic IS NOT NULL AND value_seconds IS NOT NULL)),
  CHECK ((measured_from IS NULL) = (measured_to IS NULL) AND (measured_from IS NULL OR measured_from <= measured_to))
);
CREATE UNIQUE INDEX effectiveness_baselines_scope_version ON effectiveness_baselines(organization_id, metric_id, COALESCE(application_id, '00000000-0000-0000-0000-000000000000'::uuid), version);

-- Verified Git render bundles (ADR-021): Organization scoped, insert-only and
-- immutable (the Console has SELECT/INSERT only; the trigger refuses UPDATE
-- and DELETE). Availability is computed from the cache, never stored here.
CREATE TABLE render_bundles (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  source_kind text NOT NULL DEFAULT 'git' CHECK (source_kind IN ('git','zip')),
  source_url text NOT NULL CHECK (length(source_url) <= 2048),
  CHECK ((source_kind='git' AND source_url ~ '^https://') OR (source_kind='zip' AND source_url='zip:sha256:' || source_revision AND source_revision ~ '^[0-9a-f]{64}$')),
  source_path text NOT NULL CHECK (source_path = '.' OR (source_path !~ '(^|/)\.\.?(/|$)' AND source_path !~ '^/' AND length(source_path) <= 512)),
  source_revision text NOT NULL CHECK (source_revision ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'),
  content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
  engine_contract text NOT NULL CHECK (engine_contract = 'git-render-v1'),
  renderer_version text NOT NULL CHECK (renderer_version = '0.15.0'),
  renderer_binary_sha256 text NOT NULL CHECK (renderer_binary_sha256 ~ '^[0-9a-f]{64}$'),
  contract jsonb NOT NULL CHECK (jsonb_typeof(contract) = 'object'),
  created_at timestamptz NOT NULL,
  UNIQUE(organization_id, source_url, source_path, source_revision, engine_contract)
);
CREATE FUNCTION render_bundle_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'render_bundles rows are immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER render_bundles_immutable BEFORE UPDATE OR DELETE ON render_bundles
  FOR EACH ROW EXECUTE FUNCTION render_bundle_immutable();

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


-- ADR-023: received application telemetry is distinct from orchestration state.
CREATE TABLE telemetry_sources (
 source_key text PRIMARY KEY CHECK(source_key ~ '^[0-9a-f]{64}$'),
 environment_id uuid NOT NULL REFERENCES environments(id),
 target_generation bigint NOT NULL CHECK(target_generation >= 0),
 workload_id text NOT NULL,
 target_hash text NOT NULL CHECK(target_hash ~ '^[0-9a-f]{64}$'),
 checkpoints jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(checkpoints)='object'),
 lease_owner text NOT NULL DEFAULT '', fence bigint NOT NULL DEFAULT 0 CHECK(fence >= 0),
 lease_until timestamptz NOT NULL, logs_code text NOT NULL, metrics_code text NOT NULL,
 observed_at timestamptz, last_log_at timestamptz, last_metric_at timestamptz,
 limited boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(environment_id,target_generation,workload_id,target_hash)
);
CREATE TABLE telemetry_logs (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{64}$'),
 source_key text NOT NULL REFERENCES telemetry_sources(source_key),
 timestamp timestamptz NOT NULL, pod_uid text NOT NULL, pod_name text NOT NULL,
 container_name text NOT NULL, restart_index bigint NOT NULL CHECK(restart_index >= 0),
 level text NOT NULL CHECK(level IN ('ERROR','WARN','INFO','DEBUG','UNKNOWN')),
 message text NOT NULL CHECK(octet_length(message) <= 16384)
);
CREATE INDEX telemetry_logs_source_time ON telemetry_logs(source_key,timestamp DESC,id DESC);
CREATE INDEX telemetry_logs_expiry ON telemetry_logs(timestamp);
CREATE INDEX telemetry_logs_level_time ON telemetry_logs(source_key,level,timestamp DESC);
CREATE TABLE telemetry_samples (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{64}$'),
 source_key text NOT NULL REFERENCES telemetry_sources(source_key),
 observed_at timestamptz NOT NULL, metrics_at timestamptz,
 cpu_milli double precision CHECK(cpu_milli >= 0 AND cpu_milli < 'Infinity'::float8),
 memory_bytes double precision CHECK(memory_bytes >= 0 AND memory_bytes < 'Infinity'::float8),
 cpu_limit_milli double precision CHECK(cpu_limit_milli >= 0 AND cpu_limit_milli < 'Infinity'::float8),
 memory_limit_bytes double precision CHECK(memory_limit_bytes >= 0 AND memory_limit_bytes < 'Infinity'::float8),
 ready_pods bigint NOT NULL CHECK(ready_pods >= 0),
 total_pods bigint NOT NULL CHECK(total_pods >= ready_pods), restarts bigint NOT NULL CHECK(restarts >= 0)
);
CREATE INDEX telemetry_samples_source_time ON telemetry_samples(source_key,observed_at DESC);
CREATE INDEX telemetry_samples_expiry ON telemetry_samples(observed_at);
CREATE TABLE telemetry_events (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{64}$'),
 source_key text NOT NULL REFERENCES telemetry_sources(source_key),
 timestamp timestamptz NOT NULL, pod_uid text NOT NULL, pod_name text NOT NULL, container_name text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('RESTART','WAITING','FAILED')),
 reason text NOT NULL CHECK(reason IN ('OOMKilled','Error','Completed','CrashLoopBackOff','ImagePullBackOff','ErrImagePull','CreateContainerConfigError','Failed','Unknown')),
 occurrences bigint NOT NULL CHECK(occurrences > 0)
);
CREATE INDEX telemetry_events_source_time ON telemetry_events(source_key,timestamp DESC,id DESC);
CREATE INDEX telemetry_events_expiry ON telemetry_events(timestamp);

-- Infrastructure Account diagnostic reports (ADR-022): a separate
-- Organization/account aggregate. History is never stored in
-- connections.verification and never changes connections.version. Terminal
-- scope and findings are immutable (trigger below); the Console updates
-- lifecycle, ownership and cleanup facts only.
CREATE TABLE account_diagnostic_reports (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  account_id uuid NOT NULL,
  created_by uuid NOT NULL,
  account_version bigint NOT NULL CHECK (account_version >= 1),
  purpose text NOT NULL CHECK (purpose IN ('GENERAL','USE_CASE')),
  mode text NOT NULL CHECK (mode IN ('CONDITIONS','EXECUTION')),
  scope jsonb NOT NULL CHECK (jsonb_typeof(scope) = 'object' AND octet_length(scope::text) <= 8192),
  status text NOT NULL CHECK (status IN ('QUEUED','RUNNING','CANCELLING','COMPLETED','CANCELLED','INTERRUPTED')),
  revision bigint NOT NULL CHECK (revision >= 1),
  owner text CHECK (owner IS NULL OR (owner <> '' AND char_length(owner) <= 128)),
  lease_expires_at timestamptz, heartbeat_at timestamptz,
  findings jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(findings) = 'array' AND octet_length(findings::text) <= 262144),
  completed_checks integer NOT NULL DEFAULT 0 CHECK (completed_checks >= 0),
  total_checks integer NOT NULL DEFAULT 0 CHECK (total_checks >= completed_checks),
  probe_resources jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(probe_resources) = 'array' AND octet_length(probe_resources::text) <= 16384),
  cleanup_status text NOT NULL CHECK (cleanup_status IN ('NOT_REQUIRED','PENDING','VERIFIED','UNKNOWN')),
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  started_at timestamptz, finished_at timestamptz,
  CONSTRAINT account_diagnostic_reports_account_fk FOREIGN KEY (account_id, organization_id) REFERENCES connections(id, organization_id),
  CONSTRAINT account_diagnostic_reports_author_fk FOREIGN KEY (created_by, organization_id) REFERENCES user_accounts(id, organization_id),
  CONSTRAINT account_diagnostic_reports_general_scope CHECK (purpose <> 'GENERAL' OR (mode = 'CONDITIONS' AND NOT (scope ? 'context') AND NOT (scope ? 'useCases')))
);
CREATE INDEX account_diagnostic_reports_history ON account_diagnostic_reports(organization_id, account_id, created_at DESC, id);
CREATE UNIQUE INDEX account_diagnostic_reports_active ON account_diagnostic_reports(account_id) WHERE status IN ('QUEUED','RUNNING','CANCELLING');
CREATE FUNCTION account_diagnostic_report_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.organization_id, NEW.account_id, NEW.created_by, NEW.account_version, NEW.purpose, NEW.mode, NEW.scope, NEW.created_at)
     IS DISTINCT FROM (OLD.id, OLD.organization_id, OLD.account_id, OLD.created_by, OLD.account_version, OLD.purpose, OLD.mode, OLD.scope, OLD.created_at) THEN
    RAISE EXCEPTION 'diagnostic report identity and scope are immutable' USING ERRCODE='23514';
  END IF;
  IF OLD.status IN ('COMPLETED','CANCELLED','INTERRUPTED') AND
     (NEW.status, NEW.findings, NEW.completed_checks, NEW.total_checks, NEW.finished_at, NEW.started_at)
     IS DISTINCT FROM (OLD.status, OLD.findings, OLD.completed_checks, OLD.total_checks, OLD.finished_at, OLD.started_at) THEN
    RAISE EXCEPTION 'terminal diagnostic report findings are immutable' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER account_diagnostic_reports_guard BEFORE UPDATE ON account_diagnostic_reports
  FOR EACH ROW EXECUTE FUNCTION account_diagnostic_report_guard();

CREATE TABLE schema_upgrade_history (
  version bigint PRIMARY KEY,
  migration_sha256 text NOT NULL CHECK (migration_sha256 ~ '^[0-9a-f]{64}$'),
  source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE platform_git_settings (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    url text NOT NULL,
    username text NOT NULL DEFAULT '',
    authenticated boolean NOT NULL DEFAULT false,
    ca_pem text NOT NULL DEFAULT '',
    credential_ref text NOT NULL DEFAULT '',
    version bigint NOT NULL CHECK (version > 0),
    updated_at timestamptz NOT NULL,
    updated_by text NOT NULL,
    CHECK ((authenticated AND credential_ref <> '' AND username <> '') OR
           (NOT authenticated AND credential_ref = '' AND username = ''))
);
-- ADR-030 immutable CD binding versions. Provider verification is independently CAS guarded.
CREATE TABLE delivery_bindings (
 id uuid NOT NULL, version bigint NOT NULL CHECK(version>=1),
 organization_id uuid NOT NULL REFERENCES organizations(id), name text NOT NULL CHECK(name<>''),
 provider_kind text NOT NULL CHECK(provider_kind<>''), contract_version text NOT NULL CHECK(contract_version<>''),
 management_account_id uuid NOT NULL REFERENCES connections(id), management_account_version bigint NOT NULL CHECK(management_account_version>=1),
 config jsonb NOT NULL CHECK(jsonb_typeof(config)='object'), writer_credential_ref text NOT NULL DEFAULT '',
 capabilities jsonb NOT NULL CHECK(jsonb_typeof(capabilities)='object'),
 status text NOT NULL CHECK(status IN ('UNVERIFIED','READY','REJECTED','DISABLED')),
 revision bigint NOT NULL CHECK(revision>=1), verified_at timestamptz, reason_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL,
 PRIMARY KEY(id,version), UNIQUE(organization_id,id,version),
 CHECK(status<>'READY' OR verified_at IS NOT NULL)
);
CREATE FUNCTION delivery_binding_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT (EXISTS(SELECT 1 FROM connections c WHERE c.id=NEW.management_account_id AND c.organization_id=NEW.organization_id)) THEN
  RAISE EXCEPTION 'delivery binding account scope mismatch';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.status<>'UNVERIFIED' OR NEW.revision<>1 OR NEW.verified_at IS NOT NULL THEN RAISE EXCEPTION 'invalid initial delivery binding'; END IF;
  IF NOT (EXISTS(SELECT 1 FROM connections c WHERE c.id=NEW.management_account_id AND c.version=NEW.management_account_version)) THEN RAISE EXCEPTION 'delivery binding account version mismatch'; END IF;
  IF NEW.version>1 AND NOT EXISTS(SELECT 1 FROM delivery_bindings b WHERE b.id=NEW.id AND b.organization_id=NEW.organization_id AND b.version=NEW.version-1) THEN RAISE EXCEPTION 'delivery binding predecessor missing'; END IF;
 ELSE
  IF ROW(NEW.id,NEW.version,NEW.organization_id,NEW.name,NEW.provider_kind,NEW.contract_version,NEW.management_account_id,NEW.management_account_version,NEW.config,NEW.writer_credential_ref,NEW.created_at)
   IS DISTINCT FROM ROW(OLD.id,OLD.version,OLD.organization_id,OLD.name,OLD.provider_kind,OLD.contract_version,OLD.management_account_id,OLD.management_account_version,OLD.config,OLD.writer_credential_ref,OLD.created_at) THEN RAISE EXCEPTION 'immutable delivery binding configuration'; END IF;
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'delivery binding revision mismatch'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER delivery_bindings_guard BEFORE INSERT OR UPDATE ON delivery_bindings FOR EACH ROW EXECUTE FUNCTION delivery_binding_guard();

CREATE TABLE reconciliation_private_artifacts (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), owner_id uuid NOT NULL,
 purpose text NOT NULL CHECK(purpose IN ('DELIVERY_PAYLOAD','TERRAFORM_BASELINE','TERRAFORM_REVIEW')),
 key_id text NOT NULL, nonce bytea NOT NULL, ciphertext bytea NOT NULL,
 aad_version bigint NOT NULL CHECK(aad_version=1), created_at timestamptz NOT NULL,
 UNIQUE(organization_id,owner_id,id),
 CHECK(octet_length(nonce)=12 AND octet_length(ciphertext)>=16)
);
CREATE TABLE delivery_units (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), environment_id uuid NOT NULL REFERENCES environments(id),
 target_generation bigint NOT NULL CHECK(target_generation>=0), delivery_epoch bigint NOT NULL CHECK(delivery_epoch>=0),
 unit_key text NOT NULL, workload_id text NOT NULL, phase text NOT NULL CHECK(phase IN ('PREREQUISITE','JOB','ROLLOUT','ROUTE')),
 pending_revision_id uuid, successful_revision_id uuid, target_ref jsonb NOT NULL,
 binding_id uuid, binding_version bigint, version bigint NOT NULL CHECK(version>=1),
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 UNIQUE(environment_id,target_generation,delivery_epoch,unit_key),
 FOREIGN KEY(binding_id,binding_version) REFERENCES delivery_bindings(id,version),
 CHECK((binding_id IS NULL)=(binding_version IS NULL))
);
CREATE TABLE delivery_revisions (
 id uuid PRIMARY KEY, unit_id uuid NOT NULL REFERENCES delivery_units(id), organization_id uuid NOT NULL REFERENCES organizations(id),
 environment_operation_id uuid NOT NULL REFERENCES environment_operations(id),
 artifact_id uuid NOT NULL,
 FOREIGN KEY(organization_id,unit_id,artifact_id) REFERENCES reconciliation_private_artifacts(organization_id,owner_id,id),
 payload_digest text NOT NULL CHECK(payload_digest ~ '^[0-9a-f]{64}$'), selection jsonb NOT NULL,
 created_at timestamptz NOT NULL, UNIQUE(unit_id,id)
);
ALTER TABLE delivery_units ADD CONSTRAINT delivery_units_pending_revision_fk FOREIGN KEY(id,pending_revision_id) REFERENCES delivery_revisions(unit_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE delivery_units ADD CONSTRAINT delivery_units_successful_revision_fk FOREIGN KEY(id,successful_revision_id) REFERENCES delivery_revisions(unit_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE delivery_intents (
 id uuid PRIMARY KEY, unit_id uuid NOT NULL REFERENCES delivery_units(id), revision_id uuid NOT NULL,
 environment_operation_id uuid NOT NULL REFERENCES environment_operations(id),
 action text NOT NULL CHECK(action IN ('SUBMIT','RECONCILE','WITHDRAW','ADOPT')),
 status text NOT NULL CHECK(status IN ('PENDING','RUNNING','ACKNOWLEDGED','FAILED','OUTCOME_UNKNOWN')),
 request_id text NOT NULL, request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 owner text NOT NULL, fence bigint NOT NULL CHECK(fence>=1), version bigint NOT NULL CHECK(version>=1),
 receipt jsonb NOT NULL DEFAULT '{}'::jsonb, inventory jsonb NOT NULL DEFAULT '[]'::jsonb,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 FOREIGN KEY(unit_id,revision_id) REFERENCES delivery_revisions(unit_id,id),
 UNIQUE(environment_operation_id,revision_id,action,request_id)
);
CREATE INDEX delivery_units_environment ON delivery_units(environment_id);
CREATE INDEX delivery_intents_recovery ON delivery_intents(status,updated_at);
CREATE FUNCTION delivery_immutable_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable delivery artifact or revision'; END $$;
CREATE TRIGGER delivery_revisions_immutable BEFORE UPDATE OR DELETE ON delivery_revisions FOR EACH ROW EXECUTE FUNCTION delivery_immutable_guard();
CREATE TABLE git_publication_leases (
 destination_digest text PRIMARY KEY CHECK(destination_digest ~ '^[0-9a-f]{64}$'), organization_id uuid NOT NULL REFERENCES organizations(id),
 environment_operation_id uuid NOT NULL REFERENCES environment_operations(id), owner text NOT NULL, fence bigint NOT NULL CHECK(fence>=1),
 acquired_at timestamptz NOT NULL DEFAULT now()
);

-- Read-only delivery observations have independent expiring leases; they never
-- acquire or take over a mutating Environment/intent owner.
CREATE TABLE delivery_poll_leases (
 unit_id uuid PRIMARY KEY REFERENCES delivery_units(id), intent_id uuid NOT NULL REFERENCES delivery_intents(id),
 owner text NOT NULL, fence bigint NOT NULL CHECK(fence>0), lease_until timestamptz NOT NULL,
 next_poll_at timestamptz NOT NULL, last_sequence bigint NOT NULL DEFAULT 0 CHECK(last_sequence>=0)
);
CREATE TABLE delivery_observations (
 id uuid PRIMARY KEY, unit_id uuid NOT NULL REFERENCES delivery_units(id), revision_id uuid NOT NULL,
 sequence bigint NOT NULL CHECK(sequence>0), observation jsonb NOT NULL CHECK(jsonb_typeof(observation)='object'),
 reason_code text NOT NULL, observed_at timestamptz NOT NULL, stored_at timestamptz NOT NULL,
 FOREIGN KEY(unit_id,revision_id) REFERENCES delivery_revisions(unit_id,id), UNIQUE(unit_id,sequence)
);
CREATE INDEX delivery_observations_latest ON delivery_observations(unit_id,sequence DESC);
CREATE TRIGGER delivery_observations_immutable BEFORE UPDATE OR DELETE ON delivery_observations FOR EACH ROW EXECUTE FUNCTION delivery_immutable_guard();

CREATE TABLE terraform_execution_baselines (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
 environment_id uuid NOT NULL REFERENCES environments(id),
 resource_instance_id uuid NOT NULL, execution_attempt_id uuid NOT NULL REFERENCES terraform_resource_attempts(id),
 module_bundle_id uuid NOT NULL REFERENCES terraform_module_bundles(id),
 instance_version bigint NOT NULL CHECK(instance_version>0), state_version bigint NOT NULL CHECK(state_version>0),
 lineage text NOT NULL CHECK(lineage<>''), serial bigint NOT NULL CHECK(serial>=0),
 private_artifact_ref uuid NOT NULL, dependencies jsonb NOT NULL CHECK(jsonb_typeof(dependencies)='array'),
 created_at timestamptz NOT NULL,
 UNIQUE(resource_instance_id,execution_attempt_id),
 FOREIGN KEY(resource_instance_id,organization_id) REFERENCES terraform_resource_instances(id,organization_id),
 FOREIGN KEY(organization_id,resource_instance_id,private_artifact_ref) REFERENCES reconciliation_private_artifacts(organization_id,owner_id,id)
);
CREATE INDEX terraform_execution_baselines_latest ON terraform_execution_baselines(organization_id,resource_instance_id,created_at DESC,id DESC);
CREATE FUNCTION terraform_baseline_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM terraform_resource_instances i
   JOIN terraform_resource_attempts a ON a.resource_instance_id=i.id
   JOIN terraform_states st ON st.resource_instance_id=i.id
   JOIN reconciliation_private_artifacts p ON p.id=NEW.private_artifact_ref
   WHERE i.id=NEW.resource_instance_id AND i.organization_id=NEW.organization_id AND i.environment_id=NEW.environment_id
   AND i.module_bundle_id=NEW.module_bundle_id AND i.version=NEW.instance_version AND i.status IN ('READY','UNREFERENCED')
   AND a.id=NEW.execution_attempt_id AND a.status='SUCCEEDED'
   AND st.version=NEW.state_version AND st.lineage=NEW.lineage AND st.serial=NEW.serial
   AND p.organization_id=i.organization_id AND p.owner_id=i.id AND p.purpose='TERRAFORM_BASELINE')
 THEN RAISE EXCEPTION 'invalid acknowledged Terraform execution baseline' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER terraform_execution_baselines_guard BEFORE INSERT ON terraform_execution_baselines FOR EACH ROW EXECUTE FUNCTION terraform_baseline_guard();
CREATE TRIGGER terraform_execution_baselines_immutable BEFORE UPDATE OR DELETE ON terraform_execution_baselines FOR EACH ROW EXECUTE FUNCTION delivery_immutable_guard();

CREATE TABLE drift_scans (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id),
 environment_id uuid NOT NULL REFERENCES environments(id), operation_id uuid REFERENCES environment_operations(id),
 request_id uuid NOT NULL, request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 environment_version bigint NOT NULL CHECK(environment_version>0),
 status text NOT NULL CHECK(status IN ('QUEUED','RUNNING','SUCCEEDED','FAILED','INTERRUPTED','SKIPPED')),
 result text NOT NULL CHECK(result IN ('IN_SYNC','DRIFTED','UNKNOWN','BASELINE_UNAVAILABLE','NOT_APPLICABLE')),
 scope jsonb NOT NULL CHECK(jsonb_typeof(scope)='array'), findings jsonb NOT NULL CHECK(jsonb_typeof(findings)='array'),
 owner text NOT NULL DEFAULT '', fence bigint NOT NULL DEFAULT 0 CHECK(fence>=0), version bigint NOT NULL CHECK(version>0),
 started_at timestamptz, finished_at timestamptz, created_at timestamptz NOT NULL, reason_code text NOT NULL DEFAULT '',
 UNIQUE(environment_id,request_id), UNIQUE(id,organization_id,environment_id),
 CHECK((status='QUEUED' AND operation_id IS NULL AND fence=0 AND owner='') OR status='SKIPPED' OR (operation_id IS NOT NULL AND fence>0 AND owner<>''))
);
CREATE UNIQUE INDEX drift_scans_one_active ON drift_scans(environment_id) WHERE status IN ('QUEUED','RUNNING');
CREATE INDEX drift_scans_history ON drift_scans(organization_id,environment_id,created_at DESC,id DESC);
CREATE FUNCTION drift_scan_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM environments e JOIN applications a ON a.id=e.application_id WHERE e.id=NEW.environment_id AND a.organization_id=NEW.organization_id)
 THEN RAISE EXCEPTION 'drift scan scope mismatch' USING ERRCODE='23514'; END IF;
 IF NEW.operation_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM environment_operations op WHERE op.id=NEW.operation_id AND op.environment_id=NEW.environment_id)
 THEN RAISE EXCEPTION 'drift scan operation mismatch' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.status NOT IN ('QUEUED','RUNNING') OR ROW(NEW.id,NEW.organization_id,NEW.environment_id,NEW.request_id,NEW.request_digest,NEW.environment_version,NEW.scope,NEW.created_at)
    IS DISTINCT FROM ROW(OLD.id,OLD.organization_id,OLD.environment_id,OLD.request_id,OLD.request_digest,OLD.environment_version,OLD.scope,OLD.created_at)
  THEN RAISE EXCEPTION 'immutable drift scan evidence' USING ERRCODE='23514'; END IF;
  IF NEW.version<>OLD.version+1 OR (OLD.status='QUEUED' AND NEW.status NOT IN ('RUNNING','SKIPPED')) OR (OLD.status='RUNNING' AND NEW.status NOT IN ('SUCCEEDED','FAILED','INTERRUPTED'))
  THEN RAISE EXCEPTION 'invalid drift scan transition' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER drift_scans_guard BEFORE INSERT OR UPDATE ON drift_scans FOR EACH ROW EXECUTE FUNCTION drift_scan_guard();

CREATE TABLE drift_repair_reviews (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), environment_id uuid NOT NULL REFERENCES environments(id),
 scan_id uuid NOT NULL, actor_id uuid NOT NULL, request_id uuid NOT NULL,
 request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 operation_id uuid NOT NULL REFERENCES environment_operations(id),
 status text NOT NULL CHECK(status IN ('PREPARING','READY','BLOCKED','EXPIRED','CONSUMED','FAILED')),
 safe_review jsonb NOT NULL CHECK(jsonb_typeof(safe_review)='object'), private_refs jsonb NOT NULL CHECK(jsonb_typeof(private_refs)='object'),
 token_hash text NOT NULL CHECK(token_hash ~ '^[0-9a-f]{64}$'), token_cipher jsonb NOT NULL CHECK(jsonb_typeof(token_cipher)='object'),
 environment_version bigint NOT NULL CHECK(environment_version>0), version bigint NOT NULL CHECK(version>0),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL,
 consumed_operation_id uuid REFERENCES environment_operations(id), confirmation_request_id uuid,
 FOREIGN KEY(scan_id,organization_id,environment_id) REFERENCES drift_scans(id,organization_id,environment_id),
 FOREIGN KEY(actor_id,organization_id) REFERENCES user_accounts(id,organization_id),
 UNIQUE(environment_id,actor_id,request_id),
 CHECK(safe_review->>'id'=id::text AND safe_review->>'status'=status)
);
CREATE FUNCTION drift_repair_review_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM environment_operations op WHERE op.id=NEW.operation_id AND op.environment_id=NEW.environment_id AND op.kind='DRIFT_REPAIR')
 THEN RAISE EXCEPTION 'repair preparation operation mismatch' USING ERRCODE='23514'; END IF;
 IF NEW.consumed_operation_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM environment_operations op WHERE op.id=NEW.consumed_operation_id AND op.environment_id=NEW.environment_id AND op.kind='DRIFT_REPAIR')
 THEN RAISE EXCEPTION 'repair confirmation operation mismatch' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF ROW(NEW.id,NEW.organization_id,NEW.environment_id,NEW.scan_id,NEW.actor_id,NEW.request_id,NEW.request_digest,NEW.operation_id,NEW.token_hash,NEW.token_cipher,NEW.environment_version,NEW.expires_at,NEW.created_at)
   IS DISTINCT FROM ROW(OLD.id,OLD.organization_id,OLD.environment_id,OLD.scan_id,OLD.actor_id,OLD.request_id,OLD.request_digest,OLD.operation_id,OLD.token_hash,OLD.token_cipher,OLD.environment_version,OLD.expires_at,OLD.created_at)
  THEN RAISE EXCEPTION 'immutable repair review pins' USING ERRCODE='23514'; END IF;
  IF NEW.version<>OLD.version+1 OR NOT ((OLD.status='PREPARING' AND NEW.status IN ('READY','BLOCKED','FAILED')) OR (OLD.status='READY' AND NEW.status='CONSUMED'))
  THEN RAISE EXCEPTION 'invalid repair review transition' USING ERRCODE='23514'; END IF;
  IF OLD.status='READY' AND NEW.private_refs IS DISTINCT FROM OLD.private_refs THEN RAISE EXCEPTION 'immutable reviewed plan refs' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER drift_repair_reviews_guard BEFORE INSERT OR UPDATE ON drift_repair_reviews FOR EACH ROW EXECUTE FUNCTION drift_repair_review_guard();

-- Runtime roles get neither EXECUTE nor DELETE. Owner retention is separate.
CREATE FUNCTION drift_repair_artifact_expired(artifact_id uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
  SELECT 1 FROM reconciliation_private_artifacts p
  JOIN terraform_resource_instances i ON i.id=p.owner_id AND i.organization_id=p.organization_id
  JOIN environments e ON e.id=i.environment_id
  WHERE p.id=artifact_id AND p.purpose='TERRAFORM_REVIEW' AND p.created_at<now()-interval '24 hours'
  AND i.status IN ('READY','UNREFERENCED','DESTROYED') AND e.active_operation_id IS NULL
  AND NOT EXISTS(SELECT 1 FROM terraform_state_locks l WHERE l.resource_instance_id=i.id)
  AND NOT EXISTS(SELECT 1 FROM terraform_resource_attempts a WHERE a.resource_instance_id=i.id AND a.status IN ('RUNNING','INTERRUPTED','NEEDS_ATTENTION'))
  AND NOT EXISTS(SELECT 1 FROM drift_repair_reviews r
    JOIN environment_operations o ON o.id=r.operation_id
    LEFT JOIN environment_operations c ON c.id=r.consumed_operation_id
    WHERE r.environment_id=e.id AND (r.status='PREPARING' OR r.expires_at>=now()-interval '24 hours'
      OR o.status NOT IN ('SUCCEEDED','FAILED','RECOVERED')
      OR (r.consumed_operation_id IS NOT NULL AND c.status<>'SUCCEEDED')))
 );
$$;
CREATE FUNCTION reconciliation_artifact_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND OLD.purpose='TERRAFORM_REVIEW'
 AND session_user=current_user AND EXISTS(SELECT 1 FROM pg_namespace n JOIN pg_roles r ON r.rolname=current_user WHERE n.nspname=current_schema() AND NOT r.rolsuper AND (n.nspowner=r.oid OR (n.nspowner='pg_database_owner'::regrole AND EXISTS(SELECT 1 FROM pg_database d WHERE d.datname=current_database() AND d.datdba=r.oid))))
 THEN
  IF drift_repair_artifact_expired(OLD.id) THEN RETURN OLD; END IF;
 END IF;
 RAISE EXCEPTION 'immutable reconciliation artifact';
END $$;
CREATE TRIGGER reconciliation_private_artifacts_immutable BEFORE UPDATE OR DELETE ON reconciliation_private_artifacts FOR EACH ROW EXECUTE FUNCTION reconciliation_artifact_guard();

CREATE TABLE catalog_sources (
 organization_id uuid NOT NULL REFERENCES organizations(id),
 source_key text NOT NULL,
 version bigint NOT NULL CHECK(version > 0),
 document jsonb NOT NULL CHECK(jsonb_typeof(document) = 'object'),
 PRIMARY KEY(organization_id, source_key)
);
CREATE TABLE catalog_releases (
 organization_id uuid NOT NULL,
 source_key text NOT NULL,
 tag text NOT NULL,
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 document jsonb NOT NULL CHECK(jsonb_typeof(document) = 'object'),
 PRIMARY KEY(organization_id, source_key, tag),
 FOREIGN KEY(organization_id, source_key) REFERENCES catalog_sources(organization_id, source_key)
);
CREATE FUNCTION catalog_release_immutable_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'catalog releases are immutable' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER catalog_release_immutable BEFORE UPDATE OR DELETE ON catalog_releases
 FOR EACH ROW EXECUTE FUNCTION catalog_release_immutable_guard();
CREATE TABLE environment_catalog_bindings (
 environment_id uuid PRIMARY KEY REFERENCES environments(id),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 desired_source text NOT NULL,
 desired_tag text NOT NULL,
 applied_source text,
 applied_tag text,
 revision bigint NOT NULL CHECK(revision>0),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 FOREIGN KEY(organization_id,desired_source,desired_tag) REFERENCES catalog_releases(organization_id,source_key,tag),
 FOREIGN KEY(organization_id,applied_source,applied_tag) REFERENCES catalog_releases(organization_id,source_key,tag),
 CHECK ((applied_source IS NULL) = (applied_tag IS NULL))
);
ALTER TABLE active_resources ADD CONSTRAINT active_resources_catalog_release_fk
 FOREIGN KEY(organization_id,catalog_source,catalog_tag) REFERENCES catalog_releases(organization_id,source_key,tag);
ALTER TABLE active_resources ADD CONSTRAINT active_resources_definition_provenance_check CHECK (
 (resource_definition_id IS NOT NULL AND catalog_source IS NULL AND catalog_tag IS NULL AND catalog_definition_key IS NULL)
 OR (resource_definition_id IS NULL AND catalog_source IS NOT NULL AND catalog_tag IS NOT NULL AND catalog_definition_key IS NOT NULL));
