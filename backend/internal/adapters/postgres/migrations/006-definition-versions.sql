CREATE TABLE resource_definition_versions (
 organization_id uuid NOT NULL REFERENCES organizations(id),
 definition_key text NOT NULL,
 revision_id text NOT NULL,
 number bigint NOT NULL CHECK(number>0),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object' AND snapshot->>'key'=definition_key AND snapshot->>'revisionId'=revision_id),
 created_at timestamptz NOT NULL,
 archived boolean NOT NULL DEFAULT false,
 PRIMARY KEY(organization_id,revision_id),
 UNIQUE(organization_id,definition_key,number),
 UNIQUE(organization_id,definition_key,revision_id)
);
CREATE TABLE resource_definition_policies (
 organization_id uuid NOT NULL REFERENCES organizations(id),
 definition_key text NOT NULL,
 generation bigint NOT NULL CHECK(generation>0),
 active_revision text NOT NULL,
 PRIMARY KEY(organization_id,definition_key),
 FOREIGN KEY(organization_id,definition_key,active_revision) REFERENCES resource_definition_versions(organization_id,definition_key,revision_id)
);
CREATE TABLE environment_definition_selections (
 environment_id uuid PRIMARY KEY REFERENCES environments(id),
 revision bigint NOT NULL CHECK(revision>=0),
 document jsonb NOT NULL
);
CREATE FUNCTION definition_revision_immutable_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable definition revision'; END IF;
 IF ROW(NEW.organization_id,NEW.definition_key,NEW.revision_id,NEW.number,NEW.snapshot,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.organization_id,OLD.definition_key,OLD.revision_id,OLD.number,OLD.snapshot,OLD.created_at) THEN
 RAISE EXCEPTION 'immutable definition revision'; END IF;
 IF NEW.archived AND EXISTS(SELECT 1 FROM resource_definition_policies WHERE organization_id=NEW.organization_id AND active_revision=NEW.revision_id) THEN RAISE EXCEPTION 'active revision cannot be archived'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER definition_revision_immutable BEFORE UPDATE OR DELETE ON resource_definition_versions FOR EACH ROW EXECUTE FUNCTION definition_revision_immutable_guard();


-- Capture current editor configurations, without manufacturing old histories.
INSERT INTO resource_definition_versions(organization_id,definition_key,revision_id,number,snapshot,created_at)
SELECT rd.organization_id,rd.definition_key,'legacy-'||rd.id::text,1,
 jsonb_build_object('key',rd.definition_key,'name',rd.definition_name,'resourceType',rt.resource_type_key,
 'driverType',rd.driver_type,'driverAccount',COALESCE(c.connection_key,''),'driverInputs',rd.driver_inputs,
 'provision',rd.provision_rules,'sourceFingerprint',COALESCE(rd.source_fingerprint,''),'version',rd.version,
 'criteria',COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('env_type',mc.env_type,'app_id',mc.app_id,'env_id',mc.env_id,'res_id',mc.res_id,'class',mc.class)) ORDER BY mc.ordinal) FROM matching_criteria mc WHERE mc.resource_definition_id=rd.id),'[]'::jsonb),
 'revisionId','legacy-'||rd.id::text),now()
FROM resource_definitions rd JOIN resource_types rt ON rt.id=rd.resource_type_id LEFT JOIN connections c ON c.id=rd.connection_id;
INSERT INTO resource_definition_policies(organization_id,definition_key,generation,active_revision)
SELECT organization_id,definition_key,1,revision_id FROM resource_definition_versions;

-- Configuration baseline is deliberately not marked as successfully applied.
INSERT INTO environment_definition_selections(environment_id,revision,document)
SELECT e.id,0,jsonb_build_object('applicationKey',a.application_key,'environmentKey',e.environment_key,
 'revision',0,'appliedRevision',0,'frozen',true,'appliedVerified',false,'baseline',defs.items,'desired',defs.items)
FROM environments e JOIN applications a ON a.id=e.application_id
LEFT JOIN environment_catalog_bindings b ON b.environment_id=e.id
LEFT JOIN catalog_releases cr ON cr.organization_id=b.organization_id AND cr.source_key=b.desired_source AND cr.tag=b.desired_tag
CROSS JOIN LATERAL (
 SELECT COALESCE(jsonb_agg(chosen.snapshot ORDER BY chosen.key),'[]'::jsonb) AS items FROM (
 SELECT DISTINCT ON (key) key,snapshot FROM (
  SELECT v.definition_key AS key,v.snapshot,1 AS priority FROM resource_definition_versions v WHERE v.organization_id=a.organization_id
  UNION ALL
  SELECT c->'snapshot'->>'key', CASE WHEN COALESCE(c->>'binding','')<>'' THEN jsonb_set(c->'snapshot','{driverAccount}',COALESCE(b.document->'desired'->'accounts'->(c->>'binding'),'""'::jsonb)) ELSE c->'snapshot' END,2
  FROM jsonb_array_elements(COALESCE(cr.document->'components','[]'::jsonb)) c WHERE c->'snapshot' IS NOT NULL
 ) candidates ORDER BY key,priority
 ) chosen
) defs;
