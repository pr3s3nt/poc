package environment

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// Optional scalar pointers preserve false/zero independently from inheritance.
type SeccompProfile struct {
	Type             string `json:"type" yaml:"type"`
	LocalhostProfile string `json:"localhostProfile,omitempty" yaml:"localhostProfile,omitempty"`
}
type Capabilities struct {
	Add  []string `json:"add,omitempty" yaml:"add,omitempty"`
	Drop []string `json:"drop,omitempty" yaml:"drop,omitempty"`
}
type ContainerSecurityContext struct {
	RunAsNonRoot             *bool           `json:"runAsNonRoot,omitempty" yaml:"runAsNonRoot,omitempty"`
	RunAsUser                *int64          `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	RunAsGroup               *int64          `json:"runAsGroup,omitempty" yaml:"runAsGroup,omitempty"`
	Privileged               *bool           `json:"privileged,omitempty" yaml:"privileged,omitempty"`
	AllowPrivilegeEscalation *bool           `json:"allowPrivilegeEscalation,omitempty" yaml:"allowPrivilegeEscalation,omitempty"`
	ReadOnlyRootFilesystem   *bool           `json:"readOnlyRootFilesystem,omitempty" yaml:"readOnlyRootFilesystem,omitempty"`
	Capabilities             *Capabilities   `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	SeccompProfile           *SeccompProfile `json:"seccompProfile,omitempty" yaml:"seccompProfile,omitempty"`
}
type PodSecurityContext struct {
	RunAsNonRoot        *bool           `json:"runAsNonRoot,omitempty" yaml:"runAsNonRoot,omitempty"`
	RunAsUser           *int64          `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	RunAsGroup          *int64          `json:"runAsGroup,omitempty" yaml:"runAsGroup,omitempty"`
	FSGroup             *int64          `json:"fsGroup,omitempty" yaml:"fsGroup,omitempty"`
	FSGroupChangePolicy string          `json:"fsGroupChangePolicy,omitempty" yaml:"fsGroupChangePolicy,omitempty"`
	SupplementalGroups  []int64         `json:"supplementalGroups,omitempty" yaml:"supplementalGroups,omitempty"`
	SeccompProfile      *SeccompProfile `json:"seccompProfile,omitempty" yaml:"seccompProfile,omitempty"`
}
type DefaultsPolicy struct {
	Template    *RuntimeDefaults `json:"template,omitempty" yaml:"template,omitempty"`
	Version     string           `json:"version" yaml:"version"`
	Environment string           `json:"environment" yaml:"environment"`
}
type RollingUpdate struct {
	MaxUnavailable any    `json:"maxUnavailable,omitempty" yaml:"maxUnavailable,omitempty"`
	MaxSurge       any    `json:"maxSurge,omitempty" yaml:"maxSurge,omitempty"`
	Partition      *int32 `json:"partition,omitempty" yaml:"partition,omitempty"`
}
type UpdateStrategy struct {
	Type          string         `json:"type" yaml:"type"`
	RollingUpdate *RollingUpdate `json:"rollingUpdate,omitempty" yaml:"rollingUpdate,omitempty"`
}
type PodDisruptionBudget struct {
	Enabled                    bool   `json:"enabled" yaml:"enabled"`
	MinAvailable               any    `json:"minAvailable,omitempty" yaml:"minAvailable,omitempty"`
	MaxUnavailable             any    `json:"maxUnavailable,omitempty" yaml:"maxUnavailable,omitempty"`
	UnhealthyPodEvictionPolicy string `json:"unhealthyPodEvictionPolicy,omitempty" yaml:"unhealthyPodEvictionPolicy,omitempty"`
}
type ClaimTemplate struct {
	Name             string   `json:"name" yaml:"name"`
	Size             string   `json:"size" yaml:"size"`
	AccessModes      []string `json:"accessModes" yaml:"accessModes"`
	StorageClassName *string  `json:"storageClassName,omitempty" yaml:"storageClassName,omitempty"`
	VolumeMode       string   `json:"volumeMode,omitempty" yaml:"volumeMode,omitempty"`
}
type ClaimMount struct {
	Claim     string `json:"claim" yaml:"claim"`
	Container string `json:"container" yaml:"container"`
	Path      string `json:"path" yaml:"path"`
	SubPath   string `json:"subPath,omitempty" yaml:"subPath,omitempty"`
	ReadOnly  bool   `json:"readOnly" yaml:"readOnly"`
}
type ClaimRetention struct {
	WhenDeleted string `json:"whenDeleted" yaml:"whenDeleted"`
	WhenScaled  string `json:"whenScaled" yaml:"whenScaled"`
}

func validSeccomp(s *SeccompProfile) bool {
	return s == nil || s.Type == "RuntimeDefault" && s.LocalhostProfile == "" || s.Type == "Localhost" && CleanSubPath(s.LocalhostProfile) && s.LocalhostProfile != ""
}
func validIDs(ids ...*int64) bool {
	for _, id := range ids {
		if id != nil && *id < 0 {
			return false
		}
	}
	return true
}

var capabilityName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func (s *ContainerSecurityContext) Validate() error {
	if s == nil {
		return nil
	}
	if !validIDs(s.RunAsUser, s.RunAsGroup) || !validSeccomp(s.SeccompProfile) || s.Privileged != nil && *s.Privileged || s.AllowPrivilegeEscalation != nil && *s.AllowPrivilegeEscalation || s.RunAsNonRoot != nil && *s.RunAsNonRoot && s.RunAsUser != nil && *s.RunAsUser == 0 {
		return fmt.Errorf("%w: container security context", ErrExecution)
	}
	if s.Capabilities != nil {
		for _, names := range [][]string{s.Capabilities.Add, s.Capabilities.Drop} {
			seen := map[string]bool{}
			for _, name := range names {
				if !capabilityName.MatchString(name) || seen[name] {
					return ErrExecution
				}
				seen[name] = true
			}
		}
	}
	return nil
}

// IntOrPercent accepts JSON integer or percent, never numeric strings or booleans.
func IntOrPercent(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, n >= 0 && n <= math.MaxInt32
	case float64:
		return int(n), n >= 0 && n <= math.MaxInt32 && n == float64(int(n))
	case json.Number:
		i, e := strconv.Atoi(string(n))
		return i, e == nil && i >= 0 && i <= math.MaxInt32
	case string:
		if !regexp.MustCompile(`^(0|[1-9][0-9]?)%$|^100%$`).MatchString(n) {
			return 0, false
		}
		i, _ := strconv.Atoi(n[:len(n)-1])
		return i, true
	}
	return 0, false
}
func (e *WorkloadExecution) ValidateRuntime(containers map[string]Container, replicas int) error {
	for _, c := range containers {
		for _, p := range []struct {
			value  *Probe
			single bool
		}{{c.ReadinessProbe, false}, {c.LivenessProbe, true}, {c.StartupProbe, true}} {
			if err := p.value.Validate(p.single); err != nil {
				return err
			}
		}
		if err := c.SecurityContext.Validate(); err != nil {
			return err
		}
	}
	if e == nil {
		return nil
	}
	if e.Defaults != nil && e.Defaults.Template != nil && !e.HasTemplate() {
		return ErrExecution
	}
	if p := e.Defaults; p != nil && (p.Version != "workload-defaults-v1" || p.Environment != "staging" && p.Environment != "production") {
		return ErrExecution
	}
	if p := e.PodSecurityContext; p != nil {
		if !validIDs(p.RunAsUser, p.RunAsGroup, p.FSGroup) || !validSeccomp(p.SeccompProfile) || p.FSGroupChangePolicy != "" && p.FSGroupChangePolicy != "Always" && p.FSGroupChangePolicy != "OnRootMismatch" {
			return ErrExecution
		}
		for _, g := range p.SupplementalGroups {
			if g < 0 {
				return ErrExecution
			}
		}
		if p.RunAsNonRoot != nil && *p.RunAsNonRoot && p.RunAsUser != nil && *p.RunAsUser == 0 {
			return ErrExecution
		}
	}
	if e.ServiceAccountName != "" && !volumeName.MatchString(e.ServiceAccountName) {
		return ErrExecution
	}
	if e.RevisionHistoryLimit != nil && (*e.RevisionHistoryLimit < 0 || e.Kind == "CronJob") {
		return ErrExecution
	}
	if s := e.UpdateStrategy; s != nil {
		switch e.Kind {
		case "Deployment":
			if s.Type != "Recreate" && s.Type != "RollingUpdate" {
				return ErrExecution
			}
			if len(e.Mounts) > 0 && s.Type != "Recreate" {
				return fmt.Errorf("%w: persistent Deployment requires Recreate", ErrExecution)
			}
		case "StatefulSet":
			if s.Type != "RollingUpdate" && s.Type != "OnDelete" {
				return ErrExecution
			}
		default:
			return ErrExecution
		}
		if r := s.RollingUpdate; r != nil {
			if s.Type != "RollingUpdate" {
				return ErrExecution
			}
			if e.Kind == "StatefulSet" {
				if r.MaxUnavailable != nil || r.MaxSurge != nil || r.Partition != nil && *r.Partition < 0 {
					return ErrExecution
				}
			} else {
				if r.Partition != nil {
					return ErrExecution
				}
				u, s := 0, 1
				var ok bool
				if r.MaxUnavailable != nil {
					u, ok = IntOrPercent(r.MaxUnavailable)
					if !ok {
						return ErrExecution
					}
				}
				if r.MaxSurge != nil {
					s, ok = IntOrPercent(r.MaxSurge)
					if !ok {
						return ErrExecution
					}
				}
				if u == 0 && s == 0 {
					return ErrExecution
				}
			}
		}
	}
	if p := e.PodDisruptionBudget; p != nil {
		if !p.Enabled {
			if p.MinAvailable != nil || p.MaxUnavailable != nil || p.UnhealthyPodEvictionPolicy != "" {
				return ErrExecution
			}
		} else {
			if e.Kind != "Deployment" || (replicas >= 0 && replicas < 2) || len(e.Mounts) > 0 || (p.MinAvailable == nil) == (p.MaxUnavailable == nil) {
				return ErrExecution
			}
			v := p.MinAvailable
			if v == nil {
				v = p.MaxUnavailable
			}
			if _, ok := IntOrPercent(v); !ok {
				return ErrExecution
			}
			if p.UnhealthyPodEvictionPolicy != "" && p.UnhealthyPodEvictionPolicy != "AlwaysAllow" && p.UnhealthyPodEvictionPolicy != "IfHealthyBudget" {
				return ErrExecution
			}
		}
	}
	if len(e.VolumeClaimTemplates) > 16 || len(e.ClaimMounts) > 128 {
		return ErrExecution
	}
	if (len(e.VolumeClaimTemplates) > 0 || len(e.ClaimMounts) > 0 || e.ClaimRetention != nil) && e.Kind != "StatefulSet" {
		return ErrExecution
	}
	if r := e.ClaimRetention; r != nil && (r.WhenDeleted != "Retain" || r.WhenScaled != "Retain") {
		return ErrExecution
	}
	names := map[string]bool{}
	paths := map[string][]string{}
	for _, v := range e.Volumes {
		names[v.Name] = true
	}
	for _, c := range e.VolumeClaimTemplates {
		if !volumeName.MatchString(c.Name) || names[c.Name] || !sizeQuantity.MatchString(c.Size) || !regexp.MustCompile(`[1-9]`).MatchString(c.Size) || len(c.AccessModes) == 0 || len(c.AccessModes) > 3 {
			return ErrExecution
		}
		names[c.Name] = true
		modes := map[string]bool{}
		for _, m := range c.AccessModes {
			if modes[m] || m != "ReadWriteOnce" && m != "ReadWriteMany" && m != "ReadOnlyMany" && m != "ReadWriteOncePod" {
				return ErrExecution
			}
			modes[m] = true
		}
		if c.VolumeMode != "" && c.VolumeMode != "Filesystem" {
			return ErrExecution
		}
		if c.StorageClassName != nil && *c.StorageClassName != "" && !volumeName.MatchString(*c.StorageClassName) {
			return ErrExecution
		}
	}
	for _, m := range e.Mounts {
		paths[m.Container] = append(paths[m.Container], m.Path)
	}
	for _, m := range e.VolumeMounts {
		paths[m.Container] = append(paths[m.Container], m.Path)
	}
	for _, m := range e.SecretFiles {
		paths[m.Container] = append(paths[m.Container], m.Path)
	}
	for _, m := range e.ClaimMounts {
		found := false
		for _, c := range e.VolumeClaimTemplates {
			found = found || c.Name == m.Claim
		}
		_, container := containers[m.Container]
		if !found || !container || !safeMountPath(m.Path) || !CleanSubPath(m.SubPath) {
			return ErrExecution
		}
		for _, p := range paths[m.Container] {
			if pathsOverlap(p, m.Path) {
				return ErrExecution
			}
		}
		paths[m.Container] = append(paths[m.Container], m.Path)
	}
	return nil
}

// ResolveDefaults is pure and returns an independent effective copy and leaf provenance.
// The frozen v1 policy never changes; another policy requires another version.
func (m ModuleSpec) ResolveDefaults(profiles ...*RuntimeDefaults) (ModuleSpec, map[string]string) {
	raw, _ := json.Marshal(m)
	var tree map[string]any
	_ = json.Unmarshal(raw, &tree)
	sources := map[string]string{}
	var mark func(any, string, string)
	mark = func(v any, path, source string) {
		if obj, ok := v.(map[string]any); ok {
			for k, x := range obj {
				p := k
				if path != "" {
					p = path + "." + k
				}
				mark(x, p, source)
			}
		} else {
			sources[path] = source
		}
	}
	mark(tree, "", "workload")
	if m.Execution == nil || m.Execution.Defaults == nil {
		var out ModuleSpec
		_ = json.Unmarshal(raw, &out)
		return out, sources
	}
	e := tree["workloadExecution"].(map[string]any)
	fill := func(obj map[string]any, key string, v any, path, source string) {
		if _, ok := obj[key]; !ok {
			obj[key] = v
			p := key
			if path != "" {
				p = path + "." + key
			}
			mark(v, p, source)
		}
	}
	object := func(obj map[string]any, key string) map[string]any {
		if v, ok := obj[key].(map[string]any); ok {
			return v
		}
		v := map[string]any{}
		obj[key] = v
		return v
	}
	// Merge only missing leaves; the desired tree always wins.
	var merge func(map[string]any, map[string]any, string)
	merge = func(dst, src map[string]any, path string) {
		for k, v := range src {
			p := k
			if path != "" {
				p = path + "." + k
			}
			if old, ok := dst[k]; ok {
				a, aok := old.(map[string]any)
				b, bok := v.(map[string]any)
				if aok && bok {
					merge(a, b, p)
				}
			} else {
				dst[k] = v
				mark(v, p, "template")
			}
		}
	}
	var profileDefaults *RuntimeDefaults
	if len(profiles) > 0 {
		profileDefaults = profiles[0]
	} else {
		profileDefaults = m.Execution.Defaults.Template
	}
	if profileDefaults != nil {
		p := profileDefaults
		b, _ := json.Marshal(p)
		var profile map[string]any
		_ = json.Unmarshal(b, &profile)
		defaults := object(e, "defaults")
		defaults["template"] = profile
		for name, raw := range tree["containers"].(map[string]any) {
			c := raw.(map[string]any)
			if c["role"] != nil && c["role"] != "app" {
				continue
			}
			fields := map[string]any{}
			for _, key := range []string{"image", "ports", "readinessProbe", "livenessProbe", "startupProbe"} {
				if v, ok := profile[key]; ok {
					fields[key] = v
				}
			}

			if v, ok := profile["resources"]; ok {
				fields["resources"] = v
			}
			if v, ok := profile["containerSecurityContext"]; ok {
				fields["securityContext"] = v
			}
			copied, _ := json.Marshal(fields)
			var independent map[string]any
			_ = json.Unmarshal(copied, &independent)
			merge(c, independent, "containers."+name)
		}
		if m.Execution.ServiceEnabled == nil || *m.Execution.ServiceEnabled {
			if v, ok := profile["service"]; ok {
				fill(tree, "service", v, "", "template")
			}
		}
		for _, key := range []string{"volumes", "volumeMounts"} {
			if v, ok := profile[key]; ok {
				fill(e, key, v, "workloadExecution", "template")
			}
		}
		if v, ok := profile["podSecurityContext"]; ok {
			merge(e, map[string]any{"podSecurityContext": v}, "workloadExecution")
		}
		if p.ProductionReplicas != nil && p.MultiReplicaSafe && m.Execution.Defaults.Environment == "production" && m.Execution.Kind == "Deployment" {
			fill(tree, "replicas", *p.ProductionReplicas, "", "template")
		}
		if p.Replicas != nil && (*p.Replicas <= 1 || p.MultiReplicaSafe) && m.Execution.Kind == "Deployment" {
			fill(tree, "replicas", *p.Replicas, "", "template")
		}
	}
	prod := m.Execution.Defaults.Environment == "production"
	cpu, mem, history := "100m", "128Mi", 3
	if prod {
		cpu, mem, history = "200m", "256Mi", 10
	}
	for name, raw := range tree["containers"].(map[string]any) {
		c := raw.(map[string]any)
		path := "containers." + name
		if c["role"] == nil || c["role"] == "app" {
			r := object(c, "resources")
			requests := object(r, "requests")
			limits := object(r, "limits")
			fill(requests, "cpu", cpu, path+".resources.requests", "environment")
			fill(requests, "memory", mem, path+".resources.requests", "environment")
			fill(limits, "memory", "512Mi", path+".resources.limits", "platform")
		}
		s := object(c, "securityContext")
		fill(s, "privileged", false, path+".securityContext", "platform")
		fill(s, "allowPrivilegeEscalation", false, path+".securityContext", "platform")
		for _, key := range []string{"readinessProbe", "livenessProbe", "startupProbe"} {
			if p, ok := c[key].(map[string]any); ok {
				period, fail := 10, 3
				if key == "startupProbe" {
					period, fail = 5, 30
				}
				fill(p, "periodSeconds", period, path+"."+key, "platform")
				fill(p, "timeoutSeconds", 2, path+"."+key, "platform")
				fill(p, "failureThreshold", fail, path+"."+key, "platform")
				fill(p, "successThreshold", 1, path+"."+key, "platform")
				if c["startupProbe"] != nil {
					fill(p, "initialDelaySeconds", 0, path+"."+key, "platform")
				}
			}
		}
	}
	fill(e, "automountServiceAccountToken", false, "workloadExecution", "platform")
	s := object(e, "podSecurityContext")
	fill(s, "seccompProfile", map[string]any{"type": "RuntimeDefault"}, "workloadExecution.podSecurityContext", "platform")
	if m.Execution.Kind != "CronJob" {
		fill(tree, "replicas", 1, "", "platform")
		// Correct root path for provenance.
		if sources[".replicas"] != "" {
			sources["replicas"] = sources[".replicas"]
			delete(sources, ".replicas")
		}
		fill(e, "revisionHistoryLimit", history, "workloadExecution", "environment")
		strategy := map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"maxUnavailable": 0, "maxSurge": 1}}
		if m.Execution.Kind == "StatefulSet" {
			strategy = map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"partition": 0}}
		} else if len(m.Execution.Mounts) > 0 {
			strategy = map[string]any{"type": "Recreate"}
		}
		if _, ok := e["updateStrategy"]; !ok {
			fill(e, "updateStrategy", strategy, "workloadExecution", "platform")
		} else if current, ok := e["updateStrategy"].(map[string]any); ok && current["type"] == "RollingUpdate" {
			rolling := object(current, "rollingUpdate")
			if m.Execution.Kind == "StatefulSet" {
				fill(rolling, "partition", 0, "workloadExecution.updateStrategy.rollingUpdate", "platform")
			} else {
				fill(rolling, "maxUnavailable", 0, "workloadExecution.updateStrategy.rollingUpdate", "platform")
				fill(rolling, "maxSurge", 1, "workloadExecution.updateStrategy.rollingUpdate", "platform")
			}
		}
	} else {
		if s, ok := e["schedule"].(map[string]any); ok {
			fill(s, "timeZone", "Asia/Bangkok", "workloadExecution.schedule", "environment")
			fill(s, "concurrencyPolicy", "Forbid", "workloadExecution.schedule", "platform")
			for k, v := range map[string]any{"backoffLimit": 0, "successfulJobsHistoryLimit": 3, "failedJobsHistoryLimit": 3} {
				fill(s, k, v, "workloadExecution.schedule", "platform")
			}
		}
	}
	if p, ok := e["podDisruptionBudget"].(map[string]any); ok && p["enabled"] == true {
		if _, min := p["minAvailable"]; !min {
			fill(p, "maxUnavailable", 1, "workloadExecution.podDisruptionBudget", "platform")
		}
		fill(p, "unhealthyPodEvictionPolicy", "AlwaysAllow", "workloadExecution.podDisruptionBudget", "platform")
	}
	if len(m.Execution.VolumeClaimTemplates) > 0 {
		fill(e, "persistentVolumeClaimRetentionPolicy", map[string]any{"whenDeleted": "Retain", "whenScaled": "Retain"}, "workloadExecution", "platform")
	}
	encoded, _ := json.Marshal(tree)
	var out ModuleSpec
	_ = json.Unmarshal(encoded, &out)
	return out, sources
}

// RuntimeDefaults is immutable template authoring metadata, covered by its pin.
// MultiReplicaSafe is required before a profile can propose production replicas >1.
type RuntimeDefaults struct {
	Image                    string                         `json:"image,omitempty" yaml:"image,omitempty"`
	Ports                    []ContainerPort                `json:"ports,omitempty" yaml:"ports,omitempty"`
	ReadinessProbe           *Probe                         `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	LivenessProbe            *Probe                         `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty"`
	StartupProbe             *Probe                         `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty"`
	Service                  *Service                       `json:"service,omitempty" yaml:"service,omitempty"`
	Volumes                  []PodVolume                    `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	VolumeMounts             []VolumeMount                  `json:"volumeMounts,omitempty" yaml:"volumeMounts,omitempty"`
	Resources                *ContainerResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	ContainerSecurityContext *ContainerSecurityContext      `json:"containerSecurityContext,omitempty" yaml:"containerSecurityContext,omitempty"`
	PodSecurityContext       *PodSecurityContext            `json:"podSecurityContext,omitempty" yaml:"podSecurityContext,omitempty"`
	ProductionReplicas       *int                           `json:"productionReplicas,omitempty" yaml:"productionReplicas,omitempty"`
	Replicas                 *int                           `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	MultiReplicaSafe         bool                           `json:"multiReplicaSafe,omitempty" yaml:"multiReplicaSafe,omitempty"`
}

// EffectiveService reads frozen template policy without materializing overrides.
func (m ModuleSpec) EffectiveService() *Service {
	effective, _ := m.ResolveDefaults()
	return effective.Service
}
