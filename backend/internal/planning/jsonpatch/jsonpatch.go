// Package jsonpatch builds and applies the deterministic RFC 6902 patches that a
// Deployment Delta attaches to one module or to the shared object
// (UC-05 BR-05/BR-06). Callers prove base + patch = target with Apply.
package jsonpatch

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"orchestrator/internal/domain/deployment"
)

// patchOp is the domain JSON Patch operation value object.
type patchOp = deployment.JSONPatchOperation

// appendToken is the JSON Pointer token that appends to an array.
const appendToken = "-"

// EscapeToken encodes one JSON Pointer reference token.
func EscapeToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

// UnescapeToken decodes one JSON Pointer reference token.
func UnescapeToken(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	return strings.ReplaceAll(token, "~0", "~")
}

// Diff returns the deterministic patch that turns from into to:
//   - objects: walk the union of keys in lexical order; a missing key is
//     removed, a new key is added and a shared key recurses;
//   - arrays: recurse into common indexes, remove the tail from the highest
//     index down, then append new elements with `/-`;
//   - primitives or values of different kinds: replace.
func Diff(from, to any) []patchOp {
	ops := diff("", from, to)
	if ops == nil {
		ops = []patchOp{}
	}
	return ops
}

func diff(path string, from, to any) []patchOp {
	if fromMap, ok := from.(map[string]any); ok {
		if toMap, ok := to.(map[string]any); ok {
			return diffObject(path, fromMap, toMap)
		}
	}
	if fromList, ok := from.([]any); ok {
		if toList, ok := to.([]any); ok {
			return diffArray(path, fromList, toList)
		}
	}
	if reflect.DeepEqual(from, to) {
		return nil
	}
	return []patchOp{{Op: deployment.PatchReplace, Path: path, Value: to}}
}

func diffObject(path string, from, to map[string]any) []patchOp {
	keys := make([]string, 0, len(from)+len(to))
	for k := range from {
		keys = append(keys, k)
	}
	for k := range to {
		if _, ok := from[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var ops []patchOp
	for _, k := range keys {
		child := path + "/" + EscapeToken(k)
		fv, fok := from[k]
		tv, tok := to[k]
		switch {
		case fok && !tok:
			ops = append(ops, patchOp{Op: deployment.PatchRemove, Path: child})
		case !fok && tok:
			ops = append(ops, patchOp{Op: deployment.PatchAdd, Path: child, Value: tv})
		default:
			ops = append(ops, diff(child, fv, tv)...)
		}
	}
	return ops
}

func diffArray(path string, from, to []any) []patchOp {
	common := min(len(from), len(to))
	var ops []patchOp
	for i := 0; i < common; i++ {
		ops = append(ops, diff(path+"/"+strconv.Itoa(i), from[i], to[i])...)
	}
	for i := len(from) - 1; i >= common; i-- {
		ops = append(ops, patchOp{Op: deployment.PatchRemove, Path: path + "/" + strconv.Itoa(i)})
	}
	for i := common; i < len(to); i++ {
		ops = append(ops, patchOp{Op: deployment.PatchAdd, Path: path + "/" + appendToken, Value: to[i]})
	}
	return ops
}

// Apply returns a new document with every operation applied in order. The
// input document is never modified.
func Apply(doc any, ops []patchOp) (any, error) {
	current, err := clone(doc)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		if err := op.Validate(); err != nil {
			return nil, fmt.Errorf("jsonpatch: %w", err)
		}
		current, err = applyOne(current, op)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}

func applyOne(doc any, op patchOp) (any, error) {
	if op.Path == "" {
		if op.Op == deployment.PatchRemove {
			return nil, fmt.Errorf("jsonpatch: cannot remove the document root")
		}
		return clone(op.Value)
	}
	tokens := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
	for i := range tokens {
		tokens[i] = UnescapeToken(tokens[i])
	}
	last := tokens[len(tokens)-1]
	var value any
	if op.Op != deployment.PatchRemove {
		var err error
		if value, err = clone(op.Value); err != nil {
			return nil, err
		}
	}
	updated, err := update(doc, tokens[:len(tokens)-1], func(parent any) (any, error) {
		return applyToParent(parent, last, op, value)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// update walks to the parent container and replaces it with the result of fn,
// rebuilding the slices along the way because arrays can change length.
func update(node any, tokens []string, fn func(any) (any, error)) (any, error) {
	if len(tokens) == 0 {
		return fn(node)
	}
	token := tokens[0]
	switch container := node.(type) {
	case map[string]any:
		child, ok := container[token]
		if !ok {
			return nil, fmt.Errorf("jsonpatch: path segment %q not found", token)
		}
		updated, err := update(child, tokens[1:], fn)
		if err != nil {
			return nil, err
		}
		container[token] = updated
		return container, nil
	case []any:
		idx, err := index(token, len(container))
		if err != nil {
			return nil, err
		}
		updated, err := update(container[idx], tokens[1:], fn)
		if err != nil {
			return nil, err
		}
		container[idx] = updated
		return container, nil
	default:
		return nil, fmt.Errorf("jsonpatch: path segment %q has no container", token)
	}
}

func applyToParent(parent any, last string, op patchOp, value any) (any, error) {
	switch container := parent.(type) {
	case map[string]any:
		_, exists := container[last]
		switch op.Op {
		case deployment.PatchAdd:
			container[last] = value
		case deployment.PatchReplace:
			if !exists {
				return nil, fmt.Errorf("jsonpatch: replace %s: key missing", op.Path)
			}
			container[last] = value
		case deployment.PatchRemove:
			if !exists {
				return nil, fmt.Errorf("jsonpatch: remove %s: key missing", op.Path)
			}
			delete(container, last)
		}
		return container, nil
	case []any:
		switch op.Op {
		case deployment.PatchAdd:
			if last == appendToken {
				return append(container, value), nil
			}
			idx, err := index(last, len(container)+1)
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(container)+1)
			out = append(out, container[:idx]...)
			out = append(out, value)
			return append(out, container[idx:]...), nil
		case deployment.PatchReplace:
			idx, err := index(last, len(container))
			if err != nil {
				return nil, err
			}
			container[idx] = value
			return container, nil
		case deployment.PatchRemove:
			idx, err := index(last, len(container))
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(container)-1)
			out = append(out, container[:idx]...)
			return append(out, container[idx+1:]...), nil
		}
	}
	return nil, fmt.Errorf("jsonpatch: path %s has no container parent", op.Path)
}

// index parses an array index token that must be below limit.
func index(token string, limit int) (int, error) {
	idx, err := strconv.Atoi(token)
	if err != nil || idx < 0 || idx >= limit || (len(token) > 1 && token[0] == '0') {
		return 0, fmt.Errorf("jsonpatch: invalid array index %q", token)
	}
	return idx, nil
}

func clone(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("jsonpatch: marshal: %w", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("jsonpatch: unmarshal: %w", err)
	}
	return out, nil
}
