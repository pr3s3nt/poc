CREATE TABLE monitoring_connections (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 connection_key text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 UNIQUE(organization_id,connection_key),
 UNIQUE(organization_id,id)
);
CREATE TABLE monitoring_bindings (
 environment_id uuid PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 connection_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 FOREIGN KEY(organization_id,connection_id) REFERENCES monitoring_connections(organization_id,id) ON DELETE RESTRICT
);
