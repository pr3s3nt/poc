package planning

import (
	"reflect"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
)

func module(image string, args ...string) environment.Module {
	return environment.Module{
		Profile: environment.ModuleProfile,
		Spec: environment.ModuleSpec{Containers: map[string]environment.Container{
			"main": {ID: "main", Image: image, Args: args},
		}},
	}
}

func deltaJSON(t *testing.T, d deployment.DeltaDocument) string {
	t.Helper()
	b, err := canon.Bytes(d)
	if err != nil {
		t.Fatalf("canon: %v", err)
	}
	return string(b)
}

func buildAndVerify(t *testing.T, base, candidate environment.Document) deployment.DeltaDocument {
	t.Helper()
	delta, err := DiffDeploymentSets(base, candidate)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	applied, err := ApplyHumanitecDelta(base, delta)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	want, _ := canon.Map(candidate)
	got, _ := canon.Map(applied)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("base + delta != candidate\n got: %v\nwant: %v", got, want)
	}
	return delta
}

func TestDelta_NoOpIsEmptyObject(t *testing.T) {
	base := environment.NewDocument()
	base.Modules["api"] = module("api:v1")
	delta := buildAndVerify(t, base, base)
	if got := deltaJSON(t, delta); got != "{}" {
		t.Fatalf("no-op delta must be {}, got %s", got)
	}
	if !delta.IsEmpty() {
		t.Fatal("no-op delta must report empty")
	}
}

func TestDelta_ModuleAddCarriesFullModule(t *testing.T) {
	base := environment.NewDocument()
	candidate := environment.NewDocument()
	candidate.Modules["api"] = module("api:v1")
	delta := buildAndVerify(t, base, candidate)
	want := `{"modules":{"add":{"api":{"profile":"humanitec/default-module","spec":{"containers":{"main":{"id":"main","image":"api:v1"}}}}}}}`
	if got := deltaJSON(t, delta); got != want {
		t.Fatalf("delta\n got: %s\nwant: %s", got, want)
	}
}

func TestDelta_ModuleRemoveListsSortedIDs(t *testing.T) {
	base := environment.NewDocument()
	base.Modules["worker"] = module("w:v1")
	base.Modules["api"] = module("api:v1")
	base.Modules["keep"] = module("k:v1")
	candidate := environment.NewDocument()
	candidate.Modules["keep"] = module("k:v1")
	delta := buildAndVerify(t, base, candidate)
	if got, want := deltaJSON(t, delta), `{"modules":{"remove":["api","worker"]}}`; got != want {
		t.Fatalf("delta\n got: %s\nwant: %s", got, want)
	}
}

func TestDelta_ModuleUpdateIsRelativeToTheModule(t *testing.T) {
	base := environment.NewDocument()
	base.Modules["api"] = module("api:v1")
	base.Modules["other"] = module("o:v1")
	candidate := environment.NewDocument()
	candidate.Modules["api"] = module("api:v2")
	candidate.Modules["other"] = module("o:v1")
	delta := buildAndVerify(t, base, candidate)
	want := `{"modules":{"update":{"api":[{"op":"replace","path":"/spec/containers/main/image","value":"api:v2"}]}}}`
	if got := deltaJSON(t, delta); got != want {
		t.Fatalf("delta\n got: %s\nwant: %s", got, want)
	}
}

func TestDelta_ModuleArrayDiffIsIndexBased(t *testing.T) {
	base := environment.NewDocument()
	base.Modules["api"] = module("api:v1", "a", "b", "c", "d")
	candidate := environment.NewDocument()
	candidate.Modules["api"] = module("api:v1", "a", "x")
	delta := buildAndVerify(t, base, candidate)
	want := `{"modules":{"update":{"api":[` +
		`{"op":"replace","path":"/spec/containers/main/args/1","value":"x"},` +
		`{"op":"remove","path":"/spec/containers/main/args/3"},` +
		`{"op":"remove","path":"/spec/containers/main/args/2"}]}}}`
	if got := deltaJSON(t, delta); got != want {
		t.Fatalf("delta\n got: %s\nwant: %s", got, want)
	}

	grown := environment.NewDocument()
	grown.Modules["api"] = module("api:v1", "a", "b", "c", "d", "e", "f")
	delta = buildAndVerify(t, base, grown)
	want = `{"modules":{"update":{"api":[` +
		`{"op":"add","path":"/spec/containers/main/args/-","value":"e"},` +
		`{"op":"add","path":"/spec/containers/main/args/-","value":"f"}]}}}`
	if got := deltaJSON(t, delta); got != want {
		t.Fatalf("delta\n got: %s\nwant: %s", got, want)
	}
}

func TestDelta_SharedPatchIsRelativeToSharedObject(t *testing.T) {
	base := environment.NewDocument()
	base.Shared["cache"] = environment.ResourceEntry{Type: "redis", Class: "default", Params: map[string]any{"memory": 2}}
	base.Shared["gone"] = environment.ResourceEntry{Type: "s3", Class: "default"}
	base.Shared["kept"] = environment.ResourceEntry{Type: "postgres", Class: "default"}
	candidate := environment.NewDocument()
	candidate.Shared["cache"] = environment.ResourceEntry{Type: "redis", Class: "default", Params: map[string]any{"memory": 4}}
	candidate.Shared["db"] = environment.ResourceEntry{Type: "postgres", Class: "default"}
	candidate.Shared["kept"] = environment.ResourceEntry{Type: "postgres", Class: "default"}
	delta := buildAndVerify(t, base, candidate)
	want := `{"shared":[` +
		`{"op":"replace","path":"/cache/params/memory","value":4},` +
		`{"op":"add","path":"/db","value":{"class":"default","type":"postgres"}},` +
		`{"op":"remove","path":"/gone"}]}`
	if got := deltaJSON(t, delta); got != want {
		t.Fatalf("delta\n got: %s\nwant: %s", got, want)
	}
}

func TestDelta_CombinesModulesAndShared(t *testing.T) {
	base := environment.NewDocument()
	base.Modules["old"] = module("old:v1")
	base.Modules["api"] = module("api:v1")
	candidate := environment.NewDocument()
	candidate.Modules["api"] = module("api:v2")
	candidate.Modules["new"] = module("new:v1")
	candidate.Shared["db"] = environment.ResourceEntry{Type: "postgres", Class: "default"}
	delta := buildAndVerify(t, base, candidate)
	if delta.Modules == nil || len(delta.Modules.Add) != 1 || len(delta.Modules.Remove) != 1 || len(delta.Modules.Update) != 1 || len(delta.Shared) != 1 {
		t.Fatalf("unexpected delta: %s", deltaJSON(t, delta))
	}
}

func TestApplyHumanitecDelta_RejectsInconsistentDocuments(t *testing.T) {
	base := environment.NewDocument()
	base.Modules["api"] = module("api:v1")
	cases := map[string]deployment.DeltaDocument{
		"add existing module":   {Modules: &deployment.ModuleDelta{Add: map[string]environment.Module{"api": module("x")}}},
		"remove missing module": {Modules: &deployment.ModuleDelta{Remove: []string{"ghost"}}},
		"update missing module": {Modules: &deployment.ModuleDelta{Update: map[string][]deployment.JSONPatchOperation{"ghost": {{Op: "remove", Path: "/spec"}}}}},
		"bad shared patch":      {Shared: []deployment.JSONPatchOperation{{Op: "remove", Path: "/ghost"}}},
		"unknown module field":  {Modules: &deployment.ModuleDelta{Update: map[string][]deployment.JSONPatchOperation{"api": {{Op: "add", Path: "/bogus", Value: 1}}}}},
	}
	for name, delta := range cases {
		if _, err := ApplyHumanitecDelta(base, delta); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func TestPlan_DeltaIsHumanitecShapedAndDeterministic(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if first.Delta.Modules == nil || len(first.Delta.Modules.Add) != 1 {
		t.Fatalf("first deploy must add one module, got %s", deltaJSON(t, first.Delta))
	}
	if _, ok := first.Delta.Modules.Add["backend"]; !ok {
		t.Fatalf("expected modules.add.backend, got %s", deltaJSON(t, first.Delta))
	}
	for i := 0; i < 3; i++ {
		again, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if again.PlanHash != first.PlanHash {
			t.Fatalf("plan hash is not deterministic: %s vs %s", again.PlanHash, first.PlanHash)
		}
		if deltaJSON(t, again.Delta) != deltaJSON(t, first.Delta) {
			t.Fatal("delta serialization is not deterministic")
		}
		a, _ := canon.Bytes(again)
		b, _ := canon.Bytes(first)
		if string(a) != string(b) {
			t.Fatal("plan serialization is not deterministic")
		}
	}
	serialized, err := canon.Map(first)
	if err != nil {
		t.Fatalf("canon: %v", err)
	}
	if _, ok := serialized["delta"]; ok {
		t.Fatal("the plan snapshot must not embed a whole-document delta; the Delta Snapshot is persisted separately")
	}
}

func TestPlan_RedeployingSameScoreIsNoOpDelta(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.BaseSet = first.CandidateSet
	req.Before = req.After
	plan, err := svc.Plan(req)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got := deltaJSON(t, plan.Delta); got != "{}" {
		t.Fatalf("expected no-op delta {}, got %s", got)
	}
}

func TestPlan_RemoveWorkloadDeltaListsModule(t *testing.T) {
	svc := NewService()
	first, err := svc.Plan(testRequest(t, application.ProfileInternalK8s, "backend"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	req.BaseSet = first.CandidateSet
	req.Before = req.After
	req.After = nil
	plan, err := svc.Plan(req)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Delta.Modules == nil || !reflect.DeepEqual(plan.Delta.Modules.Remove, []string{"backend"}) {
		t.Fatalf("expected modules.remove [backend], got %s", deltaJSON(t, plan.Delta))
	}
}
