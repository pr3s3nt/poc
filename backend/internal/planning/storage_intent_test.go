package planning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

func TestStorageParametersDoNotSilentlyReplaceOrResizeClaims(t *testing.T) {
	before := environment.NewDocument()
	before.Shared["data"] = environment.ResourceEntry{Type: "storage", Params: map[string]any{"storage": "2Gi"}}
	catalog := Catalog{Types: map[string]resource.Type{"storage": {Outputs: []resource.OutputField{{Name: "claimName", Type: "string"}, {Name: "namespace", Type: "string"}}}}}
	for _, size := range []string{"1Gi", "3Gi"} {
		after := environment.NewDocument()
		after.Shared["data"] = environment.ResourceEntry{Type: "storage", Params: map[string]any{"storage": size}}
		var staged *StageError
		if err := validateStorageIntent(before, after, catalog); !errors.As(err, &staged) || staged.Reason != ReasonStorageImmutable {
			t.Fatalf("size %s admitted: %v", size, err)
		}
	}
	if err := validateStorageIntent(before, before, catalog); err != nil {
		t.Fatal(err)
	}
}
