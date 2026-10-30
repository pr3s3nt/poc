package fleet

import (
	"context"
	"encoding/json"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
	"testing"
)

func TestCompletedExecutionComparisonKeepsBusinessSpecAndIgnoresProvenance(t *testing.T) {
	i := packageIntent()
	i.Revision.Phase = domain.Job
	job := configMap()
	job.APIVersion = "batch/v1"
	job.Kind = "Job"
	job.Object["apiVersion"] = job.APIVersion
	job.Object["kind"] = job.Kind
	delete(job.Object, "data")
	md := job.Object["metadata"].(map[string]any)
	md["labels"].(map[string]any)["orchestrator.io/deployment-id"] = "first"
	job.Object["spec"] = map[string]any{"template": map[string]any{"spec": map[string]any{"restartPolicy": "Never", "containers": []any{map[string]any{"name": "work", "image": "example:v1"}}}}}
	pkg, err := PackageUnit(i, []execution.Manifest{job})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pkg)
	md["labels"].(map[string]any)["orchestrator.io/deployment-id"] = "next"
	p := &Provider{}
	same, err := p.SameExecution(context.Background(), raw, []execution.Manifest{job})
	if err != nil || !same {
		t.Fatal("unchanged execution lost receipt", err)
	}
	spec := job.Object["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	pod := template["spec"].(map[string]any)
	pod["containers"].([]any)[0].(map[string]any)["image"] = "example:v2"
	same, err = p.SameExecution(context.Background(), raw, []execution.Manifest{job})
	if err != nil || same {
		t.Fatal("changed execution accepted", err)
	}
	spec["ttlSecondsAfterFinished"] = 30
	if _, err := PackageUnit(i, []execution.Manifest{job}); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal("TTL recreation race admitted", err)
	}
}
