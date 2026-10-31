package pending

import (
	"fmt"
	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

func validateCatalogDataUpdates(plan *planning.Plan, previous func(planning.Node) *planning.Plan, types map[string]resource.Type) error {
	if err := catalogapp.ValidateDataUpdates(plan, previous, types); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	return nil
}
