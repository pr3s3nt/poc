package postgres

import (
	"context"
	appservice "orchestrator/internal/application/application"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence/persistencetest"
	"testing"
)

func TestDeliveryBindingsSurvivePostgresReopen(t *testing.T) {
	first, url := openFresh(t)
	b := persistencetest.DeliveryBindings(t, first)
	persistencetest.DeliverySelections(t, first, b)
	r := persistencetest.ManagedDelivery(t, first, b)
	first.Close()
	next, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	persistencetest.AssertDeliveryBinding(t, next, b)
	persistencetest.AssertManagedDelivery(t, next, r)
}

func TestManagedDeliveryPrivilegesAreLimited(t *testing.T) {
	db := freshDatabase(t)
	console := connectAs(t, db.Console)
	state := connectAs(t, db.State)
	for _, table := range []string{"platform_delivery_defaults", "application_delivery_repositories"} {
		denied(t, console, "no allocation deletion", `DELETE FROM `+table)
		denied(t, state, "no application repository reads", `SELECT * FROM `+table)
	}
}

type pinnedDefault struct{ selection domain.Selection }

func (d pinnedDefault) DefaultSelection(context.Context, string) (domain.Selection, error) {
	return d.selection, nil
}
func TestApplicationCreationPersistsCDPinsOnPostgres(t *testing.T) {
	st, _ := openFresh(t)
	defer st.Close()
	b := persistencetest.DeliveryBindings(t, st)
	svc := appservice.NewService(st)
	selection := domain.Selection{Mode: domain.CD, BindingID: b.ID, BindingVersion: b.Version}
	svc.SetDeliveryDefaults(pinnedDefault{selection})
	result, err := svc.Create(context.Background(), appservice.CreateCommand{OrganizationKey: "delivery-org", Name: "Pinned"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range result.Environments {
		stored, err := st.GetEnvironment(context.Background(), result.Application.Key, e.Key)
		if err != nil || stored.DesiredDelivery != selection || stored.AppliedDelivery != selection {
			t.Fatal("initial pins lost", err)
		}
	}
}
