-- Add optional delivery and drift state; existing Environments remain DIRECT/off.
ALTER TABLE environments
 ADD COLUMN desired_delivery jsonb NOT NULL DEFAULT '{"mode":"DIRECT","selfHeal":false}' CHECK(jsonb_typeof(desired_delivery)='object'),
 ADD COLUMN applied_delivery jsonb NOT NULL DEFAULT '{"mode":"DIRECT","selfHeal":false}' CHECK(jsonb_typeof(applied_delivery)='object'),
 ADD COLUMN delivery_config_revision bigint NOT NULL DEFAULT 0 CHECK(delivery_config_revision>=0),
 ADD COLUMN delivery_epoch bigint NOT NULL DEFAULT 0 CHECK(delivery_epoch>=0);

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
