// Package placeholder parses and resolves the ${...} syntax used by Score
// documents, Deployment Sets and Resource Definition driver inputs.
//
// Three forms exist:
//
//	${context.<path>}                                  context value
//	${externals.<name>[.<output>]}                     Deployment Set private resource
//	${shared.<id>[.<output>]}                          Deployment Set shared resource
//	${resources['TYPE[.CLASS][#ID]'].outputs.<name>}   Resource Reference in driver inputs
//	${resources.TYPE.outputs.<name>}                   short Resource Reference
//
// `$${...}` is an escaped literal and never resolves.
package placeholder

import (
	"fmt"
	"regexp"
	"strings"
)

// Pattern matches one placeholder, including the escaped form $${...}.
var Pattern = regexp.MustCompile(`\$\$?\{([^}]*)\}`)

// Kind classifies a placeholder reference.
type Kind string

// Placeholder kinds.
const (
	KindResource Kind = "resource"
	KindContext  Kind = "context"
)

// Ref is a parsed placeholder reference. Resource holds the binding key the
// consumer uses: `externals.<name>`, `shared.<id>` or a raw descriptor.
type Ref struct {
	Kind      Kind
	Resource  string
	OutputKey string
	Context   string
	Raw       string
}

// SetScope names the two Deployment Set placeholder scopes.
const (
	ScopeExternals = "externals"
	ScopeShared    = "shared"
)

// Parse reads one placeholder body.
func Parse(body string) (Ref, error) {
	trimmed := strings.TrimSpace(body)
	switch {
	case strings.HasPrefix(trimmed, "context."):
		return Ref{Kind: KindContext, Context: strings.TrimPrefix(trimmed, "context."), Raw: trimmed}, nil
	case strings.HasPrefix(trimmed, ScopeExternals+"."), strings.HasPrefix(trimmed, ScopeShared+"."):
		segments := strings.Split(trimmed, ".")
		if len(segments) < 2 || segments[1] == "" {
			return Ref{}, fmt.Errorf("placeholder: %q has no resource name", body)
		}
		if len(segments) > 3 {
			return Ref{}, fmt.Errorf("placeholder: %q has more segments than <scope>.<name>.<output>", body)
		}
		ref := Ref{Kind: KindResource, Resource: segments[0] + "." + segments[1], Raw: trimmed}
		if len(segments) == 3 {
			if segments[2] == "" {
				return Ref{}, fmt.Errorf("placeholder: %q has an empty output", body)
			}
			ref.OutputKey = segments[2]
		}
		return ref, nil
	case strings.HasPrefix(trimmed, "resources"):
		descriptor, output, err := parseResourceReference(trimmed)
		if err != nil {
			return Ref{}, err
		}
		return Ref{Kind: KindResource, Resource: descriptor, OutputKey: output, Raw: trimmed}, nil
	default:
		return Ref{}, fmt.Errorf("placeholder: unsupported reference %q", body)
	}
}

// parseResourceReference reads the driver-input Resource Reference form.
func parseResourceReference(body string) (string, string, error) {
	rest, ok := strings.CutPrefix(body, "resources")
	if !ok || rest == "" {
		return "", "", fmt.Errorf("placeholder: %q is not a resource reference", body)
	}
	var raw string
	switch {
	case strings.HasPrefix(rest, "['"), strings.HasPrefix(rest, "[\""):
		quote := rest[1]
		end := strings.Index(rest[2:], string(quote)+"]")
		if end < 0 {
			return "", "", fmt.Errorf("placeholder: %q has an unterminated descriptor", body)
		}
		raw = rest[2 : 2+end]
		rest = rest[2+end+2:]
	case strings.HasPrefix(rest, "."):
		rest = rest[1:]
		idx := strings.Index(rest, ".")
		if idx < 0 {
			return "", "", fmt.Errorf("placeholder: %q has no output segment", body)
		}
		raw = rest[:idx]
		rest = rest[idx:]
	default:
		return "", "", fmt.Errorf("placeholder: %q is not a resource reference", body)
	}
	output, ok := strings.CutPrefix(rest, ".outputs.")
	if !ok || output == "" {
		return "", "", fmt.Errorf("placeholder: %q has no .outputs.<name> segment", body)
	}
	return raw, output, nil
}

// LooksLikeResourceReference reports a placeholder that opens a resource
// reference but does not parse, so it is rejected instead of ignored.
func LooksLikeResourceReference(body string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(body), "resources")
	if !ok || rest == "" {
		return false
	}
	return rest[0] == '.' || rest[0] == '['
}

// Refs returns every non-escaped placeholder reference inside s.
func Refs(s string) ([]Ref, error) {
	var refs []Ref
	for _, m := range Pattern.FindAllStringSubmatch(s, -1) {
		if strings.HasPrefix(m[0], "$$") {
			continue
		}
		ref, err := Parse(m[1])
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// Resolver supplies values for placeholder references.
type Resolver interface {
	ResolveResource(binding, outputKey string) (any, error)
	ResolveContext(path string) (any, error)
}

// ExpandString replaces every placeholder in s. When the whole string is a
// single placeholder the typed value is returned; otherwise values are
// stringified.
func ExpandString(s string, r Resolver) (any, error) {
	matches := Pattern.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s, nil
	}
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) && !strings.HasPrefix(s, "$$") {
		return resolveBody(s[matches[0][2]:matches[0][3]], r)
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(s[last:m[0]])
		raw := s[m[0]:m[1]]
		body := s[m[2]:m[3]]
		if strings.HasPrefix(raw, "$$") {
			b.WriteString("${" + body + "}")
		} else {
			value, err := resolveBody(body, r)
			if err != nil {
				return nil, err
			}
			b.WriteString(Stringify(value))
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String(), nil
}

func resolveBody(body string, r Resolver) (any, error) {
	ref, err := Parse(body)
	if err != nil {
		return nil, err
	}
	if ref.Kind == KindResource {
		return r.ResolveResource(ref.Resource, ref.OutputKey)
	}
	return r.ResolveContext(ref.Context)
}

// ExpandTree walks a JSON document tree and expands every string placeholder.
func ExpandTree(v any, r Resolver) (any, error) {
	switch t := v.(type) {
	case string:
		return ExpandString(t, r)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, value := range t {
			expanded, err := ExpandTree(value, r)
			if err != nil {
				return nil, err
			}
			out[k] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			expanded, err := ExpandTree(value, r)
			if err != nil {
				return nil, err
			}
			out[i] = expanded
		}
		return out, nil
	default:
		return v, nil
	}
}

// WalkStrings visits every string leaf of a JSON document tree in a
// deterministic order, reporting the JSON Pointer of each leaf.
func WalkStrings(v any, base string, fn func(path, value string) error) error {
	switch t := v.(type) {
	case string:
		return fn(base, t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			if err := WalkStrings(t[k], base+"/"+escapePointer(k), fn); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range t {
			if err := WalkStrings(item, fmt.Sprintf("%s/%d", base, i), fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func escapePointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}

// Stringify renders a resolved value for string interpolation.
func Stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case bool:
		return fmt.Sprintf("%t", t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}
