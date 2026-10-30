package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestGitSettingsPersistenceAndCompareAndSet(t *testing.T) {
	db := freshDatabase(t)
	ctx := context.Background()
	first, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "git-org", Name: "Git"}); err != nil {
		t.Fatal(err)
	}
	record := persistence.GitSettings{OrganizationKey: "git-org", URL: "https://git.example.com/team", Username: "reader", Authenticated: true, CredentialRef: "opaque-credential-reference", Version: 1, UpdatedAt: time.Now().UTC(), UpdatedBy: ids.New()}
	if err = first.SaveGitSettings(ctx, 0, record); err != nil {
		t.Fatal(err)
	}
	if err = first.SaveGitSettings(ctx, 0, record); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale creation accepted", err)
	}
	first.Close()
	second, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, err := second.GetGitSettings(ctx, "git-org")
	if err != nil || got.CredentialRef != record.CredentialRef || got.Version != 1 {
		t.Fatal("settings did not survive reconnect", err)
	}
	if _, err = second.GetGitSettings(ctx, "other-org"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign Organization accessed settings")
	}
	record.Version = 2
	if err = second.SaveGitSettings(ctx, 1, record); err != nil {
		t.Fatal(err)
	}
	if err = second.SaveGitSettings(ctx, 1, record); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale update accepted", err)
	}
	denied(t, connectAs(t, db.State), "Git settings access", `SELECT * FROM platform_git_settings`)
	denied(t, connectAs(t, db.Console), "Git settings DDL", `ALTER TABLE platform_git_settings ADD COLUMN forbidden text`)
}
