package planner

import (
	"fmt"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Run keeps the phase order observable by the judge.
func Run(casePath string) Result {
	c, err := LoadCase(casePath)
	if err != nil {
		return rejected(err)
	}
	if err = ValidateHumanitecDocuments(c); err != nil {
		return rejected(err)
	}

	before, err := ConvertScoreToWorkloadFragment(c.BeforeScore, c.ResourceTypes)
	if err != nil {
		return rejected(err)
	}
	after, err := ConvertScoreToWorkloadFragment(c.AfterScore, c.ResourceTypes)
	if err != nil {
		return rejected(err)
	}
	if err = ValidateBeforeFragment(c.CurrentDeploymentSet, before, after); err != nil {
		return rejected(err)
	}

	candidate, err := ApplyDeploymentDelta(c.CurrentDeploymentSet, before, after)
	if err != nil {
		return rejected(err)
	}
	delta := BuildDeploymentDelta(c.CurrentDeploymentSet, candidate)

	graph, err := BuildInitialResourceGraph(candidate, c.ResourceTypes)
	if err != nil {
		return rejected(err)
	}
	contracts := newTerraformCache(c)
	refs, err := ExpandResourceGraph(c, graph)
	if err != nil {
		return rejected(err)
	}
	if err = validateReferenceOutputs(graph, refs, contracts); err != nil {
		return rejected(err)
	}

	records, err := InspectTerraformSource(c, graph, contracts)
	if err != nil {
		return rejected(err)
	}
	classification := ClassifyActiveResources(graph, c.ActiveResources)
	batches, err := TopologicalBatches(graph)
	if err != nil {
		return rejected(err)
	}

	return RenderResult(delta, candidate, graph, records, classification, batches)
}

func rejected(err error) Result {
	pe, ok := err.(*PlannerError)
	if !ok {
		pe = &PlannerError{Phase: "load", Code: "INVALID_INPUT", Path: err.Error()}
	}
	errDoc := Document{"phase": pe.Phase, "code": pe.Code}
	if pe.Path != "" {
		errDoc["path"] = pe.Path
	}
	return Result{"status": "REJECTED", "error": errDoc}
}

// LoadCase reads the PlannerCase manifest and every document it points at.
func LoadCase(path string) (*LoadedCase, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	raw, err := readFile(absolute)
	if err != nil {
		return nil, err
	}
	var manifest CaseManifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}

	c := &LoadedCase{Root: filepath.Dir(absolute), Manifest: manifest, ResourceTypes: map[string]*ResourceType{}}

	resolve := func(relative string) string { return filepath.Join(c.Root, filepath.FromSlash(relative)) }

	context, err := loadYAML(resolve(manifest.Spec.Context))
	if err != nil {
		return nil, err
	}
	c.Context = asDocument(context)

	current, err := loadYAML(resolve(manifest.Spec.CurrentDeploymentSet))
	if err != nil {
		return nil, err
	}
	c.CurrentDeploymentSet = asDocument(current)
	if c.CurrentDeploymentSet == nil {
		c.CurrentDeploymentSet = Document{}
	}

	if manifest.Spec.BeforeScore != nil {
		score, err := loadYAML(resolve(*manifest.Spec.BeforeScore))
		if err != nil {
			return nil, err
		}
		c.BeforeScore = asDocument(score)
	}
	if manifest.Spec.AfterScore != nil {
		score, err := loadYAML(resolve(*manifest.Spec.AfterScore))
		if err != nil {
			return nil, err
		}
		c.AfterScore = asDocument(score)
	}

	for _, relative := range manifest.Spec.ResourceTypes {
		doc, err := loadYAML(resolve(relative))
		if err != nil {
			return nil, err
		}
		rt, err := parseResourceType(asDocument(doc))
		if err != nil {
			return nil, err
		}
		c.ResourceTypes[rt.ID] = rt
	}

	for _, relative := range manifest.Spec.ResourceDefinitions {
		doc, err := loadYAML(resolve(relative))
		if err != nil {
			return nil, err
		}
		def, err := parseDefinition(asDocument(doc))
		if err != nil {
			return nil, err
		}
		c.ResourceDefinitions = append(c.ResourceDefinitions, def)
	}

	if manifest.Spec.ActiveResources != "" {
		doc, err := loadYAML(resolve(manifest.Spec.ActiveResources))
		if err != nil {
			return nil, err
		}
		if list, ok := doc.([]any); ok {
			for _, item := range list {
				c.ActiveResources = append(c.ActiveResources, asDocument(item))
			}
		}
	}
	return c, nil
}

func parseResourceType(doc Document) (*ResourceType, error) {
	if doc == nil || stringAt(doc, "kind") != "ResourceType" {
		return nil, fmt.Errorf("not a ResourceType document")
	}
	entity := mapAt(doc, "entity")
	return &ResourceType{
		ID:      stringAt(mapAt(doc, "metadata"), "id"),
		Inputs:  mapAt(mapAt(entity, "inputs_schema"), "properties"),
		Outputs: mapAt(mapAt(entity, "outputs_schema"), "properties"),
	}, nil
}

func parseDefinition(doc Document) (*Definition, error) {
	if doc == nil || stringAt(doc, "kind") != "Definition" {
		return nil, fmt.Errorf("not a Definition document")
	}
	entity := mapAt(doc, "entity")
	def := &Definition{
		ID:           stringAt(mapAt(doc, "metadata"), "id"),
		Type:         stringAt(entity, "type"),
		DriverType:   stringAt(entity, "driver_type"),
		DriverInputs: mapAt(entity, "driver_inputs"),
		Provision:    mapAt(entity, "provision"),
	}
	if list, ok := entity["criteria"].([]any); ok {
		for _, item := range list {
			criterion := asDocument(item)
			if criterion == nil {
				criterion = Document{}
			}
			def.Criteria = append(def.Criteria, criterion)
		}
	}
	return def, nil
}

// ValidateHumanitecDocuments checks the envelope level invariants.
func ValidateHumanitecDocuments(c *LoadedCase) error {
	if c.BeforeScore == nil && c.AfterScore == nil {
		return newError("validate", "MISSING_SCORE", "")
	}
	if c.BeforeScore != nil && c.AfterScore != nil {
		beforeName := stringAt(mapAt(c.BeforeScore, "metadata"), "name")
		afterName := stringAt(mapAt(c.AfterScore, "metadata"), "name")
		if beforeName != afterName {
			return newError("validate", "WORKLOAD_NAME_CHANGED", "/metadata/name")
		}
	}
	return nil
}

func ConvertScoreToWorkloadFragment(score Document, types map[string]*ResourceType) (*WorkloadFragment, error) {
	return convertScore(score, types)
}

// ValidateBeforeFragment proves the before Score describes the current state.
func ValidateBeforeFragment(current Document, before, after *WorkloadFragment) error {
	modules := mapAt(current, "modules")
	shared := mapAt(current, "shared")

	if before == nil {
		if _, exists := modules[after.WorkloadID]; exists {
			return newError("before-check", "WORKLOAD_ALREADY_EXISTS", pointer("modules", after.WorkloadID))
		}
		return nil
	}

	module, exists := modules[before.WorkloadID]
	if !exists || !deepEqual(module, before.Module) {
		return newError("before-check", "BEFORE_MISMATCH", pointer("modules", before.WorkloadID))
	}
	for _, id := range sortedKeys(before.Shared) {
		entry, exists := shared[id]
		if !exists || !deepEqual(entry, before.Shared[id]) {
			return newError("before-check", "BEFORE_MISMATCH", pointer("shared", id))
		}
	}
	return nil
}

// ApplyDeploymentDelta builds the Candidate Deployment Set by replacing only
// the target workload's contribution.
func ApplyDeploymentDelta(current Document, before, after *WorkloadFragment) (Document, error) {
	modules := Document{}
	if existing := mapAt(current, "modules"); existing != nil {
		modules = asDocument(deepCopy(existing))
	}
	shared := Document{}
	if existing := mapAt(current, "shared"); existing != nil {
		shared = asDocument(deepCopy(existing))
	}

	workloadID := ""
	if after != nil {
		workloadID = after.WorkloadID
	} else if before != nil {
		workloadID = before.WorkloadID
	}

	if after == nil {
		delete(modules, workloadID)
	} else {
		modules[workloadID] = deepCopy(after.Module)
	}

	if before != nil {
		for _, id := range sortedKeys(before.Shared) {
			if after == nil || after.Shared[id] == nil {
				delete(shared, id)
			}
		}
	}
	if after != nil {
		for _, id := range sortedKeys(after.Shared) {
			entry := after.Shared[id]
			existing, exists := shared[id]
			declaredBefore := before != nil && before.Shared[id] != nil
			if exists && !declaredBefore && !deepEqual(existing, entry) {
				return nil, newError("convert", "SHARED_CONFLICT", pointer("shared", id))
			}
			shared[id] = deepCopy(entry)
		}
	}

	candidate := Document{"modules": modules}
	if len(shared) > 0 {
		candidate["shared"] = shared
	}
	return candidate, nil
}

// BuildDeploymentDelta expresses current -> candidate in Humanitec Delta form.
func BuildDeploymentDelta(current, candidate Document) Document {
	currentModules := mapAt(current, "modules")
	candidateModules := mapAt(candidate, "modules")

	add := Document{}
	update := Document{}
	var remove []any
	for _, id := range unionKeys(currentModules, candidateModules) {
		currentModule, inCurrent := currentModules[id]
		candidateModule, inCandidate := candidateModules[id]
		switch {
		case !inCurrent && inCandidate:
			add[id] = candidateModule
		case inCurrent && !inCandidate:
			remove = append(remove, id)
		default:
			if ops := diffPatch(currentModule, candidateModule, ""); len(ops) > 0 {
				update[id] = documentsToList(ops)
			}
		}
	}

	modules := Document{}
	if len(add) > 0 {
		modules["add"] = add
	}
	if len(remove) > 0 {
		modules["remove"] = remove
	}
	if len(update) > 0 {
		modules["update"] = update
	}

	delta := Document{}
	if len(modules) > 0 {
		delta["modules"] = modules
	}

	currentShared := mapAt(current, "shared")
	if currentShared == nil {
		currentShared = Document{}
	}
	candidateShared := mapAt(candidate, "shared")
	if candidateShared == nil {
		candidateShared = Document{}
	}
	if ops := diffPatch(currentShared, candidateShared, ""); len(ops) > 0 {
		delta["shared"] = documentsToList(ops)
	}
	return delta
}

func documentsToList(ops []Document) []any {
	out := make([]any, 0, len(ops))
	for _, op := range ops {
		out = append(out, op)
	}
	return out
}

func BuildInitialResourceGraph(candidate Document, types map[string]*ResourceType) (*ResourceGraph, error) {
	return buildInitialGraph(candidate, types)
}

func ExpandResourceGraph(c *LoadedCase, g *ResourceGraph) ([]pendingRef, error) {
	return expandGraph(c, g)
}

// InspectTerraformSource resolves the Terraform contract of every node whose
// Definition uses the Terraform Driver.
func InspectTerraformSource(c *LoadedCase, g *ResourceGraph, tc *terraformCache) ([]*TerraformRecord, error) {
	var records []*TerraformRecord
	for _, key := range g.descriptors() {
		node := g.Nodes[key]
		if node.Definition == nil || node.Definition.DriverType != "humanitec/terraform" {
			continue
		}
		module, err := tc.moduleFor(node.Definition, key)
		if err != nil {
			return nil, err
		}
		record, err := buildTerraformRecord(node, module, c.Context, c.ResourceTypes)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// ClassifyActiveResources compares the desired graph with Active Resources.
func ClassifyActiveResources(g *ResourceGraph, active []Document) Document {
	activeSet := map[string]bool{}
	for _, item := range active {
		metadata := mapAt(item, "metadata")
		class := stringAt(metadata, "class")
		if class == "" {
			class = "default"
		}
		descriptor := stringAt(metadata, "type") + "." + class + "#" + stringAt(metadata, "res_id")
		activeSet[descriptor] = true
	}

	existing := []any{}
	created := []any{}
	unreferenced := []any{}
	for _, key := range g.descriptors() {
		if activeSet[key] {
			existing = append(existing, key)
		} else {
			created = append(created, key)
		}
	}
	var stale []string
	for key := range activeSet {
		if _, ok := g.Nodes[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		unreferenced = append(unreferenced, key)
	}

	return Document{"existing": existing, "new": created, "unreferenced": unreferenced}
}

func TopologicalBatches(g *ResourceGraph) ([][]string, error) { return topologicalBatches(g) }
