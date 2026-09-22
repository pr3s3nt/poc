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

func assertRoundTrip(t *testing.T, from, to any, ops []patchOp) {
	t.Helper()
	applied, err := Apply(from, ops)
	if err != nil {
		t.Fatalf("apply %+v: %v", ops, err)
	}
	if !reflect.DeepEqual(applied, to) {
		t.Fatalf("from + delta != to\n got: %#v\nwant: %#v", applied, to)
	}
}

func TestDiffAndApplyKeepInvariant(t *testing.T) {
	from := decode(t, `{"modules":{"backend":{"replicas":1}},"shared":{}}`)
	to := decode(t, `{"modules":{"backend":{"replicas":2},"worker":{"replicas":1}},"shared":{"db":{"type":"postgres"}}}`)
	ops := Diff(from, to)
	if len(ops) == 0 {
		t.Fatal("expected a non-empty patch")
	}
	assertRoundTrip(t, from, to, ops)
}

func TestDiffWalksObjectKeysInLexicalOrder(t *testing.T) {
	from := decode(t, `{"b":2,"a":1,"d":{"y":1,"x":1}}`)
	to := decode(t, `{"c":4,"b":3,"d":{"z":1,"x":2}}`)
	want := []patchOp{
		{Op: "remove", Path: "/a"},
		{Op: "replace", Path: "/b", Value: 3.0},
		{Op: "add", Path: "/c", Value: 4.0},
		{Op: "replace", Path: "/d/x", Value: 2.0},
		{Op: "remove", Path: "/d/y"},
		{Op: "add", Path: "/d/z", Value: 1.0},
	}
	for i := 0; i < 5; i++ {
		if got := Diff(from, to); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: diff\n got: %+v\nwant: %+v", i, got, want)
		}
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffNestedObjectRecursesInsteadOfReplacing(t *testing.T) {
	from := decode(t, `{"spec":{"containers":{"main":{"image":"a:1","variables":{"A":"1","B":"2"}}}}}`)
	to := decode(t, `{"spec":{"containers":{"main":{"image":"a:1","variables":{"A":"1","B":"3"}}}}}`)
	want := []patchOp{{Op: "replace", Path: "/spec/containers/main/variables/B", Value: "3"}}
	if got := Diff(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got: %+v\nwant: %+v", got, want)
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffArrayReplacesCommonIndexes(t *testing.T) {
	from := decode(t, `{"args":["a","b","c"]}`)
	to := decode(t, `{"args":["a","x","c"]}`)
	want := []patchOp{{Op: "replace", Path: "/args/1", Value: "x"}}
	if got := Diff(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got: %+v\nwant: %+v", got, want)
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffArrayRecursesIntoCommonObjectElements(t *testing.T) {
	from := decode(t, `{"items":[{"k":1,"v":1}]}`)
	to := decode(t, `{"items":[{"k":1,"v":2}]}`)
	want := []patchOp{{Op: "replace", Path: "/items/0/v", Value: 2.0}}
	if got := Diff(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got: %+v\nwant: %+v", got, want)
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffArrayAppendsWithDashPath(t *testing.T) {
	from := decode(t, `{"args":["a"]}`)
	to := decode(t, `{"args":["b","c","d"]}`)
	want := []patchOp{
		{Op: "replace", Path: "/args/0", Value: "b"},
		{Op: "add", Path: "/args/-", Value: "c"},
		{Op: "add", Path: "/args/-", Value: "d"},
	}
	if got := Diff(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got: %+v\nwant: %+v", got, want)
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffArrayRemovesTailFromHighestIndex(t *testing.T) {
	from := decode(t, `{"args":["a","b","c","d"]}`)
	to := decode(t, `{"args":["z"]}`)
	want := []patchOp{
		{Op: "replace", Path: "/args/0", Value: "z"},
		{Op: "remove", Path: "/args/3"},
		{Op: "remove", Path: "/args/2"},
		{Op: "remove", Path: "/args/1"},
	}
	if got := Diff(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got: %+v\nwant: %+v", got, want)
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffTypeChangeIsReplace(t *testing.T) {
	from := decode(t, `{"a":{"b":1},"c":[1],"d":"x"}`)
	to := decode(t, `{"a":"flat","c":{"k":1},"d":2}`)
	want := []patchOp{
		{Op: "replace", Path: "/a", Value: "flat"},
		{Op: "replace", Path: "/c", Value: map[string]any{"k": 1.0}},
		{Op: "replace", Path: "/d", Value: 2.0},
	}
	if got := Diff(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got: %+v\nwant: %+v", got, want)
	}
	assertRoundTrip(t, from, to, want)
}

func TestDiffOfEqualDocumentsIsEmpty(t *testing.T) {
	doc := decode(t, `{"a":[1,{"b":2}],"c":{}}`)
	if got := Diff(doc, doc); len(got) != 0 {
		t.Fatalf("expected no operations, got %+v", got)
	}
}

func TestApplyRemovesMissingKeyIsAnError(t *testing.T) {
	doc := decode(t, `{"a":1}`)
	if _, err := Apply(doc, []patchOp{{Op: "remove", Path: "/missing"}}); err == nil {
		t.Fatal("expected an error when removing a missing key")
	}
}

func TestApplyRejectsReplaceOfMissingKeyAndBadIndex(t *testing.T) {
	doc := decode(t, `{"a":[1]}`)
	for _, bad := range []patchOp{
		{Op: "replace", Path: "/missing", Value: 1.0},
		{Op: "replace", Path: "/a/1", Value: 1.0},
		{Op: "remove", Path: "/a/-"},
		{Op: "add", Path: "/a/5", Value: 1.0},
		{Op: "move", Path: "/a"},
	} {
		if _, err := Apply(doc, []patchOp{bad}); err == nil {
			t.Fatalf("expected %+v to be rejected", bad)
		}
	}
}

func TestApplyDoesNotMutateInput(t *testing.T) {
	doc := decode(t, `{"a":[1,2]}`)
	if _, err := Apply(doc, []patchOp{{Op: "remove", Path: "/a/1"}}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(doc, decode(t, `{"a":[1,2]}`)) {
		t.Fatalf("input was mutated: %#v", doc)
	}
}

func TestPointerEscaping(t *testing.T) {
	from := decode(t, `{}`)
	to := decode(t, `{"a/b":1,"c~d":2}`)
	ops := Diff(from, to)
	paths := map[string]bool{}
	for _, o := range ops {
		paths[o.Path] = true
	}
	if !paths["/a~1b"] || !paths["/c~0d"] {
		t.Fatalf("expected escaped pointers, got %v", ops)
	}
	assertRoundTrip(t, from, to, ops)
}
