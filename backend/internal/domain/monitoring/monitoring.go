// Package monitoring owns identities and configuration for external observation.
package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("monitoring: invalid configuration")
var ErrCredentialChange = errors.New("monitoring: endpoint change requires explicit credentials")
var ErrStaleBinding = errors.New("monitoring: binding needs refresh")
var ErrInUse = errors.New("monitoring: configuration still in use")
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

// Scope is supplied by authenticated application/Environment lookup, never URL selectors.
type Scope struct {
	Organization string `json:"organization"`
	Application  string `json:"application"`
	Environment  string `json:"environment"`
	Generation   int64  `json:"generation"`
	ClusterUID   string `json:"clusterUid"`
	Namespace    string `json:"namespace"`
}

func (s Scope) Validate() error {
	if s.Organization == "" || s.Application == "" || s.Environment == "" || s.ClusterUID == "" || !namePattern.MatchString(s.Namespace) || s.Generation < 0 {
		return ErrInvalid
	}
	return nil
}

// Target is restored from owned deployment controller inventory.
type Target struct {
	Scope                     Scope
	Workload, Kind, Name, UID string
	PodUIDs                   []string
	Containers                []string
	CPULimitMilli             *float64
	MemoryLimitBytes          *float64
}

func (t Target) Validate() error {
	if t.ValidateDesired() != nil || t.UID == "" {
		return ErrInvalid
	}
	return nil
}

// ValidateDesired checks an admitted controller before Kubernetes assigns its UID.
func (t Target) ValidateDesired() error {
	if t.Scope.Validate() != nil || !namePattern.MatchString(t.Workload) || !namePattern.MatchString(t.Name) || (t.Kind != "Deployment" && t.Kind != "StatefulSet") {
		return ErrInvalid
	}
	return nil
}

type RulesConfig struct {
	NotificationLabels map[string]string `json:"notificationLabels,omitempty"`
	RestartWindow      time.Duration     `json:"restartWindow"`
	RestartThreshold   int               `json:"restartThreshold"`
	AvailabilityFor    time.Duration     `json:"availabilityFor"`
	RestartFor         time.Duration     `json:"restartFor"`
	OOMFor             time.Duration     `json:"oomFor"`
	Severity           string            `json:"severity"`
	Provenance         string            `json:"provenance"`
}

func DevRules() RulesConfig {
	return RulesConfig{RestartWindow: 5 * time.Minute, RestartThreshold: 3, AvailabilityFor: 5 * time.Minute, RestartFor: time.Minute, OOMFor: time.Minute, Severity: "warning", Provenance: "development-example-v1", NotificationLabels: map[string]string{"orchestrator_test": "true"}}
}
func (c RulesConfig) Validate() error {
	if len(c.NotificationLabels) > 16 {
		return ErrInvalid
	}
	for k, v := range c.NotificationLabels {
		if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(k) || len(v) > 256 || strings.HasPrefix(k, "orchestrator_") && k != "orchestrator_test" || k == "severity" || k == "alertname" {
			return ErrInvalid
		}
	}
	if c.RestartWindow < time.Minute || c.RestartWindow > 24*time.Hour || c.RestartWindow%time.Second != 0 || c.RestartThreshold < 1 || c.RestartThreshold > 10000 || c.Provenance == "" || (c.Severity != "warning" && c.Severity != "critical") {
		return ErrInvalid
	}
	for _, d := range []time.Duration{c.AvailabilityFor, c.RestartFor, c.OOMFor} {
		if d < 0 || d > 24*time.Hour || d%time.Second != 0 {
			return ErrInvalid
		}
	}
	return nil
}
func RuleName(t Target) string {
	sum := sha256.Sum256([]byte(t.Scope.Organization + "\x00" + t.Scope.Application + "\x00" + t.Scope.Environment + "\x00" + strconv.FormatInt(t.Scope.Generation, 10) + "\x00" + t.Scope.ClusterUID + "\x00" + t.Workload))
	return "orch-monitor-" + hex.EncodeToString(sum[:12])
}

// Connection stores only nonsecret metadata and a private opaque reference.
type CredentialReceipt struct {
	Key       string `json:"key"`
	Reference string `json:"reference"`
}
type Connection struct {
	Deleting           bool                `json:"deleting,omitempty"`
	RetiredCredentials []CredentialReceipt `json:"retiredCredentials,omitempty"`
	Profile            Profile             `json:"profile"`
	ID                 string              `json:"id"`
	Organization       string              `json:"organization"`
	Key                string              `json:"key"`
	Name               string              `json:"name"`
	Revision           int64               `json:"revision"`
	LokiRetentionDays  int                 `json:"lokiRetentionDays,omitempty"`
	LokiURL            string              `json:"lokiUrl,omitempty"`
	CredentialRefs     map[string]string   `json:"credentialRefs,omitempty"`
	Verification       Verification        `json:"verification"`
	PrometheusURL      string              `json:"prometheusUrl"`
	AlertmanagerURL    string              `json:"alertmanagerUrl"`
	GrafanaURL         string              `json:"grafanaUrl"`
	DashboardUID       string              `json:"dashboardUid"`
	CredentialRef      string              `json:"credentialRef,omitempty"`
	CA                 string              `json:"ca,omitempty"`
	ClusterUID         string              `json:"clusterUid"`
	RuleLabels         map[string]string   `json:"ruleLabels"`
	Rules              RulesConfig         `json:"rules"`
	VerifiedAt         *time.Time          `json:"verifiedAt,omitempty"`
}
type Binding struct {
	ProfileVersion     string                     `json:"profileVersion,omitempty"`
	WorkloadProfiles   map[string]WorkloadProfile `json:"workloadProfiles,omitempty"`
	Organization       string                     `json:"organization"`
	Application        string                     `json:"application"`
	Environment        string                     `json:"environment"`
	ConnectionID       string                     `json:"connectionId"`
	ConnectionRevision int64                      `json:"connectionRevision"`
	ClusterUID         string                     `json:"clusterUid"`
	Generation         int64                      `json:"generation"`
	Revision           int64                      `json:"revision"`
	LogsSource         string                     `json:"logsSource,omitempty"`
	MetricsSource      string                     `json:"metricsSource,omitempty"`
}

// Verification is operational evidence for a configuration revision, not a new config.
type Verification struct {
	Revision     int64           `json:"revision"`
	Connectivity map[string]bool `json:"connectivity"`
	Identity     map[string]bool `json:"identity"`
	Coverage     map[string]bool `json:"coverage"`
	ConfigSync   string          `json:"configSync"`
	CheckedAt    *time.Time      `json:"checkedAt,omitempty"`
}

func (c Connection) Credential(provider string) (string, string) {
	if ref, ok := c.CredentialRefs[provider]; ok {
		return c.Key + "-" + provider, ref
	}
	if provider != "loki" {
		return c.Key, c.CredentialRef
	}
	return c.Key + "-" + provider, ""
}

func CloneConnection(c Connection) Connection {
	c.RetiredCredentials = append([]CredentialReceipt(nil), c.RetiredCredentials...)
	b, _ := json.Marshal(c)
	var out Connection
	_ = json.Unmarshal(b, &out)
	return out
}

func ValidateRuleLabels(labels map[string]string) error {
	if len(labels) > 32 {
		return ErrInvalid
	}
	for k, v := range labels {
		if k == "" || len(k) > 253 || len(v) > 63 || strings.HasPrefix(k, "orchestrator") || k == "app.kubernetes.io/managed-by" || k == "app.kubernetes.io/name" {
			return ErrInvalid
		}
		if v != "" && !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_.-]*[a-zA-Z0-9])?$`).MatchString(v) {
			return ErrInvalid
		}
		parts := strings.Split(k, "/")
		if (len(parts) == 2 && !namePattern.MatchString(parts[0])) || len(parts) > 2 || !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_.-]*[a-zA-Z0-9])?$`).MatchString(parts[len(parts)-1]) {
			return ErrInvalid
		}
	}
	return nil
}
