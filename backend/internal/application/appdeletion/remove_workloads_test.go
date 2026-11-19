package appdeletion

import (
	"context"
	"testing"

	"orchestrator/internal/application/workloadconfig"
)

type emptyWorkloads struct{ Workloads }

func (emptyWorkloads) List(context.Context, string, string) (workloadconfig.View, error) {
	return workloadconfig.View{}, nil
}

func TestRemoveEmptyWorkloadsNeedsNoDeliveryPreview(t *testing.T) {
	runner := runner{s: &Service{workloads: emptyWorkloads{}}, appKey: "empty"}
	if code := runner.removeWorkloads(context.Background(), 0, "staging"); code != "" {
		t.Fatalf("empty environment cleanup returned %q", code)
	}
}
