package renderengine

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaxObjects bounds the objects one rendered workload may hold (ADR-024).
const MaxObjects = 128

// ParseResourceOutput decodes the YAML/JSON array of Kubernetes objects a
// resources template returned (ADR-024). Empty output is a no-op. Each item
// must be an object with apiVersion, kind and metadata.name; deeper rules are
// the platform guard's.
func ParseResourceOutput(file, output string) ([]map[string]any, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, nil
	}
	var items []any
	if err := yaml.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, &PatchError{File: file, Reason: "the output is not an array of Kubernetes objects"}
	}
	if len(items) > MaxObjects {
		return nil, &PatchError{File: file, Reason: "too many objects"}
	}
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		encoded, err := json.Marshal(normalizeYAML(item))
		if err != nil {
			return nil, &PatchError{File: file, Operation: i + 1, Reason: "the object cannot be encoded"}
		}
		var object map[string]any
		if err := json.Unmarshal(encoded, &object); err != nil || object == nil {
			return nil, &PatchError{File: file, Operation: i + 1, Reason: "the item is not an object"}
		}
		api, _ := object["apiVersion"].(string)
		kind, _ := object["kind"].(string)
		meta, _ := object["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		if api == "" || kind == "" || name == "" {
			return nil, &PatchError{File: file, Operation: i + 1, Reason: "every object needs apiVersion, kind and metadata.name"}
		}
		out = append(out, object)
	}
	return out, nil
}
