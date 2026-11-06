package postgres

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"

	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestIDPApplicationNamesPersistAndConcurrentNamesCannotCollide(t *testing.T) {
	a, b, url := twoRenameBackends(t)
	ctx := context.Background()
	if err := a.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: "nangcap", Name: "Nâng Cấp"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, name := range []string{"Quản lý đơn hàng", "QUẢN LÝ ĐƠN HÀNG"} {
		st := []*Store{a, b}[i]
		wg.Add(1)
		go func(st *Store, name string) {
			defer wg.Done()
			_, err := appcreate.NewService(st).Create(ctx, appcreate.CreateCommand{OrganizationKey: "nangcap", Name: name})
			results <- err
		}(st, name)
	}
	wg.Wait()
	close(results)
	winners, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, appcreate.ErrDuplicate):
			var field *appcreate.FieldError
			if !errors.As(err, &field) || field.Field != "name" {
				t.Fatalf("conflict must be actionable on Name: %v", err)
			}
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	apps, err := a.ListApplications(ctx)
	if err != nil || winners != 1 || conflicts != 1 || len(apps) != 1 {
		t.Fatalf("winners=%d conflicts=%d apps=%d: %v", winners, conflicts, len(apps), err)
	}
	app := apps[0]
	if !regexp.MustCompile(`^idp-nangcap-quan-ly-don-hang-[a-z2-7]{6}$`).MatchString(app.Key) || app.ID == app.Key {
		t.Fatalf("UUID and readable code not separated: %+v", app)
	}
	// The globally unique database key also protects inserts without prechecks.
	clash := app
	clash.ID, clash.Name = ids.New(), "Different name"
	if err := a.SaveApplication(ctx, clash); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatalf("database accepted duplicate code: %v", err)
	}
	app.Name = "Renamed display"
	if err := a.SaveApplication(ctx, app); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetApplication(ctx, app.Key)
	if err != nil || got.ID != app.ID || got.Name != app.Name {
		t.Fatalf("identity lost on reopen: %+v %v", got, err)
	}
	envs, err := reopened.ListEnvironments(ctx, app.Key)
	if err != nil || len(envs) != 2 {
		t.Fatalf("partial or duplicate environments: %+v %v", envs, err)
	}
	for _, env := range envs {
		if env.ApplicationID != app.ID || env.NamespaceIdentity != "idp-nangcap-quan-ly-don-hang-"+env.Key+"-"+app.Key[len(app.Key)-6:] {
			t.Fatalf("namespace changed on display rename/reopen: %+v", env)
		}
	}
}
