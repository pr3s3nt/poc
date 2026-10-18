package score

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// ADR-016: optional `metadata.class` (product extension). Missing is the
// default class; an explicit class is kept exactly as written.

func withMetadata(metadata map[string]any) map[string]any {
	doc := base()
	doc["metadata"] = metadata
	return doc
}

func TestMetadataClassMissingMeansDefaultAndIsNotInvented(t *testing.T) {
	f := fragment(t, base())
	if f.Module.Class != "" || f.Module.WorkloadClass() != "default" {
		t.Fatalf("class = %q effective %q", f.Module.Class, f.Module.WorkloadClass())
	}
}

func TestMetadataClassExplicitValuesAreKeptVerbatim(t *testing.T) {
	for _, class := range []string{"gold", "default", "team-a_1"} {
		f := fragment(t, withMetadata(map[string]any{"name": "backend", "class": class}))
		if f.Module.Class != class || f.Module.WorkloadClass() != class {
			t.Fatalf("%q became %q", class, f.Module.Class)
		}
	}
}

func TestMetadataClassRejectsBlankMalformedAndWrongTypes(t *testing.T) {
	for name, class := range map[string]any{
		"blank": "", "spaces": "  ", "upper": "Gold", "dot": "a.b", "space": "a b", "leading dash": "-a", "too long": string(make([]byte, 64)),
		"null": nil, "number": 3, "object": map[string]any{"a": 1}, "list": []any{"a"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := FromMap(withMetadata(map[string]any{"name": "backend", "class": class}))
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("must be rejected: %v", err)
			}
		})
	}
	if _, err := FromMap(withMetadata(map[string]any{"name": "backend", "labels": "x"})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("other metadata fields stay unsupported: %v", err)
	}
}

func TestMetadataClassSurvivesJSONRoundTrip(t *testing.T) {
	parsed, err := FromMap(withMetadata(map[string]any{"name": "backend", "class": "gold"}))
	if err != nil {
		t.Fatal(err)
	}
	again, err := FromMap(mustMap(t, parsed))
	if err != nil || !reflect.DeepEqual(again.Metadata, parsed.Metadata) || again.Metadata.Class != "gold" {
		t.Fatalf("round trip: %+v %v", again, err)
	}
}

func mustMap(t *testing.T, doc *Document) map[string]any {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
