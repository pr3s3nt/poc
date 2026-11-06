-- Owner-only transition. Adds the public endpoint hostname ledger (EP-03/05/06).
-- Existing product rows, installation identity and encrypted state are unchanged.
CREATE TABLE endpoint_reservations (
 hostname text CONSTRAINT endpoint_reservations_pkey PRIMARY KEY
  CONSTRAINT endpoint_reservations_hostname_check CHECK (length(hostname) <= 253 AND hostname ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$'),
 organization_id uuid NOT NULL CONSTRAINT endpoint_reservations_organization_id_fkey REFERENCES organizations(id),
 application_id uuid NOT NULL CONSTRAINT endpoint_reservations_application_id_fkey REFERENCES applications(id),
 environment_id uuid NOT NULL CONSTRAINT endpoint_reservations_environment_id_fkey REFERENCES environments(id),
 target_generation bigint NOT NULL CONSTRAINT endpoint_reservations_target_generation_check CHECK (target_generation >= 0),
 workload_id text NOT NULL CONSTRAINT endpoint_reservations_workload_id_check CHECK (length(workload_id) BETWEEN 1 AND 253),
 reserved_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX endpoint_reservations_owner_idx ON endpoint_reservations(environment_id, target_generation, workload_id);
