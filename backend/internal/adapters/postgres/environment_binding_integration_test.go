package postgres

import (
	"context"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
)

// A fresh database holds identity-only Applications: Save/Get/List carry no
// target, new Environments stay unconfigured and reopening is idempotent.
func TestApplicationIdentityOnlyFreshAndReopen(t *testing.T) {
	ctx := context.Background()
	url := freshDatabase(t).Console
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
		var markers, ledgers int
		if err := reopened.pool.QueryRow(ctx, `SELECT count(*), (SELECT count(*) FROM pg_class WHERE relname='schema_migrations') FROM schema_baseline`).Scan(&markers, &ledgers); err != nil || markers != 1 || ledgers != 0 {
			t.Fatalf("baseline marker rows = %d, ledgers = %d (%v)", markers, ledgers, err)
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
		"retired scope value":         `UPDATE environments e SET infrastructure_scope='LEGACY_APPLICATION' ` + byKey,
		"empty scope value":           `UPDATE environments e SET infrastructure_scope='' ` + byKey,
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
