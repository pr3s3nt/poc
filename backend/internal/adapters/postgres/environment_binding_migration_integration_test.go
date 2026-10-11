package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// preChangeSchema installs migrations 1..5 (the schema before ADR-011) and a
// ledger so Open applies only the new migrations over real old rows.
// kubernetesAuth is the historical authentication type of the Kubernetes rows:
// HOST_CONTEXT is rejected by migration 8; KUBECONFIG reaches migration 9, which
// rejects the retained Application targets (T26).
func preChangeSchema(t *testing.T, url, kubernetesAuth string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for i, sql := range []string{migration, migration2, migration3, migration4, migration5} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("pre-change migration %d: %v", i+1, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, i+1); err != nil {
			t.Fatal(err)
		}
	}
	const (
		org   = "11111111-1111-4111-8111-111111111111"
		lab   = "22222222-2222-4222-8222-222222222222"
		cloud = "33333333-3333-4333-8333-333333333333"
		defc  = "44444444-4444-4444-8444-444444444444"
	)
	stmts := []string{
		`INSERT INTO organizations(id,organization_key,name,default_connection_key) VALUES('` + org + `','acme','Acme','default')`,
		`INSERT INTO connections(id,organization_id,connection_key,kind,config,secret_ref,status,verification,name,authentication_type) VALUES
		 ('` + defc + `','` + org + `','default','KUBERNETES','{}','fake-credential://default','READY','{}','default','` + kubernetesAuth + `'),
		 ('` + lab + `','` + org + `','lab','KUBERNETES','{}','fake-credential://lab','READY','{}','lab','` + kubernetesAuth + `'),
		 ('` + cloud + `','` + org + `','cloud','AWS','{"region":"eu-west-1"}','secret://connections/cloud','READY','{}','cloud','AWS_ACCESS_KEY')`,
		`UPDATE organizations SET default_connection_id='` + defc + `' WHERE id='` + org + `'`,
		`INSERT INTO applications(id,application_key,organization_id,name,subdomain,execution_profile,connection_id,region,runtime_status,version,configuration_provider) VALUES
		 ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','old-k8s','` + org + `','Old','old','internal-k8s','` + lab + `','','READY',3,'vault'),
		 ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','old-aws','` + org + `','OldAws','oldaws','aws-eks','` + cloud + `','eu-west-1','PENDING',1,'vault')`,
		`INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version) VALUES
		 ('a1111111-1111-4111-8111-111111111111','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','staging','Staging','staging','app-old-k8s-staging',5),
		 ('a2222222-2222-4222-8222-222222222222','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','production','Production','production','app-old-k8s-production',1),
		 ('b1111111-1111-4111-8111-111111111111','bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','staging','Staging','staging','app-old-aws-staging',1)`,
	}
	for _, sql := range stmts {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("old row: %v\n%s", err, sql)
		}
	}
}

// A database whose Applications still hold a retired execution target is
// refused by migration 9 before any column is dropped: nothing is converted to
// an Environment binding and nothing is lost silently (T26). Reset is T29.
func TestMigration9RejectsRetainedApplicationTargetsWithoutMutation(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	preChangeSchema(t, url, "KUBECONFIG")
	st, err := Open(ctx, url)
	if err == nil {
		st.Close()
		t.Fatal("database with retained Application targets opened")
	}
	if !errors.Is(err, ErrLegacyApplicationData) || strings.Contains(err.Error(), "constraint") {
		t.Fatalf("open error = %v", err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var columns, ledger, bound int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='applications' AND column_name IN ('execution_profile','connection_id','region','runtime_status','configuration_provider')`).Scan(&columns); err != nil || columns != 5 {
		t.Fatalf("retained Application columns were altered: %d %v", columns, err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&ledger); err != nil || ledger != 5 {
		t.Fatalf("ledger changed: %d %v", ledger, err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM applications WHERE connection_id IS NOT NULL`).Scan(&bound); err != nil || bound != 2 {
		t.Fatalf("retained Application rows changed: %d %v", bound, err)
	}
}

// A fresh database holds identity-only Applications: Save/Get/List carry no
// target, new Environments stay unconfigured and reopening is idempotent.
func TestApplicationIdentityOnlyFreshAndReopen(t *testing.T) {
	ctx := context.Background()
	url := persistencetest.FreshPostgresDatabase(t)
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveOrganization(ctx, application.Organization{ID: "55555555-5555-4555-8555-555555555555", Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	fresh := application.Application{ID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Key: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", OrganizationKey: "acme", Name: "Fresh", Subdomain: "fresh", Version: 1}
	if err := st.SaveApplication(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(ctx, environment.Environment{Key: key, ApplicationID: fresh.ID, ApplicationKey: fresh.Key, Name: key, Type: key, NamespaceIdentity: "app-fresh-" + key, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	var columns int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='applications' AND column_name IN ('execution_profile','connection_id','region','runtime_status','configuration_provider')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("retired Application columns remain: %d %v", columns, err)
	}
	st.Close()
	for i := 0; i < 2; i++ {
		reopened, err := Open(ctx, url)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		if got, err := reopened.GetApplication(ctx, fresh.Key); err != nil || got != fresh {
			t.Fatalf("reopen %d: application = %+v %v", i, got, err)
		}
		if list, err := reopened.ListApplications(ctx); err != nil || len(list) != 1 || list[0] != fresh {
			t.Fatalf("reopen %d: list = %+v %v", i, list, err)
		}
		for _, key := range []string{"staging", "production"} {
			e, err := reopened.GetEnvironment(ctx, fresh.Key, key)
			if err != nil || e.Configured() || e.Status() != application.RuntimeUnconfigured || e.InfrastructureScope != environment.ScopeEnvironment {
				t.Fatalf("reopen %d: %s must stay UNCONFIGURED: %+v %v", i, key, e, err)
			}
		}
		var count int
		_ = reopened.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count)
		if count != 9 {
			t.Fatalf("ledger rows = %d", count)
		}
		reopened.Close()
	}
}

func TestEnvironmentBindingConsistencyCheckAndEditableBinding(t *testing.T) {
	ctx := context.Background()
	st, _ := openFresh(t)
	fixture := persistencetest.EnvironmentBinding(t, st)
	exec := func(sql string, args ...any) error {
		_, err := st.pool.Exec(ctx, sql, args...)
		return err
	}
	byKey := `FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key='staging'`
	// ADR-012 dropped the permanent trigger: a configured binding may be
	// replaced by an owner-checked versioned write, but never made inconsistent.
	var triggers int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE tgname='environments_binding_immutable'`).Scan(&triggers); err != nil || triggers != 0 {
		t.Fatalf("immutable binding trigger must be gone: %d %v", triggers, err)
	}
	if err := exec(`UPDATE environments e SET connection_id=(SELECT id FROM connections WHERE connection_key='cloud'),execution_profile='aws-eks',region='eu-west-1',runtime_status='PENDING' `+byKey, fixture.ApplicationKey); err != nil {
		t.Fatalf("replacing a consistent binding must be legal: %v", err)
	}
	for name, sql := range map[string]string{
		"clear connection": `UPDATE environments e SET connection_id=NULL,execution_profile='',region='',runtime_status='UNCONFIGURED' ` + byKey,
	} {
		_ = name
		_ = sql
	}
	if err := exec(`UPDATE environments e SET connection_id=(SELECT id FROM connections WHERE connection_key='lab'),execution_profile='internal-k8s',region='',runtime_status='READY' `+byKey, fixture.ApplicationKey); err != nil {
		t.Fatal(err)
	}
	// Runtime status and the operational version stay writable.
	if err := exec(`UPDATE environments e SET runtime_status='PENDING',version=e.version+1 `+byKey, fixture.ApplicationKey); err != nil {
		t.Fatalf("runtime/version update must stay legal: %v", err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{Key: "unset", ApplicationKey: fixture.ApplicationKey, Name: "unset", Type: "qa", NamespaceIdentity: "app-unset", Version: 1}); err != nil {
		t.Fatal(err)
	}
	// Consistency: no connection requires the UNCONFIGURED shape, a connection
	// requires a supported profile, an AWS region and a PENDING/READY status.
	for name, sql := range map[string]string{
		"profile without connection":  `UPDATE environments e SET execution_profile='internal-k8s' FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key='unset'`,
		"status without connection":   `UPDATE environments e SET runtime_status='READY' FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key='unset'`,
		"configured but unconfigured": `UPDATE environments e SET runtime_status='UNCONFIGURED' ` + byKey,
		"configured bad status":       `UPDATE environments e SET runtime_status='BROKEN' ` + byKey,
		"bad scope value":             `UPDATE environments e SET infrastructure_scope='SHARED' ` + byKey,
	} {
		if err := exec(sql, fixture.ApplicationKey); err == nil {
			t.Fatalf("%s must violate the consistency check", name)
		}
	}
	// An AWS binding needs a region.
	if _, err := st.pool.Exec(ctx, `INSERT INTO environments(id,application_id,environment_key,name,environment_type,namespace_identity,version,connection_id,execution_profile,region,runtime_status)
		SELECT gen_random_uuid(),a.id,'qa','qa','qa','app-qa',1,(SELECT id FROM connections WHERE connection_key='cloud'),'aws-eks','','PENDING' FROM applications a WHERE a.application_key=$1`, fixture.ApplicationKey); err == nil {
		t.Fatal("aws-eks without region must be rejected")
	}
	if _, err := st.BindEnvironment(ctx, persistence.EnvironmentBinding{ApplicationKey: fixture.ApplicationKey, EnvironmentKey: "staging", ConnectionKey: "lab", Profile: application.ProfileInternalK8s, RuntimeStatus: application.RuntimeReady, Scope: environment.ScopeEnvironment, ExpectedVersion: 99}); err == nil {
		t.Fatal("a stale expected version must reject a bind")
	}
}
