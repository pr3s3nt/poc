package conformance

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

// writeDefinition writes one challenge-shaped Resource Definition whose
// `entity` ends with criteriaYAML (empty means the key is absent).
func writeDefinition(t *testing.T, dir, id, criteriaYAML string) string {
	t.Helper()
	body := "apiVersion: entity.humanitec.io/v1b1\n" +
		"kind: Definition\n" +
		"metadata:\n" +
		"  id: " + id + "\n" +
		"entity:\n" +
		"  name: " + id + "\n" +
		"  type: postgres\n" +
		"  driver_type: humanitec/echo\n" +
		criteriaYAML
	name := id + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return name
}

// TestReadDefinitionsCriteriaShapes pins the challenge rule (PROBLEM.md §13):
// a Definition without criteria or with `criteria: []` is not a candidate,
// while an explicit `{}` criterion is a wildcard with score 0.
func TestReadDefinitionsCriteriaShapes(t *testing.T) {
	cases := []struct {
		name     string
		criteria string
		want     []resource.Criterion
	}{
		{name: "missing", criteria: "", want: nil},
		{name: "empty-list", criteria: "  criteria: []\n", want: nil},
		{name: "wildcard", criteria: "  criteria:\n  - {}\n", want: []resource.Criterion{{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file := writeDefinition(t, dir, "pg-"+tc.name, tc.criteria)

			defs, err := readDefinitions(dir, []string{file})
			if err != nil {
				t.Fatalf("readDefinitions: %v", err)
			}
			if tc.want == nil {
				if len(defs) != 0 {
					t.Fatalf("definition must be left out of the challenge catalog, got %+v", defs)
				}
				return
			}
			if len(defs) != 1 {
				t.Fatalf("want exactly one definition, got %d", len(defs))
			}
			def := defs[0]
			if !reflect.DeepEqual(def.Criteria, tc.want) {
				t.Fatalf("criteria = %+v, want %+v", def.Criteria, tc.want)
			}
			if err := def.Validate(); err != nil {
				t.Fatalf("wildcard definition must satisfy the product invariant: %v", err)
			}
			criterion, score, ok := def.BestCriterion(resource.MatchContext{
				EnvironmentType: "production",
				ApplicationID:   "any-app",
				EnvironmentID:   "any-env",
				ResourceID:      "any-res",
				Class:           "any-class",
			})
			if !ok || score != 0 || criterion != (resource.Criterion{}) {
				t.Fatalf("wildcard must match any context with score 0, got %+v score=%d ok=%v", criterion, score, ok)
			}
		})
	}
}

// TestReadDefinitionsKeepsOrderAroundSkippedDefinitions checks that skipping a
// criteria-less Definition does not drop or reorder its neighbours.
func TestReadDefinitionsKeepsOrderAroundSkippedDefinitions(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		writeDefinition(t, dir, "pg-a", "  criteria:\n  - class: large\n"),
		writeDefinition(t, dir, "pg-missing", ""),
		writeDefinition(t, dir, "pg-empty", "  criteria: []\n"),
		writeDefinition(t, dir, "pg-b", "  criteria:\n  - {}\n"),
	}
	defs, err := readDefinitions(dir, files)
	if err != nil {
		t.Fatalf("readDefinitions: %v", err)
	}
	var keys []string
	for _, def := range defs {
		keys = append(keys, def.Key)
	}
	if want := []string{"pg-a", "pg-b"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("definition keys = %v, want %v", keys, want)
	}
	if got := defs[0].Criteria; !reflect.DeepEqual(got, []resource.Criterion{{Class: "large"}}) {
		t.Fatalf("specific criterion changed: %+v", got)
	}
}

// TestProductPlannerRejectsCriteriaLessDefinition guards UC-03 BR-07: the
// adapter filtering must not hide the product invariant, so a catalog that
// still contains a Definition without criteria fails fast.
func TestProductPlannerRejectsCriteriaLessDefinition(t *testing.T) {
	names := caseNames(t, FixtureRoot)
	if len(names) == 0 {
		t.Skip("no fixtures found")
	}
	fixture, err := Load(FixtureRoot, "01-noop")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, criteria := range [][]resource.Criterion{nil, {}} {
		req := fixture.Request
		req.Catalog.Definitions = append(append([]resource.Definition(nil), req.Catalog.Definitions...),
			resource.Definition{
				Key:             "no-criteria",
				ResourceTypeKey: planning.TypeNamespace,
				DriverType:      resource.DriverEcho,
				Criteria:        criteria,
			})
		_, err := planning.NewService().Plan(req)
		if err == nil {
			t.Fatalf("planner accepted a definition with criteria %#v", criteria)
		}
		if want := `resource: definition "no-criteria" has no matching criteria`; !strings.Contains(err.Error(), want) {
			t.Fatalf("criteria %#v: error = %q, want it to contain %q", criteria, err, want)
		}
	}
}
