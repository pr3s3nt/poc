// Package target reads the Organization's Driver Accounts for planning
// (ADR-017). An Environment has no execution target of its own: accounts and
// regions belong to the Resource Definitions that use them.
package target

import (
	"context"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

// Accounts returns the Organization's Driver Accounts by key from the same
// snapshot view the catalog was read from, so planning sees one consistent
// account set (UC-03 V02-09). Configuration and credentials stay in the
// records; planning only reads identity, kind, status and version.
func Accounts(ctx context.Context, view persistence.ApplicationRepository, organizationKey string) (map[string]appdomain.Connection, error) {
	list, err := view.ListConnections(ctx, organizationKey)
	if err != nil {
		return nil, err
	}
	out := make(map[string]appdomain.Connection, len(list))
	for _, account := range list {
		out[account.Key] = account
	}
	return out, nil
}
