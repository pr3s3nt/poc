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
ALTER TABLE active_resources ALTER COLUMN resource_definition_id DROP NOT NULL;
ALTER TABLE active_resources ADD COLUMN catalog_source text, ADD COLUMN catalog_tag text, ADD COLUMN catalog_definition_key text;
ALTER TABLE active_resources ADD CONSTRAINT active_resources_catalog_release_fk
 FOREIGN KEY(organization_id,catalog_source,catalog_tag) REFERENCES catalog_releases(organization_id,source_key,tag);
ALTER TABLE active_resources ADD CONSTRAINT active_resources_definition_provenance_check CHECK (
 (resource_definition_id IS NOT NULL AND catalog_source IS NULL AND catalog_tag IS NULL AND catalog_definition_key IS NULL)
 OR (resource_definition_id IS NULL AND catalog_source IS NOT NULL AND catalog_tag IS NOT NULL AND catalog_definition_key IS NOT NULL));
