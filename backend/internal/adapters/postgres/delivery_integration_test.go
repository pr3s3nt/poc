package postgres

import (
	"context"
	"orchestrator/internal/ports/persistence/persistencetest"
	"testing"
)

func TestDeliveryBindingsSurvivePostgresReopen(t *testing.T) {
	first, url := openFresh(t)
	b := persistencetest.DeliveryBindings(t, first)
	persistencetest.DeliverySelections(t, first, b)
	first.Close()
	next, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	persistencetest.AssertDeliveryBinding(t, next, b)
}
