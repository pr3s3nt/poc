package transition

import "testing"

func TestRunIDForIsDeterministicAndScoped(t *testing.T) {
	a := RunIDFor("app-1", "staging", 1)
	if a != RunIDFor("app-1", "staging", 1) {
		t.Fatal("the run id must be deterministic")
	}
	for _, other := range []string{RunIDFor("app-2", "staging", 1), RunIDFor("app-1", "production", 1), RunIDFor("app-1", "staging", 2)} {
		if other == a {
			t.Fatalf("run id %q must differ per application, environment and generation", a)
		}
	}
	if len(a) != len("transition-")+12 || a[:11] != "transition-" {
		t.Fatalf("unexpected run id shape %q", a)
	}
}
