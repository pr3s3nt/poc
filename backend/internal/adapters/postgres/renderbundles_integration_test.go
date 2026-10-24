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
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func renderRecord(org, revision string) renderbundle.Bundle {
	return renderbundle.Bundle{ID: ids.New(), OrganizationKey: org,
		Source:         renderbundle.Source{Kind: renderbundle.SourceKindGit, URL: "https://github.com/pr3s3nt/poc.git", Path: "backend/render-bundles/standard", Revision: revision, ContentSHA256: strings.Repeat("c", 64)},
		EngineContract: renderbundle.EngineContract, RendererVersion: renderbundle.RendererVersion, RendererBinary: strings.Repeat("b", 64),
		Contract: renderbundle.Contract{Manifest: renderbundle.Manifest{APIVersion: renderbundle.APIVersion, Kind: renderbundle.Kind, Provisioners: []string{}, Patches: []string{}},
			InputsSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}, Defaults: map[string]any{"a": 1.0}, InputContract: renderbundle.InputContract},
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

// Git render bundle records (ADR-021) are Organization scoped, idempotent per
// pin, immutable for the Console login and survive a restart.
func TestRenderBundlesAreScopedImmutableAndDurable(t *testing.T) {
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
	stored, err := first.InsertRenderBundle(ctx, a)
	if err != nil || !stored.SameVerifiedContent(a) || stored.ID != a.ID {
		t.Fatalf("insert = %+v, %v", stored, err)
	}
	again := renderRecord("acme", a.Source.Revision)
	if repeated, err := first.InsertRenderBundle(ctx, again); err != nil || repeated.ID != a.ID {
		t.Fatalf("same pin must return the existing record: %+v, %v", repeated, err)
	}
	drift := renderRecord("acme", a.Source.Revision)
	drift.Source.ContentSHA256 = strings.Repeat("d", 64)
	if existing, err := first.InsertRenderBundle(ctx, drift); err != nil || existing.SameVerifiedContent(drift) {
		t.Fatalf("drifted content must not replace the pin: %+v, %v", existing, err)
	}
	foreign := renderRecord("other", a.Source.Revision)
	if f, err := first.InsertRenderBundle(ctx, foreign); err != nil || f.ID == a.ID {
		t.Fatalf("another Organization gets its own record: %+v, %v", f, err)
	}
	if _, err := first.InsertRenderBundle(ctx, renderRecord("missing", a.Source.Revision)); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown Organization = %v", err)
	}
	if _, err := first.GetRenderBundle(ctx, "other", a.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign ID must be not found: %v", err)
	}
	first.Close()

	console, err := pgx.Connect(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close(ctx)
	for _, sql := range []string{`UPDATE render_bundles SET content_sha256=repeat('e',64)`, `DELETE FROM render_bundles`} {
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
	list, err := second.ListRenderBundles(ctx, "acme")
	if err != nil || len(list) != 1 || !list[0].SameVerifiedContent(a) || list[0].Contract.Defaults["a"] != 1.0 {
		t.Fatalf("records after restart = %+v, %v", list, err)
	}
	got, err := second.GetRenderBundle(ctx, "acme", a.ID)
	if err != nil || got.ID != a.ID || got.OrganizationKey != "acme" {
		t.Fatalf("get = %+v, %v", got, err)
	}
}
