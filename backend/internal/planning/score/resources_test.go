package score

import (
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
)

// withResources returns the base document whose main container declares the
// given `resources` value verbatim.
func withResources(resources any) map[string]any {
	doc := base()
	main := doc["containers"].(map[string]any)["main"].(map[string]any)
	main["resources"] = resources
	return doc
}

func parseRaw(t *testing.T, raw string) (*Document, error) {
	t.Helper()
	return Parse([]byte(raw))
}

func TestParse_ContainerResourcesFullRequestsAndLimits(t *testing.T) {
	doc := withResources(map[string]any{
		"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		"limits":   map[string]any{"cpu": "500m", "memory": "512Mi"},
	})
	f := fragment(t, doc)
	got := f.Module.Spec.Containers["main"].Resources
	want := &environment.ContainerResourceRequirements{
		Requests: &environment.ComputeResources{CPU: "100m", Memory: "128Mi"},
		Limits:   &environment.ComputeResources{CPU: "500m", Memory: "512Mi"},
	}
	assertResources(t, got, want)
}

func TestParse_ContainerResourcesPartialRequestsAndLimits(t *testing.T) {
	doc := withResources(map[string]any{
		"requests": map[string]any{"memory": "64Mi"},
		"limits":   map[string]any{"cpu": "250m"},
	})
	f := fragment(t, doc)
	got := f.Module.Spec.Containers["main"].Resources
	want := &environment.ContainerResourceRequirements{
		Requests: &environment.ComputeResources{Memory: "64Mi"},
		Limits:   &environment.ComputeResources{CPU: "250m"},
	}
	assertResources(t, got, want)

	doc = withResources(map[string]any{"limits": map[string]any{"memory": "1Gi"}})
	f = fragment(t, doc)
	got = f.Module.Spec.Containers["main"].Resources
	want = &environment.ContainerResourceRequirements{Limits: &environment.ComputeResources{Memory: "1Gi"}}
	assertResources(t, got, want)
}

func TestParse_ContainerResourcesOmitted(t *testing.T) {
	f := fragment(t, base())
	if got := f.Module.Spec.Containers["main"].Resources; got != nil {
		t.Fatalf("omitted resources must stay absent, got %#v", got)
	}
	tree := moduleJSON(t, f.Module)
	if strings.Contains(tree, "\"resources\"") {
		t.Fatalf("omitted resources must not appear in the fragment: %s", tree)
	}
}

// Valid values keep their exact spelling; the parser does not normalise
// Kubernetes quantities.
func TestParse_ContainerResourcesPreserveValuesVerbatim(t *testing.T) {
	doc := withResources(map[string]any{
		"requests": map[string]any{"cpu": "0.25", "memory": "134217728"},
		"limits":   map[string]any{"cpu": "1", "memory": "1.5Gi"},
	})
	f := fragment(t, doc)
	tree := moduleJSON(t, f.Module)
	want := `"resources":{"requests":{"cpu":"0.25","memory":"134217728"},"limits":{"cpu":"1","memory":"1.5Gi"}}`
	if !strings.Contains(tree, want) {
		t.Fatalf("resources were not preserved verbatim:\nwant substring %s\ngot %s", want, tree)
	}
}

func TestParse_ContainerResourcesRejectsInvalidShapes(t *testing.T) {
	cases := map[string]string{
		"unknown branch":           `{"requests":{"cpu":"1"},"claims":{"cpu":"1"}}`,
		"unknown resource key":     `{"requests":{"cpu":"1","nvidia.com/gpu":"1"}}`,
		"ephemeral storage key":    `{"limits":{"ephemeral-storage":"1Gi"}}`,
		"number value":             `{"requests":{"cpu":1}}`,
		"number memory value":      `{"limits":{"memory":512}}`,
		"null value":               `{"requests":{"cpu":null}}`,
		"null branch":              `{"requests":null}`,
		"null resources":           `null`,
		"empty string value":       `{"requests":{"cpu":""}}`,
		"empty string limit":       `{"limits":{"memory":""}}`,
		"branch is a string":       `{"requests":"100m"}`,
		"resources is a list":      `[{"cpu":"1"}]`,
		"resources is a string":    `"small"`,
		"boolean value":            `{"limits":{"cpu":true}}`,
		"nested object as a value": `{"limits":{"cpu":{"value":"1"}}}`,
	}
	for name, resources := range cases {
		t.Run(name, func(t *testing.T) {
			raw := `{"apiVersion":"score.dev/v1b1","metadata":{"name":"backend"},` +
				`"containers":{"main":{"image":"backend:dev","resources":` + resources + `}}}`
			if _, err := parseRaw(t, raw); err == nil {
				t.Fatalf("expected %s to be rejected", resources)
			}
		})
	}
}

func TestParse_ContainerResourcesRejectsInvalidShapesThroughFromMap(t *testing.T) {
	for name, resources := range map[string]any{
		"unknown key":  map[string]any{"requests": map[string]any{"storage": "1Gi"}},
		"number value": map[string]any{"limits": map[string]any{"cpu": 2}},
		"nil value":    map[string]any{"limits": map[string]any{"memory": nil}},
		"empty value":  map[string]any{"requests": map[string]any{"memory": ""}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := FromMap(withResources(resources)); err == nil {
				t.Fatalf("expected %v to be rejected", resources)
			}
		})
	}
}

// Every container keeps its own requirements.
func TestFragment_ContainerResourcesStayPerContainer(t *testing.T) {
	doc := base()
	doc["containers"] = map[string]any{
		"main": map[string]any{
			"image":     "backend:dev",
			"variables": map[string]any{"PGHOST": "${resources.db.host}"},
			"resources": map[string]any{"requests": map[string]any{"cpu": "100m"}},
		},
		"sidecar": map[string]any{
			"image":     "sidecar:dev",
			"resources": map[string]any{"limits": map[string]any{"memory": "64Mi"}},
		},
		"plain": map[string]any{"image": "plain:dev"},
	}
	f := fragment(t, doc)
	assertResources(t, f.Module.Spec.Containers["main"].Resources,
		&environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "100m"}})
	assertResources(t, f.Module.Spec.Containers["sidecar"].Resources,
		&environment.ContainerResourceRequirements{Limits: &environment.ComputeResources{Memory: "64Mi"}})
	if f.Module.Spec.Containers["plain"].Resources != nil {
		t.Fatal("a container without resources must not gain requirements")
	}
}

func assertResources(t *testing.T, got, want *environment.ContainerResourceRequirements) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("container resources:\nwant %s\ngot  %s", wantJSON, gotJSON)
	}
}

func moduleJSON(t *testing.T, m environment.Module) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
