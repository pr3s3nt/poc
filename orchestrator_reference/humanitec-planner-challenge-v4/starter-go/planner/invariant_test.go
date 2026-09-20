package planner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testcasesRoot locates the public testcase directory. CHALLENGE_TESTCASES
// overrides the search, which otherwise walks up from the package directory
// and looks for a "testcases" folder holding 01-noop.
func testcasesRoot(t *testing.T) string {
	t.Helper()
	if fromEnv := os.Getenv("CHALLENGE_TESTCASES"); fromEnv != "" {
		return fromEnv
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot determine working directory: %v", err)
	}
	for i := 0; i < 6; i++ {
		matches, _ := filepath.Glob(filepath.Join(dir, "*", "testcases", "01-noop"))
		if len(matches) == 0 {
			matches, _ = filepath.Glob(filepath.Join(dir, "testcases", "01-noop"))
		}
		if len(matches) > 0 {
			return filepath.Dir(matches[0])
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("testcases directory not found")
	return ""
}

// TestDeltaInvariant proves currentDeploymentSet + delta == deploymentSet for
// every accepted public testcase.
func TestDeltaInvariant(t *testing.T) {
	root := testcasesRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("testcases unavailable: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			result := Run(filepath.Join(root, name, "case.yaml"))
			if result["status"] != "ACCEPTED" {
				return
			}
			c, err := LoadCase(filepath.Join(root, name, "case.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			delta, _ := result["delta"].(Document)
			candidate, _ := result["deploymentSet"].(Document)
			applied, err := applyDelta(c.CurrentDeploymentSet, delta)
			if err != nil {
				t.Fatalf("apply delta: %v", err)
			}
			if !deepEqual(applied, candidate) {
				t.Fatalf("current + delta != deploymentSet\napplied:  %#v\nexpected: %#v", applied, candidate)
			}
		})
	}
}

// applyDelta replays a Humanitec Deployment Delta onto a Deployment Set.
func applyDelta(current, delta Document) (Document, error) {
	modules := Document{}
	if existing := mapAt(current, "modules"); existing != nil {
		modules = asDocument(deepCopy(existing))
	}
	shared := Document{}
	if existing := mapAt(current, "shared"); existing != nil {
		shared = asDocument(deepCopy(existing))
	}

	deltaModules := mapAt(delta, "modules")
	for id, module := range mapAt(deltaModules, "add") {
		modules[id] = deepCopy(module)
	}
	if list, ok := deltaModules["remove"].([]any); ok {
		for _, id := range list {
			delete(modules, id.(string))
		}
	}
	for id, ops := range mapAt(deltaModules, "update") {
		patch, err := toOps(ops)
		if err != nil {
			return nil, err
		}
		updated, err := applyPatch(modules[id], patch)
		if err != nil {
			return nil, err
		}
		modules[id] = updated
	}

	if ops, ok := delta["shared"]; ok {
		patch, err := toOps(ops)
		if err != nil {
			return nil, err
		}
		updated, err := applyPatch(shared, patch)
		if err != nil {
			return nil, err
		}
		shared = asDocument(updated)
	}

	out := Document{"modules": modules}
	if len(shared) > 0 {
		out["shared"] = shared
	}
	return out, nil
}

func toOps(v any) ([]Document, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]Document, 0, len(list))
	for _, item := range list {
		out = append(out, asDocument(item))
	}
	return out, nil
}

// TestPointerEscaping covers RFC 6901 escaping of keys holding ~ and /.
func TestPointerEscaping(t *testing.T) {
	from := Document{"a/b": 1, "c~d": Document{"e": 1}}
	to := Document{"a/b": 2, "c~d": Document{"e": 1}, "new/key": "v"}
	ops := diffPatch(from, to, "")
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d: %#v", len(ops), ops)
	}
	if ops[0]["path"] != "/a~1b" || ops[0]["op"] != "replace" {
		t.Fatalf("bad first op: %#v", ops[0])
	}
	if ops[1]["path"] != "/new~1key" || ops[1]["op"] != "add" {
		t.Fatalf("bad second op: %#v", ops[1])
	}
	applied, err := applyPatch(from, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(applied, to) {
		t.Fatalf("round trip mismatch: %#v", applied)
	}
}

// TestArrayDiff covers common-prefix diffing, tail removal and "/-" appends.
func TestArrayDiff(t *testing.T) {
	from := Document{"args": []any{"a", "b", "c"}}
	to := Document{"args": []any{"a", "x"}}
	ops := diffPatch(from, to, "")
	applied, err := applyPatch(from, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(applied, to) {
		t.Fatalf("array diff round trip mismatch: %#v (ops %#v)", applied, ops)
	}

	grow := Document{"args": []any{"a", "b", "c", "d"}}
	ops = diffPatch(to, grow, "")
	applied, err = applyPatch(to, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !deepEqual(applied, grow) {
		t.Fatalf("array append round trip mismatch: %#v (ops %#v)", applied, ops)
	}
}

// TestEscapedPlaceholder keeps $${...} out of the graph.
func TestEscapedPlaceholder(t *testing.T) {
	found := scanPlaceholders("$${resources.db.host} and ${resources.cache.host}")
	if len(found) != 1 || found[0].Body != "resources.cache.host" {
		t.Fatalf("unexpected placeholders: %#v", found)
	}
}

// TestAgainstExpected compares the planner output with every public
// expected/result.json, accepted and rejected alike.
func TestAgainstExpected(t *testing.T) {
	root := testcasesRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("testcases unavailable: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		expectedBytes, err := os.ReadFile(filepath.Join(root, name, "expected", "result.json"))
		if err != nil {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			var expected any
			if err := json.Unmarshal(expectedBytes, &expected); err != nil {
				t.Fatalf("invalid expected JSON: %v", err)
			}
			produced, err := json.Marshal(Run(filepath.Join(root, name, "case.yaml")))
			if err != nil {
				t.Fatal(err)
			}
			var actual any
			if err := json.Unmarshal(produced, &actual); err != nil {
				t.Fatal(err)
			}
			if !deepEqual(expected, actual) {
				want, _ := json.MarshalIndent(expected, "", "  ")
				got, _ := json.MarshalIndent(actual, "", "  ")
				t.Fatalf("result mismatch\nexpected:\n%s\nactual:\n%s", want, got)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no expected results were compared")
	}
}
