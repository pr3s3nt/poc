package connection_test

import (
	"context"
	"errors"
	"orchestrator/internal/ports/persistence"
	"strings"
	"testing"

	"orchestrator/internal/application/connection"
	"orchestrator/internal/application/gitaccess"
	"orchestrator/internal/domain/application"
)

func TestRegisterGit_StoresTokenOutsideRecordAndDerivesScope(t *testing.T) {
	ctx := context.Background()
	_, creds, _, svc := uploadFixture(t)
	conn, err := svc.RegisterGit(ctx, "acme", connection.RegisterGitCommand{Name: "Team Git", URL: "https://github.com/my-team/", Username: "writer", Token: "private-token"})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := svc.ListAccounts(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.Kind == application.ConnectionGit {
			t.Fatal("Git connection projected as infrastructure account")
		}
	}
	if _, err := svc.GetAccount(ctx, "acme", conn.Key); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("Git credential exposed through infrastructure account operations", err)
	}
	if conn.Kind != application.ConnectionGit || conn.AuthenticationType != application.AuthGitToken || conn.Status != application.ConnectionReady {
		t.Fatalf("unexpected connection: %+v", conn)
	}
	if conn.Verification["verified"] != false {
		t.Fatal("registration must not claim live Git access was verified")
	}
	if conn.Config["url"] != "https://github.com/my-team" {
		t.Fatalf("scope not normalized: %v", conn.Config)
	}
	if strings.Contains(conn.SecretRef, "private-token") {
		t.Fatal("credential bytes leaked into the record")
	}
	if gitaccess.ConnectionRepositoryOwner(conn) != "my-team" {
		t.Fatal("owner scope not derived")
	}
	raw, err := creds.Get(ctx, "acme", conn.Key, conn.SecretRef)
	if err != nil || !strings.Contains(string(raw), "private-token") || !strings.Contains(string(raw), "writer") {
		t.Fatalf("stored credential missing: %v", err)
	}
}

func TestRegisterGit_RejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	_, _, _, svc := uploadFixture(t)
	for name, cmd := range map[string]connection.RegisterGitCommand{
		"host only":    {Name: "g", URL: "https://github.com", Username: "u", Token: "t"},
		"repository":   {Name: "g", URL: "https://github.com/team/repo", Username: "u", Token: "t"},
		"non https":    {Name: "g", URL: "http://github.com/team", Username: "u", Token: "t"},
		"empty token":  {Name: "g", URL: "https://github.com/team", Username: "u"},
		"empty user":   {Name: "g", URL: "https://github.com/team", Token: "t"},
		"userinfo URL": {Name: "g", URL: "https://u@github.com/team", Username: "u", Token: "t"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.RegisterGit(ctx, "acme", cmd); err == nil {
				t.Fatal("invalid Git connection accepted")
			}
		})
	}
}
