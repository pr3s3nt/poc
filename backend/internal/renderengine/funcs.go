// Package renderengine is the deterministic template engine of Git render
// bundles (ADR-021, contract RB-C04): the closed git-render-v1 helper set,
// bundle tree validation, the killable worker subprocess that executes
// author templates and the score-k8s compatible set/delete patch semantics.
package renderengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// FuncMap returns the closed git-render-v1 helper set. Every helper is
// deterministic: no environment, file, network, shell, clock or randomness.
// Helpers return errors instead of panicking.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"dict":        dict,
		"list":        func(items ...any) []any { return append([]any{}, items...) },
		"append":      appendList,
		"get":         get,
		"hasKey":      hasKey,
		"set":         set,
		"unset":       unset,
		"merge":       merge,
		"deepCopy":    deepCopy,
		"keys":        keys,
		"sortAlpha":   sortAlpha,
		"toJson":      toJSON,
		"fromJson":    fromJSON,
		"quote":       quote,
		"default":     defaultValue,
		"required":    required,
		"fail":        fail,
		"escapeScore": escapeScore,
	}
}

func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict needs key/value pairs")
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}

func asList(v any) ([]any, error) {
	switch list := v.(type) {
	case nil:
		return []any{}, nil
	case []any:
		return list, nil
	case []string:
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = item
		}
		return out, nil
	}
	return nil, errors.New("value is not a list")
}

func appendList(list any, value any) ([]any, error) {
	items, err := asList(list)
	if err != nil {
		return nil, err
	}
	return append(append([]any{}, items...), value), nil
}

func asMap(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil, errors.New("value is not an object")
	}
	return m, nil
}

// get returns the value of key, or nil when the key is absent.
func get(m any, key string) (any, error) {
	object, err := asMap(m)
	if err != nil {
		return nil, err
	}
	return object[key], nil
}

func hasKey(m any, key string) (bool, error) {
	object, err := asMap(m)
	if err != nil {
		return false, err
	}
	_, ok := object[key]
	return ok, nil
}

// set and unset change the object in place and return it, like their Sprig
// counterparts. Template data is the worker's private copy.
func set(m any, key string, value any) (map[string]any, error) {
	object, err := asMap(m)
	if err != nil {
		return nil, err
	}
	object[key] = value
	return object, nil
}

func unset(m any, key string) (map[string]any, error) {
	object, err := asMap(m)
	if err != nil {
		return nil, err
	}
	delete(object, key)
	return object, nil
}

// merge recursively overlays later objects on earlier ones into a new object.
// Arrays and scalars are replaced; no input is changed.
func merge(objects ...any) (map[string]any, error) {
	out := map[string]any{}
	for _, raw := range objects {
		object, err := asMap(raw)
		if err != nil {
			return nil, err
		}
		out = overlay(out, object)
	}
	return out, nil
}

func overlay(base, top map[string]any) map[string]any {
	out, _ := deepCopy(base).(map[string]any)
	for key, value := range top {
		if child, ok := value.(map[string]any); ok {
			if prior, ok := out[key].(map[string]any); ok {
				out[key] = overlay(prior, child)
				continue
			}
		}
		out[key] = deepCopy(value)
	}
	return out
}

func deepCopy(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = deepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return value
	}
}

// keys returns the object keys sorted, so ordered output is deterministic.
func keys(m any) ([]any, error) {
	object, err := asMap(m)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(object))
	for key := range object {
		names = append(names, key)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = name
	}
	return out, nil
}

func sortAlpha(list any) ([]any, error) {
	items, err := asList(list)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(items))
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("sortAlpha needs a list of strings")
		}
		names[i] = text
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = name
	}
	return out, nil
}

func toJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", errors.New("toJson: the value cannot be encoded")
	}
	return string(raw), nil
}

func fromJSON(text string) (any, error) {
	var out any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, errors.New("fromJson: the text is not JSON")
	}
	return out, nil
}

func quote(values ...any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%q", stringValue(value)))
	}
	return strings.Join(parts, " ")
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case fmt.Stringer:
		return value.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

// defaultValue returns given unless it is absent or nil; false, 0 and the
// empty string are explicit values and are kept.
func defaultValue(fallback any, given ...any) any {
	if len(given) == 0 || given[0] == nil {
		return fallback
	}
	return given[0]
}

// required rejects an absent or nil value only.
func required(message string, value any) (any, error) {
	if value == nil {
		return nil, errors.New(message)
	}
	return value, nil
}

func fail(message string) (string, error) { return "", errors.New(message) }

// escapeScore escapes every literal $ exactly once, so Score placeholder
// substitution restores the original text. Lists and objects are escaped
// element by element; other scalars are returned unchanged.
func escapeScore(v any) any {
	switch value := v.(type) {
	case string:
		return strings.ReplaceAll(value, "$", "$$")
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = escapeScore(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = escapeScore(item)
		}
		return out
	default:
		return value
	}
}
