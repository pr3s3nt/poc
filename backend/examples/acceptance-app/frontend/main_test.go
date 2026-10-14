package main

import "testing"

func TestChecksIncludeCacheAndDefaultToFail(t *testing.T) {
	checks := newChecks()
	names := []string{"backend connection", "environment", "secret", "database", "cache"}
	if len(checks) != len(names) {
		t.Fatalf("rows = %d", len(checks))
	}
	for i, c := range checks {
		if c.Name != names[i] || c.Passed {
			t.Fatalf("row %d = %+v", i, c)
		}
	}
}

func TestApplyChecksReportsCacheFailureAndMissingCheck(t *testing.T) {
	checks := newChecks()
	applyChecks(checks, map[string]bool{"environment": true, "secret": true, "database": true, "cache": false})
	want := map[string]bool{"backend connection": true, "environment": true, "secret": true, "database": true, "cache": false}
	for _, c := range checks {
		if c.Passed != want[c.Name] {
			t.Errorf("%s = %v, want %v", c.Name, c.Passed, want[c.Name])
		}
	}
	// An older backend that does not report "cache" must not pass the row.
	checks = newChecks()
	applyChecks(checks, map[string]bool{"environment": true, "secret": true, "database": true})
	if checks[4].Passed {
		t.Fatal("a missing cache check must fail")
	}
}
