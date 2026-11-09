package deployment

import (
	"fmt"
	"testing"

	deliveryapp "orchestrator/internal/application/delivery"
)

// UC-06 CD-10: a determined rollout failure has its own safe summary, and the
// history projection keeps it instead of replacing it with the legacy text.
func TestRolloutFailuresHaveSafeSummaries(t *testing.T) {
	for cause, want := range map[error]string{
		deliveryapp.ErrRolloutDegraded: FailureRolloutDegraded,
		deliveryapp.ErrRolloutNotReady: FailureRolloutNotReady,
	} {
		wrapped := fmt.Errorf("workload backend: %w", cause)
		if got := PublicFailure(wrapped); got != want {
			t.Fatalf("PublicFailure(%v) = %q", cause, got)
		}
		if SafeFailureReason(want) != want {
			t.Fatalf("history hid the rollout summary %q", want)
		}
		if FailureCode(want) == "" {
			t.Fatalf("rollout summary %q has no Console code", want)
		}
	}
	if FailureCode(FailureRuntime) != "" || FailureCode("raw dial tcp text") != "" {
		t.Fatal("only known rollout summaries carry a code")
	}
}
