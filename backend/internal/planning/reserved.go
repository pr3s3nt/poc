package planning

import (
	"errors"
)

// BuiltinClusterKey is the historical, reserved Definition identity of the
// retired implicit internal cluster (ADR-013, superseded by ADR-016). Nothing
// is admitted under it any more and no user may take it; a stored copy is
// refused rather than ignored.
const BuiltinClusterKey = "builtin-existing-cluster"

// ErrReservedDefinition reports a stored Definition that uses the reserved key.
var ErrReservedDefinition = errors.New("planning: definition key " + BuiltinClusterKey + " is reserved and retired")

// ErrRetiredTarget reports an Environment still carrying the retired internal
// deployment Connection. ADR-016 has no migration: a fresh baseline is required.
var ErrRetiredTarget = errors.New("planning: the environment carries a retired internal deployment connection; the baseline must be recreated")
