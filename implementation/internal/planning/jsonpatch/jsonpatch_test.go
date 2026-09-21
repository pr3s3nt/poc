package jsonpatch

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestDiffAndApplyKeepInvariant(t *testing.T) {
	from := decode(t, `{"modules":{"backend":{"replicas":1}},"shared":{}}`)
	to := decode(t, `{"modules":{"backend":{"replicas":2},"worker":{"replicas":1}},"shared":{"db":{"type":"postgres"}}}`)

	ops := Diff(from, to)
	if len(ops) == 0 {
		t.Fatal("expected a non-empty patch")
	}
	applied, err := Apply(from, ops)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(applied, to) {
		t.Fatalf("from + delta != to\n got: %#v\nwant: %#v", applied, to)
	}
}

func TestDiffIsDeterministicAndSorted(t *testing.T) {
	from := decode(t, `{"a":1,"b":2}`)
	to := decode(t, `{"b":3,"c":4}`)
	first := Diff(from, to)
	second := Diff(from, to)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("diff is not deterministic")
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].Path > first[i].Path {
			t.Fatalf("patch is not sorted by path: %v", first)
		}
	}
}

func TestApplyRemovesMissingKeyIsAnError(t *testing.T) {
	doc := decode(t, `{"a":1}`)
	if _, err := Apply(doc, []Op{{Op: "remove", Path: "/missing"}}); err == nil {
		t.Fatal("expected an error when removing a missing key")
	}
}

func TestPointerEscaping(t *testing.T) {
	from := decode(t, `{}`)
	to := decode(t, `{"a/b":1,"c~d":2}`)
	ops := Diff(from, to)
	paths := map[string]bool{}
	for _, op := range ops {
		paths[op.Path] = true
	}
	if !paths["/a~1b"] || !paths["/c~0d"] {
		t.Fatalf("expected escaped pointers, got %v", ops)
	}
	applied, err := Apply(from, ops)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(applied, to) {
		t.Fatalf("escaped pointers did not round-trip: %#v", applied)
	}
}
