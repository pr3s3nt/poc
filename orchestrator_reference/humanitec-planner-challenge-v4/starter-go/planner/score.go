package planner

import (
	"strings"
)

const scoreAPIVersion = "score.dev/v1b1"

type scoreResource struct {
	Name   string
	Type   string
	Class  string
	ID     string
	Shared bool
	Params any
}

// target is the Deployment Set prefix the Score name maps to.
func (r scoreResource) target() string {
	if r.Shared {
		return "shared." + r.ID
	}
	return "externals." + r.Name
}

// convertScore turns one Score workload into its Deployment Set contribution.
func convertScore(score Document, types map[string]*ResourceType) (*WorkloadFragment, error) {
	if score == nil {
		return nil, nil
	}
	if stringAt(score, "apiVersion") != scoreAPIVersion {
		return nil, newError("validate", "INVALID_SCORE", "/apiVersion")
	}
	workloadID := stringAt(mapAt(score, "metadata"), "name")
	if workloadID == "" {
		return nil, newError("validate", "INVALID_SCORE", "/metadata/name")
	}

	resources, err := readScoreResources(score, types)
	if err != nil {
		return nil, err
	}

	module := Document{"profile": "humanitec/default-module"}
	containers, err := convertContainers(score, resources, types)
	if err != nil {
		return nil, err
	}
	module["spec"] = Document{"containers": containers}

	externals := Document{}
	shared := Document{}
	for _, name := range sortedKeys(mapAt(score, "resources")) {
		res := resources[name]
		entry := Document{"type": res.Type, "class": res.Class}
		if res.Params != nil {
			base := pointer("resources", name, "params")
			params, err := rewritePlaceholders(res.Params, base, resources, types)
			if err != nil {
				return nil, err
			}
			entry["params"] = params
		}
		if res.Shared {
			if existing, ok := shared[res.ID]; ok && !deepEqual(existing, entry) {
				return nil, newError("convert", "SHARED_CONFLICT", pointer("resources", name))
			}
			shared[res.ID] = entry
		} else {
			externals[name] = entry
		}
	}
	if len(externals) > 0 {
		module["externals"] = externals
	}

	return &WorkloadFragment{WorkloadID: workloadID, Module: module, Shared: shared}, nil
}

func readScoreResources(score Document, types map[string]*ResourceType) (map[string]scoreResource, error) {
	out := map[string]scoreResource{}
	declared := mapAt(score, "resources")
	for _, name := range sortedKeys(declared) {
		spec := asDocument(declared[name])
		if spec == nil {
			return nil, newError("validate", "INVALID_SCORE", pointer("resources", name))
		}
		res := scoreResource{Name: name, Type: stringAt(spec, "type"), Class: stringAt(spec, "class")}
		if res.Type == "" {
			return nil, newError("validate", "INVALID_SCORE", pointer("resources", name, "type"))
		}
		if res.Class == "" {
			res.Class = "default"
		}
		if id := stringAt(spec, "id"); id != "" {
			res.ID = id
			res.Shared = true
		}
		rt, ok := types[res.Type]
		if !ok {
			return nil, newError("validate", "UNKNOWN_RESOURCE_TYPE", pointer("resources", name, "type"))
		}
		if params, ok := spec["params"]; ok && params != nil {
			res.Params = params
			if err := validateInputs(params, rt, pointer("resources", name, "params")); err != nil {
				return nil, err
			}
		}
		out[name] = res
	}
	return out, nil
}

// validateInputs enforces the Resource Type inputs_schema subset.
func validateInputs(params any, rt *ResourceType, path string) error {
	doc := asDocument(params)
	if doc == nil {
		return newError("validate", "INVALID_SCORE", path)
	}
	for _, key := range sortedKeys(doc) {
		if _, ok := rt.Inputs[key]; !ok {
			return newError("validate", "INVALID_SCORE", path+"/"+escapePointer(key))
		}
	}
	return nil
}

func convertContainers(score Document, resources map[string]scoreResource, types map[string]*ResourceType) (Document, error) {
	containers := Document{}
	declared := mapAt(score, "containers")
	for _, name := range sortedKeys(declared) {
		spec := asDocument(declared[name])
		if spec == nil {
			return nil, newError("validate", "INVALID_SCORE", pointer("containers", name))
		}
		out := Document{"id": name}
		for _, key := range sortedKeys(spec) {
			value := deepCopy(spec[key])
			if key == "variables" {
				base := pointer("containers", name, "variables")
				rewritten, err := rewritePlaceholders(value, base, resources, types)
				if err != nil {
					return nil, err
				}
				value = rewritten
			}
			out[key] = value
		}
		containers[name] = out
	}
	return containers, nil
}

// rewritePlaceholders maps ${resources.<name>[.<output>]} onto the Deployment
// Set form and validates the referenced resource and output.
func rewritePlaceholders(v any, base string, resources map[string]scoreResource, types map[string]*ResourceType) (any, error) {
	return mapStrings(v, base, func(path, s string) (any, error) {
		var b strings.Builder
		cursor := 0
		for _, ph := range scanPlaceholders(s) {
			b.WriteString(s[cursor:ph.Start])
			replacement, err := rewriteOne(ph.Body, path, resources, types)
			if err != nil {
				return nil, err
			}
			b.WriteString(replacement)
			cursor = ph.End
		}
		b.WriteString(s[cursor:])
		return b.String(), nil
	})
}

func rewriteOne(body, path string, resources map[string]scoreResource, types map[string]*ResourceType) (string, error) {
	segments := strings.Split(body, ".")
	if segments[0] != "resources" {
		return "${" + body + "}", nil
	}
	// Only ${resources.<name>} and ${resources.<name>.<output>} are in scope.
	if len(segments) < 2 || segments[1] == "" {
		return "", newError("convert", "UNKNOWN_RESOURCE", path)
	}
	if len(segments) > 3 {
		return "", newError("convert", "UNKNOWN_OUTPUT", path)
	}
	name := segments[1]
	res, ok := resources[name]
	if !ok {
		return "", newError("convert", "UNKNOWN_RESOURCE", path)
	}
	out := res.target()
	if len(segments) == 3 {
		output := segments[2]
		if output == "" {
			return "", newError("convert", "UNKNOWN_OUTPUT", path)
		}
		if _, ok := types[res.Type].Outputs[output]; !ok {
			return "", newError("convert", "UNKNOWN_OUTPUT", path)
		}
		out += "." + output
	}
	return "${" + out + "}", nil
}
