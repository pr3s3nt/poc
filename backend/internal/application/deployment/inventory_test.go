package deployment

import (
	"context"
	"reflect"
	"testing"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/execution"
)

func owned(kind, name string) execution.Manifest {
	api := "v1"
	switch kind {
	case "Deployment", "StatefulSet":
		api = "apps/v1"
	case "Job":
		api = "batch/v1"
	case "HorizontalPodAutoscaler":
		api = "autoscaling/v2"
	}
	return execution.Manifest{APIVersion: api, Kind: kind, Name: name, Object: map[string]any{"metadata": map[string]any{"labels": map[string]any{"orchestrator.io/workload": "web"}}}}
}

func TestInventoryUnionKeepsPreviousObjectsUntilPruned(t *testing.T) {
	previous := []execution.ObjectRef{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", Owners: []string{"d1"}},
		{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "web", Owners: []string{"d1"}},
	}
	next := renderedInventory([]execution.Manifest{
		owned("Deployment", "web"),
		owned("ConfigMap", "web-files"),
		{APIVersion: "v1", Kind: "Secret", Name: "web-env", Secret: true, Auxiliary: true, Object: map[string]any{}},
		{APIVersion: "v1", Kind: "ServiceAccount", Name: "web-vault", Auxiliary: true, Object: map[string]any{}},
	}, "d2")
	if len(next) != 2 {
		t.Fatalf("auxiliaries must not be inventory: %v", next)
	}
	union, stale := mergeInventory(previous, next)
	if len(union) != 3 || len(stale) != 1 || stale[0].Kind != "HorizontalPodAutoscaler" {
		t.Fatalf("union %v stale %v", union, stale)
	}
	for _, ref := range union {
		if ref.Kind == "Deployment" && !reflect.DeepEqual(ref.Owners, []string{"d2", "d1"}) {
			t.Fatalf("an object in both sets may carry either owner: %v", ref.Owners)
		}
	}
	// The persisted form round-trips through the TargetRef JSON shape.
	instance := domain.WorkloadInstance{TargetRef: withInventory(map[string]any{}, union)}
	raw := map[string]any{}
	for key, value := range instance.TargetRef {
		raw[key] = value
	}
	if got := instanceInventory(domain.WorkloadInstance{TargetRef: raw}); !reflect.DeepEqual(got, union) {
		t.Fatalf("round trip: %v != %v", got, union)
	}
	if ref := withInventory(map[string]any{inventoryKey: []any{}}, nil); ref[inventoryKey] != nil {
		t.Fatal("an empty inventory is not persisted")
	}
}

type phaseRecorder struct {
	calls []string
}

func (p *phaseRecorder) Apply(_ context.Context, _ execution.Target, manifests []execution.Manifest) error {
	call := "apply"
	for _, m := range manifests {
		call += " " + m.Kind
	}
	p.calls = append(p.calls, call)
	return nil
}

func (p *phaseRecorder) WaitReady(_ context.Context, _ execution.Target, refs []execution.WorkloadRef) error {
	call := "wait"
	for _, r := range refs {
		call += " " + r.Kind
	}
	p.calls = append(p.calls, call)
	return nil
}

func (p *phaseRecorder) Remove(context.Context, execution.Target, string) error { return nil }

func TestJobsCompleteBeforeRolloutObjects(t *testing.T) {
	recorder := &phaseRecorder{}
	s := &Service{deployer: recorder}
	manifests := []execution.Manifest{owned("Deployment", "web"), owned("Job", "web-migrate"), owned("ConfigMap", "web-files"), owned("StatefulSet", "web-cache")}
	if err := s.applyPhases(context.Background(), execution.Target{Namespace: "ns"}, "web", manifests); err != nil {
		t.Fatal(err)
	}
	want := []string{"apply Job ConfigMap", "wait Job", "apply Deployment StatefulSet", "wait Deployment StatefulSet"}
	if !reflect.DeepEqual(recorder.calls, want) {
		t.Fatalf("got %v want %v", recorder.calls, want)
	}
	recorder.calls = nil
	if err := s.applyPhases(context.Background(), execution.Target{Namespace: "ns"}, "web", manifests[:1]); err != nil {
		t.Fatal(err)
	}
	if want := []string{"apply Deployment", "wait Deployment"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Fatalf("without Jobs one apply: %v", recorder.calls)
	}
	if err := s.removeObjects(context.Background(), execution.Target{}, "web", []execution.ObjectRef{{Kind: "ConfigMap"}}); err == nil {
		t.Fatal("a deployer without inventory support must fail closed")
	}
}
