package bootstrap

import (
	"context"
	"orchestrator/internal/application/installationsetup"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupBootstrapDefersSeedAndRestoresIdentity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	file := filepath.Join(dir, "token")
	token := strings.Repeat("x", 32)
	if err := os.WriteFile(file, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Seed: seed.Defaults(), StatePath: filepath.Join(dir, "state.json"), SetupTokenFile: file, Adapters: AdapterFake}
	opts.Seed.Profile = "production"
	opts.Seed.Catalog = seed.CatalogNone
	app, err := Build(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	orgs, err := app.Store.(persistence.InstallationSetup).ListOrganizations(ctx)
	if err != nil || len(orgs) != 0 {
		t.Fatal("seed consumed initialization", err)
	}
	if err := installationsetup.New(app.Store, token).Initialize(ctx, installationsetup.Claim{Token: token, OrganizationKey: "team", OrganizationName: "Team", Username: "admin", Password: "private-password", ConfirmPassword: "private-password"}); err != nil {
		t.Fatal(err)
	}
	app.Close()
	again, err := Build(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	orgs, err = again.Store.(persistence.InstallationSetup).ListOrganizations(ctx)
	if err != nil || len(orgs) != 1 || orgs[0].Key != "team" {
		t.Fatal("restart changed identity", err)
	}
}
