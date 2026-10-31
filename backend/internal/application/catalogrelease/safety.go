package catalogrelease

import (
	"fmt"
	domain "orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

// Catalog publication does not establish database/data migration capability.
// Existing database and persistent-storage updates require a separate reviewed
// workflow until an explicit, verified upgrade capability is available.
func ValidateDataUpdates(plan *planning.Plan, previous func(planning.Node) *planning.Plan, types map[string]resource.Type) error {
	for _, intent := range plan.ResourceIntents(previous) {
		if intent.Intent == planning.IntentCreate || intent.Intent == planning.IntentRetain {
			continue
		}
		dataResource := intent.ResourceType == "postgres" || intent.ResourceType == "mysql" || intent.ResourceType == "redis"
		fields := map[string]bool{}
		for _, field := range types[intent.ResourceType].Outputs {
			fields[field.Name] = !field.Secret
		}
		dataResource = dataResource || (fields["claimName"] && fields["namespace"])
		if dataResource {
			return fmt.Errorf("%w: catalog update of database or persistent storage requires a separately reviewed migration; retain its current component pin", domain.ErrInvalid)
		}
	}
	return nil
}
