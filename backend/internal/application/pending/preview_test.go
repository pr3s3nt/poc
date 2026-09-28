package pending

import (
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
)

func TestValidateImageRegistryForFleetPreview(t *testing.T) {
	doc := &score.Document{Containers: map[string]environment.Container{"main": {Image: "10.96.91.170:80/library/app:v1"}}}
	if err := validateImageRegistry(doc, "10.96.91.170:80"); err != nil {
		t.Fatal(err)
	}
	doc.Containers["main"] = environment.Container{Image: "docker.io/library/app:v1"}
	if err := validateImageRegistry(doc, "10.96.91.170:80"); err == nil {
		t.Fatal("foreign registry accepted")
	}
	if err := validateImageRegistry(doc, ""); err != nil {
		t.Fatalf("direct delivery must stay unchanged: %v", err)
	}
}
