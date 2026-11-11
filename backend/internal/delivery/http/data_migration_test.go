package http

import (
	"fmt"
	"testing"

	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/application/pending"
)

// Issue #68 scenario 5: a refused running-database change has its own code.
func TestDataMigrationRequiredHasItsOwnCode(t *testing.T) {
	err := fmt.Errorf("%w: %w", pending.ErrInvalid, fmt.Errorf("%w: detail", catalogapp.ErrDataMigrationRequired))
	f, ok := classify(err)
	if !ok || f.code != codeDataMigrationRequired {
		t.Fatalf("classified as %+v", f)
	}
}
