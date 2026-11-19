ALTER TABLE user_accounts ADD COLUMN oidc_issuer text NOT NULL DEFAULT '';
ALTER TABLE user_accounts ADD COLUMN oidc_subject text NOT NULL DEFAULT '';
ALTER TABLE user_accounts ADD CONSTRAINT user_accounts_oidc_identity CHECK (
  (oidc_issuer = '' AND oidc_subject = '') OR
  (oidc_issuer <> '' AND oidc_subject <> '' AND password_hash = '')
);
CREATE UNIQUE INDEX user_accounts_oidc_identity_key ON user_accounts(oidc_issuer,oidc_subject) WHERE oidc_issuer <> '';
