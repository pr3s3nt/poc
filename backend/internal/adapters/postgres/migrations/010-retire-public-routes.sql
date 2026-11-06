-- Owner-only transition (ADR-037). Endpoint hostnames gain path sharing (EP-07);
-- Application subdomains and the retired public-route pending flag are removed.
CREATE TABLE endpoint_hosts (
 hostname text CONSTRAINT endpoint_hosts_pkey PRIMARY KEY
  CONSTRAINT endpoint_hosts_hostname_check CHECK (length(hostname) <= 253 AND hostname ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$'),
 organization_id uuid NOT NULL CONSTRAINT endpoint_hosts_organization_id_fkey REFERENCES organizations(id),
 application_id uuid NOT NULL CONSTRAINT endpoint_hosts_application_id_fkey REFERENCES applications(id),
 environment_id uuid NOT NULL CONSTRAINT endpoint_hosts_environment_id_fkey REFERENCES environments(id),
 target_generation bigint NOT NULL CONSTRAINT endpoint_hosts_target_generation_check CHECK (target_generation >= 0),
 reserved_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX endpoint_hosts_owner_idx ON endpoint_hosts(environment_id, target_generation);
CREATE TABLE endpoint_paths (
 hostname text NOT NULL CONSTRAINT endpoint_paths_hostname_fkey REFERENCES endpoint_hosts(hostname),
 path text NOT NULL CONSTRAINT endpoint_paths_path_check CHECK (length(path) <= 1024 AND path ~ '^/'),
 workload_id text NOT NULL CONSTRAINT endpoint_paths_workload_id_check CHECK (length(workload_id) BETWEEN 1 AND 253),
 reserved_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT endpoint_paths_pkey PRIMARY KEY (hostname, path)
);
CREATE INDEX endpoint_paths_workload_idx ON endpoint_paths(workload_id);
-- A version-9 claim granted the whole hostname: keep it as the workload's '/'.
INSERT INTO endpoint_hosts(hostname,organization_id,application_id,environment_id,target_generation,reserved_at)
 SELECT hostname,organization_id,application_id,environment_id,target_generation,reserved_at FROM endpoint_reservations;
INSERT INTO endpoint_paths(hostname,path,workload_id,reserved_at)
 SELECT hostname,'/',workload_id,reserved_at FROM endpoint_reservations;
DROP TABLE endpoint_reservations;
ALTER TABLE applications DROP COLUMN subdomain;
ALTER TABLE environments DROP COLUMN public_routes_pending;
UPDATE workload_drafts SET workload_document = workload_document #- '{service,publicRoutes}'
 WHERE jsonb_typeof(workload_document->'service') = 'object' AND workload_document->'service' ? 'publicRoutes';
