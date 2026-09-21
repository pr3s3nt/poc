// Package environment holds Environment and immutable Deployment Set state.
package environment

import (
	"fmt"
	"sort"
)

// ModuleProfile is the workload profile every module of the MVP uses.
const ModuleProfile = "humanitec/default-module"

// ResourceEntry is one resource declaration inside a Deployment Set. Private
// entries live under `modules.<id>.externals.<name>`, shared entries under
// `shared.<id>`.
type ResourceEntry struct {
	Type   string         `json:"type"`
	Class  string         `json:"class"`
	Params map[string]any `json:"params,omitempty"`
}

// Port is one Service port of a workload.
type Port struct {
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// Service exposes workload ports inside the cluster.
type Service struct {
	Ports map[string]Port `json:"ports"`
}

// Probe is an HTTP readiness or liveness probe path.
type Probe struct {
	Path string `json:"path"`
	Port int    `json:"port"`
}

// Container is one container of a workload module. The id repeats the map key,
// which is the Humanitec module form.
type Container struct {
	ID             string            `json:"id,omitempty"`
	Image          string            `json:"image"`
	Command        []string          `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Variables      map[string]string `json:"variables,omitempty"`
	LivenessProbe  *Probe            `json:"livenessProbe,omitempty"`
	ReadinessProbe *Probe            `json:"readinessProbe,omitempty"`
}

// ModuleSpec is the workload body of a module.
type ModuleSpec struct {
	Containers map[string]Container `json:"containers"`
	Service    *Service             `json:"service,omitempty"`
	Replicas   *int                 `json:"replicas,omitempty"`
}

// ReplicaCount returns the declared replica count, defaulting to one.
func (m ModuleSpec) ReplicaCount() int {
	if m.Replicas == nil || *m.Replicas <= 0 {
		return 1
	}
	return *m.Replicas
}

// Module is the Deployment Set contribution of one Score document.
type Module struct {
	Profile   string                   `json:"profile"`
	Spec      ModuleSpec               `json:"spec"`
	Externals map[string]ResourceEntry `json:"externals,omitempty"`
}

// Document is the full desired state of an Environment.
type Document struct {
	Modules map[string]Module        `json:"modules"`
	Shared  map[string]ResourceEntry `json:"shared"`
}

// NewDocument returns an empty, non-nil Deployment Set document.
func NewDocument() Document {
	return Document{Modules: map[string]Module{}, Shared: map[string]ResourceEntry{}}
}

// ModuleIDs lists module names in deterministic order.
func (d Document) ModuleIDs() []string { return sortedKeys(d.Modules) }

// SharedIDs lists shared resource IDs in deterministic order.
func (d Document) SharedIDs() []string { return sortedKeys(d.Shared) }

// ExternalNames lists the private dependency names of a module in order.
func (m Module) ExternalNames() []string { return sortedKeys(m.Externals) }

// ContainerNames lists container names in deterministic order.
func (m Module) ContainerNames() []string { return sortedKeys(m.Spec.Containers) }

// Validate reports structural problems in a Deployment Set document.
func (d Document) Validate() error {
	for _, id := range d.ModuleIDs() {
		m := d.Modules[id]
		if m.Profile == "" {
			return fmt.Errorf("environment: module %q has no profile", id)
		}
		if len(m.Spec.Containers) == 0 {
			return fmt.Errorf("environment: module %q has no container", id)
		}
		for _, name := range m.ContainerNames() {
			if m.Spec.Containers[name].Image == "" {
				return fmt.Errorf("environment: module %q container %q has no image", id, name)
			}
		}
		for _, name := range m.ExternalNames() {
			if m.Externals[name].Type == "" {
				return fmt.Errorf("environment: module %q external %q has no type", id, name)
			}
		}
	}
	for _, id := range d.SharedIDs() {
		if d.Shared[id].Type == "" {
			return fmt.Errorf("environment: shared resource %q has no type", id)
		}
	}
	return nil
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
