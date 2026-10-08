package environment

import (
	"regexp"
	"strings"
	"testing"
)

var dns1123 = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func TestNamespaceForKeepsGenerationZeroAndIsolatesLaterGenerations(t *testing.T) {
	identity := "app-0f0e0d0c-0b0a-4908-8706-050403020100-production"
	if NamespaceFor(identity, 0) != identity {
		t.Fatal("generation 0 must keep the legacy namespace")
	}
	seen := map[string]int64{identity: 0}
	for g := int64(1); g <= 120; g++ {
		ns := NamespaceFor(identity, g)
		if len(ns) > 63 || !dns1123.MatchString(ns) || !regexp.MustCompile(`-g[0-9]+-[0-9a-f]{8}$`).MatchString(ns) || !strings.HasPrefix(ns, identity[:20]) {
			t.Fatalf("generation %d namespace %q breaks the contract", g, ns)
		}
		if previous, dup := seen[ns]; dup {
			t.Fatalf("generations %d and %d share namespace %q", previous, g, ns)
		}
		seen[ns] = g
	}
}

func TestNamespaceForNeverTruncatesAwayGenerationUniqueness(t *testing.T) {
	long := strings.Repeat("a", 62)
	first, second := NamespaceFor(long, 1), NamespaceFor(long, 2)
	if len(first) > 63 || len(second) > 63 || first == second || !strings.Contains(first, "-g1-") || !strings.Contains(second, "-g2-") {
		t.Fatalf("%q %q", first, second)
	}
	if NamespaceFor(long, 1) != first {
		t.Fatal("namespace must be deterministic")
	}
	if NamespaceFor(long+"b"[:0]+"", 1) != first || NamespaceFor(strings.Repeat("a", 61)+"-", 1) == first {
		t.Fatal("distinct identities must differ")
	}
}

func TestScopeIDDelimiterCannotCollideWithWorkloadScopes(t *testing.T) {
	if ScopeID("app", "staging", 0) != "app.staging" || ScopeID("app", "staging", 3) != "app.staging~g3" {
		t.Fatal("scope identity")
	}
	// A generation-0 workload named "g1" is "app.staging.g1"; generation 1 is "app.staging~g1".
	if ScopeID("app", "staging", 1) == "app.staging.g1" {
		t.Fatal("a dot-prefixed delimiter would collide with a generation-0 workload scope")
	}
}
