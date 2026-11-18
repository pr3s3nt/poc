package postgres

import (
	"context"
	"errors"
	"orchestrator/internal/application/installationsetup"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSetupAcrossPostgresBackendsHasOneWinner(t *testing.T) {
	ctx := context.Background()
	db := freshDatabase(t)
	a, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(ctx, db.Console)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	token := strings.Repeat("x", 32)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i, st := range []*Store{a, b} {
		wg.Add(1)
		go func(i int, st *Store) {
			defer wg.Done()
			key := []string{"first", "second"}[i]
			err := installationsetup.New(st, token).Initialize(ctx, installationsetup.Claim{Token: token, OrganizationKey: key, OrganizationName: key, Username: key, Password: "private-password", ConfirmPassword: "private-password"})
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, installationsetup.ErrClosed) {
				t.Errorf("claim: %v", err)
			}
		}(i, st)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("winners %d", wins.Load())
	}
	orgs, err := a.ListOrganizations(ctx)
	if err != nil || len(orgs) != 1 {
		t.Fatal(orgs, err)
	}
	users, err := b.ListUserAccounts(ctx, orgs[0].Key)
	if err != nil || len(users) != 1 {
		t.Fatal("non-atomic initialization", err)
	}
	if available, err := installationsetup.New(b, token).Available(ctx); err != nil || available {
		t.Fatal(available, err)
	}
}
