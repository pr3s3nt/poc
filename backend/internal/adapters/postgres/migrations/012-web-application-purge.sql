-- Issue #94. The owner must opt the installation in. Runtime gets no DDL
-- privilege and can only invoke the single, identity-scoped maintenance routine.
CREATE TABLE application_purge_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false
);
REVOKE ALL ON application_purge_policy FROM PUBLIC;

CREATE FUNCTION application_purge_scope_matches(app_id uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT app_id::text = current_setting('orchestrator.application_purge_id', true)
 AND EXISTS(
  SELECT 1 FROM pg_namespace n JOIN pg_roles r ON r.rolname=current_user
  WHERE n.nspname=current_schema() AND NOT r.rolsuper AND (
   n.nspowner=r.oid OR (n.nspowner='pg_database_owner'::regrole AND
    EXISTS(SELECT 1 FROM pg_database d WHERE d.datname=current_database() AND d.datdba=r.oid)))
 )
$$;
REVOKE ALL ON FUNCTION application_purge_scope_matches(uuid) FROM PUBLIC;

CREATE OR REPLACE FUNCTION terraform_retained() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE app_id uuid;
BEGIN
  IF TG_TABLE_NAME='terraform_resource_instances' THEN
    app_id := OLD.application_id;
  ELSE
    SELECT application_id INTO app_id FROM terraform_resource_instances WHERE id=OLD.resource_instance_id;
  END IF;
  IF COALESCE(current_setting('orchestrator.application_purge_id',true),'')<>''
     AND has_schema_privilege(current_user,current_schema(),'CREATE') THEN
   IF application_purge_scope_matches(app_id) THEN RETURN OLD; END IF;
  END IF;
  RAISE EXCEPTION '% rows are retained', TG_TABLE_NAME USING ERRCODE='23514';
END $$;

CREATE OR REPLACE FUNCTION terraform_dependency_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' THEN
    IF (NEW.consumer_instance_id, NEW.provider_instance_id, NEW.organization_id, NEW.environment_id, NEW.target_generation)
       IS DISTINCT FROM (OLD.consumer_instance_id, OLD.provider_instance_id, OLD.organization_id, OLD.environment_id, OLD.target_generation)
       OR NEW.version <= OLD.version THEN
      RAISE EXCEPTION 'terraform dependency identity is immutable and its version only advances' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  END IF;
  IF COALESCE(current_setting('orchestrator.application_purge_id',true),'')<>''
     AND has_schema_privilege(current_user,current_schema(),'CREATE') THEN
   IF application_purge_scope_matches((SELECT application_id FROM terraform_resource_instances WHERE id=OLD.consumer_instance_id)) THEN
    RETURN OLD;
   END IF;
  END IF;
  IF NOT (EXISTS (SELECT 1 FROM terraform_resource_instances c WHERE c.id=OLD.consumer_instance_id AND c.status IN ('READY','DESTROYED'))) THEN
    RAISE EXCEPTION 'terraform dependency is retained while its consumer may still hold the provider' USING ERRCODE='23514';
  END IF;
  RETURN OLD;
END $$;

CREATE OR REPLACE FUNCTION delivery_immutable_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE app_id uuid;
BEGIN
 IF TG_OP='DELETE' THEN
  IF TG_TABLE_NAME='terraform_execution_baselines' THEN
   SELECT application_id INTO app_id FROM terraform_resource_instances WHERE id=OLD.resource_instance_id;
  ELSE
   SELECT e.application_id INTO app_id FROM delivery_units u JOIN environments e ON e.id=u.environment_id WHERE u.id=OLD.unit_id;
  END IF;
  IF COALESCE(current_setting('orchestrator.application_purge_id',true),'')<>''
     AND has_schema_privilege(current_user,current_schema(),'CREATE') THEN
   IF application_purge_scope_matches(app_id) THEN RETURN OLD; END IF;
  END IF;
 END IF;
 RAISE EXCEPTION 'immutable delivery artifact or revision';
END $$;

CREATE OR REPLACE FUNCTION reconciliation_artifact_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE app_id uuid;
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.purpose='DELIVERY_PAYLOAD' THEN
   SELECT e.application_id INTO app_id FROM delivery_units u JOIN environments e ON e.id=u.environment_id
   WHERE u.id=OLD.owner_id AND u.organization_id=OLD.organization_id;
  ELSE
   SELECT application_id INTO app_id FROM terraform_resource_instances WHERE id=OLD.owner_id AND organization_id=OLD.organization_id;
  END IF;
  IF COALESCE(current_setting('orchestrator.application_purge_id',true),'')<>''
     AND has_schema_privilege(current_user,current_schema(),'CREATE') THEN
   IF application_purge_scope_matches(app_id) THEN RETURN OLD; END IF;
  END IF;
 END IF;
 IF TG_OP='DELETE' AND OLD.purpose='TERRAFORM_REVIEW'
 AND session_user=current_user AND EXISTS(SELECT 1 FROM pg_namespace n JOIN pg_roles r ON r.rolname=current_user WHERE n.nspname=current_schema() AND NOT r.rolsuper AND (n.nspowner=r.oid OR (n.nspowner='pg_database_owner'::regrole AND EXISTS(SELECT 1 FROM pg_database d WHERE d.datname=current_database() AND d.datdba=r.oid))))
 THEN
  IF drift_repair_artifact_expired(OLD.id) THEN RETURN OLD; END IF;
 END IF;
 RAISE EXCEPTION 'immutable reconciliation artifact';
END $$;

-- The product schema is trusted. Pin the concrete schema first and pg_temp
-- last for relation lookup. The function's owner is the schema owner.
SELECT set_config('search_path', format('%I, pg_temp', current_schema()), true);
CREATE FUNCTION purge_one_application(org_key text, app_key text, expected_id uuid, confirmation_name text, prior_stopped boolean)
RETURNS text LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE
 target_id uuid;
 target_org uuid;
 target_name text;
 deletion_running boolean;
 held boolean;
 environment_ids uuid[];
 instance_ids uuid[];
 unit_ids uuid[];
 operation_ids uuid[];
 deployment_ids uuid[];
 resource_ids uuid[];
 web_invocation boolean := session_user <> current_user;
BEGIN
 IF web_invocation AND NOT EXISTS(SELECT 1 FROM application_purge_policy WHERE singleton AND enabled FOR SHARE) THEN
  RETURN 'DISABLED';
 END IF;
 SELECT a.id,a.organization_id,a.name,COALESCE(a.deletion->>'status'='RUNNING',false)
  INTO target_id,target_org,target_name,deletion_running
 FROM applications a JOIN organizations o ON o.id=a.organization_id
 WHERE o.organization_key=org_key AND a.application_key=app_key
 FOR UPDATE OF a;
 IF NOT FOUND THEN RETURN 'NOT_FOUND'; END IF;
 IF target_id IS DISTINCT FROM expected_id OR target_name IS DISTINCT FROM confirmation_name OR confirmation_name='' THEN
  RETURN 'MISMATCH';
 END IF;

 SELECT ARRAY(SELECT id FROM environments WHERE application_id=target_id ORDER BY id FOR UPDATE) INTO environment_ids;
 SELECT ARRAY(SELECT id FROM environment_operations WHERE environment_id=ANY(environment_ids) ORDER BY id FOR UPDATE) INTO operation_ids;
 IF web_invocation THEN
  IF deletion_running OR EXISTS(SELECT 1 FROM environment_operations WHERE id=ANY(operation_ids) AND status IN ('ACTIVE','RECOVERING')) THEN
   RETURN 'BUSY';
  END IF;
  SELECT EXISTS(SELECT 1 FROM environment_operations WHERE id=ANY(operation_ids) AND status='INTERRUPTED')
      OR EXISTS(SELECT 1 FROM terraform_state_locks WHERE environment_operation_id=ANY(operation_ids)) INTO held;
  IF held AND prior_stopped IS NOT TRUE THEN RETURN 'RECOVERY_REQUIRED'; END IF;
 END IF;

 SELECT ARRAY(SELECT id FROM terraform_resource_instances WHERE environment_id=ANY(environment_ids)) INTO instance_ids;
 SELECT ARRAY(SELECT id FROM delivery_units WHERE environment_id=ANY(environment_ids)) INTO unit_ids;
 SELECT ARRAY(SELECT id FROM deployments WHERE environment_id=ANY(environment_ids)) INTO deployment_ids;
 SELECT ARRAY(
  SELECT r.id FROM active_resources r
  WHERE r.organization_id=target_org AND (
  (r.scope_type='APPLICATION' AND r.scope_id=app_key) OR
  EXISTS (SELECT 1 FROM environments e
    CROSS JOIN LATERAL (SELECT app_key || '.' || e.environment_key AS base_scope) scope
    WHERE e.id=ANY(environment_ids) AND starts_with(r.scope_id,scope.base_scope) AND (
    (r.scope_type IN ('ENVIRONMENT','SHARED') AND substring(r.scope_id FROM length(scope.base_scope)+1) ~ '^(~g[1-9][0-9]*)?$') OR
    (r.scope_type='WORKLOAD' AND substring(r.scope_id FROM length(scope.base_scope)+1) ~ '^(~g[1-9][0-9]*)?[.][^.]+$')
  ))
)
AND NOT EXISTS (
  SELECT 1 FROM deployment_resources d WHERE d.active_resource_id=r.id
   AND d.deployment_id <> ALL(deployment_ids)
)
AND NOT EXISTS (
  SELECT 1 FROM terraform_resource_instances i WHERE i.active_resource_id=r.id
   AND i.id <> ALL(instance_ids)
)
AND (r.last_deployment_id IS NULL OR r.last_deployment_id = ANY(deployment_ids))
 ) INTO resource_ids;

 PERFORM set_config('orchestrator.application_purge_id',target_id::text,true);
 SET CONSTRAINTS ALL DEFERRED;

 DELETE FROM drift_repair_reviews WHERE environment_id = ANY(environment_ids);
 DELETE FROM drift_scans WHERE environment_id = ANY(environment_ids);
 DELETE FROM terraform_execution_baselines WHERE resource_instance_id = ANY(instance_ids);
 DELETE FROM terraform_resource_attempts WHERE resource_instance_id = ANY(instance_ids);
 DELETE FROM terraform_state_locks WHERE resource_instance_id = ANY(instance_ids);
 DELETE FROM terraform_executor_capabilities WHERE resource_instance_id = ANY(instance_ids);
 DELETE FROM terraform_states WHERE resource_instance_id = ANY(instance_ids);
 DELETE FROM terraform_resource_dependencies WHERE environment_id = ANY(environment_ids);

 DELETE FROM delivery_poll_leases WHERE unit_id = ANY(unit_ids);
 DELETE FROM delivery_observations WHERE unit_id = ANY(unit_ids);
 DELETE FROM delivery_intents WHERE unit_id = ANY(unit_ids);
 DELETE FROM delivery_revisions WHERE unit_id = ANY(unit_ids);
 DELETE FROM git_publication_leases WHERE environment_operation_id = ANY(operation_ids);
 DELETE FROM reconciliation_private_artifacts p
  WHERE p.organization_id=target_org AND (
    (p.purpose='DELIVERY_PAYLOAD' AND p.owner_id = ANY(unit_ids))
    OR (p.purpose IN ('TERRAFORM_BASELINE','TERRAFORM_REVIEW') AND p.owner_id = ANY(instance_ids))
  );
 DELETE FROM delivery_units WHERE id = ANY(unit_ids);

 DELETE FROM telemetry_logs WHERE source_key IN (SELECT source_key FROM telemetry_sources WHERE environment_id = ANY(environment_ids));
 DELETE FROM telemetry_samples WHERE source_key IN (SELECT source_key FROM telemetry_sources WHERE environment_id = ANY(environment_ids));
 DELETE FROM telemetry_events WHERE source_key IN (SELECT source_key FROM telemetry_sources WHERE environment_id = ANY(environment_ids));
 DELETE FROM telemetry_sources WHERE environment_id = ANY(environment_ids);

 DELETE FROM endpoint_paths WHERE hostname IN (SELECT hostname FROM endpoint_hosts WHERE environment_id = ANY(environment_ids));
 DELETE FROM endpoint_hosts WHERE environment_id = ANY(environment_ids);
 DELETE FROM workload_instances WHERE environment_id = ANY(environment_ids);
 DELETE FROM environment_definition_selections WHERE environment_id = ANY(environment_ids);
 DELETE FROM environment_catalog_bindings WHERE environment_id = ANY(environment_ids);

 DELETE FROM environment_operations WHERE id = ANY(operation_ids);
 DELETE FROM terraform_resource_instances WHERE id = ANY(instance_ids);
 DELETE FROM deployment_resources WHERE deployment_id = ANY(deployment_ids);
 DELETE FROM active_resources WHERE id = ANY(resource_ids);
 DELETE FROM deployments WHERE id = ANY(deployment_ids);
 DELETE FROM deployment_sets WHERE environment_id = ANY(environment_ids);
 DELETE FROM configuration_revisions WHERE environment_id = ANY(environment_ids);
 DELETE FROM environments WHERE id = ANY(environment_ids);
 DELETE FROM effectiveness_baselines WHERE application_id =target_id;
 DELETE FROM application_delivery_repositories WHERE application_id =target_id;
 DELETE FROM applications WHERE id=target_id;

 SET CONSTRAINTS ALL IMMEDIATE;
 PERFORM set_config('orchestrator.application_purge_id','',true);
 RETURN 'PURGED';
END $$;
SELECT set_config('search_path', format('%I', current_schema()), true);
REVOKE ALL ON FUNCTION purge_one_application(text,text,uuid,text,boolean) FROM PUBLIC;
