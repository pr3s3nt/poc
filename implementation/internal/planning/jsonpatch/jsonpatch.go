// Package jsonpatch builds and applies the deterministic RFC 6902 patch that
// represents a Deployment Delta. The invariant current + delta = candidate is
// the contract every caller depends on (UC-06 BR-01).
package jsonpatch

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Op is one RFC 6902 operation.
type Op struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

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

// Diff returns the deterministic patch that turns from into to.
// Arrays are compared as whole values so the patch never depends on index heuristics.
func Diff(from, to any) []Op {
	ops := diff("", from, to)
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Path != ops[j].Path {
			return ops[i].Path < ops[j].Path
		}
		return ops[i].Op < ops[j].Op
	})
	if ops == nil {
		ops = []Op{}
	}
	return ops
}

func diff(path string, from, to any) []Op {
	fromMap, fromOK := from.(map[string]any)
	toMap, toOK := to.(map[string]any)
	if fromOK && toOK {
		var ops []Op
		keys := map[string]bool{}
		for k := range fromMap {
			keys[k] = true
		}
		for k := range toMap {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			child := path + "/" + EscapeToken(k)
			fv, fok := fromMap[k]
			tv, tok := toMap[k]
			switch {
			case fok && !tok:
				ops = append(ops, Op{Op: "remove", Path: child})
			case !fok && tok:
				ops = append(ops, Op{Op: "add", Path: child, Value: tv})
			default:
				ops = append(ops, diff(child, fv, tv)...)
			}
		}
		return ops
	}
	if reflect.DeepEqual(from, to) {
		return nil
	}
	return []Op{{Op: "replace", Path: path, Value: to}}
}

// Apply returns a new document with every operation applied in order.
func Apply(doc any, ops []Op) (any, error) {
	current, err := clone(doc)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		current, err = applyOne(current, op)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}

func applyOne(doc any, op Op) (any, error) {
	if op.Path == "" {
		switch op.Op {
		case "add", "replace":
			return clone(op.Value)
		default:
			return nil, fmt.Errorf("jsonpatch: cannot %s the document root", op.Op)
		}
	}
	tokens := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
	for i := range tokens {
		tokens[i] = UnescapeToken(tokens[i])
	}
	parent, err := navigate(doc, tokens[:len(tokens)-1])
	if err != nil {
		return nil, err
	}
	last := tokens[len(tokens)-1]
	switch container := parent.(type) {
	case map[string]any:
		switch op.Op {
		case "add", "replace":
			value, err := clone(op.Value)
			if err != nil {
				return nil, err
			}
			container[last] = value
		case "remove":
			if _, ok := container[last]; !ok {
				return nil, fmt.Errorf("jsonpatch: remove %s: key missing", op.Path)
			}
			delete(container, last)
		default:
			return nil, fmt.Errorf("jsonpatch: unsupported op %q", op.Op)
		}
	case []any:
		idx, err := strconv.Atoi(last)
		if err != nil || idx < 0 || idx >= len(container) {
			return nil, fmt.Errorf("jsonpatch: invalid array index in %s", op.Path)
		}
		if op.Op != "replace" {
			return nil, fmt.Errorf("jsonpatch: only replace is supported on array elements")
		}
		value, err := clone(op.Value)
		if err != nil {
			return nil, err
		}
		container[idx] = value
	default:
		return nil, fmt.Errorf("jsonpatch: path %s has no container parent", op.Path)
	}
	return doc, nil
}

func navigate(doc any, tokens []string) (any, error) {
	current := doc
	for _, token := range tokens {
		switch container := current.(type) {
		case map[string]any:
			next, ok := container[token]
			if !ok {
				return nil, fmt.Errorf("jsonpatch: path segment %q not found", token)
			}
			current = next
		case []any:
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx >= len(container) {
				return nil, fmt.Errorf("jsonpatch: invalid array index %q", token)
			}
			current = container[idx]
		default:
			return nil, fmt.Errorf("jsonpatch: path segment %q has no container", token)
		}
	}
	return current, nil
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
