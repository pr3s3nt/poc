package telemetry

import "testing"

func TestLogLevelRequiresRecognizedContent(t *testing.T) {
	for raw, want := range map[string]string{"ERROR failure": "ERROR", "WARN retry": "WARN", "INFO stderr is not necessarily an error": "INFO", "database error rate": "UNKNOWN", `{"level":"fatal","message":"stopped"}`: "ERROR", `{"severity":"warning"}`: "WARN", `{"level":"invented"}`: "UNKNOWN", "[DEBUG] trace": "DEBUG"} {
		if got := Level(raw); got != want {
			t.Fatalf("level(%q)=%s want %s", raw, got, want)
		}
	}
}
