// Package conformance runs the product planner over the read-only planner
// challenge fixtures in orchestrator_reference/humanitec-planner-challenge-v4.
//
// The fixtures are the reference for Humanitec planning semantics. Two
// documented product decisions make a byte-for-byte comparison impossible, and
// the comparison filters them out instead of bending the product to the harness:
//
//   - UC-08 executes resource nodes only, so the workload node is never matched
//     to a Resource Definition and never appears in a provision batch.
//   - Every Environment gets implicit profile infrastructure (a namespace and a
//     cluster), which the challenge has no concept of.
//
// For accepted fixtures, Deployment Set shape, the Humanitec-shaped Delta
// document and its base + delta = candidate invariant, resource identity, params, matching, references, co-provision, batching, Terraform
// contracts and Active Resource classification are compared against expected
// artifacts. Rejected fixtures currently assert rejection status only; matching
// the reference error code/phase/path is a later error-contract increment.
package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
)

// FixtureRoot is the read-only challenge bundle.
const FixtureRoot = "../../../orchestrator_reference/humanitec-planner-challenge-v4"

// Synthetic catalog entries that stand in for the implicit infrastructure the
// challenge does not model. They are filtered out of every comparison.
const (
	syntheticNamespaceDefinition = "conformance-namespace"
	syntheticClusterDefinition   = "conformance-cluster"
	syntheticConnectionKey       = "fixture-cluster"
	syntheticNamespace           = "fixture-namespace"
)

type caseManifest struct {
	Spec struct {
		Context              string   `yaml:"context"`
		CurrentDeploymentSet string   `yaml:"currentDeploymentSet"`
		BeforeScore          *string  `yaml:"beforeScore"`
		AfterScore           *string  `yaml:"afterScore"`
		ResourceTypes        []string `yaml:"resourceTypes"`
		ResourceDefinitions  []string `yaml:"resourceDefinitions"`
		ActiveResources      string   `yaml:"activeResources"`
		TerraformSourceMap   []struct {
			URL       string `yaml:"url"`
			Rev       string `yaml:"rev"`
			Directory string `yaml:"directory"`
		} `yaml:"terraformSourceMap"`
	} `yaml:"spec"`
}

// Case is one loaded fixture.
type Case struct {
	Name     string
	Root     string
	Request  planning.Request
	Expected Expected
}

// Expected holds the fixture's expected artifacts.
type Expected struct {
	Rejected      bool
	ErrorCode     string
	Delta         map[string]any
	DeploymentSet map[string]any
	Plan          map[string]any
}

// Load reads one fixture directory into a planning request.
func Load(root, name string) (*Case, error) {
	dir := filepath.Join(root, "testcases", name)
	var manifest caseManifest
	if err := readYAML(filepath.Join(dir, "case.yaml"), &manifest); err != nil {
		return nil, err
	}

	var context map[string]any
	if err := readYAML(filepath.Join(dir, manifest.Spec.Context), &context); err != nil {
		return nil, err
	}

	current := environment.NewDocument()
	if manifest.Spec.CurrentDeploymentSet != "" {
		if err := readYAMLInto(filepath.Join(dir, manifest.Spec.CurrentDeploymentSet), &current); err != nil {
			return nil, err
		}
		if current.Modules == nil {
			current.Modules = map[string]environment.Module{}
		}
		if current.Shared == nil {
			current.Shared = map[string]environment.ResourceEntry{}
		}
	}

	before, err := readScore(dir, manifest.Spec.BeforeScore)
	if err != nil {
		return nil, err
	}
	after, err := readScore(dir, manifest.Spec.AfterScore)
	if err != nil {
		return nil, err
	}

	types, err := readResourceTypes(dir, manifest.Spec.ResourceTypes)
	if err != nil {
		return nil, err
	}
	definitions, err := readDefinitions(dir, manifest.Spec.ResourceDefinitions)
	if err != nil {
		return nil, err
	}

	appKey := stringAt(context, "app_id")
	envKey := stringAt(context, "env_id")
	app := application.Application{
		Key:             appKey,
		OrganizationKey: stringAt(context, "org_id"),
		Name:            appKey,
		Profile:         application.ProfileInternalK8s,
		ConnectionKey:   syntheticConnectionKey,
		RuntimeStatus:   application.RuntimeReady,
		Version:         1,
	}
	env := environment.Environment{
		Key:               envKey,
		ApplicationKey:    appKey,
		Name:              envKey,
		Type:              stringAt(context, "env_type"),
		NamespaceIdentity: syntheticNamespace,
		Version:           1,
	}
	planCtx := planning.Context{OrganizationKey: app.OrganizationKey, App: app, Env: env}

	active, err := readActiveResources(dir, manifest.Spec.ActiveResources, planCtx)
	if err != nil {
		return nil, err
	}

	catalog := planning.Catalog{Types: types, Definitions: definitions}
	addSyntheticCatalog(&catalog)

	workloadID := ""
	switch {
	case after != nil:
		workloadID = after.Metadata.Name
	case before != nil:
		workloadID = before.Metadata.Name
	}

	sources := map[string]string{}
	for _, entry := range manifest.Spec.TerraformSourceMap {
		key := entry.URL
		if entry.Rev != "" {
			key += "@" + entry.Rev
		}
		sources[key] = filepath.Join(dir, filepath.FromSlash(entry.Directory))
	}

	expected, err := readExpected(dir)
	if err != nil {
		return nil, err
	}

	return &Case{
		Name: name,
		Root: dir,
		Request: planning.Request{
			OrganizationKey: app.OrganizationKey,
			App:             app,
			Env:             env,
			Connection:      application.Connection{Key: syntheticConnectionKey, Status: application.ConnectionReady},
			BaseSet:         current,
			Before:          before,
			After:           after,
			WorkloadID:      workloadID,
			Catalog:         catalog,
			Active:          active,
			Terraform:       &FixtureInspector{Sources: sources},
		},
		Expected: *expected,
	}, nil
}

// addSyntheticCatalog registers the Resource Types and Definitions the implicit
// namespace and cluster nodes need. The challenge has no equivalent.
func addSyntheticCatalog(catalog *planning.Catalog) {
	if _, ok := catalog.Types[planning.TypeNamespace]; !ok {
		catalog.Types[planning.TypeNamespace] = resource.Type{
			Key:     planning.TypeNamespace,
			Outputs: []resource.OutputField{{Name: "name", Type: "string"}},
		}
	}
	if _, ok := catalog.Types[planning.TypeCluster]; !ok {
		catalog.Types[planning.TypeCluster] = resource.Type{
			Key: planning.TypeCluster,
			Outputs: []resource.OutputField{
				{Name: "name", Type: "string"},
				{Name: "kubeContext", Type: "string"},
			},
		}
	}
	catalog.Definitions = append(catalog.Definitions,
		resource.Definition{
			Key:             syntheticNamespaceDefinition,
			ResourceTypeKey: planning.TypeNamespace,
			DriverType:      resource.DriverEcho,
			Criteria:        []resource.Criterion{{}},
		},
		resource.Definition{
			Key:             syntheticClusterDefinition,
			ResourceTypeKey: planning.TypeCluster,
			DriverType:      resource.DriverEcho,
			Criteria:        []resource.Criterion{{}},
		},
	)
}

// IsSynthetic reports a node the challenge does not model.
func IsSynthetic(descriptor string) bool {
	return strings.HasPrefix(descriptor, planning.TypeNamespace+".") ||
		strings.HasPrefix(descriptor, planning.TypeCluster+".")
}

func readScore(dir string, name *string) (*score.Document, error) {
	if name == nil || *name == "" {
		return nil, nil
	}
	var raw map[string]any
	if err := readYAML(filepath.Join(dir, *name), &raw); err != nil {
		return nil, err
	}
	return score.FromMap(raw)
}

func readResourceTypes(dir string, names []string) (map[string]resource.Type, error) {
	out := map[string]resource.Type{}
	for _, name := range names {
		var doc struct {
			Metadata struct {
				ID string `yaml:"id"`
			} `yaml:"metadata"`
			Entity struct {
				InputsSchema struct {
					Properties map[string]struct {
						Type string `yaml:"type"`
					} `yaml:"properties"`
				} `yaml:"inputs_schema"`
				OutputsSchema struct {
					Properties map[string]struct {
						Type string `yaml:"type"`
					} `yaml:"properties"`
				} `yaml:"outputs_schema"`
			} `yaml:"entity"`
		}
		if err := readYAML(filepath.Join(dir, name), &doc); err != nil {
			return nil, err
		}
		typ := resource.Type{Key: doc.Metadata.ID}
		for _, key := range sortedKeys(doc.Entity.InputsSchema.Properties) {
			typ.Inputs = append(typ.Inputs, resource.InputField{
				Name: key, Type: schemaType(doc.Entity.InputsSchema.Properties[key].Type),
			})
		}
		for _, key := range sortedKeys(doc.Entity.OutputsSchema.Properties) {
			typ.Outputs = append(typ.Outputs, resource.OutputField{
				Name: key, Type: schemaType(doc.Entity.OutputsSchema.Properties[key].Type),
			})
		}
		out[typ.Key] = typ
	}
	return out, nil
}

func readDefinitions(dir string, names []string) ([]resource.Definition, error) {
	var out []resource.Definition
	for _, name := range names {
		var doc struct {
			Metadata struct {
				ID string `yaml:"id"`
			} `yaml:"metadata"`
			Entity struct {
				Type         string           `yaml:"type"`
				DriverType   string           `yaml:"driver_type"`
				Criteria     []map[string]any `yaml:"criteria"`
				DriverInputs map[string]any   `yaml:"driver_inputs"`
				Provision    map[string]any   `yaml:"provision"`
			} `yaml:"entity"`
		}
		if err := readYAML(filepath.Join(dir, name), &doc); err != nil {
			return nil, err
		}
		def := resource.Definition{
			Key:             doc.Metadata.ID,
			ResourceTypeKey: doc.Entity.Type,
			DriverType:      driverType(doc.Entity.DriverType),
			DriverInputs:    normalizeTree(doc.Entity.DriverInputs),
		}
		for _, criterion := range doc.Entity.Criteria {
			def.Criteria = append(def.Criteria, resource.Criterion{
				EnvironmentType: stringAt(criterion, "env_type"),
				ApplicationID:   stringAt(criterion, "app_id"),
				EnvironmentID:   stringAt(criterion, "env_id"),
				ResourceID:      stringAt(criterion, "res_id"),
				Class:           stringAt(criterion, "class"),
			})
		}
		// The challenge never considers a Definition without criteria or with
		// `criteria: []`; only an explicit `{}` is a wildcard. Leave such a
		// Definition out of the catalog instead of inventing a wildcard, so the
		// product invariant of UC-03 BR-07 stays intact.
		if len(def.Criteria) == 0 {
			continue
		}
		if len(doc.Entity.Provision) > 0 {
			def.Provision = map[string]resource.ProvisionRule{}
			for key, raw := range doc.Entity.Provision {
				rule, _ := normalizeTree(map[string]any{"rule": raw})["rule"].(map[string]any)
				provision := resource.ProvisionRule{}
				if value, ok := rule["is_dependent"].(bool); ok {
					provision.IsDependent = value
				}
				if value, ok := rule["match_dependents"].(bool); ok {
					provision.MatchDependents = value
				}
				if params, ok := rule["params"].(map[string]any); ok {
					provision.Params = params
				}
				def.Provision[key] = provision
			}
		}
		out = append(out, def)
	}
	return out, nil
}

func readActiveResources(dir, name string, ctx planning.Context) ([]resource.ActiveResource, error) {
	if name == "" {
		return nil, nil
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	var entries []struct {
		Metadata struct {
			Type  string `yaml:"type"`
			Class string `yaml:"class"`
			ResID string `yaml:"res_id"`
		} `yaml:"metadata"`
		Status struct {
			DefinitionID string `yaml:"resource_definition_id"`
		} `yaml:"status"`
	}
	if err := readYAML(path, &entries); err != nil {
		return nil, err
	}
	var out []resource.ActiveResource
	for _, entry := range entries {
		class := entry.Metadata.Class
		if class == "" {
			class = planning.ClassDefault
		}
		descriptor, err := resource.NewDescriptor(entry.Metadata.Type, class, entry.Metadata.ResID)
		if err != nil {
			return nil, err
		}
		scope, err := planning.ScopeFor(ctx, descriptor)
		if err != nil {
			return nil, err
		}
		out = append(out, resource.ActiveResource{
			OrganizationKey: ctx.OrganizationKey,
			Descriptor:      descriptor,
			Scope:           scope,
			DefinitionKey:   entry.Status.DefinitionID,
			Status:          resource.StatusReady,
		})
	}
	return out, nil
}

func readExpected(dir string) (*Expected, error) {
	expected := &Expected{}
	resultPath := filepath.Join(dir, "expected", "result.json")
	payload, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, fmt.Errorf("conformance: read %s: %w", resultPath, err)
	}
	var result struct {
		Status string `json:"status"`
		Error  struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	if strings.EqualFold(result.Status, "REJECTED") {
		expected.Rejected = true
		expected.ErrorCode = result.Error.Code
		return expected, nil
	}

	if err := readYAML(filepath.Join(dir, "expected", "delta.yaml"), &expected.Delta); err != nil {
		return nil, err
	}
	if err := readYAML(filepath.Join(dir, "expected", "deployment-set.yaml"), &expected.DeploymentSet); err != nil {
		return nil, err
	}
	if err := readYAML(filepath.Join(dir, "expected", "challenge-plan.yaml"), &expected.Plan); err != nil {
		return nil, err
	}
	return expected, nil
}

func readYAML(path string, out any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("conformance: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("conformance: parse %s: %w", path, err)
	}
	return nil
}

// readYAMLInto converts YAML to JSON first, so typed structs use their JSON tags.
func readYAMLInto(path string, out any) error {
	var raw map[string]any
	if err := readYAML(path, &raw); err != nil {
		return err
	}
	payload, err := json.Marshal(normalizeTree(raw))
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

// normalizeTree turns a YAML tree into a JSON-compatible tree.
func normalizeTree(in map[string]any) map[string]any {
	converted, _ := normalizeValue(in).(map[string]any)
	return converted
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, value := range t {
			out[key] = normalizeValue(value)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for key, value := range t {
			out[fmt.Sprint(key)] = normalizeValue(value)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			out[i] = normalizeValue(value)
		}
		return out
	case int:
		return float64(t)
	default:
		return v
	}
}

func driverType(raw string) resource.DriverType {
	switch raw {
	case "humanitec/terraform":
		return resource.DriverTerraform
	default:
		return resource.DriverEcho
	}
}

func schemaType(raw string) string {
	switch raw {
	case "string", "number", "bool":
		return raw
	default:
		return "any"
	}
}

func stringAt(doc map[string]any, key string) string {
	if value, ok := doc[key].(string); ok {
		return value
	}
	return ""
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
