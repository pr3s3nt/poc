-- Platform policy is selected once; application repository identity is private.
ALTER TABLE applications ADD CONSTRAINT applications_id_organization_key UNIQUE(id,organization_id);
CREATE TABLE platform_delivery_defaults (
 organization_id uuid PRIMARY KEY REFERENCES organizations(id),
 binding_id uuid NOT NULL,
 binding_version bigint NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 FOREIGN KEY(organization_id,binding_id,binding_version) REFERENCES delivery_bindings(organization_id,id,version)
);
CREATE TABLE application_delivery_repositories (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES organizations(id),
 application_id uuid NOT NULL,
 binding_id uuid NOT NULL,
 binding_version bigint NOT NULL,
 name text NOT NULL CHECK(name<>''),
 marker text NOT NULL CHECK(marker<>''),
 status text NOT NULL CHECK(status IN ('PENDING','READY')),
 remote_id text NOT NULL DEFAULT '',
 clone_url text NOT NULL DEFAULT '',
 branch text NOT NULL DEFAULT '',
 revision bigint NOT NULL CHECK(revision>0),
 created_at timestamptz NOT NULL,
 UNIQUE(organization_id,application_id),
 FOREIGN KEY(application_id,organization_id) REFERENCES applications(id,organization_id),
 FOREIGN KEY(organization_id,binding_id,binding_version) REFERENCES delivery_bindings(organization_id,id,version),
 CHECK ((status='PENDING' AND remote_id='' AND clone_url='' AND branch='') OR (status='READY' AND remote_id<>'' AND clone_url LIKE 'https://%' AND branch<>''))
);
CREATE FUNCTION application_delivery_repository_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.status<>'PENDING' OR NEW.revision<>1 THEN RAISE EXCEPTION 'invalid initial repository allocation'; END IF;
 ELSE
  IF ROW(NEW.id,NEW.organization_id,NEW.application_id,NEW.binding_id,NEW.binding_version,NEW.name,NEW.marker,NEW.created_at)
   IS DISTINCT FROM ROW(OLD.id,OLD.organization_id,OLD.application_id,OLD.binding_id,OLD.binding_version,OLD.name,OLD.marker,OLD.created_at)
   THEN RAISE EXCEPTION 'repository allocation identity is immutable'; END IF;
  IF OLD.status='READY' OR NEW.status<>'READY' OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'repository acknowledgement is immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER application_delivery_repository_guard BEFORE INSERT OR UPDATE ON application_delivery_repositories FOR EACH ROW EXECUTE FUNCTION application_delivery_repository_guard();
