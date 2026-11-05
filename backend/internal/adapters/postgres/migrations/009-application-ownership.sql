-- Creator identity is nullable only for retained Applications; never infer it.
ALTER TABLE applications ADD COLUMN owner_user_id uuid;
ALTER TABLE applications ADD CONSTRAINT applications_owner_organization_fk
  FOREIGN KEY (owner_user_id, organization_id) REFERENCES user_accounts(id, organization_id);

CREATE FUNCTION application_owner_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.owner_user_id IS DISTINCT FROM OLD.owner_user_id THEN
    RAISE EXCEPTION 'application owner is immutable' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER application_owner_immutable BEFORE UPDATE ON applications
  FOR EACH ROW EXECUTE FUNCTION application_owner_immutable();
