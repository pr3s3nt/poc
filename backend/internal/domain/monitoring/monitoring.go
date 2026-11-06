// Package monitoring owns identities and configuration for external observation.
package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("monitoring: invalid configuration")
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
}

func (t Target) Validate() error {
	if t.Scope.Validate() != nil || !namePattern.MatchString(t.Workload) || !namePattern.MatchString(t.Name) || t.UID == "" || (t.Kind != "Deployment" && t.Kind != "StatefulSet") {
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
type Connection struct {
	ID              string            `json:"id"`
	Organization    string            `json:"organization"`
	Key             string            `json:"key"`
	Name            string            `json:"name"`
	Revision        int64             `json:"revision"`
	PrometheusURL   string            `json:"prometheusUrl"`
	AlertmanagerURL string            `json:"alertmanagerUrl"`
	GrafanaURL      string            `json:"grafanaUrl"`
	DashboardUID    string            `json:"dashboardUid"`
	CredentialRef   string            `json:"credentialRef,omitempty"`
	CA              string            `json:"ca,omitempty"`
	ClusterUID      string            `json:"clusterUid"`
	RuleLabels      map[string]string `json:"ruleLabels"`
	Rules           RulesConfig       `json:"rules"`
	VerifiedAt      *time.Time        `json:"verifiedAt,omitempty"`
}
type Binding struct {
	Organization       string `json:"organization"`
	Application        string `json:"application"`
	Environment        string `json:"environment"`
	ConnectionID       string `json:"connectionId"`
	ConnectionRevision int64  `json:"connectionRevision"`
	ClusterUID         string `json:"clusterUid"`
	Generation         int64  `json:"generation"`
	Revision           int64  `json:"revision"`
}
