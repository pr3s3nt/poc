package workloadspec

import (
	"errors"
	"reflect"
	"testing"
)

// Issue #63: the authored variable order survives storage and the Deployment Set.

func withVariables(order []any) map[string]any {
	doc := base()
	doc["resources"] = map[string]any{}
	container := map[string]any{"image": "backend:dev", "variables": map[string]any{"ZETA": "z", "ALPHA": "a", "MID": "m"}}
	if order != nil {
		container["variableOrder"] = order
	}
	doc["containers"] = map[string]any{"main": container}
	return doc
}

func TestVariableOrderIsKeptThroughStorageAndTheDeploymentSet(t *testing.T) {
	parsed, err := FromMap(withVariables([]any{"ZETA", "ALPHA", "MID"}))
	if err != nil {
		t.Fatal(err)
	}
	stored := parsed.Map()["containers"].(map[string]any)["main"].(map[string]any)["variableOrder"]
	if !reflect.DeepEqual(stored, []any{"ZETA", "ALPHA", "MID"}) {
		t.Fatalf("stored order %v", stored)
	}
	f := fragment(t, withVariables([]any{"ZETA", "ALPHA", "MID"}))
	if got := f.Module.Spec.Containers["main"].VariableOrder; !reflect.DeepEqual(got, []string{"ZETA", "ALPHA", "MID"}) {
		t.Fatalf("module order %v", got)
	}
}

func TestVariableOrderIsOptionalAndMayListSomeVariables(t *testing.T) {
	for _, order := range [][]any{nil, {"MID"}} {
		if _, err := FromMap(withVariables(order)); err != nil {
			t.Fatalf("%v rejected: %v", order, err)
		}
	}
	parsed, err := FromMap(withVariables(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, present := parsed.Map()["containers"].(map[string]any)["main"].(map[string]any)["variableOrder"]; present {
		t.Fatal("an omitted order must stay omitted")
	}
}

func TestVariableOrderRejectsDuplicatesAndUndeclaredNames(t *testing.T) {
	for _, order := range [][]any{{"ZETA", "ZETA"}, {"OTHER"}, {""}} {
		if _, err := FromMap(withVariables(order)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%v: err = %v, want ErrInvalid", order, err)
		}
	}
}
