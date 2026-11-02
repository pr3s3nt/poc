package gitaccess

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

func TestScopeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		scope, repo string
		want        bool
	}{
		{"https://git.example.com/team", "https://git.example.com/team/catalog.git", true},
		{"https://git.example.com/team", "https://git.example.com/teams/catalog.git", false},
		{"https://git.example.com/team", "https://git.example.com:444/team/catalog.git", false},
		{"https://git.example.com/team", "https://git.example.com.evil/team/catalog.git", false},
		{"https://git.example.com/team", "https://git.example.com/team/../other", false},
		{"https://git.example.com/team", "https://git.example.com/team/%2e%2e/other", false},
		{"https://git.example.com", "https://token@git.example.com/team", false},
		{"https://git.example.com/team/", "https://GIT.example.com/team", true},
	} {
		if Matches(tc.scope, tc.repo) != tc.want {
			t.Errorf("scope boundary: %#v", tc)
		}
	}
}
func TestSecretRetentionCASAndOrganizationIsolation(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	creds := credentialmemory.New()
	for _, org := range []string{"one", "two"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(st, creds)
	d := Draft{URL: "https://git.example.com/team", Authenticated: true, Username: "reader", Token: "test-token-sentinel"}
	v, err := svc.Save(ctx, "one", "actor", d)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(v)
	if strings.Contains(string(serialized), d.Token) || strings.Contains(string(serialized), "credentialRef") {
		t.Fatal("safe view leaked credential")
	}
	saved, err := st.GetGitSettings(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ = json.Marshal(saved)
	if strings.Contains(string(serialized), d.Token) {
		t.Fatal("persistence contains token")
	}
	for _, org := range []string{"one", "two"} {
		a, err := svc.ResolveGitAccess(ctx, org, "https://git.example.com/team/catalog.git")
		if err != nil {
			t.Fatal(err)
		}
		if (a.Token != "") != (org == "one") {
			t.Fatal("organization credential isolation failed")
		}
	}
	if a, err := svc.ResolveGitAccess(ctx, "one", "https://git.example.com/other/catalog.git"); err != nil || a.Token != "" {
		t.Fatal("credential escaped scope")
	}
	d.Token = ""
	d.ExpectedVersion = v.Version
	if _, err := svc.Save(ctx, "one", "actor", d); err != nil {
		t.Fatal("blank token retention failed", err)
	}
	if _, err := svc.Save(ctx, "one", "actor", d); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale write accepted")
	}
	d.ExpectedVersion = 2
	d.URL = "https://git.example.com/other"
	if _, err := svc.Save(ctx, "one", "actor", d); !errors.Is(err, ErrInvalid) {
		t.Fatal("scope change reused token")
	}
	d.Authenticated = false
	if _, err := svc.Save(ctx, "one", "actor", d); err != nil {
		t.Fatal(err)
	}
	saved, _ = st.GetGitSettings(ctx, "one")
	if saved.CredentialRef != "" || saved.Username != "" {
		t.Fatal("anonymous retained auth")
	}
}
func TestDraftDoesNotSaveAndRejectsInvalidCA(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	_ = st.SaveOrganization(ctx, application.Organization{Key: "org", Name: "Org"})
	svc := New(st, credentialmemory.New())
	d := Draft{URL: "https://git.example.com/team", Authenticated: true, Username: "reader", Token: "draft-token"}
	if _, err := svc.DraftAccess(ctx, "org", "https://git.example.com/team/catalog.git", d); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetGitSettings(ctx, "org"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("test draft persisted settings")
	}
	d.CAPEM = "not a certificate"
	if _, err := svc.Save(ctx, "org", "actor", d); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid CA accepted")
	}
}

// Writer access follows rotation while preserving tenant and destination scope.
func TestPlatformWriterRotationAndScope(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	creds := durableWriterTestStore{credentialmemory.New()}
	for _, org := range []string{"one", "two"} {
		if err := st.SaveOrganization(ctx, application.Organization{Key: org, Name: org}); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(st, creds)
	d := Draft{URL: "https://github.com/team", Authenticated: true, Username: "writer", Token: "first"}
	v, err := svc.Save(ctx, "one", "actor", d)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"first", "rotated"} {
		if token == "rotated" {
			d.ExpectedVersion, d.Token = v.Version, token
			if _, err := svc.Save(ctx, "one", "actor", d); err != nil {
				t.Fatal(err)
			}
		}
		raw, _, err := Writer(ctx, st, creds, "one", "https://github.com/team/app")
		var secret struct{ Username, Password string }
		if err != nil || json.Unmarshal(raw, &secret) != nil || secret.Password != token {
			t.Fatal("writer did not resolve current credential")
		}
	}
	for _, destination := range []string{"https://github.com/other/app", "https://github.com/team-other/app", "https://example.com/team/app"} {
		if _, _, err := Writer(ctx, st, creds, "one", destination); err == nil {
			t.Fatal("writer crossed destination scope")
		}
	}
	if _, _, err := Writer(ctx, st, creds, "two", "https://github.com/team/app"); err == nil {
		t.Fatal("writer crossed tenant scope")
	}
}

type durableWriterTestStore struct{ *credentialmemory.Store }

func (durableWriterTestStore) Durable() bool { return true }
