// Package workloadspec converts a Generic Workload document into a Deployment Set workload
// fragment. The API contract boundary accepts JSON, which is a subset of YAML.
package workloadspec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
)

// SupportedAPIVersion is the only Workload API version accepted by the MVP.
const SupportedAPIVersion = "orchestrator.dev/workload/v1"

var bindingPatternForFile = regexp.MustCompile(`^\$\{resources\.([A-Za-z0-9_-]+)\.([A-Za-z_][A-Za-z0-9_-]*)\}$`)

var publicPathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9._~-]+(?:/[A-Za-z0-9._~-]+)*)?$`)

// Metadata names the single workload described by a Workload document.
type Metadata struct {
	Name string `json:"name"`
	// Class is the optional workload class (product extension, ADR-016). A
	// missing class means the default class; an explicit value is kept as
	// written through import, edit, export and storage.
	Class string `json:"class,omitempty"`
	// Annotations are Workload-native per-workload extension values (ADR-024),
	// handed to Git render bundles as input; never copied into manifests.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ResourceSpec is one resource dependency declared by a Workload workload. A
// non-empty ID makes the resource shared; otherwise it is private.
type ResourceSpec struct {
	Type   string         `json:"type"`
	Class  string         `json:"class,omitempty"`
	ID     string         `json:"id,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// Document is a Workload document describing exactly one workload.
type Document struct {
	replicaSet bool
	Execution  *environment.WorkloadExecution   `json:"workloadExecution,omitempty"`
	APIVersion string                           `json:"apiVersion"`
	Metadata   Metadata                         `json:"metadata"`
	Containers map[string]environment.Container `json:"containers"`
	Service    *environment.Service             `json:"service,omitempty"`
	Resources  map[string]ResourceSpec          `json:"resources,omitempty"`
	Replicas   int                              `json:"replicas,omitempty"`
}

// Fragment is the Deployment Set contribution of one Workload document.
type Fragment struct {
	WorkloadID string
	Module     environment.Module
	Shared     map[string]environment.ResourceEntry
}

// ErrInvalid classifies every failure to read or validate a caller-supplied
// Workload document, so delivery needs no message parsing.
var ErrInvalid = errors.New("workload: invalid document")

type invalidError struct{ err error }

func (e *invalidError) Error() string        { return e.err.Error() }
func (e *invalidError) Unwrap() error        { return e.err }
func (e *invalidError) Is(target error) bool { return target == ErrInvalid }

// Parse reads a Workload document from JSON bytes. Failures match ErrInvalid.
func Parse(b []byte) (*Document, error) {
	doc, err := parse(b)
	if err != nil {
		return nil, &invalidError{err: err}
	}
	return doc, nil
}

func parse(b []byte) (*Document, error) {
	b, replicaSet, err := normalizeDocument(b)
	if err != nil {
		return nil, err
	}
	if err := rejectPublicRoutes(b); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("workload: invalid document: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("workload: trailing input")
	}
	doc.replicaSet = replicaSet
	if err := validateContainerResources(b); err != nil {
		return nil, err
	}
	if err := validateMetadataClass(b); err != nil {
		return nil, err
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// normalizeDocument admits the simple application form and expands it to the same
// multi-container contract used by authoring, planning and persisted snapshots.
func normalizeDocument(b []byte) ([]byte, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return nil, false, fmt.Errorf("workload: expected an object")
	}
	_, replicaSet := raw["replicas"]
	if replicaSet && string(raw["replicas"]) == "null" {
		return nil, false, fmt.Errorf("workload: replicas must be an integer")
	}
	if name, ok := raw["name"]; ok {
		if _, exists := raw["metadata"]; exists {
			return nil, false, fmt.Errorf("workload: declare name once")
		}
		metadata := map[string]json.RawMessage{"name": name}
		for _, field := range []string{"class", "annotations"} {
			if v, ok := raw[field]; ok {
				metadata[field] = v
				delete(raw, field)
			}
		}
		raw["metadata"], _ = json.Marshal(metadata)
		delete(raw, "name")
		if _, ok := raw["apiVersion"]; !ok {
			raw["apiVersion"], _ = json.Marshal(SupportedAPIVersion)
		}
	}
	if image, ok := raw["image"]; ok {
		if _, exists := raw["containers"]; exists {
			return nil, false, fmt.Errorf("workload: image shorthand and containers are exclusive")
		}
		container := map[string]json.RawMessage{"image": image}
		if env, ok := raw["env"]; ok {
			container["variables"] = env
			delete(raw, "env")
		}
		raw["containers"], _ = json.Marshal(map[string]any{"main": container})
		delete(raw, "image")
	}
	if ports, ok := raw["ports"]; ok {
		var list []struct {
			Name          string `json:"name"`
			ContainerPort int    `json:"containerPort"`
		}
		dec := json.NewDecoder(bytes.NewReader(ports))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&list); err != nil || len(list) == 0 {
			return nil, false, fmt.Errorf("workload: invalid ports")
		}
		var service map[string]json.RawMessage
		if v, ok := raw["service"]; ok {
			if json.Unmarshal(v, &service) != nil || service == nil {
				return nil, false, fmt.Errorf("workload: invalid service")
			}
		} else {
			service = map[string]json.RawMessage{}
		}
		enabled := false
		if v, ok := service["enabled"]; ok {
			if json.Unmarshal(v, &enabled) != nil {
				return nil, false, fmt.Errorf("workload: invalid service.enabled")
			}
		}
		servicePort := 0
		if v, ok := service["port"]; ok {
			if json.Unmarshal(v, &servicePort) != nil {
				return nil, false, fmt.Errorf("workload: invalid service.port")
			}
		}
		if len(service) > 2 {
			return nil, false, fmt.Errorf("workload: unsupported shorthand service field")
		}
		mapped := map[string]environment.Port{}
		for _, p := range list {
			if p.Name == "" || p.ContainerPort < 1 || p.ContainerPort > 65535 {
				return nil, false, fmt.Errorf("workload: invalid port")
			}
			if _, exists := mapped[p.Name]; exists {
				return nil, false, fmt.Errorf("workload: duplicate port")
			}
			exposed := p.ContainerPort
			if servicePort != 0 {
				if len(list) != 1 || servicePort < 1 || servicePort > 65535 {
					return nil, false, fmt.Errorf("workload: ambiguous service port")
				}
				exposed = servicePort
			}
			mapped[p.Name] = environment.Port{Port: exposed, TargetPort: p.ContainerPort}
		}
		if enabled {
			raw["service"], _ = json.Marshal(environment.Service{Ports: mapped})
		} else {
			delete(raw, "service")
		}
		var containers map[string]map[string]json.RawMessage
		if json.Unmarshal(raw["containers"], &containers) != nil || len(containers) != 1 {
			return nil, false, fmt.Errorf("workload: ports shorthand requires one container")
		}
		for _, c := range containers {
			c["ports"] = ports
		}
		raw["containers"], _ = json.Marshal(containers)
		delete(raw, "ports")
	}
	encoded, err := json.Marshal(raw)
	return encoded, replicaSet, err
}

// FromMap reads a Workload document from a decoded JSON tree.
func FromMap(m map[string]any) (*Document, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, &invalidError{err: fmt.Errorf("workload: marshal: %w", err)}
	}
	return Parse(b)
}

// Validate enforces the structural rules of UC-06 PRE-05.
func (d Document) Validate() error {
	if d.Replicas < 0 {
		return fmt.Errorf("workload: replicas must be nonnegative")
	}
	if d.APIVersion != SupportedAPIVersion {
		return fmt.Errorf("workload: unsupported apiVersion %q, want %q", d.APIVersion, SupportedAPIVersion)
	}
	if d.Metadata.Name == "" {
		return fmt.Errorf("workload: metadata.name is required")
	}
	if err := environment.ValidateContainerRoles(d.Containers); err != nil {
		return err
	}
	if len(d.Containers) == 0 {
		return fmt.Errorf("workload: workload %q declares no container", d.Metadata.Name)
	}
	if err := environment.ValidateAnnotations(d.Metadata.Annotations); err != nil {
		return fmt.Errorf("workload: metadata.annotations: %w", err)
	}
	for _, name := range sortedKeys(d.Containers) {
		seenPorts := map[string]bool{}
		for _, port := range d.Containers[name].Ports {
			if port.Name == "" || seenPorts[port.Name] || (port.Protocol != "" && port.Protocol != "TCP" && port.Protocol != "UDP" && port.Protocol != "SCTP") || port.ContainerPort < 1 || port.ContainerPort > 65535 {
				return fmt.Errorf("workload: invalid or duplicate container port")
			}
			seenPorts[port.Name] = true
		}
		for _, probe := range []struct {
			value  *environment.Probe
			single bool
		}{{d.Containers[name].ReadinessProbe, false}, {d.Containers[name].LivenessProbe, true}, {d.Containers[name].StartupProbe, true}} {
			if err := probe.value.Validate(probe.single); err != nil {
				return err
			}
		}
		if d.Containers[name].Image == "" {
			return fmt.Errorf("workload: container %q has no image", name)
		}
	}
	for _, alias := range d.ResourceAliases() {
		if d.Resources[alias].Type == "" {
			return fmt.Errorf("workload: resource %q has no type", alias)
		}
	}
	resources := map[string]bool{}
	for alias := range d.Resources {
		resources[alias] = true
	}
	replicas := d.Replicas
	if replicas == 0 && !d.replicaSet {
		replicas = 1
	}
	if err := d.Execution.Validate(d.Containers, replicas, d.Service != nil && len(d.Service.Ports) > 0, resources); err != nil {
		return err
	}
	if d.Execution != nil {
		for _, file := range d.Execution.SecretFiles {
			match := bindingPatternForFile.FindStringSubmatch(file.Ref)
			if match == nil || d.Resources[match[1]].Type != "environment" {
				return environment.ErrExecution
			}
		}
	}
	return nil
}

// rejectPublicRoutes refuses the retired service.publicRoutes of a new
// Workload or draft (ADR-037); historical Deployment Sets are read elsewhere.
func rejectPublicRoutes(b []byte) error {
	var raw struct {
		Service map[string]json.RawMessage `json:"service"`
	}
	if json.Unmarshal(b, &raw) == nil {
		_, port := raw.Service["publicPort"]
		if _, present := raw.Service["publicRoutes"]; present && !port {
			return environment.ErrPublicRoutesRemoved // publicPort keeps its own error
		}
	}
	return nil
}

// ResourceAliases lists declared resource aliases in deterministic order.
func (d Document) ResourceAliases() []string { return sortedKeys(d.Resources) }

// binding is the Deployment Set placeholder prefix a Workload resource maps to.
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

// Fragment converts the Workload document into its Deployment Set contribution.
// Workload `params` are validated against the Resource Type input contract and
// `${resources.<alias>[.<output>]}` placeholders are rewritten onto the
// Deployment Set form (UC-06 MS-02, BR-07).
func (d Document) Fragment(types map[string]resource.Type) (*Fragment, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	for _, alias := range d.ResourceAliases() {
		spec := d.Resources[alias]
		if spec.Type == "workload" {
			return nil, fmt.Errorf("workload: workload is a rendering type and cannot be declared as a dependency")
		}
		if spec.Type == "environment" || spec.Type == "service" {
			if err := validateVirtualResource(alias, spec); err != nil {
				return nil, err
			}
			continue
		}
		typ, ok := types[spec.Type]
		if !ok {
			return nil, fmt.Errorf("workload: resource %q uses unregistered resource type %q", alias, spec.Type)
		}
		if err := typ.ValidateParams(spec.Params); err != nil {
			return nil, fmt.Errorf("workload: resource %q params: %w", alias, err)
		}
	}

	module := environment.Module{
		Profile:     environment.ModuleProfile,
		Class:       d.Metadata.Class,
		Annotations: cloneAnnotations(d.Metadata.Annotations),
		Spec: environment.ModuleSpec{
			Execution:  d.Execution.Clone(),
			Containers: map[string]environment.Container{},
			Service:    d.Service,
		},
	}
	if module.Spec.Execution != nil {
		for i, file := range module.Spec.Execution.SecretFiles {
			ref, err := d.rewrite(file.Ref, types, "secret file")
			if err != nil {
				return nil, err
			}
			module.Spec.Execution.SecretFiles[i].Ref = ref
		}
		for i, mount := range module.Spec.Execution.Mounts {
			spec := d.Resources[mount.Resource]
			module.Spec.Execution.Mounts[i].Resource = spec.binding(mount.Resource)
		}
	}
	if d.Replicas > 0 || d.replicaSet {
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
				return nil, fmt.Errorf("workload: shared resource %q is declared twice with different content", spec.ID)
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
		return "", fmt.Errorf("workload: %s references a resource without a name", where)
	}
	if len(segments) > 3 {
		return "", fmt.Errorf("workload: %s has more segments than resources.<name>.<output>", where)
	}
	alias := segments[1]
	spec, ok := d.Resources[alias]
	if !ok {
		return "", fmt.Errorf("workload: %s references undeclared resource %q", where, alias)
	}
	out := spec.binding(alias)
	if len(segments) == 3 {
		output := segments[2]
		if output == "" {
			return "", fmt.Errorf("workload: %s references an empty output", where)
		}
		if spec.Type == "environment" {
			if alias != "env" {
				return "", fmt.Errorf("workload: Application keys require resources.env")
			}
			return "${context.uc12." + output + "}", nil
		}
		if spec.Type == "service" {
			if output != "url" {
				return "", fmt.Errorf("workload: service output must be url")
			}
			workload, _ := spec.Params["workload"].(string)
			port, _ := spec.Params["port"].(string)
			return "${context.service." + workload + "." + port + "}", nil
		}
		typ, ok := types[spec.Type]
		if !ok {
			return "", fmt.Errorf("workload: resource type %q is not registered", spec.Type)
		}
		if _, ok := typ.Output(output); !ok {
			return "", fmt.Errorf("workload: %s binds output %q which is not in the %q output contract", where, output, typ.Key)
		}
		out += "." + output
	}
	return "${" + out + "}", nil
}

func validateVirtualResource(alias string, spec ResourceSpec) error {
	if spec.ID != "" || spec.Class != "" {
		return fmt.Errorf("workload: virtual resource %q cannot declare id or class", alias)
	}
	if spec.Type == "environment" {
		if alias != "env" || len(spec.Params) != 0 {
			return fmt.Errorf("workload: Application keys require resources.env without params")
		}
		return nil
	}
	workload, wok := spec.Params["workload"].(string)
	port, pok := spec.Params["port"].(string)
	if !wok || !pok || workload == "" || port == "" || len(spec.Params) != 2 {
		return fmt.Errorf("workload: service %q requires workload and port", alias)
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

func cloneAnnotations(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Map returns the normalized expanded contract, preserving explicit zero replicas.
func (d Document) Map() map[string]any {
	raw, _ := json.Marshal(d)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if d.replicaSet {
		out["replicas"] = d.Replicas
	}
	return out
}
