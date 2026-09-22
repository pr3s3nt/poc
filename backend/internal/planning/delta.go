package planning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/jsonpatch"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/platform/canon"
)

// DeltaBuilder produces the Humanitec-shaped Deployment Delta and the Candidate
// Deployment Set of one workload change (UC-05 MS-04, UC-06 BR-10, UC-07 BR-07).
type DeltaBuilder struct{}

// DeltaResult is the transient Delta document with the Candidate Set it yields.
type DeltaResult struct {
	Delta     deployment.DeltaDocument
	Candidate environment.Document
}

// BuildHumanitecDelta replaces the contribution of one workload, diffs the
// Candidate Set against current and proves current + delta = candidate.
func (DeltaBuilder) BuildHumanitecDelta(current environment.Document, workloadID string, before, after *score.Fragment) (DeltaResult, error) {
	candidate, err := buildCandidate(current, workloadID, before, after)
	if err != nil {
		return DeltaResult{}, err
	}
	if err := candidate.Validate(); err != nil {
		return DeltaResult{}, err
	}
	delta, err := DiffDeploymentSets(current, candidate)
	if err != nil {
		return DeltaResult{}, err
	}
	if err := VerifyDelta(current, delta, candidate); err != nil {
		return DeltaResult{}, err
	}
	return DeltaResult{Delta: delta, Candidate: candidate}, nil
}

// DiffDeploymentSets returns the deterministic Delta that turns base into
// candidate. Modules only in candidate are added whole, modules only in base
// are removed by sorted ID, and changed modules get a patch relative to the
// module. Shared changes are one patch relative to the shared object.
func DiffDeploymentSets(base, candidate environment.Document) (deployment.DeltaDocument, error) {
	modules := &deployment.ModuleDelta{}
	for _, id := range candidate.ModuleIDs() {
		if _, exists := base.Modules[id]; exists {
			continue
		}
		if modules.Add == nil {
			modules.Add = map[string]environment.Module{}
		}
		modules.Add[id] = candidate.Modules[id]
	}
	for _, id := range base.ModuleIDs() {
		next, exists := candidate.Modules[id]
		if !exists {
			modules.Remove = append(modules.Remove, id)
			continue
		}
		from, err := canon.Map(base.Modules[id])
		if err != nil {
			return deployment.DeltaDocument{}, err
		}
		to, err := canon.Map(next)
		if err != nil {
			return deployment.DeltaDocument{}, err
		}
		if ops := jsonpatch.Diff(from, to); len(ops) > 0 {
			if modules.Update == nil {
				modules.Update = map[string][]deployment.JSONPatchOperation{}
			}
			modules.Update[id] = ops
		}
	}
	fromShared, err := sharedTree(base)
	if err != nil {
		return deployment.DeltaDocument{}, err
	}
	toShared, err := sharedTree(candidate)
	if err != nil {
		return deployment.DeltaDocument{}, err
	}
	delta := deployment.DeltaDocument{Modules: modules, Shared: jsonpatch.Diff(fromShared, toShared)}
	return delta.Normalized(), nil
}

// ApplyHumanitecDelta applies a Delta to base and returns the resulting
// Deployment Set. Adding an existing module, removing or updating a missing one
// and patches that leave the Deployment Set shape are errors.
func ApplyHumanitecDelta(base environment.Document, delta deployment.DeltaDocument) (environment.Document, error) {
	if err := delta.Validate(); err != nil {
		return environment.Document{}, err
	}
	out := normalize(base)
	if delta.Modules != nil {
		for _, id := range delta.Modules.Remove {
			if _, exists := out.Modules[id]; !exists {
				return environment.Document{}, fmt.Errorf("planning: delta removes unknown module %q", id)
			}
			delete(out.Modules, id)
		}
		for _, id := range sortedKeys(delta.Modules.Add) {
			if _, exists := out.Modules[id]; exists {
				return environment.Document{}, fmt.Errorf("planning: delta adds module %q which already exists", id)
			}
			out.Modules[id] = delta.Modules.Add[id]
		}
		for _, id := range sortedKeys(delta.Modules.Update) {
			current, exists := out.Modules[id]
			if !exists {
				return environment.Document{}, fmt.Errorf("planning: delta updates unknown module %q", id)
			}
			tree, err := canon.Map(current)
			if err != nil {
				return environment.Document{}, err
			}
			patched, err := jsonpatch.Apply(tree, delta.Modules.Update[id])
			if err != nil {
				return environment.Document{}, fmt.Errorf("planning: module %q: %w", id, err)
			}
			var next environment.Module
			if err := decodeStrict(patched, &next); err != nil {
				return environment.Document{}, fmt.Errorf("planning: module %q: %w", id, err)
			}
			out.Modules[id] = next
		}
	}
	if len(delta.Shared) > 0 {
		tree, err := sharedTree(out)
		if err != nil {
			return environment.Document{}, err
		}
		patched, err := jsonpatch.Apply(tree, delta.Shared)
		if err != nil {
			return environment.Document{}, fmt.Errorf("planning: shared: %w", err)
		}
		shared := map[string]environment.ResourceEntry{}
		if err := decodeStrict(patched, &shared); err != nil {
			return environment.Document{}, fmt.Errorf("planning: shared: %w", err)
		}
		out.Shared = shared
	}
	return out, nil
}

// VerifyDelta enforces the invariant base + delta = candidate.
func VerifyDelta(base environment.Document, delta deployment.DeltaDocument, candidate environment.Document) error {
	applied, err := ApplyHumanitecDelta(base, delta)
	if err != nil {
		return fmt.Errorf("planning: delta is not applicable: %w", err)
	}
	got, err := canon.Map(applied)
	if err != nil {
		return err
	}
	want, err := canon.Map(candidate)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("planning: invariant violated: base + delta != candidate")
	}
	return nil
}

// sharedTree returns the shared object as a generic tree, never nil.
func sharedTree(doc environment.Document) (map[string]any, error) {
	if doc.Shared == nil {
		return map[string]any{}, nil
	}
	return canon.Map(doc.Shared)
}

// decodeStrict converts a patched tree back into a typed value and rejects
// fields the Deployment Set does not define.
func decodeStrict(tree any, out any) error {
	b, err := canon.Bytes(tree)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("patched document does not fit the Deployment Set: %w", err)
	}
	return nil
}
