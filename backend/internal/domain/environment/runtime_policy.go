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

// TemplateDefaultsSnapshot is verified metadata covered by TemplatePin, not a policy.
type TemplateDefaultsSnapshot struct {
	Environment string           `json:"environment"`
	Profile     *RuntimeDefaults `json:"profile"`
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
	return e.validateRuntime(containers, replicas, false)
}
func (e *WorkloadExecution) validateRuntime(containers map[string]Container, replicas int, resolved bool) error {
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
	if e.TemplateDefaults != nil && (!e.HasTemplate() || e.TemplateDefaults.Profile == nil || e.TemplateDefaults.Environment == "") {
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
			if e.Kind != "Deployment" || (replicas >= 0 && replicas < 2) || len(e.Mounts) > 0 || (p.MinAvailable == nil) == (p.MaxUnavailable == nil) && !(!resolved && e.HasTemplate() && p.MinAvailable == nil) {
				return ErrExecution
			}
			v := p.MinAvailable
			if v == nil {
				v = p.MaxUnavailable
			}
			if _, ok := IntOrPercent(v); !ok && !(v == nil && !resolved && e.HasTemplate()) {
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
