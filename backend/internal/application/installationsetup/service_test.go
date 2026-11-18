package installationsetup

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/application"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestInitializationAuthorizationRaceAndRestart(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "state.json")
	st, err := store.NewWithSnapshot(file)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("x", 32)
	svc := New(st, token)
	claim := Claim{Token: token, OrganizationKey: "team", OrganizationName: "Team", Username: "admin", Password: "private-password", ConfirmPassword: "private-password"}
	wrong := claim
	wrong.Token = "wrong"
	if err := svc.Initialize(ctx, wrong); !errors.Is(err, ErrToken) {
		t.Fatal(err)
	}
	bad := claim
	bad.ConfirmPassword = "other"
	if err := svc.Initialize(ctx, bad); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.Initialize(ctx, claim)
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, ErrClosed) {
				t.Errorf("claim: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("winners: %d", wins.Load())
	}
	restored, err := store.NewWithSnapshot(file)
	if err != nil {
		t.Fatal(err)
	}
	available, err := New(restored, token).Available(ctx)
	if err != nil || available {
		t.Fatalf("restart reopened: %v %v", available, err)
	}
	accounts, err := restored.ListUserAccounts(ctx, "team")
	if err != nil || len(accounts) != 1 || accounts[0].PasswordHash == claim.Password {
		t.Fatal("invalid durable identity", err)
	}
	_, who, err := authentication.NewService(restored).SignIn(ctx, claim.Username, claim.Password)
	if err != nil || who.OrganizationKey != "team" {
		t.Fatal("cannot sign in", err)
	}
}
func TestExistingOrganizationAndDisabledSetupCannotClaim(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	token := strings.Repeat("x", 32)
	_ = st.SaveOrganization(ctx, application.Organization{Key: "old", Name: "Old"})
	if available, err := New(st, token).Available(ctx); err != nil || available {
		t.Fatal(available, err)
	}
	if err := New(st, "").Initialize(ctx, Claim{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
