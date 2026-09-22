package kubernetes

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

func renderContainers(t *testing.T, containers map[string]environment.Container) map[string]map[string]any {
	t.Helper()
	module := environment.Module{
		Profile: environment.ModuleProfile,
		Spec:    environment.ModuleSpec{Containers: containers},
	}
	manifests, err := NewRenderer().Render(context.Background(), execution.RenderRequest{
		WorkloadID: "backend",
		Module:     module,
		Namespace:  "acceptance-dev",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := map[string]map[string]any{}
	for _, m := range manifests {
		if m.Kind != "Deployment" {
			continue
		}
		items := m.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
		for _, item := range items {
			c := item.(map[string]any)
			out[c["name"].(string)] = c["resources"].(map[string]any)
		}
	}
	if len(out) != len(containers) {
		t.Fatalf("rendered %d containers, want %d", len(out), len(containers))
	}
	return out
}

func assertRendered(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Fatalf("container resources:\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
}

func TestRender_DeclaredContainerResources(t *testing.T) {
	got := renderContainers(t, map[string]environment.Container{
		"main": {
			Image: "backend:dev",
			Resources: &environment.ContainerResourceRequirements{
				Requests: &environment.ComputeResources{CPU: "100m", Memory: "128Mi"},
				Limits:   &environment.ComputeResources{CPU: "500m", Memory: "512Mi"},
			},
		},
	})
	assertRendered(t, got["main"], map[string]any{
		"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		"limits":   map[string]any{"cpu": "500m", "memory": "512Mi"},
	})
}

// A missing request field falls back to the declared limit of the same field,
// else to the platform default; limits only contain the fields the Score
// declared.
func TestRender_PartialContainerResourcesFillMissingRequests(t *testing.T) {
	got := renderContainers(t, map[string]environment.Container{
		"cpu-only": {
			Image: "a:dev",
			Resources: &environment.ContainerResourceRequirements{
				Requests: &environment.ComputeResources{CPU: "250m"},
			},
		},
		"memory-only": {
			Image: "b:dev",
			Resources: &environment.ContainerResourceRequirements{
				Requests: &environment.ComputeResources{Memory: "64Mi"},
				Limits:   &environment.ComputeResources{Memory: "128Mi"},
			},
		},
		"limits-only": {
			Image: "c:dev",
			Resources: &environment.ContainerResourceRequirements{
				Limits: &environment.ComputeResources{CPU: "1"},
			},
		},
		"empty-branches": {
			Image: "d:dev",
			Resources: &environment.ContainerResourceRequirements{
				Requests: &environment.ComputeResources{},
				Limits:   &environment.ComputeResources{},
			},
		},
	})
	assertRendered(t, got["cpu-only"], map[string]any{
		"requests": map[string]any{"cpu": "250m", "memory": DefaultMemoryRequest},
	})
	assertRendered(t, got["memory-only"], map[string]any{
		"requests": map[string]any{"cpu": DefaultCPURequest, "memory": "64Mi"},
		"limits":   map[string]any{"memory": "128Mi"},
	})
	assertRendered(t, got["limits-only"], map[string]any{
		"requests": map[string]any{"cpu": "1", "memory": DefaultMemoryRequest},
		"limits":   map[string]any{"cpu": "1"},
	})
	assertRendered(t, got["empty-branches"], map[string]any{
		"requests": map[string]any{"cpu": DefaultCPURequest, "memory": DefaultMemoryRequest},
	})
}

func TestRender_OmittedContainerResourcesKeepDefaultRequestsWithoutLimits(t *testing.T) {
	got := renderContainers(t, map[string]environment.Container{"main": {Image: "backend:dev"}})
	assertRendered(t, got["main"], map[string]any{
		"requests": map[string]any{"cpu": "10m", "memory": "32Mi"},
	})
}

func TestRender_ContainerResourcesArePerContainer(t *testing.T) {
	got := renderContainers(t, map[string]environment.Container{
		"main": {
			Image: "backend:dev",
			Resources: &environment.ContainerResourceRequirements{
				Requests: &environment.ComputeResources{CPU: "100m", Memory: "128Mi"},
				Limits:   &environment.ComputeResources{Memory: "256Mi"},
			},
		},
		"sidecar": {Image: "sidecar:dev"},
	})
	assertRendered(t, got["main"], map[string]any{
		"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		"limits":   map[string]any{"memory": "256Mi"},
	})
	assertRendered(t, got["sidecar"], map[string]any{
		"requests": map[string]any{"cpu": "10m", "memory": "32Mi"},
	})
}

func TestRender_DoesNotMutateModuleResources(t *testing.T) {
	requests := &environment.ComputeResources{CPU: "100m"}
	limits := &environment.ComputeResources{}
	containers := map[string]environment.Container{
		"main": {
			Image:     "backend:dev",
			Resources: &environment.ContainerResourceRequirements{Requests: requests, Limits: limits},
		},
		"plain": {Image: "plain:dev"},
	}
	renderContainers(t, containers)
	if *requests != (environment.ComputeResources{CPU: "100m"}) {
		t.Fatalf("renderer mutated declared requests: %#v", *requests)
	}
	if *limits != (environment.ComputeResources{}) {
		t.Fatalf("renderer mutated declared limits: %#v", *limits)
	}
	if containers["plain"].Resources != nil {
		t.Fatal("renderer attached requirements to a container without resources")
	}
}

// A limit below the platform default must not produce a request above it:
// the request falls back to the limit of the same field (UC-06 BR-11).
func TestRender_LimitsOnlyBelowDefaultUseLimitAsRequest(t *testing.T) {
	got := renderContainers(t, map[string]environment.Container{
		"cpu": {
			Image: "a:dev",
			Resources: &environment.ContainerResourceRequirements{
				Limits: &environment.ComputeResources{CPU: "5m"},
			},
		},
		"memory": {
			Image: "b:dev",
			Resources: &environment.ContainerResourceRequirements{
				Limits: &environment.ComputeResources{Memory: "16Mi"},
			},
		},
		"both": {
			Image: "c:dev",
			Resources: &environment.ContainerResourceRequirements{
				Limits: &environment.ComputeResources{CPU: "5m", Memory: "16Mi"},
			},
		},
	})
	assertRendered(t, got["cpu"], map[string]any{
		"requests": map[string]any{"cpu": "5m", "memory": DefaultMemoryRequest},
		"limits":   map[string]any{"cpu": "5m"},
	})
	assertRendered(t, got["memory"], map[string]any{
		"requests": map[string]any{"cpu": DefaultCPURequest, "memory": "16Mi"},
		"limits":   map[string]any{"memory": "16Mi"},
	})
	assertRendered(t, got["both"], map[string]any{
		"requests": map[string]any{"cpu": "5m", "memory": "16Mi"},
		"limits":   map[string]any{"cpu": "5m", "memory": "16Mi"},
	})
}

// A declared request wins over the limit of the same field and is preserved
// even when it exceeds that limit. The renderer does not repair declared
// values; the Kubernetes API rejects such a manifest at apply time.
func TestRender_DeclaredRequestAboveLimitIsPreserved(t *testing.T) {
	got := renderContainers(t, map[string]environment.Container{
		"main": {
			Image: "a:dev",
			Resources: &environment.ContainerResourceRequirements{
				Requests: &environment.ComputeResources{CPU: "500m"},
				Limits:   &environment.ComputeResources{CPU: "100m", Memory: "16Mi"},
			},
		},
	})
	assertRendered(t, got["main"], map[string]any{
		"requests": map[string]any{"cpu": "500m", "memory": "16Mi"},
		"limits":   map[string]any{"cpu": "100m", "memory": "16Mi"},
	})
}

// Values derived at render time never reach the module or Candidate Set.
func TestRender_LimitFallbackDoesNotWriteBackRequests(t *testing.T) {
	requirements := &environment.ContainerResourceRequirements{
		Limits: &environment.ComputeResources{CPU: "5m", Memory: "16Mi"},
	}
	renderContainers(t, map[string]environment.Container{"main": {Image: "a:dev", Resources: requirements}})
	if requirements.Requests != nil {
		t.Fatalf("renderer wrote derived requests into the module: %#v", requirements.Requests)
	}
	if *requirements.Limits != (environment.ComputeResources{CPU: "5m", Memory: "16Mi"}) {
		t.Fatalf("renderer mutated declared limits: %#v", *requirements.Limits)
	}
}
