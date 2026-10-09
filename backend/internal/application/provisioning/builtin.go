package provisioning

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/planning"
	"orchestrator/internal/ports/persistence"
)

// ErrBuiltinCollision reports a stored Definition that uses the reserved
// builtin key with content other than the trusted system definition.
var ErrBuiltinCollision = errors.New("provisioning: reserved definition key " + planning.BuiltinClusterKey + " holds foreign content")

// EnsureBuiltinCluster idempotently admits the exact trusted system Definition
// so Definition foreign keys of progress and Active Resource rows stay valid
// (ADR-013). It never overwrites an existing record and is never called by
// Preview.
func EnsureBuiltinCluster(ctx context.Context, st persistence.Store, organizationKey string) error {
	check := func(ctx context.Context) (bool, error) {
		defs, err := st.ListResourceDefinitions(ctx, organizationKey)
		if err != nil {
			return false, err
		}
		for _, d := range defs {
			if d.Key != planning.BuiltinClusterKey {
				continue
			}
			if !planning.IsBuiltinClusterDefinition(d) {
				return false, ErrBuiltinCollision
			}
			return true, nil
		}
		return false, nil
	}
	exists, err := check(ctx)
	if err != nil || exists {
		return err
	}
	err = st.CreateResourceDefinition(ctx, organizationKey, planning.BuiltinClusterDefinition())
	if errors.Is(err, persistence.ErrDuplicate) {
		// A concurrent admission may have won; verify what is stored.
		if exists, err = check(ctx); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("provisioning: builtin cluster definition admission raced")
		}
		return nil
	}
	return err
}
