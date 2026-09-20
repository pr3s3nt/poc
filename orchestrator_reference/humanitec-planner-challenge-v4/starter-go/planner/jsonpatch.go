package planner

import (
	"fmt"
	"strconv"
	"strings"
)

// diffPatch produces a deterministic RFC 6902 patch turning from into to.
//
// Object keys are walked in lexical order over the union of both sides.
// Arrays diff their common prefix, then remove the tail from the highest index
// down, then append the remaining tail with "/-".
func diffPatch(from, to any, path string) []Document {
	var ops []Document

	fromMap, fromIsMap := from.(map[string]any)
	toMap, toIsMap := to.(map[string]any)
	if fromIsMap && toIsMap {
		for _, k := range unionKeys(fromMap, toMap) {
			fv, inFrom := fromMap[k]
			tv, inTo := toMap[k]
			child := path + "/" + escapePointer(k)
			switch {
			case inFrom && !inTo:
				ops = append(ops, Document{"op": "remove", "path": child})
			case !inFrom && inTo:
				ops = append(ops, Document{"op": "add", "path": child, "value": tv})
			default:
				ops = append(ops, diffPatch(fv, tv, child)...)
			}
		}
		return ops
	}

	fromArr, fromIsArr := from.([]any)
	toArr, toIsArr := to.([]any)
	if fromIsArr && toIsArr {
		common := len(fromArr)
		if len(toArr) < common {
			common = len(toArr)
		}
		for i := 0; i < common; i++ {
			ops = append(ops, diffPatch(fromArr[i], toArr[i], fmt.Sprintf("%s/%d", path, i))...)
		}
		for i := len(fromArr) - 1; i >= common; i-- {
			ops = append(ops, Document{"op": "remove", "path": fmt.Sprintf("%s/%d", path, i)})
		}
		for i := common; i < len(toArr); i++ {
			ops = append(ops, Document{"op": "add", "path": path + "/-", "value": toArr[i]})
		}
		return ops
	}

	if !deepEqual(from, to) {
		ops = append(ops, Document{"op": "replace", "path": path, "value": to})
	}
	return ops
}

func unescapeToken(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	return strings.ReplaceAll(token, "~0", "~")
}

func splitPointer(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		parts[i] = unescapeToken(p)
	}
	return parts
}

// applyPatch applies ops to a deep copy of doc.
func applyPatch(doc any, ops []Document) (any, error) {
	result := deepCopy(doc)
	for _, op := range ops {
		var err error
		result, err = applyOp(result, op)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func applyOp(root any, op Document) (any, error) {
	kind, _ := op["op"].(string)
	path, _ := op["path"].(string)
	switch kind {
	case "add", "replace", "remove":
	default:
		return nil, fmt.Errorf("unsupported op %q", kind)
	}
	return setIn(root, splitPointer(path), kind, op["value"])
}

// setIn applies one operation at the pointer tokens and returns the new node.
func setIn(node any, tokens []string, kind string, value any) (any, error) {
	if len(tokens) == 0 {
		if kind == "remove" {
			return nil, fmt.Errorf("cannot remove the document root")
		}
		return value, nil
	}
	token := tokens[0]

	switch container := node.(type) {
	case map[string]any:
		if len(tokens) == 1 {
			switch kind {
			case "add", "replace":
				container[token] = value
			case "remove":
				if _, ok := container[token]; !ok {
					return nil, fmt.Errorf("remove: missing key %q", token)
				}
				delete(container, token)
			}
			return container, nil
		}
		child, ok := container[token]
		if !ok {
			return nil, fmt.Errorf("missing pointer segment %q", token)
		}
		updated, err := setIn(child, tokens[1:], kind, value)
		if err != nil {
			return nil, err
		}
		container[token] = updated
		return container, nil

	case []any:
		if len(tokens) == 1 && kind == "add" && token == "-" {
			return append(container, value), nil
		}
		idx, err := strconv.Atoi(token)
		if err != nil || idx < 0 || idx > len(container) {
			return nil, fmt.Errorf("bad array index %q", token)
		}
		if len(tokens) == 1 {
			switch kind {
			case "add":
				out := make([]any, 0, len(container)+1)
				out = append(out, container[:idx]...)
				out = append(out, value)
				return append(out, container[idx:]...), nil
			case "replace":
				if idx >= len(container) {
					return nil, fmt.Errorf("replace: index %d out of range", idx)
				}
				container[idx] = value
				return container, nil
			case "remove":
				if idx >= len(container) {
					return nil, fmt.Errorf("remove: index %d out of range", idx)
				}
				out := make([]any, 0, len(container)-1)
				out = append(out, container[:idx]...)
				return append(out, container[idx+1:]...), nil
			}
		}
		if idx >= len(container) {
			return nil, fmt.Errorf("bad array index %q", token)
		}
		updated, err := setIn(container[idx], tokens[1:], kind, value)
		if err != nil {
			return nil, err
		}
		container[idx] = updated
		return container, nil

	default:
		return nil, fmt.Errorf("pointer segment %q hits a leaf", token)
	}
}
