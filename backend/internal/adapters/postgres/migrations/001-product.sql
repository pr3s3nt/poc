ALTER TABLE applications
  ADD COLUMN status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DELETING','DELETED')),
  ADD COLUMN deletion jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(deletion)='object');
ALTER TABLE applications DROP CONSTRAINT applications_organization_id_name_key;
ALTER TABLE applications DROP CONSTRAINT applications_subdomain_key;
DROP INDEX applications_organization_lower_name_key;
CREATE UNIQUE INDEX applications_organization_lower_name_key ON applications(organization_id, lower(name)) WHERE status <> 'DELETED';
CREATE UNIQUE INDEX applications_subdomain_key ON applications(subdomain) WHERE status <> 'DELETED';
ALTER TABLE render_bundles DROP CONSTRAINT render_bundles_source_url_check;
ALTER TABLE render_bundles ADD COLUMN source_kind text NOT NULL DEFAULT 'git' CHECK (source_kind IN ('git','zip'));
ALTER TABLE render_bundles ADD CONSTRAINT render_bundles_source_url_check CHECK (length(source_url) <= 2048);
ALTER TABLE render_bundles ADD CONSTRAINT render_bundles_check CHECK ((source_kind='git' AND source_url ~ '^https://') OR (source_kind='zip' AND source_url='zip:sha256:' || source_revision AND source_revision ~ '^[0-9a-f]{64}$'));
