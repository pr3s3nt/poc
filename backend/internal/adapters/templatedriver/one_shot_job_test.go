package templatedriver

import (
	"context"
	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/renderengine"
	"testing"
)

func TestOneShotJobRendersAndGuardsExecution(t *testing.T) {
	files := readBundle(t, standardDir)
	if _, err := renderengine.Validate(files); err != nil {
		t.Fatal(err)
	}
	renderer, selections := gitRenderer(t, map[string]map[string][]byte{"standard": files})
	req := gitRequest(selections["standard"])
	req.Module.Spec.Service = nil
	req.Module.Spec.Replicas = nil
	zero := int32(0)
	deadline := int64(300)
	req.Module.Spec.Execution = &environment.WorkloadExecution{Version: 1, Kind: "Job", Job: &environment.JobPolicy{BackoffLimit: &zero, ActiveDeadlineSeconds: &deadline}, WaitForJobs: []string{"prepare"}}
	c := req.Module.Spec.Containers["main"]
	c.Command = []string{"/bin/sh", "-c"}
	c.Args = []string{"echo migrate"}
	req.Module.Spec.Containers["main"] = c
	objects, err := renderer.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var job *execution.Manifest
	for i := range objects {
		if objects[i].Kind == "Job" {
			job = &objects[i]
		}
	}
	if job == nil {
		t.Fatal("primary Job absent")
	}
	spec := object(job.Object["spec"])
	if spec["backoffLimit"] != float64(0) || spec["activeDeadlineSeconds"] != float64(300) || spec["replicas"] != nil || spec["selector"] != nil || spec["strategy"] != nil {
		t.Fatalf("invalid Job spec: %v", spec)
	}
	pod := object(object(spec["template"])["spec"])
	if pod["restartPolicy"] != "Never" {
		t.Fatal("Job restart policy")
	}
	// Mutation tests exercise the same guard as production, including omission/default rules.
	delivery, err := kubernetes.PrepareDelivery(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	spec["backoffLimit"] = float64(1)
	if guard(req, delivery, objects) == nil {
		t.Fatal("extra retry accepted")
	}
	spec["backoffLimit"] = float64(0)
	pod["restartPolicy"] = "OnFailure"
	if guard(req, delivery, objects) == nil {
		t.Fatal("implicit application restart accepted")
	}
	pod["restartPolicy"] = "Never"
	delete(spec, "activeDeadlineSeconds")
	if guard(req, delivery, objects) == nil {
		t.Fatal("dropped Job deadline accepted")
	}

}
