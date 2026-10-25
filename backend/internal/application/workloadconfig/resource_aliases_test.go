package workloadconfig

import (
	"reflect"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
)

func TestSharedAliasesSurviveReconstructionIncludingManyToOneAndNestedParams(t *testing.T) {
	doc := score.Document{APIVersion: score.SupportedAPIVersion, Metadata: score.Metadata{Name: "api"},
		Containers: map[string]environment.Container{"web": {Image: "i", Variables: map[string]string{
			"A": "${resources.db.host}", "B": "${resources.also_db.host}", "C": "${resources.shared_1.host}", "D": "${resources.private.host}",
		}}}, Resources: map[string]score.ResourceSpec{
			"db": {Type: "postgres", ID: "a"}, "also_db": {Type: "postgres", ID: "a"}, "shared_1": {Type: "postgres", ID: "b"},
			"private": {Type: "postgres", Params: map[string]any{"database": "prefix-${resources.also_db.host}"}},
		}}
	container := doc.Containers["web"]
	container.Args = []string{"${resources.shared_1.host}", "$${resources.db.host}"}
	doc.Containers["web"] = container
	fragment, err := doc.Fragment(pgTypes())
	if err != nil {
		t.Fatal(err)
	}
	set := environment.NewDocument()
	set.Modules["api"], set.Shared = fragment.Module, fragment.Shared
	metadata := KeepResourceAliases(nil, set, &doc)
	raw, err := ReconstructVerified("api", fragment.Module, set, pgTypes(), metadata["api"])
	if err != nil {
		t.Fatal(err)
	}
	restored, err := score.FromMap(raw)
	if err != nil || !reflect.DeepEqual(restored.Containers["web"].Variables, doc.Containers["web"].Variables) || !reflect.DeepEqual(restored.Resources, doc.Resources) || !reflect.DeepEqual(restored.Containers["web"].Args, container.Args) {
		t.Fatalf("aliases or nested bindings changed: %+v %v", restored, err)
	}
	// Historical sets retain the deterministic generated-name fallback.
	legacy, err := ReconstructScore("api", fragment.Module, set)
	if err != nil || legacy["resources"].(map[string]any)["shared_1"] == nil {
		t.Fatalf("legacy reconstruction failed: %v", err)
	}
}

func TestAliasMetadataCannotRetargetBindingsOrAuthorizeLiteralReadback(t *testing.T) {
	doc := score.Document{APIVersion: score.SupportedAPIVersion, Metadata: score.Metadata{Name: "api"},
		Containers: map[string]environment.Container{"web": {Image: "i", Variables: map[string]string{"A": "${resources.db.host}"}}},
		Resources:  map[string]score.ResourceSpec{"db": {Type: "postgres", ID: "a"}},
	}
	fragment, _ := doc.Fragment(pgTypes())
	set := environment.NewDocument()
	set.Modules["api"], set.Shared = fragment.Module, fragment.Shared
	metadata := KeepResourceAliases(nil, set, &doc)
	metadata["api"].References["/containers/web/variables/A"] = "${resources.other.host}"
	raw, err := ReconstructScore("api", fragment.Module, set, metadata["api"])
	if err != nil || raw["containers"].(map[string]any)["web"].(map[string]any)["variables"].(map[string]any)["A"] != "${resources.db.host}" {
		t.Fatalf("stale metadata retargeted a binding: %v %v", raw, err)
	}
	fragment.Module.Spec.Containers["web"].Variables["A"] = "unproven"
	if _, err := ReconstructScore("api", fragment.Module, set, metadata["api"]); err == nil {
		t.Fatal("alias metadata authorized an unproven literal")
	}
	delete(set.Modules, "api")
	if got := KeepResourceAliases(metadata, set, nil); got != nil {
		t.Fatalf("removed workload kept stale metadata: %+v", got)
	}
}
