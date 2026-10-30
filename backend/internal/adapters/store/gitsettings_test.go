package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

func TestGitSettingsSnapshotReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	ctx := context.Background()
	first, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.SaveOrganization(ctx, application.Organization{Key: "org", Name: "Org"}); err != nil {
		t.Fatal(err)
	}
	record := persistence.GitSettings{OrganizationKey: "org", URL: "https://git.example.com/team", Version: 1, UpdatedAt: time.Now().UTC(), UpdatedBy: "actor"}
	if err = first.Transact(ctx, func(ctx context.Context) error { return first.SaveGitSettings(ctx, 0, record) }); err != nil {
		t.Fatal(err)
	}
	second, err := NewWithSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.GetGitSettings(ctx, "org")
	if err != nil || got.URL != record.URL || got.Version != 1 {
		t.Fatal("settings lost on reload", err)
	}
}
