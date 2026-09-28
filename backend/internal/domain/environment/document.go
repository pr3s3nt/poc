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
	Ports        map[string]Port `json:"ports"`
	PublicPort   string          `json:"publicPort,omitempty"`
	PublicRoutes []PublicPath    `json:"publicRoutes,omitempty"`
}

// PublicPath maps a URL prefix on the Environment host to a named Service port.
type PublicPath struct {
	Path string `json:"path"`
	Port string `json:"port"`
}

// Routes includes the legacy publicPort alias without changing stored documents.
func (s *Service) Routes() []PublicPath {
	if s == nil {
		return nil
	}
	routes := append([]PublicPath(nil), s.PublicRoutes...)
	if s.PublicPort != "" {
		routes = append(routes, PublicPath{Path: "/", Port: s.PublicPort})
	}
	return routes
}

// Canonical returns an equivalent Service shape for no-op comparisons.
// Kubernetes defaults an omitted targetPort to port and protocol to TCP.
func (s *Service) Canonical() *Service {
	if s == nil {
		return nil
	}
	out := *s
	out.PublicRoutes = s.Routes()
	out.PublicPort = ""
	sort.Slice(out.PublicRoutes, func(i, j int) bool { return out.PublicRoutes[i].Path < out.PublicRoutes[j].Path })
	out.Ports = make(map[string]Port, len(s.Ports))
	for name, port := range s.Ports {
		if port.TargetPort == 0 {
			port.TargetPort = port.Port
		}
		if port.Protocol == "" {
			port.Protocol = "TCP"
		}
		out.Ports[name] = port
	}
	return &out
}

// Probe is an HTTP readiness or liveness probe path.
type Probe struct {
	Path string `json:"path"`
	Port int    `json:"port"`
}

// ComputeResources is one `requests` or `limits` branch of a container. Values
// are the Score strings exactly as declared; an empty field is not declared.
type ComputeResources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// ContainerResourceRequirements holds the optional Score
// `containers.*.resources` of one container. It belongs to the workload module
// and is never a Resource Graph node (UC-05 BR-07, UC-06 BR-11).
type ContainerResourceRequirements struct {
	Requests *ComputeResources `json:"requests,omitempty"`
	Limits   *ComputeResources `json:"limits,omitempty"`
}

// Clone returns an independent copy, so a module never aliases the Score it was
// converted from.
func (r *ContainerResourceRequirements) Clone() *ContainerResourceRequirements {
	if r == nil {
		return nil
	}
	out := &ContainerResourceRequirements{}
	if r.Requests != nil {
		requests := *r.Requests
		out.Requests = &requests
	}
	if r.Limits != nil {
		limits := *r.Limits
		out.Limits = &limits
	}
	return out
}

// Container is one container of a workload module. The id repeats the map key,
// which is the Humanitec module form.
type Container struct {
	ID             string                         `json:"id,omitempty"`
	Image          string                         `json:"image"`
	Command        []string                       `json:"command,omitempty"`
	Args           []string                       `json:"args,omitempty"`
	Variables      map[string]string              `json:"variables,omitempty"`
	Resources      *ContainerResourceRequirements `json:"resources,omitempty"`
	LivenessProbe  *Probe                         `json:"livenessProbe,omitempty"`
	ReadinessProbe *Probe                         `json:"readinessProbe,omitempty"`
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
