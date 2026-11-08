package planning

import (
	"orchestrator/internal/domain/environment"
	"testing"
)

func TestJobCompletionDependencyGraph(t *testing.T) {
	job := environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Version: 1, Kind: "Job"}}}
	consumer := environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", WaitForJobs: []string{"migrate"}}}}
	set := environment.Document{Modules: map[string]environment.Module{"migrate": job, "web": consumer}}
	build := func(set environment.Document, intermediate bool) (*graphBuilder, error) {
		b := newGraphBuilder(Context{}, Catalog{})
		b.allowIntermediate = intermediate
		err := b.buildFromSet(set, nil)
		return b, err
	}
	b, err := build(set, false)
	if err != nil {
		t.Fatal(err)
	}
	g := b.graph()
	found := false
	for _, e := range g.Edges {
		if e.Reason == ReasonJobCompletion {
			found = true
		}
	}
	if !found {
		t.Fatal("completion edge missing")
	}
	if _, _, err := schedule(g); err != nil {
		t.Fatal(err)
	}
	delete(set.Modules, "migrate")
	if _, err := build(set, false); err == nil {
		t.Fatal("missing Job accepted")
	}
	if _, err := build(set, true); err != nil {
		t.Fatal("incomplete draft rejected", err)
	}
	set.Modules["migrate"] = consumer
	if _, err := build(set, false); err == nil {
		t.Fatal("non-Job target accepted")
	}
	job.Spec.Execution.WaitForJobs = []string{"second"}
	second := environment.Module{Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Version: 1, Kind: "Job", WaitForJobs: []string{"migrate"}}}}
	set.Modules["migrate"] = job
	set.Modules["second"] = second
	b, err = build(set, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := schedule(b.graph()); err == nil {
		t.Fatal("completion cycle accepted")
	}
}

func TestJobCompletionFinalSetRejectsRetainedConsumerAndMixedCycle(t *testing.T) {
	set := environment.Document{Modules: map[string]environment.Module{
		"web":     {Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Version: 1, Kind: "Deployment", WaitForJobs: []string{"migrate"}}, Containers: map[string]environment.Container{"main": {Image: "web"}}}},
		"migrate": {Spec: environment.ModuleSpec{Execution: &environment.WorkloadExecution{Version: 1, Kind: "Job"}, Containers: map[string]environment.Container{"main": {Image: "job"}}}},
	}}
	if err := ValidateJobDependencies(set); err != nil {
		t.Fatal(err)
	}
	job := set.Modules["migrate"]
	job.Spec.Containers["main"] = environment.Container{Image: "job", Variables: map[string]string{"URL": "${context.service.web.http}"}}
	set.Modules["migrate"] = job
	if ValidateJobDependencies(set) == nil {
		t.Fatal("mixed Service/completion cycle accepted")
	}
	delete(set.Modules, "migrate")
	if ValidateJobDependencies(set) == nil {
		t.Fatal("removing Job with unchanged consumer accepted")
	}
}
