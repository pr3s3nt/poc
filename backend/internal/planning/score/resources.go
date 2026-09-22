package score

import (
	"encoding/json"
	"fmt"
)

// Allowed keys of `containers.*.resources` (UC-05 BR-07, UC-06 BR-11).
var (
	resourceBranches = map[string]bool{"requests": true, "limits": true}
	computeKeys      = map[string]bool{"cpu": true, "memory": true}
)

// validateContainerResources checks the raw `containers.*.resources` trees.
// The typed decoder already rejects unknown keys and non-string values, but it
// silently accepts `null` and empty strings, which would make a declared field
// indistinguishable from an omitted one. A value must be a non-empty string;
// Kubernetes quantity syntax is validated by the Kubernetes API at apply time,
// not here.
func validateContainerResources(b []byte) error {
	var raw struct {
		Containers map[string]map[string]json.RawMessage `json:"containers"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("score: invalid document: %w", err)
	}
	for _, name := range sortedKeys(raw.Containers) {
		value, declared := raw.Containers[name]["resources"]
		if !declared {
			continue
		}
		where := fmt.Sprintf("container %q resources", name)
		branches, err := object(value, where)
		if err != nil {
			return err
		}
		for _, branch := range sortedKeys(branches) {
			if !resourceBranches[branch] {
				return fmt.Errorf("score: %s: unsupported field %q, want requests or limits", where, branch)
			}
			fields, err := object(branches[branch], where+"."+branch)
			if err != nil {
				return err
			}
			for _, key := range sortedKeys(fields) {
				if !computeKeys[key] {
					return fmt.Errorf("score: %s.%s: unsupported resource %q, want cpu or memory", where, branch, key)
				}
				var quantity *string
				if err := json.Unmarshal(fields[key], &quantity); err != nil || quantity == nil || *quantity == "" {
					return fmt.Errorf("score: %s.%s.%s must be a non-empty string", where, branch, key)
				}
			}
		}
	}
	return nil
}

// object decodes a JSON object and rejects null or any other JSON type.
func object(value json.RawMessage, where string) (map[string]json.RawMessage, error) {
	var out map[string]json.RawMessage
	if err := json.Unmarshal(value, &out); err != nil || out == nil {
		return nil, fmt.Errorf("score: %s must be an object", where)
	}
	return out, nil
}
