package renderbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Input schema limits (RB-C03).
const (
	MaxSchemaDepth = 8
	MaxInputBytes  = 64 << 10
)

// schemaKeywords is the closed JSON Schema subset of version 1. Any other
// keyword fails inspection rather than being ignored.
var schemaKeywords = map[string]bool{
	"type": true, "properties": true, "required": true, "additionalProperties": true, "items": true,
	"enum": true, "default": true, "title": true, "description": true, "minimum": true, "maximum": true,
	"minLength": true, "maxLength": true, "pattern": true, "minItems": true, "maxItems": true,
}

var schemaTypes = map[string]bool{"string": true, "boolean": true, "integer": true, "number": true, "object": true, "array": true}

// secretName rejects inputs whose name says they carry a credential: render
// inputs are literal non-secret policy parameters.
var secretName = regexp.MustCompile(`(?i)(password|passwd|secret|token|credential|private.?key)`)

// ValidateSchema checks one inputs.schema.json against the version 1 subset.
// The root is a closed object; every object schema has
// additionalProperties false; defaults and enums must satisfy their schema.
func ValidateSchema(schema map[string]any) error {
	if schema["type"] != "object" {
		return invalid("", "the inputs schema root must be an object schema")
	}
	return validateNode(schema, 1)
}

func validateNode(node map[string]any, depth int) error {
	if depth > MaxSchemaDepth {
		return invalid("", "the inputs schema is deeper than %d levels", MaxSchemaDepth)
	}
	for key := range node {
		if !schemaKeywords[key] {
			return invalid("", "the inputs schema uses an unsupported keyword")
		}
	}
	typ, _ := node["type"].(string)
	if !schemaTypes[typ] {
		return invalid("", "every inputs schema node needs a supported type")
	}
	for _, key := range []string{"title", "description"} {
		if value, ok := node[key]; ok {
			if _, text := value.(string); !text {
				return invalid("", "schema %s must be a string", key)
			}
		}
	}
	allowed := map[string][]string{
		"string":  {"minLength", "maxLength", "pattern"},
		"integer": {"minimum", "maximum"},
		"number":  {"minimum", "maximum"},
		"array":   {"items", "minItems", "maxItems"},
		"object":  {"properties", "required", "additionalProperties"},
	}
	for _, group := range allowed {
		for _, key := range group {
			if _, present := node[key]; present && !contains(allowed[typ], key) {
				return invalid("", "the keyword %s does not apply to type %s", key, typ)
			}
		}
	}
	for _, key := range []string{"minLength", "maxLength", "minItems", "maxItems"} {
		if value, ok := node[key]; ok && !nonNegativeInteger(value) {
			return invalid("", "%s must be a non-negative integer", key)
		}
	}
	for _, key := range []string{"minimum", "maximum"} {
		if value, ok := node[key]; ok {
			if _, number := value.(float64); !number {
				return invalid("", "%s must be a number", key)
			}
		}
	}
	if raw, ok := node["pattern"]; ok {
		text, isText := raw.(string)
		if !isText {
			return invalid("", "pattern must be a string")
		}
		if _, err := regexp.Compile(text); err != nil {
			return invalid("", "pattern must be a valid RE2 expression")
		}
	}
	switch typ {
	case "object":
		if node["additionalProperties"] != false {
			return invalid("", "every object schema must set additionalProperties to false")
		}
		properties := map[string]any{}
		if raw, ok := node["properties"]; ok {
			var isObject bool
			if properties, isObject = raw.(map[string]any); !isObject {
				return invalid("", "properties must be an object")
			}
		}
		for name, raw := range properties {
			if name == "" || len(name) > 128 || secretName.MatchString(name) {
				return invalid("", "a render input name is empty, too long or names a credential")
			}
			child, isObject := raw.(map[string]any)
			if !isObject {
				return invalid("", "a property schema must be an object")
			}
			if err := validateNode(child, depth+1); err != nil {
				return err
			}
		}
		if raw, ok := node["required"]; ok {
			list, isList := raw.([]any)
			if !isList {
				return invalid("", "required must be an array of property names")
			}
			for _, entry := range list {
				name, isText := entry.(string)
				if _, declared := properties[name]; !isText || !declared {
					return invalid("", "required names an undeclared property")
				}
			}
		}
	case "array":
		items, isObject := node["items"].(map[string]any)
		if !isObject {
			return invalid("", "an array schema needs an items schema")
		}
		if err := validateNode(items, depth+1); err != nil {
			return err
		}
	}
	if raw, ok := node["enum"]; ok {
		list, isList := raw.([]any)
		if !isList || len(list) == 0 {
			return invalid("", "enum must be a non-empty array")
		}
		for _, value := range list {
			if _, err := checkValue(withoutEnum(node), value, ""); err != nil {
				return invalid("", "an enum value does not match its schema")
			}
		}
	}
	if value, ok := node["default"]; ok {
		if _, err := checkValue(node, value, ""); err != nil {
			return invalid("", "a default value does not match its schema")
		}
	}
	return nil
}

func withoutEnum(node map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range node {
		if k != "enum" && k != "default" {
			out[k] = v
		}
	}
	return out
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func nonNegativeInteger(value any) bool {
	n, ok := value.(float64)
	return ok && n >= 0 && n == math.Trunc(n) && n <= math.MaxInt32
}

// InputError is ErrInputInvalid at a schema path made of declared property
// names and indexes; it never contains a submitted value or unknown key.
type InputError struct{ Path string }

func (e *InputError) Error() string {
	if e.Path == "" {
		return ErrInputInvalid.Error()
	}
	return ErrInputInvalid.Error() + " at " + e.Path
}
func (e *InputError) Is(target error) bool { return target == ErrInputInvalid }

// Effective applies schema defaults to absent keys only (never to explicit
// false, 0 or empty string), then validates types, bounds and unknown keys
// with no coercion. Arrays replace; objects recurse. The input is not changed.
func Effective(schema map[string]any, supplied map[string]any) (map[string]any, error) {
	if supplied == nil {
		supplied = map[string]any{}
	}
	value, err := checkValue(schema, deepCopy(supplied), "")
	if err != nil {
		return nil, err
	}
	out, _ := value.(map[string]any)
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > MaxInputBytes {
		return nil, &InputError{}
	}
	return out, nil
}

// checkValue validates value (already present) against node and fills
// defaults of absent nested properties.
func checkValue(node map[string]any, value any, at string) (any, error) {
	fail := func() (any, error) { return nil, &InputError{Path: at} }
	typ, _ := node["type"].(string)
	switch typ {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fail()
		}
		length := float64(utf8.RuneCountInString(text))
		if min, ok := node["minLength"].(float64); ok && length < min {
			return fail()
		}
		if max, ok := node["maxLength"].(float64); ok && length > max {
			return fail()
		}
		if pattern, ok := node["pattern"].(string); ok {
			re, err := regexp.Compile(pattern)
			if err != nil || !re.MatchString(text) {
				return fail()
			}
		}
		if strings.Contains(text, "${") {
			// Literal inputs never become resource/context references.
			return fail()
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fail()
		}
	case "integer", "number":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || (typ == "integer" && n != math.Trunc(n)) {
			return fail()
		}
		if min, ok := node["minimum"].(float64); ok && n < min {
			return fail()
		}
		if max, ok := node["maximum"].(float64); ok && n > max {
			return fail()
		}
	case "array":
		list, ok := value.([]any)
		if !ok {
			return fail()
		}
		if min, ok := node["minItems"].(float64); ok && float64(len(list)) < min {
			return fail()
		}
		if max, ok := node["maxItems"].(float64); ok && float64(len(list)) > max {
			return fail()
		}
		items, _ := node["items"].(map[string]any)
		out := make([]any, len(list))
		for i, entry := range list {
			checked, err := checkValue(items, entry, fmt.Sprintf("%s[%d]", at, i))
			if err != nil {
				return nil, err
			}
			out[i] = checked
		}
		value = out
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fail()
		}
		properties, _ := node["properties"].(map[string]any)
		for key := range object {
			if _, declared := properties[key]; !declared {
				// The unknown key is caller data and is never echoed.
				return fail()
			}
		}
		out := map[string]any{}
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child, _ := properties[name].(map[string]any)
			path := name
			if at != "" {
				path = at + "." + name
			}
			entry, present := object[name]
			if !present {
				if def, hasDefault := child["default"]; hasDefault {
					entry, present = deepCopy(def), true
				}
			}
			if !present {
				continue
			}
			checked, err := checkValue(child, entry, path)
			if err != nil {
				return nil, err
			}
			out[name] = checked
		}
		if required, ok := node["required"].([]any); ok {
			for _, entry := range required {
				name, _ := entry.(string)
				if _, present := out[name]; !present {
					path := name
					if at != "" {
						path = at + "." + name
					}
					return nil, &InputError{Path: path}
				}
			}
		}
		value = out
	default:
		return fail()
	}
	if enum, ok := node["enum"].([]any); ok {
		found := false
		for _, candidate := range enum {
			a, _ := canonical(candidate)
			b, _ := canonical(value)
			if a == b {
				found = true
				break
			}
		}
		if !found {
			return fail()
		}
	}
	return value, nil
}

// InputsHash is the canonical SHA-256 of effective inputs pinned in a plan.
func InputsHash(inputs map[string]any) (string, error) {
	if inputs == nil {
		inputs = map[string]any{}
	}
	raw, err := json.Marshal(inputs)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func canonical(v any) (string, error) {
	raw, err := json.Marshal(v)
	return string(raw), err
}

// deepCopy copies a JSON document tree (maps, arrays and scalars).
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

// DeepCopy is the exported JSON tree copy used by adapters.
func DeepCopy(v any) any { return deepCopy(v) }
