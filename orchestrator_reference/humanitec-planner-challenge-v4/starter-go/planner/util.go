package planner

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// loadYAML reads one YAML document and normalises it to JSON-like Go values.
func loadYAML(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return normalise(v), nil
}

// normalise converts YAML values into the subset used everywhere else:
// map[string]any, []any, string, bool, int, float64, nil.
func normalise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalise(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalise(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalise(val)
		}
		return out
	default:
		return v
	}
}

func asDocument(v any) Document {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func mapAt(doc Document, key string) Document {
	if doc == nil {
		return nil
	}
	return asDocument(doc[key])
}

func stringAt(doc Document, key string) string {
	if doc == nil {
		return ""
	}
	if s, ok := doc[key].(string); ok {
		return s
	}
	return ""
}

func deepEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = deepCopy(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = deepCopy(val)
		}
		return out
	default:
		return v
	}
}

func sortedKeys(doc Document) []string {
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unionKeys(a, b Document) []string {
	set := make(map[string]bool, len(a)+len(b))
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// escapePointer applies RFC 6901 token escaping.
func escapePointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

func pointer(segments ...string) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString("/")
		b.WriteString(escapePointer(s))
	}
	return b.String()
}

// placeholder is one ${...} occurrence found in a string leaf.
type placeholder struct {
	Body  string // text between ${ and }
	Start int
	End   int // index just past the closing brace
}

// scanPlaceholders returns every non-escaped ${...} in s. A $${...} sequence is
// an escaped literal and is skipped entirely.
func scanPlaceholders(s string) []placeholder {
	var out []placeholder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		escaped := false
		j := i
		if j+1 < len(s) && s[j+1] == '$' {
			escaped = true
			j++
		}
		if j+1 >= len(s) || s[j+1] != '{' {
			i = j
			continue
		}
		end := strings.IndexByte(s[j+2:], '}')
		if end < 0 {
			i = j
			continue
		}
		end = j + 2 + end
		if !escaped {
			out = append(out, placeholder{Body: s[j+2 : end], Start: i, End: end + 1})
		}
		i = end
	}
	return out
}

// walkStrings visits every string leaf of v, passing the JSON Pointer built
// from prefix. Map keys are visited in lexical order for determinism.
func walkStrings(v any, prefix string, fn func(path string, s string) error) error {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(t) {
			if err := walkStrings(t[k], prefix+"/"+escapePointer(k), fn); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range t {
			if err := walkStrings(item, fmt.Sprintf("%s/%d", prefix, i), fn); err != nil {
				return err
			}
		}
	case string:
		return fn(prefix, t)
	}
	return nil
}

// mapStrings rewrites every string leaf of v through fn.
func mapStrings(v any, prefix string, fn func(path string, s string) (any, error)) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for _, k := range sortedKeys(t) {
			val, err := mapStrings(t[k], prefix+"/"+escapePointer(k), fn)
			if err != nil {
				return nil, err
			}
			out[k] = val
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			val, err := mapStrings(item, fmt.Sprintf("%s/%d", prefix, i), fn)
			if err != nil {
				return nil, err
			}
			out[i] = val
		}
		return out, nil
	case string:
		return fn(prefix, t)
	default:
		return v, nil
	}
}

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
