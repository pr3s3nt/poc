package environment

import (
	"path"
	"regexp"
	"strings"
)

// PodVolume defines named Pod sources. Managed persistent storage remains a resource.
type PodVolume struct {
	Name      string           `json:"name" yaml:"name"`
	EmptyDir  *EmptyDirVolume  `json:"emptyDir,omitempty" yaml:"emptyDir,omitempty"`
	ConfigMap *ConfigMapVolume `json:"configMap,omitempty" yaml:"configMap,omitempty"`
	HostPath  *HostPathVolume  `json:"hostPath,omitempty" yaml:"hostPath,omitempty"`
}
type HostPathVolume struct {
	Path string `json:"path" yaml:"path"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

func (h *HostPathVolume) valid() bool {
	if len(h.Path) == 0 || len(h.Path) > 1024 || !strings.HasPrefix(h.Path, "/") || path.Clean(h.Path) != h.Path || strings.ContainsAny(h.Path, "\x00\r\n\\") {
		return false
	}
	switch h.Type {
	case "", "Directory", "DirectoryOrCreate", "File", "FileOrCreate", "Socket", "CharDevice", "BlockDevice":
		return true
	}
	return false
}

type EmptyDirVolume struct {
	Medium    string `json:"medium,omitempty" yaml:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty" yaml:"sizeLimit,omitempty"`
}
type ConfigMapVolume struct {
	Files       []ConfigFile `json:"files" yaml:"files"`
	DefaultMode int          `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
}
type VolumeMount struct {
	Volume    string `json:"volume" yaml:"volume"`
	Container string `json:"container" yaml:"container"`
	Path      string `json:"path" yaml:"path"`
	ReadOnly  bool   `json:"readOnly" yaml:"readOnly"`
	SubPath   string `json:"subPath,omitempty" yaml:"subPath,omitempty"`
}

var volumeName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var configFileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,252}$`)
var sizeQuantity = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?(?:Ki|Mi|Gi|Ti|Pi|Ei|[kMGTPE])?$`)

func CleanSubPath(p string) bool {
	return p == "" || (len(p) <= 1024 && !strings.HasPrefix(p, "/") && p != "." && p != ".." && !strings.HasPrefix(p, "../") && path.Clean(p) == p && !strings.ContainsAny(p, "\x00\r\n\\"))
}
func safeMountPath(p string) bool {
	if len(p) > 1024 || !strings.HasPrefix(p, "/") || p == "/" || path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n\\") {
		return false
	}
	for _, prefix := range []string{"/vault", "/var/run/secrets"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") || strings.HasPrefix(prefix, p+"/") {
			return false
		}
	}
	return true
}
func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// ValidateContainerRoles keeps the named binding model shared by every role.
func ValidateContainerRoles(containers map[string]Container) error {
	if len(containers) > 64 {
		return ErrExecution
	}
	apps := 0
	orders := map[int]bool{}
	for _, c := range containers {
		switch c.Role {
		case "", "app":
			apps++
			if c.InitOrder != 0 {
				return ErrExecution
			}
		case "init", "sidecar":
			if c.InitOrder < 1 || c.InitOrder > 999 || orders[c.InitOrder] {
				return ErrExecution
			}
			orders[c.InitOrder] = true
			if c.Role == "init" && (c.ReadinessProbe != nil || c.LivenessProbe != nil || c.StartupProbe != nil) {
				return ErrExecution
			}
		default:
			return ErrExecution
		}
	}
	if apps == 0 {
		return ErrExecution
	}
	return nil
}
func (e *WorkloadExecution) HasTemplate() bool { return e != nil && e.Template != (TemplatePin{}) }

func (e *WorkloadExecution) validateVolumes(containers map[string]Container) error {
	if len(e.Volumes) > 32 || len(e.VolumeMounts) > 128 {
		return ErrExecution
	}
	volumes := map[string]PodVolume{}
	for _, v := range e.Volumes {
		sources := 0
		for _, present := range []bool{v.EmptyDir != nil, v.ConfigMap != nil, v.HostPath != nil} {
			if present {
				sources++
			}
		}
		if !volumeName.MatchString(v.Name) || sources != 1 {
			return ErrExecution
		}
		if _, ok := volumes[v.Name]; ok {
			return ErrExecution
		}
		volumes[v.Name] = v
		if v.HostPath != nil && !v.HostPath.valid() {
			return ErrExecution
		}
		if d := v.EmptyDir; d != nil {
			if d.Medium != "" && d.Medium != "Memory" {
				return ErrExecution
			}
			if d.SizeLimit != "" && (len(d.SizeLimit) > 128 || !sizeQuantity.MatchString(d.SizeLimit) || !strings.ContainsAny(d.SizeLimit, "123456789")) {
				return ErrExecution
			}
		}
		if c := v.ConfigMap; c != nil {
			if len(c.Files) == 0 || len(c.Files) > 128 || (c.DefaultMode != 0 && c.DefaultMode != 0400 && c.DefaultMode != 0440 && c.DefaultMode != 0444) {
				return ErrExecution
			}
			total := 0
			names := map[string]bool{}
			for _, file := range c.Files {
				k, v := file.Name, file.Value
				if names[k] {
					return ErrExecution
				}
				names[k] = true
				if !configFileName.MatchString(k) || strings.ContainsRune(v, 0) {
					return ErrExecution
				}
				total += len(k) + len(v)
			}
			if total > 1<<20 {
				return ErrExecution
			}
		}
	}
	seen := map[string][]string{}
	for _, m := range e.Mounts {
		if !CleanSubPath(m.SubPath) {
			return ErrExecution
		}
		seen[m.Container] = append(seen[m.Container], m.Path)
	}
	for _, f := range e.SecretFiles {
		seen[f.Container] = append(seen[f.Container], f.Path)
	}
	for _, m := range e.VolumeMounts {
		v, ok := volumes[m.Volume]
		if !ok {
			return ErrExecution
		}
		if _, ok := containers[m.Container]; !ok || !safeMountPath(m.Path) || !CleanSubPath(m.SubPath) {
			return ErrExecution
		}
		if v.ConfigMap != nil {
			if !m.ReadOnly {
				return ErrExecution
			}
			if m.SubPath != "" {
				if _, ok := v.ConfigMap.Values()[m.SubPath]; !ok {
					return ErrExecution
				}
			}
		}
		for _, p := range seen[m.Container] {
			if pathsOverlap(p, m.Path) {
				return ErrExecution
			}
		}
		seen[m.Container] = append(seen[m.Container], m.Path)
	}
	return nil
}

type ConfigFile struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

func (c *ConfigMapVolume) Values() map[string]string {
	out := map[string]string{}
	for _, file := range c.Files {
		out[file.Name] = file.Value
	}
	return out
}
