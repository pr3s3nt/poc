-- Schema 7 (ADR-034): reusable Git connections for simplified CD configuration.
ALTER TABLE connections DROP CONSTRAINT connections_kind_check;
ALTER TABLE connections ADD CONSTRAINT connections_kind_check CHECK (kind IN ('AWS','KUBERNETES','GIT'));
ALTER TABLE connections DROP CONSTRAINT connections_authentication_type_check;
ALTER TABLE connections ADD CONSTRAINT connections_authentication_type_check CHECK (
  (kind='KUBERNETES' AND authentication_type='KUBECONFIG') OR
  (kind='AWS' AND authentication_type IN ('AWS_ACCESS_KEY','AWS_EC2_ROLE')) OR
  (kind='GIT' AND authentication_type='GIT_TOKEN'));
