package drift

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Comparisons are intentionally a small allowlist of operational scalars.
// Arbitrary tags, config/data, identifiers and strings remain private even if a
// provider failed to mark them sensitive. Desired comes from the corrective
// plan, never from assuming that historical State equals configuration.
type Comparison struct {
	Address string `json:"address"`
	Path    string `json:"path"`
	Desired any    `json:"desired"`
	Actual  any    `json:"actual"`
}

var instanceType = regexp.MustCompile(`^[a-z][0-9][a-z0-9-]*\.[a-z0-9]+$`)

func operationalScalar(resourceType, path string, value any) bool {
	switch resourceType {
	case "aws_instance", "aws_launch_template", "aws_db_instance", "aws_rds_cluster_instance", "aws_elasticache_cluster", "kubernetes_deployment", "kubernetes_deployment_v1", "kubernetes_stateful_set", "kubernetes_stateful_set_v1":
	default:
		return false
	}
	switch path {
	case "/instance_type", "/instance_class", "/node_type":
		s, ok := value.(string)
		return ok && len(s) <= 64 && instanceType.MatchString(s)
	case "/replicas", "/spec/replicas", "/allocated_storage":
		_, ok := value.(json.Number)
		return ok
	case "/enabled", "/deletion_protection":
		_, ok := value.(bool)
		return ok
	}
	return false
}
func atPath(value any, path string) any {
	if path == "" {
		return value
	}
	for _, key := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		m, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = m[strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")]
	}
	return value
}
func masked(mask any, path string) bool {
	if mask == true {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts {
		m, ok := mask.(map[string]any)
		if !ok {
			return false
		}
		mask = m[part]
		if mask == true {
			return true
		}
	}
	return unknown(mask)
}
func comparisons(doc document, report Report) []Comparison {
	proposed := map[string]resourceChange{}
	for _, r := range doc.Resources {
		proposed[r.Address] = r
	}
	external := map[string]resourceChange{}
	for _, r := range doc.Drift {
		external[r.Address] = r
	}
	out := []Comparison{}
	for _, finding := range report.External {
		live := external[finding.Address]
		desired, ok := proposed[finding.Address]
		if !ok {
			continue
		}
		for _, path := range finding.Paths {
			if masked(live.Change.BeforeSensitive, path) || masked(live.Change.AfterSensitive, path) || masked(desired.Change.BeforeSensitive, path) || masked(desired.Change.AfterSensitive, path) || masked(desired.Change.AfterUnknown, path) {
				continue
			}
			actual := atPath(live.Change.After, path)
			want := atPath(desired.Change.After, path)
			if operationalScalar(finding.Type, path, actual) && operationalScalar(finding.Type, path, want) {
				out = append(out, Comparison{finding.Address, path, want, actual})
			}
		}
	}
	return out
}
