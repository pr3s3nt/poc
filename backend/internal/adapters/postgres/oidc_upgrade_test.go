package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"orchestrator/internal/domain/identity"
)

func TestOIDCUpgradeFromPopulatedVersionTen(t *testing.T) {
	owner, console, roles := legacyDatabase(t, versionTenSQL)
	ctx := context.Background()
	stamp := scalar[string](t, owner, `SELECT installation_id::text||created_at::text FROM schema_baseline`)
	if changed, err := Setup(ctx, owner, roles); err != nil || !changed {
		t.Fatal("upgrade", changed, err)
	}
	if got := scalar[string](t, owner, `SELECT installation_id::text||created_at::text FROM schema_baseline`); got != stamp {
		t.Fatal("installation identity changed")
	}
	if !scalar[bool](t, owner, `SELECT password_hash='retained-password-hash' AND oidc_issuer='' AND oidc_subject='' FROM user_accounts WHERE username='retained-user'`) {
		t.Fatal("retained user changed")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM sessions WHERE token_hash='retained-token-hash'`) != 1 {
		t.Fatal("session changed")
	}
	if changed, err := Setup(ctx, owner, roles); err != nil || changed {
		t.Fatal("repeat", changed, err)
	}
	st, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	account := identity.UserAccount{ID: "77777777-7777-4777-8777-777777777777", OrganizationKey: "retained-org", Username: "sso-employee", Role: identity.RoleDeveloper, Status: identity.AccountActive, OIDCIssuer: "https://idp.test/realm", OIDCSubject: "employee-id"}
	// The existing retained organization is fixed by legacyDatabase.
	account.OrganizationKey = scalar[string](t, owner, `SELECT organization_key FROM organizations LIMIT 1`)
	if err = st.SaveUserAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetUserAccount(ctx, account.ID)
	if err != nil || got.OIDCIssuer != account.OIDCIssuer || got.OIDCSubject != account.OIDCSubject || got.PasswordHash != "" {
		t.Fatal("identity roundtrip", err)
	}
	if _, err = connectAs(t, console).Exec(ctx, `ALTER TABLE user_accounts ADD COLUMN bypass text`); err == nil {
		t.Fatal("runtime gained DDL")
	}
	if scalar[bool](t, owner, `SELECT has_table_privilege('`+roles.State+`','user_accounts','SELECT')`) {
		t.Fatal("state gained identity access")
	}
}
func TestOIDCUpgradeRollbackFromVersionTen(t *testing.T) {
	owner, _, roles := legacyDatabase(t, versionTenSQL)
	ctx := context.Background()
	c := connectAs(t, owner)
	if _, err := c.Exec(ctx, `GRANT DELETE ON schema_baseline TO `+pgx.Identifier{roles.Console}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := Setup(ctx, owner, roles); err == nil {
		t.Fatal("expected grant verification rejection")
	}
	if scalar[string](t, owner, `SELECT baseline FROM schema_baseline`) != "orchestrator-schema-v10" {
		t.Fatal("marker committed")
	}
	if scalar[int](t, owner, `SELECT count(*) FROM information_schema.columns WHERE table_name='user_accounts' AND column_name='oidc_issuer'`) != 0 {
		t.Fatal("DDL committed")
	}
	if scalar[int](t, owner, `SELECT max(version) FROM schema_upgrade_history`) != 10 {
		t.Fatal("history committed")
	}
}
