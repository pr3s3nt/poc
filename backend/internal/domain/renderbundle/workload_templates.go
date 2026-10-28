package renderbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
)

// WorkloadTemplate is immutable authoring metadata owned by a verified Git
// bundle. It never chooses infrastructure or embeds executable UI code.
type WorkloadTemplate struct {
	ID           string            `yaml:"id" json:"id"`
	Version      string            `yaml:"version" json:"version"`
	Kind         string            `yaml:"kind" json:"kind"`
	Labels       map[string]string `yaml:"labels" json:"labels"`
	Descriptions map[string]string `yaml:"descriptions,omitempty" json:"descriptions,omitempty"`
	InputsSchema map[string]any    `yaml:"inputsSchema" json:"inputsSchema"`
	Defaults     map[string]any    `yaml:"defaults" json:"defaults"`
}

var templateVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func (t WorkloadTemplate) Validate() error {
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
