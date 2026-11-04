package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func renderRecord(org, revision string) templatesource.Bundle {
	return templatesource.Bundle{ID: ids.New(), OrganizationKey: org,
		Source:         templatesource.Source{Kind: templatesource.SourceKindGit, URL: "https://github.com/pr3s3nt/poc.git", Path: "backend/template-sources/standard", Revision: revision, ContentSHA256: strings.Repeat("c", 64)},
		EngineContract: templatesource.EngineContract, RendererVersion: templatesource.RendererVersion, RendererBinary: strings.Repeat("b", 64),
		Contract: templatesource.Contract{Manifest: templatesource.Manifest{APIVersion: templatesource.APIVersion, Kind: templatesource.Kind, Resources: []string{"deployment.tpl"}, Patches: []string{}},
			InputsSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}, Defaults: map[string]any{"a": 1.0}, InputContract: templatesource.InputContract},
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

// Git render bundle records (ADR-021) are Organization scoped, idempotent per
// pin, immutable for the Console login and survive a restart.
func TestTemplateSourcesAreScopedImmutableAndDurable(t *testing.T) {
	db := freshDatabase(t)
	ctx := context.Background()
	first, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{"acme", "other"} {
		if err := first.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	a := renderRecord("acme", strings.Repeat("a", 40))
	stored, err := first.InsertTemplateSource(ctx, a)
	if err != nil || !stored.SameVerifiedContent(a) || stored.ID != a.ID {
		t.Fatalf("insert = %+v, %v", stored, err)
	}
	again := renderRecord("acme", a.Source.Revision)
	if repeated, err := first.InsertTemplateSource(ctx, again); err != nil || repeated.ID != a.ID {
		t.Fatalf("same pin must return the existing record: %+v, %v", repeated, err)
	}
	drift := renderRecord("acme", a.Source.Revision)
	drift.Source.ContentSHA256 = strings.Repeat("d", 64)
	if existing, err := first.InsertTemplateSource(ctx, drift); err != nil || existing.SameVerifiedContent(drift) {
		t.Fatalf("drifted content must not replace the pin: %+v, %v", existing, err)
	}
	foreign := renderRecord("other", a.Source.Revision)
	if f, err := first.InsertTemplateSource(ctx, foreign); err != nil || f.ID == a.ID {
		t.Fatalf("another Organization gets its own record: %+v, %v", f, err)
	}
	if _, err := first.InsertTemplateSource(ctx, renderRecord("missing", a.Source.Revision)); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown Organization = %v", err)
	}
	if _, err := first.GetTemplateSource(ctx, "other", a.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign ID must be not found: %v", err)
	}
	first.Close()

	console, err := pgx.Connect(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close(ctx)
	for _, sql := range []string{`UPDATE template_sources SET content_sha256=repeat('e',64)`, `DELETE FROM template_sources`} {
		_, err := console.Exec(ctx, sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s as Console = %v, want insufficient privilege", sql, err)
		}
	}
	second, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	list, err := second.ListTemplateSources(ctx, "acme")
	if err != nil || len(list) != 1 || !list[0].SameVerifiedContent(a) || list[0].Contract.Defaults["a"] != 1.0 {
		t.Fatalf("records after restart = %+v, %v", list, err)
	}
	got, err := second.GetTemplateSource(ctx, "acme", a.ID)
	if err != nil || got.ID != a.ID || got.OrganizationKey != "acme" {
		t.Fatalf("get = %+v, %v", got, err)
	}
}

func TestZIPSourceKindsSurvivePostgresReopen(t *testing.T) {
	ctx := context.Background()
	st, console := openFresh(t)
	if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "zip-org", Name: "ZIP"}); err != nil {
		t.Fatal(err)
	}
	render := renderRecord("zip-org", strings.Repeat("a", 64))
	render.Source.Kind = templatesource.SourceKindZIP
	render.Source.URL = "zip:sha256:" + render.Source.Revision
	first, err := st.InsertTemplateSource(ctx, render)
	if err != nil {
		t.Fatal(err)
	}
	module := testBundle(strings.Repeat("d", 64))
	module.Contract.SourceKind = "zip"
	module.Contract.SourceOrganizationKey = "zip-org"
	module.Ref.Revision = strings.Repeat("b", 64)
	module.Ref.SourceID = "zip-" + module.Ref.Revision[:56]
	savedModule, err := st.InsertModuleBundle(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	reopened, err := Open(ctx, console)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetTemplateSource(ctx, "zip-org", first.ID)
	if err != nil || got.Source.Kind != "zip" || !got.SameVerifiedContent(first) {
		t.Fatalf("render source lost on restart: %v", err)
	}
	again, err := reopened.InsertTemplateSource(ctx, render)
	if err != nil || again.ID != first.ID {
		t.Fatalf("duplicate ZIP pin: %v", err)
	}
	loaded, err := reopened.GetModuleBundle(ctx, module.Ref)
	if err != nil || loaded.Contract.SourceKind != "zip" || !loaded.SameVerifiedContent(savedModule) {
		t.Fatalf("module source lost on restart: %v", err)
	}
	if _, err := reopened.GetTemplateSource(ctx, "other", first.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-org ZIP access: %v", err)
	}
}
