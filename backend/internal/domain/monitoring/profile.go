package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"regexp"
	"strings"
)

// Profile is a typed configuration, not a query language. Empty is legacy v1 rules.
type Profile struct {
	SchemaVersion int              `json:"schemaVersion"`
	Version       string           `json:"version"`
	Defaults      *WorkloadProfile `json:"defaults,omitempty"`
}
type WorkloadProfile struct {
	Port            string             `json:"port"`
	Path            string             `json:"path"`
	IntervalSeconds int                `json:"intervalSeconds"`
	SampleLimit     int                `json:"sampleLimit"`
	Metrics         []MetricDefinition `json:"metrics"`
}
type MetricDefinition struct {
	Matchers           map[string]string `json:"matchers,omitempty"`
	DenominatorMetric  string            `json:"denominatorMetric,omitempty"`
	DenominatorLabels  map[string]string `json:"denominatorLabels,omitempty"`
	MinimumDenominator float64           `json:"minimumDenominator,omitempty"`
	Key                string            `json:"key"`
	Metric             string            `json:"metric"`
	Aggregation        string            `json:"aggregation"`
	Unit               string            `json:"unit"`
	Labels             map[string]string `json:"labels,omitempty"`
	Alert              *MetricThreshold  `json:"alert,omitempty"`
}
type MetricThreshold struct {
	Comparison string  `json:"comparison,omitempty"`
	Above      float64 `json:"above"`
	ForSeconds int     `json:"forSeconds"`
	Severity   string  `json:"severity"`
}

var metricIdentifier = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]{0,199}$`)
var labelIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,99}$`)
var portIdentifier = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,13}[a-z0-9])?$`)

func (p Profile) Validate() error {
	if p.SchemaVersion == 0 && p.Version == "" && p.Defaults == nil {
		return nil
	}
	if p.SchemaVersion != 1 || p.Version == "" || len(p.Version) > 120 {
		return ErrInvalid
	}
	if p.Defaults != nil {
		return p.Defaults.Validate()
	}
	return nil
}
func (p WorkloadProfile) Validate() error {
	if !portIdentifier.MatchString(p.Port) || !strings.HasPrefix(p.Path, "/") || len(p.Path) > 200 || strings.ContainsAny(p.Path, "?#\r\n\\") || strings.Contains(p.Path, "..") || p.IntervalSeconds < 30 || p.IntervalSeconds > 300 || p.SampleLimit < 1 || p.SampleLimit > 10000 || len(p.Metrics) > 8 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, m := range p.Metrics {
		if !labelIdentifier.MatchString(m.Key) || seen[m.Key] || !metricIdentifier.MatchString(m.Metric) || len(m.Labels) > 8 {
			return ErrInvalid
		}
		seen[m.Key] = true
		switch m.Aggregation {
		case "gauge", "rate", "p95", "rate_ratio", "gauge_ratio":
		default:
			return ErrInvalid
		}
		switch m.Unit {
		case "count", "seconds", "bytes", "per_second", "ratio":
		default:
			return ErrInvalid
		}
		for k, v := range m.Labels {
			if !labelIdentifier.MatchString(k) || strings.HasPrefix(k, "orchestrator_") || k == "cluster_uid" || k == "namespace" || len(v) > 120 {
				return ErrInvalid
			}
		}
		if len(m.Labels)+len(m.Matchers)+len(m.DenominatorLabels) > 8 {
			return ErrInvalid
		}
		for _, labels := range []map[string]string{m.Matchers, m.DenominatorLabels} {
			for k, v := range labels {
				if !labelIdentifier.MatchString(k) || strings.HasPrefix(k, "orchestrator_") || k == "cluster_uid" || k == "namespace" || len(v) > 120 {
					return ErrInvalid
				}
			}
		}
		for k, pattern := range m.Matchers {
			if _, exists := m.Labels[k]; exists {
				return ErrInvalid
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return ErrInvalid
			}
		}
		if m.Aggregation == "rate_ratio" || m.Aggregation == "gauge_ratio" {
			if !metricIdentifier.MatchString(m.DenominatorMetric) || m.MinimumDenominator <= 0 || m.MinimumDenominator > 1e15 || math.IsNaN(m.MinimumDenominator) || math.IsInf(m.MinimumDenominator, 0) {
				return ErrInvalid
			}
		} else if m.DenominatorMetric != "" || len(m.DenominatorLabels) > 0 || m.MinimumDenominator != 0 {
			return ErrInvalid
		}
		if m.Alert != nil && m.Alert.Comparison != "" && m.Alert.Comparison != "above" && m.Alert.Comparison != "below" {
			return ErrInvalid
		}
		if m.Alert != nil && (math.IsNaN(m.Alert.Above) || math.IsInf(m.Alert.Above, 0) || m.Alert.Above < 0 || m.Alert.Above > 1e15 || m.Alert.ForSeconds < 0 || m.Alert.ForSeconds > 86400 || (m.Alert.Severity != "warning" && m.Alert.Severity != "critical")) {
			return ErrInvalid
		}
	}
	return nil
}
func CloneBinding(b Binding) Binding { // JSON metadata contains mutable overrides.
	raw, _ := json.Marshal(b)
	var out Binding
	_ = json.Unmarshal(raw, &out)
	return out
}

// Pin contains only nonsecret configuration identities used by Preview/admission.
type Pin struct {
	ConnectionID             string `json:"connectionId,omitempty"`
	ConnectionRevision       int64  `json:"connectionRevision,omitempty"`
	PinnedConnectionRevision int64  `json:"pinnedConnectionRevision,omitempty"`
	BindingRevision          int64  `json:"bindingRevision,omitempty"`
	Generation               int64  `json:"generation,omitempty"`
	MetricsSource            string `json:"metricsSource,omitempty"`
	LogsSource               string `json:"logsSource,omitempty"`
	Stale                    bool   `json:"stale,omitempty"`
}

func (p Pin) Signature() string {
	if p.BindingRevision == 0 || (p.MetricsSource == "native" && p.LogsSource == "native") {
		return ""
	}
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func NewPin(b Binding, c Connection, generation int64) Pin {
	metrics := b.MetricsSource
	if metrics == "" {
		metrics = "native"
		if c.PrometheusURL != "" {
			metrics = "prometheus"
		}
	}
	logs := b.LogsSource
	if logs == "" {
		logs = "native"
	}
	p := Pin{ConnectionID: c.ID, ConnectionRevision: c.Revision, PinnedConnectionRevision: b.ConnectionRevision, BindingRevision: b.Revision, Generation: b.Generation, MetricsSource: metrics, LogsSource: logs}
	p.Stale = b.Generation != generation || ((metrics != "native" || logs != "native") && c.Revision != b.ConnectionRevision)
	return p
}
