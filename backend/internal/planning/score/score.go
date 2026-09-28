// Package score converts a Score document into a Deployment Set workload
// fragment. The API contract boundary accepts JSON, which is a subset of YAML.
package score

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
)

// SupportedAPIVersion is the only Score API version accepted by the MVP.
const SupportedAPIVersion = "score.dev/v1b1"

var publicPathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9._~-]+(?:/[A-Za-z0-9._~-]+)*)?$`)

// Metadata names the single workload described by a Score document.
type Metadata struct {
	Name string `json:"name"`
}

// ResourceSpec is one resource dependency declared by a Score workload. A
// non-empty ID makes the resource shared; otherwise it is private.
type ResourceSpec struct {
	Type   string         `json:"type"`
	Class  string         `json:"class,omitempty"`
	ID     string         `json:"id,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// Document is a Score document describing exactly one workload.
type Document struct {
	APIVersion string                           `json:"apiVersion"`
	Metadata   Metadata                         `json:"metadata"`
	Containers map[string]environment.Container `json:"containers"`
	Service    *environment.Service             `json:"service,omitempty"`
	Resources  map[string]ResourceSpec          `json:"resources,omitempty"`
	Replicas   int                              `json:"replicas,omitempty"`
}

// Fragment is the Deployment Set contribution of one Score document.
type Fragment struct {
	WorkloadID string
	Module     environment.Module
	Shared     map[string]environment.ResourceEntry
}

// Parse reads a Score document from JSON bytes.
func Parse(b []byte) (*Document, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("score: invalid document: %w", err)
	}
	if err := validateContainerResources(b); err != nil {
		return nil, err
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// FromMap reads a Score document from a decoded JSON tree.
func FromMap(m map[string]any) (*Document, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("score: marshal: %w", err)
	}
	return Parse(b)
}

// Validate enforces the structural rules of UC-06 PRE-05.
func (d Document) Validate() error {
	if d.APIVersion != SupportedAPIVersion {
		return fmt.Errorf("score: unsupported apiVersion %q, want %q", d.APIVersion, SupportedAPIVersion)
	}
	if d.Metadata.Name == "" {
		return fmt.Errorf("score: metadata.name is required")
	}
	if len(d.Containers) == 0 {
		return fmt.Errorf("score: workload %q declares no container", d.Metadata.Name)
	}
	for _, name := range sortedKeys(d.Containers) {
		if d.Containers[name].Image == "" {
			return fmt.Errorf("score: container %q has no image", name)
		}
	}
	for _, alias := range d.ResourceAliases() {
		if d.Resources[alias].Type == "" {
			return fmt.Errorf("score: resource %q has no type", alias)
		}
	}
	if d.Service != nil {
		seen := map[string]bool{}
		for _, route := range d.Service.Routes() {
			if !publicPathPattern.MatchString(route.Path) || seen[route.Path] {
				return fmt.Errorf("score: invalid or duplicate public path %q", route.Path)
			}
			seen[route.Path] = true
			if _, ok := d.Service.Ports[route.Port]; !ok {
				return fmt.Errorf("score: public Service port %q is not declared", route.Port)
			}
		}
	}
	return nil
}

// ResourceAliases lists declared resource aliases in deterministic order.
func (d Document) ResourceAliases() []string { return sortedKeys(d.Resources) }

// binding is the Deployment Set placeholder prefix a Score resource maps to.
func (r ResourceSpec) binding(alias string) string {
	if r.ID != "" {
		return placeholder.ScopeShared + "." + r.ID
	}
	return placeholder.ScopeExternals + "." + alias
}

func (r ResourceSpec) class() string {
	if r.Class == "" {
		return "default"
	}
	return r.Class
}

// Fragment converts the Score document into its Deployment Set contribution.
// Score `params` are validated against the Resource Type input contract and
// `${resources.<alias>[.<output>]}` placeholders are rewritten onto the
// Deployment Set form (UC-06 MS-02, BR-07).
func (d Document) Fragment(types map[string]resource.Type) (*Fragment, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	for _, alias := range d.ResourceAliases() {
		spec := d.Resources[alias]
		if spec.Type == "environment" || spec.Type == "service" {
			if err := validateVirtualResource(alias, spec); err != nil {
				return nil, err
			}
			continue
		}
		typ, ok := types[spec.Type]
		if !ok {
			return nil, fmt.Errorf("score: resource %q uses unregistered resource type %q", alias, spec.Type)
		}
		if err := typ.ValidateParams(spec.Params); err != nil {
			return nil, fmt.Errorf("score: resource %q params: %w", alias, err)
		}
	}

	module := environment.Module{
		Profile: environment.ModuleProfile,
		Spec: environment.ModuleSpec{
			Containers: map[string]environment.Container{},
			Service:    d.Service,
		},
	}
	if d.Replicas > 0 {
		replicas := d.Replicas
		module.Spec.Replicas = &replicas
	}
	for _, name := range sortedKeys(d.Containers) {
		container := d.Containers[name]
		container.ID = name
		container.Resources = container.Resources.Clone()
		if len(container.Variables) > 0 {
			rewritten := make(map[string]string, len(container.Variables))
			for _, key := range sortedKeys(container.Variables) {
				value, err := d.rewrite(container.Variables[key], types, fmt.Sprintf("container %s variable %s", name, key))
				if err != nil {
					return nil, err
				}
				rewritten[key] = value
			}
			container.Variables = rewritten
		}
		module.Spec.Containers[name] = container
	}

	shared := map[string]environment.ResourceEntry{}
	for _, alias := range d.ResourceAliases() {
		spec := d.Resources[alias]
		if spec.Type == "environment" || spec.Type == "service" {
			continue
		}
		entry := environment.ResourceEntry{Type: spec.Type, Class: spec.class()}
		if len(spec.Params) > 0 {
			params, err := d.rewriteTree(spec.Params, types, fmt.Sprintf("resource %s params", alias))
			if err != nil {
				return nil, err
			}
			entry.Params, _ = params.(map[string]any)
		}
		if spec.ID != "" {
			if existing, ok := shared[spec.ID]; ok && !reflect.DeepEqual(existing, entry) {
				return nil, fmt.Errorf("score: shared resource %q is declared twice with different content", spec.ID)
			}
			shared[spec.ID] = entry
			continue
		}
		if module.Externals == nil {
			module.Externals = map[string]environment.ResourceEntry{}
		}
		module.Externals[alias] = entry
	}

	return &Fragment{WorkloadID: d.Metadata.Name, Module: module, Shared: shared}, nil
}

// rewrite maps `${resources.<alias>[.<output>]}` onto the Deployment Set form.
func (d Document) rewrite(value string, types map[string]resource.Type, where string) (string, error) {
	matches := placeholder.Pattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(value[last:m[0]])
		raw := value[m[0]:m[1]]
		body := value[m[2]:m[3]]
		if strings.HasPrefix(raw, "$$") {
			b.WriteString(raw)
			last = m[1]
			continue
		}
		replacement, err := d.rewriteOne(body, types, where)
		if err != nil {
			return "", err
		}
		b.WriteString(replacement)
		last = m[1]
	}
	b.WriteString(value[last:])
	return b.String(), nil
}

func (d Document) rewriteOne(body string, types map[string]resource.Type, where string) (string, error) {
	segments := strings.Split(strings.TrimSpace(body), ".")
	if segments[0] != "resources" {
		return "${" + body + "}", nil
	}
	if len(segments) < 2 || segments[1] == "" {
		return "", fmt.Errorf("score: %s references a resource without a name", where)
	}
	if len(segments) > 3 {
		return "", fmt.Errorf("score: %s has more segments than resources.<name>.<output>", where)
	}
	alias := segments[1]
	spec, ok := d.Resources[alias]
	if !ok {
		return "", fmt.Errorf("score: %s references undeclared resource %q", where, alias)
	}
	out := spec.binding(alias)
	if len(segments) == 3 {
		output := segments[2]
		if output == "" {
			return "", fmt.Errorf("score: %s references an empty output", where)
		}
		if spec.Type == "environment" {
			if alias != "env" {
				return "", fmt.Errorf("score: Application keys require resources.env")
			}
			return "${context.uc12." + output + "}", nil
		}
		if spec.Type == "service" {
			if output != "url" {
				return "", fmt.Errorf("score: service output must be url")
			}
			workload, _ := spec.Params["workload"].(string)
			port, _ := spec.Params["port"].(string)
			return "${context.service." + workload + "." + port + "}", nil
		}
		typ, ok := types[spec.Type]
		if !ok {
			return "", fmt.Errorf("score: resource type %q is not registered", spec.Type)
		}
		if _, ok := typ.Output(output); !ok {
			return "", fmt.Errorf("score: %s binds output %q which is not in the %q output contract", where, output, typ.Key)
		}
		out += "." + output
	}
	return "${" + out + "}", nil
}

func validateVirtualResource(alias string, spec ResourceSpec) error {
	if spec.ID != "" || spec.Class != "" {
		return fmt.Errorf("score: virtual resource %q cannot declare id or class", alias)
	}
	if spec.Type == "environment" {
		if alias != "env" || len(spec.Params) != 0 {
			return fmt.Errorf("score: Application keys require resources.env without params")
		}
		return nil
	}
	workload, wok := spec.Params["workload"].(string)
	port, pok := spec.Params["port"].(string)
	if !wok || !pok || workload == "" || port == "" || len(spec.Params) != 2 {
		return fmt.Errorf("score: service %q requires workload and port", alias)
	}
	return nil
}

func (d Document) rewriteTree(v any, types map[string]resource.Type, where string) (any, error) {
	switch t := v.(type) {
	case string:
		return d.rewrite(t, types, where)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, value := range t {
			rewritten, err := d.rewriteTree(value, types, where)
			if err != nil {
				return nil, err
			}
			out[k] = rewritten
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			rewritten, err := d.rewriteTree(value, types, where)
			if err != nil {
				return nil, err
			}
			out[i] = rewritten
		}
		return out, nil
	default:
		return v, nil
	}
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
