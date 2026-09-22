package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"orchestrator/internal/planning"
	"orchestrator/internal/platform/canon"
)

func caseNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "testcases"))
	if err != nil {
		t.Skipf("planner challenge fixtures are not available: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// TestPlannerMatchesChallengeFixtures runs the product planner over every
// challenge fixture. Accepted cases compare plan artifacts; rejected cases
// currently require an error but do not yet compare its code/phase/path.
func TestPlannerMatchesChallengeFixtures(t *testing.T) {
	root := FixtureRoot
	names := caseNames(t, root)
	if len(names) == 0 {
		t.Skip("no fixtures found")
	}

	service := planning.NewService()
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			fixture, err := Load(root, name)
			if err != nil {
				if fixture == nil && fixtureRejects(root, name) {
					// A fixture the loader rejects is still a rejection.
					return
				}
				t.Fatalf("load: %v", err)
			}

			plan, planErr := service.Plan(fixture.Request)
			if fixture.Expected.Rejected {
				if planErr == nil {
					t.Fatalf("fixture expects %s but the planner accepted it", fixture.Expected.ErrorCode)
				}
				return
			}
			if planErr != nil {
				t.Fatalf("plan: %v", planErr)
			}

			assertDelta(t, plan, fixture.Expected.Delta)
			assertDeploymentSet(t, plan, fixture.Expected.DeploymentSet)
			assertGraph(t, plan, fixture.Expected.Plan)
			assertMatches(t, plan, fixture.Expected.Plan)
			assertBatches(t, plan, fixture.Expected.Plan)
			assertClassification(t, plan, fixture.Expected.Plan)
			assertTerraform(t, plan, fixture.Expected.Plan)
		})
	}
}

// fixtureRejects reports whether a fixture expects a rejected plan, used when
// the failure happens while loading the Score itself.
func fixtureRejects(root, name string) bool {
	payload, err := os.ReadFile(filepath.Join(root, "testcases", name, "expected", "result.json"))
	if err != nil {
		return false
	}
	var result struct {
		Status string `json:"status"`
	}
	return json.Unmarshal(payload, &result) == nil && result.Status == "REJECTED"
}

// assertDelta compares the Humanitec-shaped Delta document byte for byte,
// including operation order, and re-proves base + delta = candidate.
func assertDelta(t *testing.T, plan *planning.Plan, expected map[string]any) {
	t.Helper()
	actual, err := canon.Map(plan.Delta)
	if err != nil {
		t.Fatalf("canon: %v", err)
	}
	if expected == nil {
		expected = map[string]any{}
	}
	normalized := normalizeTree(expected)
	if !reflect.DeepEqual(actual, normalized) {
		t.Fatalf("delta mismatch\n got: %s\nwant: %s", mustJSON(actual), mustJSON(normalized))
	}
	if err := planning.VerifyDelta(plan.BaseSet, plan.Delta, plan.CandidateSet); err != nil {
		t.Fatal(err)
	}
}

// assertDeploymentSet compares the Candidate Deployment Set byte for byte after
// dropping an empty `shared` map, which the product always materialises and the
// challenge omits.
func assertDeploymentSet(t *testing.T, plan *planning.Plan, expected map[string]any) {
	t.Helper()
	actual, err := canon.Map(plan.CandidateSet)
	if err != nil {
		t.Fatalf("canon: %v", err)
	}
	if shared, ok := actual["shared"].(map[string]any); ok && len(shared) == 0 {
		delete(actual, "shared")
	}
	if expected == nil {
		expected = map[string]any{}
	}
	normalized := normalizeTree(expected)
	if !reflect.DeepEqual(actual, normalized) {
		t.Fatalf("candidate Deployment Set mismatch\n got: %s\nwant: %s", mustJSON(actual), mustJSON(normalized))
	}
}

func assertGraph(t *testing.T, plan *planning.Plan, expectedPlan map[string]any) {
	t.Helper()
	graph, _ := normalizeTree(expectedPlan)["resourceGraph"].(map[string]any)

	wantNodes := map[string]map[string]any{}
	if graph != nil {
		for _, item := range asList(graph["nodes"]) {
			node := item.(map[string]any)
			wantNodes[node["descriptor"].(string)] = node
		}
	}
	gotNodes := map[string]map[string]any{}
	for _, node := range plan.Graph.Nodes {
		if IsSynthetic(node.Descriptor) {
			continue
		}
		origins := []any{}
		for _, origin := range node.Origins {
			origins = append(origins, string(origin))
		}
		params := map[string]any{}
		if len(node.Params) > 0 {
			converted, err := canon.Map(node.Params)
			if err != nil {
				t.Fatalf("canon: %v", err)
			}
			params = converted
		}
		gotNodes[node.Descriptor] = map[string]any{
			"descriptor": node.Descriptor, "origins": origins, "resourceInputs": params,
		}
	}
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Fatalf("resource graph nodes mismatch\n got: %s\nwant: %s", mustJSON(gotNodes), mustJSON(wantNodes))
	}

	wantEdges := map[string]bool{}
	if graph != nil {
		for _, item := range asList(graph["edges"]) {
			edge := item.(map[string]any)
			wantEdges[edgeKey(edge["from"], edge["to"], edge["reason"], edge["path"])] = true
		}
	}
	gotEdges := map[string]bool{}
	for _, edge := range plan.Graph.Edges {
		if IsSynthetic(edge.Consumer) || IsSynthetic(edge.Provider) {
			continue
		}
		var path any
		if edge.Path != "" {
			path = edge.Path
		}
		gotEdges[edgeKey(edge.Consumer, edge.Provider, edge.Reason, path)] = true
	}
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		t.Fatalf("resource graph edges mismatch\n got: %s\nwant: %s", mustJSON(keys(gotEdges)), mustJSON(keys(wantEdges)))
	}
}

// assertMatches compares matched Definitions for resource nodes. The workload
// node is excluded: UC-08 executes resource nodes only, so the product never
// matches a Definition for it.
func assertMatches(t *testing.T, plan *planning.Plan, expectedPlan map[string]any) {
	t.Helper()
	expected := map[string]map[string]any{}
	for descriptor, item := range mapAt(normalizeTree(expectedPlan), "matchedDefinitions") {
		if isWorkload(descriptor) {
			continue
		}
		entry := item.(map[string]any)
		expected[descriptor] = map[string]any{
			"definitionId": entry["definitionId"], "score": entry["score"],
		}
	}
	actual := map[string]map[string]any{}
	for descriptor, match := range plan.Matches {
		if IsSynthetic(descriptor) {
			continue
		}
		actual[descriptor] = map[string]any{
			"definitionId": match.DefinitionKey, "score": float64(match.Specificity),
		}
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("matched definitions mismatch\n got: %s\nwant: %s", mustJSON(actual), mustJSON(expected))
	}
}

// assertBatches checks provider-first order against the expected batches,
// ignoring the workload node the product does not execute.
func assertBatches(t *testing.T, plan *planning.Plan, expectedPlan map[string]any) {
	t.Helper()
	index := map[string]int{}
	for i, batch := range plan.Batches {
		for _, descriptor := range batch {
			index[descriptor] = i
		}
	}
	for _, descriptor := range keysOfMatches(plan) {
		if _, ok := index[descriptor]; !ok {
			t.Fatalf("resource %s is missing from the provision batches", descriptor)
		}
	}

	expectedIndex := map[string]int{}
	for i, item := range asList(normalizeTree(expectedPlan)["provisionBatches"]) {
		for _, descriptor := range asList(item) {
			expectedIndex[descriptor.(string)] = i
		}
	}
	for descriptor, want := range expectedIndex {
		if isWorkload(descriptor) {
			continue
		}
		if _, ok := index[descriptor]; !ok {
			t.Fatalf("expected %s in the provision batches", descriptor)
		}
		for other, otherWant := range expectedIndex {
			if isWorkload(other) || other == descriptor {
				continue
			}
			if want < otherWant && index[descriptor] >= index[other] {
				t.Fatalf("%s must be provisioned before %s; got batches %v", descriptor, other, plan.Batches)
			}
		}
	}
}

func assertClassification(t *testing.T, plan *planning.Plan, expectedPlan map[string]any) {
	t.Helper()
	expected := mapAt(normalizeTree(expectedPlan), "activeResources")
	compare := func(field string, actual []string) {
		var want []string
		for _, item := range asList(expected[field]) {
			descriptor := item.(string)
			if isWorkload(descriptor) {
				continue
			}
			want = append(want, descriptor)
		}
		var got []string
		for _, descriptor := range actual {
			if IsSynthetic(descriptor) || isWorkload(descriptor) {
				continue
			}
			got = append(got, descriptor)
		}
		sort.Strings(want)
		sort.Strings(got)
		if len(want) == 0 && len(got) == 0 {
			return
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("active resource %s mismatch\n got: %v\nwant: %v", field, got, want)
		}
	}
	compare("existing", plan.Classification.Existing)
	compare("new", plan.Classification.New)
	compare("unreferenced", plan.Classification.Unreferenced)
}

// assertTerraform compares the planning-time Terraform contract: module outputs,
// the source fingerprint and where each variable value comes from.
func assertTerraform(t *testing.T, plan *planning.Plan, expectedPlan map[string]any) {
	t.Helper()
	expected := map[string]map[string]any{}
	for _, item := range asList(normalizeTree(expectedPlan)["terraform"]) {
		record := item.(map[string]any)
		expected[record["resource"].(string)] = record
	}
	for _, contract := range plan.Terraform {
		record, ok := expected[contract.Descriptor]
		if !ok {
			t.Fatalf("unexpected Terraform contract for %s", contract.Descriptor)
		}
		if record["definitionId"] != contract.DefinitionKey {
			t.Fatalf("%s: definition %q, want %q", contract.Descriptor, contract.DefinitionKey, record["definitionId"])
		}
		if record["fingerprint"] != contract.Fingerprint {
			t.Fatalf("%s: fingerprint %q, want %q", contract.Descriptor, contract.Fingerprint, record["fingerprint"])
		}
		var wantOutputs []string
		for _, output := range asList(record["outputs"]) {
			wantOutputs = append(wantOutputs, output.(string))
		}
		if !reflect.DeepEqual(contract.Outputs, wantOutputs) {
			t.Fatalf("%s: outputs %v, want %v", contract.Descriptor, contract.Outputs, wantOutputs)
		}
		wantInputs := map[string]map[string]any{}
		for _, item := range asList(record["inputs"]) {
			input := item.(map[string]any)
			wantInputs[input["name"].(string)] = input
		}
		if len(contract.Inputs) != len(wantInputs) {
			t.Fatalf("%s: %d inputs, want %d", contract.Descriptor, len(contract.Inputs), len(wantInputs))
		}
		for _, input := range contract.Inputs {
			want, ok := wantInputs[input.Name]
			if !ok {
				t.Fatalf("%s: unexpected Terraform input %q", contract.Descriptor, input.Name)
			}
			if want["source"] != input.Source {
				t.Fatalf("%s: input %s source %q, want %q", contract.Descriptor, input.Name, input.Source, want["source"])
			}
			if want["type"] != input.Type {
				t.Fatalf("%s: input %s type %q, want %q", contract.Descriptor, input.Name, input.Type, want["type"])
			}
			gotValue := normalizeValue(input.Value)
			if wantValue, exists := want["value"]; exists && !reflect.DeepEqual(gotValue, wantValue) {
				t.Fatalf("%s: input %s value %#v, want %#v", contract.Descriptor, input.Name, gotValue, wantValue)
			}
		}
	}
	for descriptor := range expected {
		found := false
		for _, contract := range plan.Terraform {
			if contract.Descriptor == descriptor {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing Terraform contract for %s", descriptor)
		}
	}
}

func keysOfMatches(plan *planning.Plan) []string {
	var out []string
	for descriptor := range plan.Matches {
		out = append(out, descriptor)
	}
	sort.Strings(out)
	return out
}

func isWorkload(descriptor string) bool {
	return len(descriptor) > len(planning.TypeWorkload) && descriptor[:len(planning.TypeWorkload)+1] == planning.TypeWorkload+"."
}

func edgeKey(from, to, reason, path any) string {
	return mustJSONString([]any{from, to, reason, path})
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

func mapAt(doc map[string]any, key string) map[string]any {
	out, _ := doc[key].(map[string]any)
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func mustJSON(v any) string {
	payload, _ := json.MarshalIndent(v, "", " ")
	return string(payload)
}

func mustJSONString(v any) string {
	payload, _ := json.Marshal(v)
	return string(payload)
}
