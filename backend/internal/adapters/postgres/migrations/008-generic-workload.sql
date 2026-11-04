-- Owner-only transition. No retained running application is silently converted.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workload_instances) OR EXISTS(SELECT 1 FROM deployment_workloads) THEN
  RAISE EXCEPTION 'generic workload upgrade requires a reviewed cleanup of retained workload deployments';
 END IF;
 IF EXISTS(SELECT 1 FROM active_resources a JOIN resource_definitions d ON d.id=a.resource_definition_id WHERE d.driver_type='score-k8s') THEN
  RAISE EXCEPTION 'obsolete workload definition is still referenced by a resource';
 END IF;
END $$;
-- Immutable history is removed only for the explicitly retired experimental driver.
ALTER TABLE resource_definition_versions DISABLE TRIGGER definition_revision_immutable;
DELETE FROM resource_definition_policies p USING resource_definition_versions v
 WHERE p.organization_id=v.organization_id AND p.definition_key=v.definition_key
 AND v.snapshot->>'driverType'='score-k8s';
DELETE FROM resource_definition_versions WHERE snapshot->>'driverType'='score-k8s';
ALTER TABLE resource_definition_versions ENABLE TRIGGER definition_revision_immutable;
UPDATE environment_definition_selections e SET revision=revision+1, document=jsonb_set((
 SELECT jsonb_object_agg(key, CASE WHEN key IN ('baseline','desired','applied') AND jsonb_typeof(value)='array' THEN
  COALESCE((SELECT jsonb_agg(item) FROM jsonb_array_elements(value) item WHERE item->>'driverType' IS DISTINCT FROM 'score-k8s'),'[]'::jsonb)
 ELSE value END) FROM jsonb_each(e.document)
),'{revision}',to_jsonb(revision+1)) WHERE EXISTS(SELECT 1 FROM jsonb_each(e.document) part WHERE part.key IN ('baseline','desired','applied') AND jsonb_typeof(part.value)='array' AND EXISTS(SELECT 1 FROM jsonb_array_elements(part.value) item WHERE item->>'driverType'='score-k8s'));
DELETE FROM resource_definitions WHERE driver_type='score-k8s';
DELETE FROM workload_drafts WHERE score_document->>'apiVersion'='score.dev/v1b1';
ALTER TABLE workload_drafts RENAME COLUMN score_document TO workload_document;
DROP TRIGGER render_bundles_immutable ON render_bundles;
DELETE FROM render_bundles;
DROP FUNCTION render_bundle_immutable();
ALTER TABLE render_bundles RENAME TO template_sources;
ALTER TABLE template_sources DROP CONSTRAINT render_bundles_source_kind_check;
ALTER TABLE template_sources DROP CONSTRAINT render_bundles_check;
ALTER TABLE template_sources DROP CONSTRAINT render_bundles_engine_contract_check;
ALTER TABLE template_sources DROP CONSTRAINT render_bundles_renderer_version_check;
ALTER TABLE template_sources ADD CONSTRAINT template_sources_source_kind_check CHECK (source_kind IN ('git','zip','inline'));
ALTER TABLE template_sources ADD CONSTRAINT template_sources_check CHECK ((source_kind='git' AND source_url ~ '^https://') OR (source_kind='zip' AND source_url='zip:sha256:' || source_revision AND source_revision ~ '^[0-9a-f]{64}$') OR (source_kind='inline' AND source_url='inline:sha256:' || source_revision AND source_revision ~ '^[0-9a-f]{64}$'));
ALTER TABLE template_sources ADD CONSTRAINT template_sources_engine_contract_check CHECK(engine_contract='template-v1');
ALTER TABLE template_sources ADD CONSTRAINT template_sources_renderer_version_check CHECK(renderer_version='1.0.0');
DO $$ DECLARE item record; new_name text; BEGIN
 FOR item IN SELECT conname,contype FROM pg_constraint WHERE conrelid='template_sources'::regclass AND conname LIKE 'render_bundles%' LOOP
  new_name := CASE WHEN item.contype='u' THEN 'template_sources_origin_key' ELSE replace(item.conname,'render_bundles','template_sources') END;
  EXECUTE format('ALTER TABLE template_sources RENAME CONSTRAINT %I TO %I',item.conname,new_name);
 END LOOP;
END $$;
ALTER TABLE template_sources DROP CONSTRAINT template_sources_origin_key;
ALTER TABLE template_sources ADD CONSTRAINT template_sources_origin_key UNIQUE(organization_id,source_url,source_path,source_revision,engine_contract,renderer_binary_sha256);
CREATE FUNCTION template_source_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'template_sources rows are immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER template_sources_immutable BEFORE UPDATE OR DELETE ON template_sources
  FOR EACH ROW EXECUTE FUNCTION template_source_immutable();
