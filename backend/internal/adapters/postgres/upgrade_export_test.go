package postgres

import "testing"

// LegacyUpgradeFixtureForTest is test-only access for the HTTP acceptance test
// in the external package, avoiding a bootstrap -> postgres import cycle.
func LegacyUpgradeFixtureForTest(t *testing.T) (string, string, Roles) {
	return legacyDatabase(t, sourceOldSQL)
}
