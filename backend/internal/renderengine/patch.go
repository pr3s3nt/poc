package renderengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"
)

// MaxPatchOperations bounds the operations one patch template may return.
const MaxPatchOperations = 128

// PatchOperation is one score-k8s 0.15.0 compatible patch operation: "set" or
// "delete" of a dotted path into the manifest array (tidwall/sjson syntax, as
// pinned by score-k8s). It is not an RFC 6902 operation.
type PatchOperation struct {
	Op          string `json:"op" yaml:"op"`
	Path        string `json:"path" yaml:"path"`
	Value       any    `json:"value,omitempty" yaml:"value,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ErrPatch is the class of every invalid patch output or operation.
var ErrPatch = errors.New("renderengine: patch failed")

// PatchError is a safe patch failure: file and operation number only.
type PatchError struct {
	File      string
	Operation int
	Reason    string
}

func (e *PatchError) Error() string {
	if e.Operation > 0 {
		return fmt.Sprintf("renderengine: patch %s operation %d: %s", e.File, e.Operation, e.Reason)
	}
	return fmt.Sprintf("renderengine: patch %s: %s", e.File, e.Reason)
}
func (e *PatchError) Is(target error) bool { return target == ErrPatch }

// ParsePatchOutput decodes the YAML/JSON array a patch template returned
// exactly like score-k8s (known fields only). Empty output is a no-op.
func ParsePatchOutput(file, output string) ([]PatchOperation, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, nil
	}
	var ops []PatchOperation
	decoder := yaml.NewDecoder(strings.NewReader(trimmed))
	decoder.KnownFields(true)
	if err := decoder.Decode(&ops); err != nil {
		return nil, &PatchError{File: file, Reason: "the output is not an array of set/delete operations"}
	}
	if len(ops) > MaxPatchOperations {
		return nil, &PatchError{File: file, Reason: fmt.Sprintf("more than %d operations", MaxPatchOperations)}
	}
	return ops, nil
}

// ApplyPatches applies operations in order to a copy of manifests with the
// score-k8s semantics. Every path must address inside one existing manifest:
// v1 neither inserts nor deletes objects at the array root.
func ApplyPatches(file string, manifests []map[string]any, ops []PatchOperation) ([]map[string]any, error) {
	document, err := json.Marshal(manifests)
	if err != nil {
		return nil, &PatchError{File: file, Reason: "the manifests cannot be encoded"}
	}
	for i, op := range ops {
		if err := checkPath(op.Path, len(manifests)); err != nil {
			return nil, &PatchError{File: file, Operation: i + 1, Reason: err.Error()}
		}
		switch op.Op {
		case "set":
			document, err = sjson.SetBytes(document, op.Path, normalizeYAML(op.Value))
		case "delete":
			document, err = sjson.DeleteBytes(document, op.Path)
		default:
			err = errors.New("unknown operation")
		}
		if err != nil {
			return nil, &PatchError{File: file, Operation: i + 1, Reason: "the operation cannot be applied"}
		}
	}
	var out []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(document))
	if err := decoder.Decode(&out); err != nil || len(out) != len(manifests) {
		return nil, &PatchError{File: file, Reason: "the patched manifests are not an array of objects"}
	}
	for _, object := range out {
		if object == nil {
			return nil, &PatchError{File: file, Reason: "a patch replaced a manifest object"}
		}
	}
	return out, nil
}

// checkPath requires a manifest index segment followed by at least one field.
func checkPath(path string, count int) error {
	first, rest, ok := cutUnescaped(path)
	if !ok || rest == "" {
		return errors.New("the path must address a field inside one manifest")
	}
	index, err := strconv.Atoi(first)
	if err != nil || index < 0 || index >= count || strconv.Itoa(index) != first {
		return errors.New("the path must start with an existing manifest index")
	}
	return nil
}

func cutUnescaped(path string) (string, string, bool) {
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '\\':
			i++
		case '.':
			return path[:i], path[i+1:], true
		}
	}
	return path, "", false
}

// normalizeYAML converts YAML-decoded values to JSON-compatible trees.
func normalizeYAML(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = normalizeYAML(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[fmt.Sprint(k)] = normalizeYAML(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = normalizeYAML(item)
		}
		return out
	default:
		return value
	}
}
