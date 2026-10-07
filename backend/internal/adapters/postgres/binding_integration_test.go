package postgres

import (
	"context"
	"testing"

	"orchestrator/internal/ports/persistence/persistencetest"
)

func TestPostgresEnvironmentBindingContractSurvivesRestart(t *testing.T) {
	first, url := openFresh(t)
	fixture := persistencetest.EnvironmentBinding(t, first)
	first.Close()
	reopened, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persistencetest.AssertBindingReloaded(t, reopened, fixture)
}
