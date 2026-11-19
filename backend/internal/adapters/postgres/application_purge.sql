-- This script runs only in the Application owner command. The temp target
-- was resolved and locked by Organization and Application key.
SET CONSTRAINTS ALL DEFERRED;
CREATE TEMP TABLE app_purge_environments ON COMMIT DROP AS
SELECT e.id, t.application_key || '.' || e.environment_key AS base_scope
  FROM environments e JOIN pg_temp.app_purge_target t ON e.application_id=t.application_id;
CREATE TEMP TABLE app_purge_instances ON COMMIT DROP AS
SELECT i.id FROM terraform_resource_instances i
 JOIN pg_temp.app_purge_environments e ON e.id=i.environment_id;
CREATE TEMP TABLE app_purge_units ON COMMIT DROP AS
SELECT u.id FROM delivery_units u
 JOIN pg_temp.app_purge_environments e ON e.id=u.environment_id;
CREATE TEMP TABLE app_purge_operations ON COMMIT DROP AS
SELECT o.id FROM environment_operations o
 JOIN pg_temp.app_purge_environments e ON e.id=o.environment_id;
CREATE TEMP TABLE app_purge_deployments ON COMMIT DROP AS
SELECT d.id FROM deployments d
 JOIN pg_temp.app_purge_environments e ON e.id=d.environment_id;

-- Remove app-scoped catalog references only if no other deployment or
-- instance depends on the same active resource ID.
CREATE TEMP TABLE app_purge_resources ON COMMIT DROP AS
SELECT r.id FROM active_resources r JOIN pg_temp.app_purge_target t ON t.organization_id=r.organization_id
WHERE (
  (r.scope_type='APPLICATION' AND r.scope_id=t.application_key) OR
  EXISTS (SELECT 1 FROM pg_temp.app_purge_environments e WHERE starts_with(r.scope_id,e.base_scope) AND (
    (r.scope_type IN ('ENVIRONMENT','SHARED') AND substring(r.scope_id FROM length(e.base_scope)+1) ~ '^(~g[1-9][0-9]*)?$') OR
    (r.scope_type='WORKLOAD' AND substring(r.scope_id FROM length(e.base_scope)+1) ~ '^(~g[1-9][0-9]*)?[.][^.]+$')
  ))
)
AND NOT EXISTS (
  SELECT 1 FROM deployment_resources d WHERE d.active_resource_id=r.id
   AND d.deployment_id NOT IN (SELECT id FROM pg_temp.app_purge_deployments)
)
AND NOT EXISTS (
  SELECT 1 FROM terraform_resource_instances i WHERE i.active_resource_id=r.id
   AND i.id NOT IN (SELECT id FROM pg_temp.app_purge_instances)
)
AND (r.last_deployment_id IS NULL OR r.last_deployment_id IN (SELECT id FROM pg_temp.app_purge_deployments));

-- The named audit guards hold ACCESS EXCLUSIVE locks until this transaction
-- ends. All foreign keys and other triggers remain enabled.
ALTER TABLE terraform_resource_dependencies DISABLE TRIGGER terraform_resource_dependencies_guard;
ALTER TABLE terraform_resource_attempts DISABLE TRIGGER terraform_resource_attempts_retained;
ALTER TABLE terraform_states DISABLE TRIGGER terraform_states_retained;
ALTER TABLE terraform_resource_instances DISABLE TRIGGER terraform_resource_instances_retained;
ALTER TABLE terraform_execution_baselines DISABLE TRIGGER terraform_execution_baselines_immutable;
ALTER TABLE delivery_revisions DISABLE TRIGGER delivery_revisions_immutable;
ALTER TABLE delivery_observations DISABLE TRIGGER delivery_observations_immutable;
ALTER TABLE reconciliation_private_artifacts DISABLE TRIGGER reconciliation_private_artifacts_immutable;

DELETE FROM drift_repair_reviews WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM drift_scans WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM terraform_execution_baselines WHERE resource_instance_id IN (SELECT id FROM pg_temp.app_purge_instances);
DELETE FROM terraform_resource_attempts WHERE resource_instance_id IN (SELECT id FROM pg_temp.app_purge_instances);
DELETE FROM terraform_state_locks WHERE resource_instance_id IN (SELECT id FROM pg_temp.app_purge_instances);
DELETE FROM terraform_executor_capabilities WHERE resource_instance_id IN (SELECT id FROM pg_temp.app_purge_instances);
DELETE FROM terraform_states WHERE resource_instance_id IN (SELECT id FROM pg_temp.app_purge_instances);
DELETE FROM terraform_resource_dependencies WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);

DELETE FROM delivery_poll_leases WHERE unit_id IN (SELECT id FROM pg_temp.app_purge_units);
DELETE FROM delivery_observations WHERE unit_id IN (SELECT id FROM pg_temp.app_purge_units);
DELETE FROM delivery_intents WHERE unit_id IN (SELECT id FROM pg_temp.app_purge_units);
DELETE FROM delivery_revisions WHERE unit_id IN (SELECT id FROM pg_temp.app_purge_units);
DELETE FROM delivery_units WHERE id IN (SELECT id FROM pg_temp.app_purge_units);
DELETE FROM git_publication_leases WHERE environment_operation_id IN (SELECT id FROM pg_temp.app_purge_operations);
DELETE FROM reconciliation_private_artifacts p USING pg_temp.app_purge_target t
 WHERE p.organization_id=t.organization_id AND (
   (p.purpose='DELIVERY_PAYLOAD' AND p.owner_id IN (SELECT id FROM pg_temp.app_purge_units))
   OR (p.purpose IN ('TERRAFORM_BASELINE','TERRAFORM_REVIEW') AND p.owner_id IN (SELECT id FROM pg_temp.app_purge_instances))
 );

DELETE FROM telemetry_logs WHERE source_key IN (SELECT source_key FROM telemetry_sources WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments));
DELETE FROM telemetry_samples WHERE source_key IN (SELECT source_key FROM telemetry_sources WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments));
DELETE FROM telemetry_events WHERE source_key IN (SELECT source_key FROM telemetry_sources WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments));
DELETE FROM telemetry_sources WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);

DELETE FROM endpoint_paths WHERE hostname IN (SELECT hostname FROM endpoint_hosts WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments));
DELETE FROM endpoint_hosts WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM workload_instances WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM environment_definition_selections WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM environment_catalog_bindings WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);

DELETE FROM environment_operations WHERE id IN (SELECT id FROM pg_temp.app_purge_operations);
DELETE FROM terraform_resource_instances WHERE id IN (SELECT id FROM pg_temp.app_purge_instances);
DELETE FROM deployment_resources WHERE deployment_id IN (SELECT id FROM pg_temp.app_purge_deployments);
DELETE FROM active_resources WHERE id IN (SELECT id FROM pg_temp.app_purge_resources);
DELETE FROM deployments WHERE id IN (SELECT id FROM pg_temp.app_purge_deployments);
DELETE FROM deployment_sets WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM configuration_revisions WHERE environment_id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM environments WHERE id IN (SELECT id FROM pg_temp.app_purge_environments);
DELETE FROM effectiveness_baselines WHERE application_id IN (SELECT application_id FROM pg_temp.app_purge_target);
DELETE FROM application_delivery_repositories WHERE application_id IN (SELECT application_id FROM pg_temp.app_purge_target);
DELETE FROM applications WHERE id IN (SELECT application_id FROM pg_temp.app_purge_target);

-- Drain the deferred integrity checks before restoring the trigger modes.
SET CONSTRAINTS ALL IMMEDIATE;
ALTER TABLE terraform_resource_dependencies ENABLE TRIGGER terraform_resource_dependencies_guard;
ALTER TABLE terraform_resource_attempts ENABLE TRIGGER terraform_resource_attempts_retained;
ALTER TABLE terraform_states ENABLE TRIGGER terraform_states_retained;
ALTER TABLE terraform_resource_instances ENABLE TRIGGER terraform_resource_instances_retained;
ALTER TABLE terraform_execution_baselines ENABLE TRIGGER terraform_execution_baselines_immutable;
ALTER TABLE delivery_revisions ENABLE TRIGGER delivery_revisions_immutable;
ALTER TABLE delivery_observations ENABLE TRIGGER delivery_observations_immutable;
ALTER TABLE reconciliation_private_artifacts ENABLE TRIGGER reconciliation_private_artifacts_immutable;
