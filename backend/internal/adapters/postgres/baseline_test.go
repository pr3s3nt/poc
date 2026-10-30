package postgres

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// The embedded baseline is the complete current schema: it holds no upgrade
// machinery, retired columns or permanent binding trigger. (Structural
// correctness itself is proven against a real server by the integration tests.)
func TestBaselineSQLHasNoLegacyMachinery(t *testing.T) {
	for _, forbidden := range []string{
		"schema_migrations", "ALTER COLUMN", "DROP COLUMN", "DROP TABLE", "\nUPDATE ", "\nINSERT INTO", "\nDELETE ", "DO $$",
		"environments_binding_immutable", "LEGACY_APPLICATION", "legacy", "HOST_CONTEXT", "configuration_provider",
	} {
		if strings.Contains(baselineSQL, forbidden) {
			t.Errorf("baseline.sql contains %q", forbidden)
		}
	}

	// IF NOT EXISTS is legitimate inside a trigger's validation condition. Only
	// conditional DDL would turn a fresh baseline into an implicit upgrade.
	conditionalDDL := regexp.MustCompile(`(?i)\b(?:CREATE\s+(?:UNIQUE\s+)?(?:TABLE|INDEX|SCHEMA|EXTENSION)|ADD\s+COLUMN)\s+IF\s+NOT\s+EXISTS\b`)
	if conditionalDDL.MatchString(baselineSQL) {
		t.Error("baseline.sql contains conditional upgrade DDL")
	}
	if !strings.Contains(baselineSQL, "CREATE TABLE schema_baseline") {
		t.Error("baseline.sql does not create its identity marker")
	}
}

func TestResetRequiredErrorIsSafe(t *testing.T) {
	err := resetRequired("schema differs at column environments.generation_high")
	if !errors.Is(err, ErrResetRequired) {
		t.Fatal("not classified as reset required")
	}
	text := ErrResetRequired.Error()
	for _, leak := range []string{"SELECT", "SQLSTATE", "constraint", "password", "secret"} {
		if strings.Contains(text, leak) {
			t.Errorf("error leaks %q: %s", leak, text)
		}
	}
	if !strings.Contains(text, "preserve the database") {
		t.Errorf("no preservation guidance: %s", text)
	}
}

func TestFirstDifferenceNamesObjectsOnly(t *testing.T) {
	expected := []string{"column a.b type=text notnull=true default=", "index a.i CREATE UNIQUE INDEX i ON a (b) WHERE secret-predicate"}
	if diff := firstDifference(expected, expected); diff != "" {
		t.Fatalf("equal dumps differ at %q", diff)
	}
	for name, actual := range map[string][]string{
		"missing":   {expected[0]},
		"extra":     append([]string{"column a.secret_user_table type=text notnull=false default="}, expected...),
		"different": {expected[0], "index a.i CREATE INDEX i ON a (b)"},
	} {
		diff := firstDifference(expected, actual)
		if diff == "" || strings.Contains(diff, "secret-predicate") || strings.Contains(diff, "CREATE") || strings.Contains(diff, "secret_user_table") {
			t.Errorf("%s: diff %q", name, diff)
		}
	}
	if got := objectName("function f(a integer) lang=sql"); got != "function f" {
		t.Fatalf("function name = %q", got)
	}
}
