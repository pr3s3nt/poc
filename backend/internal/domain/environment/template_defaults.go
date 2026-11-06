package environment

import "encoding/json"

// Values in these structs are supplied exclusively by the pinned template.
type RuntimeExecutionDefaults struct {
	PodSecurityContext           *PodSecurityContext  `json:"podSecurityContext,omitempty" yaml:"podSecurityContext,omitempty"`
	AutomountServiceAccountToken *bool                `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`
	UpdateStrategy               *UpdateStrategy      `json:"updateStrategy,omitempty" yaml:"updateStrategy,omitempty"`
	PersistentUpdateStrategy     *UpdateStrategy      `json:"persistentUpdateStrategy,omitempty" yaml:"persistentUpdateStrategy,omitempty"`
	RevisionHistoryLimit         *int32               `json:"revisionHistoryLimit,omitempty" yaml:"revisionHistoryLimit,omitempty"`
	Schedule                     *Schedule            `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	PodDisruptionBudget          *PodDisruptionBudget `json:"podDisruptionBudget,omitempty" yaml:"podDisruptionBudget,omitempty"`
	ClaimRetention               *ClaimRetention      `json:"persistentVolumeClaimRetentionPolicy,omitempty" yaml:"persistentVolumeClaimRetentionPolicy,omitempty"`
}

type RuntimeDefaults struct {
	Image                             string                         `json:"image,omitempty" yaml:"image,omitempty"`
	Ports                             []ContainerPort                `json:"ports,omitempty" yaml:"ports,omitempty"`
	ReadinessProbe                    *Probe                         `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	LivenessProbe                     *Probe                         `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty"`
	StartupProbe                      *Probe                         `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty"`
	Service                           *Service                       `json:"service,omitempty" yaml:"service,omitempty"`
	Volumes                           []PodVolume                    `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	VolumeMounts                      []VolumeMount                  `json:"volumeMounts,omitempty" yaml:"volumeMounts,omitempty"`
	Resources                         *ContainerResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	ContainerSecurityContext          *ContainerSecurityContext      `json:"containerSecurityContext,omitempty" yaml:"containerSecurityContext,omitempty"`
	AuxiliaryContainerSecurityContext *ContainerSecurityContext      `json:"auxiliaryContainerSecurityContext,omitempty" yaml:"auxiliaryContainerSecurityContext,omitempty"`
	Replicas                          *int                           `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	MultiReplicaSafe                  bool                           `json:"multiReplicaSafe,omitempty" yaml:"multiReplicaSafe,omitempty"`
	Execution                         *RuntimeExecutionDefaults      `json:"execution,omitempty" yaml:"execution,omitempty"`
	ProbeTimings                      map[string]*Probe              `json:"probeTimings,omitempty" yaml:"probeTimings,omitempty"`
	Environments                      map[string]*RuntimeDefaults    `json:"environments,omitempty" yaml:"environments,omitempty"`
}

func defaultsTree(value any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// mergeDefaultTree preserves every existing leaf, including false and zero.
func mergeDefaultTree(dst, src map[string]any) {
	for k, v := range src {
		if old, exists := dst[k]; exists {
			a, aok := old.(map[string]any)
			b, bok := v.(map[string]any)
			if aok && bok {
				mergeDefaultTree(a, b)
			}
		} else {
			dst[k] = v
		}
	}
}
func (p *RuntimeDefaults) ForEnvironment(name string) *RuntimeDefaults {
	if p == nil {
		return nil
	}
	base := defaultsTree(p)
	delete(base, "environments")
	if variant := p.Environments[name]; variant != nil {
		overlay := defaultsTree(variant)
		delete(overlay, "environments")
		mergeDefaultTree(overlay, base)
		base = overlay
	}
	raw, _ := json.Marshal(base)
	var out RuntimeDefaults
	_ = json.Unmarshal(raw, &out)
	return &out
}

// ResolveDefaults only merges a verified template profile. No platform or
// environment default values are supplied by this backend.
func (m ModuleSpec) ResolveDefaults(profiles ...*RuntimeDefaults) (ModuleSpec, map[string]string) {
	tree := defaultsTree(m)
	sources := map[string]string{}
	var mark func(any, string, string)
	mark = func(value any, path, source string) {
		if fields, ok := value.(map[string]any); ok {
			for k, v := range fields {
				next := k
				if path != "" {
					next = path + "." + k
				}
				mark(v, next, source)
			}
		} else {
			sources[path] = source
		}
	}
	mark(tree, "", "workload")
	finish := func() (ModuleSpec, map[string]string) {
		raw, _ := json.Marshal(tree)
		var out ModuleSpec
		_ = json.Unmarshal(raw, &out)
		return out, sources
	}
	if m.Execution == nil || !m.Execution.HasTemplate() {
		return finish()
	}
	var profile *RuntimeDefaults
	if len(profiles) > 0 {
		profile = profiles[0]
	} else if m.Execution.TemplateDefaults != nil {
		profile = m.Execution.TemplateDefaults.Profile
	}
	if profile == nil {
		return finish()
	}
	p := defaultsTree(profile)
	delete(p, "environments")
	execution := tree["workloadExecution"].(map[string]any)
	var merge func(map[string]any, map[string]any, string)
	merge = func(dst, src map[string]any, path string) {
		for k, v := range src {
			next := k
			if path != "" {
				next = path + "." + k
			}
			if old, exists := dst[k]; exists {
				a, aok := old.(map[string]any)
				b, bok := v.(map[string]any)
				if aok && bok {
					merge(a, b, next)
				}
			} else {
				dst[k] = v
				mark(v, next, "template")
			}
		}
	}
	for name, raw := range tree["containers"].(map[string]any) {
		c := raw.(map[string]any)
		fields := map[string]any{}
		app := c["role"] == nil || c["role"] == "app"
		if app {
			for _, key := range []string{"image", "ports", "resources", "readinessProbe", "livenessProbe", "startupProbe"} {
				if v, ok := p[key]; ok {
					fields[key] = v
				}
			}
			if v, ok := p["containerSecurityContext"]; ok {
				fields["securityContext"] = v
			}
		} else if v, ok := p["auxiliaryContainerSecurityContext"]; ok {
			fields["securityContext"] = v
		}
		// JSON cloning avoids aliasing defaults between containers or into desired.
		merge(c, defaultsTree(fields), "containers."+name)
		if timings, ok := p["probeTimings"].(map[string]any); ok {
			for key, raw := range timings {
				probe, enabled := c[key].(map[string]any)
				timing, valid := raw.(map[string]any)
				if enabled && valid {
					merge(probe, defaultsTree(timing), "containers."+name+"."+key)
				}
			}
		}
	}
	if m.Execution.ServiceEnabled == nil || *m.Execution.ServiceEnabled {
		if service, ok := p["service"]; ok {
			merge(tree, map[string]any{"service": service}, "")
		}
	}
	for _, key := range []string{"volumes", "volumeMounts"} {
		if v, ok := p[key]; ok {
			merge(execution, map[string]any{key: v}, "workloadExecution")
		}
	}
	if replicas, ok := p["replicas"]; ok {
		merge(tree, map[string]any{"replicas": replicas}, "")
	}
	if fields, ok := p["execution"].(map[string]any); ok {
		wanted := defaultsTree(fields)
		delete(wanted, "persistentUpdateStrategy")
		delete(wanted, "podDisruptionBudget")
		delete(wanted, "persistentVolumeClaimRetentionPolicy")
		// Schedule defaults never enable a scheduled workload or invent its expression.
		if m.Execution.Schedule == nil {
			delete(wanted, "schedule")
		}
		if len(m.Execution.Mounts) > 0 {
			if strategy, ok := fields["persistentUpdateStrategy"]; ok {
				wanted["updateStrategy"] = strategy
			}
		}
		merge(execution, wanted, "workloadExecution")
		if m.Execution.PodDisruptionBudget != nil && m.Execution.PodDisruptionBudget.Enabled {
			if raw, ok := fields["podDisruptionBudget"].(map[string]any); ok {
				budget := defaultsTree(raw)
				delete(budget, "enabled")
				if m.Execution.PodDisruptionBudget.MinAvailable != nil {
					delete(budget, "maxUnavailable")
				} else if m.Execution.PodDisruptionBudget.MaxUnavailable != nil {
					delete(budget, "minAvailable")
				}
				merge(execution["podDisruptionBudget"].(map[string]any), budget, "workloadExecution.podDisruptionBudget")
			}
		}
		if len(m.Execution.VolumeClaimTemplates) > 0 {
			if retention, ok := fields["persistentVolumeClaimRetentionPolicy"]; ok {
				merge(execution, map[string]any{"persistentVolumeClaimRetentionPolicy": retention}, "workloadExecution")
			}
		}
	}
	return finish()
}
func (m ModuleSpec) EffectiveService() *Service {
	effective, _ := m.ResolveDefaults()
	return effective.Service
}
