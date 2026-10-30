CREATE TABLE platform_git_settings (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    url text NOT NULL,
    username text NOT NULL DEFAULT '',
    authenticated boolean NOT NULL DEFAULT false,
    ca_pem text NOT NULL DEFAULT '',
    credential_ref text NOT NULL DEFAULT '',
    version bigint NOT NULL CHECK (version > 0),
    updated_at timestamptz NOT NULL,
    updated_by text NOT NULL,
    CHECK ((authenticated AND credential_ref <> '' AND username <> '') OR
           (NOT authenticated AND credential_ref = '' AND username = ''))
);
