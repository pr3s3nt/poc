package templatesource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"orchestrator/internal/domain/environment"
	"regexp"
	"sort"
	"time"
)

// WorkloadTemplate is immutable authoring metadata owned by a verified Git
// bundle. It never chooses infrastructure or embeds executable UI code.
type WorkloadTemplate struct {
	RuntimeDefaults *environment.RuntimeDefaults `yaml:"runtimeDefaults,omitempty" json:"runtimeDefaults,omitempty"`
	ID              string                       `yaml:"id" json:"id"`
	Version         string                       `yaml:"version" json:"version"`
	Kind            string                       `yaml:"kind" json:"kind"`
	Labels          map[string]string            `yaml:"labels" json:"labels"`
	Descriptions    map[string]string            `yaml:"descriptions,omitempty" json:"descriptions,omitempty"`
	InputsSchema    map[string]any               `yaml:"inputsSchema" json:"inputsSchema"`
	Defaults        map[string]any               `yaml:"defaults" json:"defaults"`
}

var templateVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func (t WorkloadTemplate) Validate() error {
	if err := validateRuntimeDefaults(t.RuntimeDefaults, t.Kind, 0); err != nil {
		return err
	}
	if !namePattern.MatchString(t.ID) || !templateVersion.MatchString(t.Version) || len(t.Labels) == 0 || len(t.Labels) > 16 || len(t.Descriptions) > 16 {
		return invalid(ManifestFile, "workload template metadata is invalid")
	}
	switch t.Kind {
	case "Deployment", "StatefulSet", "CronJob":
	default:
		return invalid(ManifestFile, "workload template runtime is unsupported")
	}
	for locale, label := range t.Labels {
		if len(locale) > 16 || locale == "" || label == "" || len(label) > 160 {
			return invalid(ManifestFile, "workload template label is invalid")
		}
	}
	for locale, label := range t.Descriptions {
		if len(locale) > 16 || locale == "" || len(label) > 512 {
			return invalid(ManifestFile, "workload template description is invalid")
		}
	}
	if err := ValidateSchema(t.InputsSchema); err != nil {
		return err
	}
	if _, err := Effective(t.InputsSchema, t.Defaults); err != nil {
		return err
	}
	return nil
}

// SHA256 identifies all authoring metadata, schema and defaults, independent
// of map iteration order or the bundle that advertised the template.
func (t WorkloadTemplate) SHA256() string {
	b, _ := json.Marshal(t)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (t WorkloadTemplate) Clone() WorkloadTemplate {
	raw, _ := json.Marshal(t)
	var out WorkloadTemplate
	_ = json.Unmarshal(raw, &out)
	return out
}

func validateWorkloadTemplates(templates []WorkloadTemplate) error {
	if len(templates) > 32 {
		return invalid(ManifestFile, "at most 32 workload templates are allowed")
	}
	seen := map[string]bool{}
	for _, t := range templates {
		if err := t.Validate(); err != nil {
			return err
		}
		key := t.ID + "@" + t.Version
		if seen[key] {
			return invalid(ManifestFile, "duplicate workload template version")
		}
		seen[key] = true
	}
	return nil
}

// TemplateCatalog deduplicates repeated inspections while detecting conflicting
// immutable versions in one Organization. Callers pass only scoped records.
func TemplateCatalog(records map[string]Bundle) ([]WorkloadTemplate, error) {
	byVersion := map[string]WorkloadTemplate{}
	for _, record := range records {
		retired := false
		for _, capability := range record.Contract.Manifest.WorkloadCapabilities {
			if capability == "workload-defaults-v1" {
				retired = true
			}
		}
		if retired {
			continue
		} // Retired test sources cannot be selected for new authoring.
		for _, t := range record.Contract.Manifest.WorkloadTemplates {
			if err := t.Validate(); err != nil {
				return nil, err
			}
			key := t.ID + "@" + t.Version
			if previous, ok := byVersion[key]; ok && previous.SHA256() != t.SHA256() {
				return nil, invalid(ManifestFile, "conflicting workload template version")
			}
			byVersion[key] = t.Clone()
		}
	}
	keys := make([]string, 0, len(byVersion))
	for key := range byVersion {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]WorkloadTemplate, 0, len(keys))
	for _, key := range keys {
		out = append(out, byVersion[key])
	}
	return out, nil
}

func MatchWorkloadTemplate(templates []WorkloadTemplate, id, version, digest, kind string, inputs map[string]any) (WorkloadTemplate, map[string]any, error) {
	for _, t := range templates {
		if t.ID != id || t.Version != version || t.SHA256() != digest || t.Kind != kind {
			continue
		}
		combined := map[string]any{}
		for key, value := range t.Defaults {
			combined[key] = DeepCopy(value)
		}
		for key, value := range inputs {
			combined[key] = DeepCopy(value)
		}
		effective, err := Effective(t.InputsSchema, combined)
		if err != nil {
			return WorkloadTemplate{}, nil, err
		}
		return t.Clone(), effective, nil
	}
	return WorkloadTemplate{}, nil, ErrUnsupported
}

func validateRuntimeDefaults(p *environment.RuntimeDefaults, kind string, depth int) error {
	if p == nil {
		return nil
	}
	if depth > 1 || len(p.Environments) > 16 {
		return invalid(ManifestFile, "invalid runtime environment profiles")
	}
	for name, variant := range p.Environments {
		if !namePattern.MatchString(name) || variant == nil || len(variant.Environments) > 0 {
			return invalid(ManifestFile, "invalid runtime environment profile")
		}
		if err := validateRuntimeDefaults(p.ForEnvironment(name), kind, depth+1); err != nil {
			return err
		}
	}
	if err := p.ContainerSecurityContext.Validate(); err != nil {
		return err
	}
	if err := p.AuxiliaryContainerSecurityContext.Validate(); err != nil {
		return err
	}
	if p.Replicas != nil && (*p.Replicas < 0 || *p.Replicas > 1 && !p.MultiReplicaSafe || kind != "Deployment" && *p.Replicas != 1) {
		return invalid(ManifestFile, "replicas need compatible runtime profile")
	}
	container := environment.Container{Image: p.Image, Resources: p.Resources, SecurityContext: p.ContainerSecurityContext, ReadinessProbe: p.ReadinessProbe, LivenessProbe: p.LivenessProbe, StartupProbe: p.StartupProbe, Ports: p.Ports}
	if err := (&environment.WorkloadExecution{Kind: kind}).ValidateRuntime(map[string]environment.Container{"main": container}, 1); err != nil {
		return err
	}
	if err := (environment.ModuleSpec{Execution: &environment.WorkloadExecution{Template: environment.TemplatePin{ID: "profile-validation"}}, Containers: map[string]environment.Container{"main": container}}).ValidateComputePolicy(); err != nil {
		return err
	}
	if e := p.Execution; e != nil {
		execution := environment.WorkloadExecution{Version: 1, Kind: kind, PodSecurityContext: e.PodSecurityContext, AutomountServiceAccountToken: e.AutomountServiceAccountToken, UpdateStrategy: e.UpdateStrategy, RevisionHistoryLimit: e.RevisionHistoryLimit, ClaimRetention: e.ClaimRetention}
		if err := execution.ValidateRuntime(map[string]environment.Container{"main": {Image: "profile-validation"}}, 1); err != nil {
			return err
		}
		if e.PersistentUpdateStrategy != nil && (kind != "Deployment" || e.PersistentUpdateStrategy.Type != "Recreate" || e.PersistentUpdateStrategy.RollingUpdate != nil) {
			return invalid(ManifestFile, "persistent strategy must preserve storage safety")
		}
		if schedule := e.Schedule; schedule != nil {
			if kind != "CronJob" || schedule.Expression != "" {
				return invalid(ManifestFile, "schedule default must not invent a cron expression")
			}
			if schedule.TimeZone != "" {
				if schedule.TimeZone == "Local" {
					return invalid(ManifestFile, "timezone must be stable")
				}
				if _, err := time.LoadLocation(schedule.TimeZone); err != nil {
					return invalid(ManifestFile, "invalid template timezone")
				}
			}
			if schedule.ConcurrencyPolicy != "" && schedule.ConcurrencyPolicy != "Forbid" && schedule.ConcurrencyPolicy != "Allow" && schedule.ConcurrencyPolicy != "Replace" {
				return invalid(ManifestFile, "invalid concurrency default")
			}
		}
		if budget := e.PodDisruptionBudget; budget != nil {
			if kind != "Deployment" || (budget.MinAvailable == nil) == (budget.MaxUnavailable == nil) {
				return invalid(ManifestFile, "invalid PDB defaults")
			}
			quantity := budget.MinAvailable
			if quantity == nil {
				quantity = budget.MaxUnavailable
			}
			if _, ok := environment.IntOrPercent(quantity); !ok {
				return invalid(ManifestFile, "invalid PDB quantity")
			}
		}
	}
	for key, probe := range p.ProbeTimings {
		if key != "readinessProbe" && key != "livenessProbe" && key != "startupProbe" || probe == nil || probe.Type != "" || probe.Path != "" || probe.Port != 0 || len(probe.Command) > 0 || probe.Scheme != "" {
			return invalid(ManifestFile, "probe timing defaults cannot declare endpoints")
		}
		for _, n := range []*int{probe.PeriodSeconds, probe.TimeoutSeconds, probe.FailureThreshold, probe.SuccessThreshold} {
			if n != nil && *n < 1 {
				return invalid(ManifestFile, "invalid probe timing default")
			}
		}
		if probe.InitialDelaySeconds != nil && *probe.InitialDelaySeconds < 0 || key != "readinessProbe" && probe.SuccessThreshold != nil && *probe.SuccessThreshold != 1 {
			return invalid(ManifestFile, "invalid probe timing default")
		}
	}
	return nil
}
