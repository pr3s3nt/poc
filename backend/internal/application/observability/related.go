package observability

import (
	"context"
	"errors"
	"net/url"
	"orchestrator/internal/adapters/prometheus"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"sort"
	"time"
)

// Identity is resolved by the existing scoped State resource read. No provider
// state, credential, host or client-supplied ownership label enters this view.
type ResourceIdentity struct {
	ID, Name, Type, Status string
	Generation             int64
}
type RelatedObservation struct {
	Source            string                       `json:"source,omitempty"`
	ID                string                       `json:"id"`
	Name              string                       `json:"name"`
	Kind              string                       `json:"kind"`
	Type              string                       `json:"type,omitempty"`
	Provenance        string                       `json:"provenance"`
	CollectorWorkload string                       `json:"collectorWorkload"`
	State             string                       `json:"state"`
	Configuration     string                       `json:"configuration"`
	ProfileVersion    string                       `json:"profileVersion,omitempty"`
	MetricsAt         *time.Time                   `json:"metricsAt,omitempty"`
	From              time.Time                    `json:"from"`
	To                time.Time                    `json:"to"`
	Metrics           map[string]MetricObservation `json:"metrics"`
	Alerts            []prometheus.Alert           `json:"alerts"`
	GrafanaURL        string                       `json:"grafanaUrl,omitempty"`
}

func (m *Monitoring) validateObservationMappings(ctx context.Context, scope telemetry.Scope, generation int64, c monitoring.Connection, profiles map[string]monitoring.WorkloadProfile, resources, dependencies map[string]monitoring.ObservationMapping, external bool) error {
	if len(resources) > 8 || len(dependencies) > 8 {
		return monitoring.ErrInvalid
	}
	for kind, mappings := range map[string]map[string]monitoring.ObservationMapping{"resource": resources, "dependency": dependencies} {
		for id, mapping := range mappings {
			if (kind == "resource" && !ids.ValidUUID(id)) || (kind == "dependency" && !workloadShape.MatchString(id)) || !workloadShape.MatchString(mapping.CollectorWorkload) || len(mapping.MetricKeys) < 1 || len(mapping.MetricKeys) > 8 {
				return monitoring.ErrInvalid
			}
			seen := map[string]bool{}
			for _, key := range mapping.MetricKeys {
				if key == "" || len(key) > 100 || seen[key] {
					return monitoring.ErrInvalid
				}
				seen[key] = true
			}
			// A native withdrawal may retain obsolete mappings without consulting an
			// external source or blocking cleanup of its previously owned artifacts.
			if !external {
				continue
			}
			profile := c.Profile.Defaults
			if override, ok := profiles[mapping.CollectorWorkload]; ok {
				profile = &override
			}
			if profile == nil {
				return monitoring.ErrInvalid
			}
			known := map[string]bool{}
			for _, metric := range profile.Metrics {
				known[metric.Key] = true
			}
			for key := range seen {
				if !known[key] {
					return monitoring.ErrInvalid
				}
			}
			if kind == "resource" {
				if m.ResourceIdentity == nil {
					return ErrUnavailable
				}
				resource, e := m.ResourceIdentity(ctx, scope, id)
				if e != nil {
					return e
				}
				if resource.ID != id || resource.Generation != generation || resource.Status == "DESTROYED" {
					return persistence.ErrNotFound
				}
			}
		}
	}
	return nil
}
func projectObservation(id, kind string, mapping monitoring.ObservationMapping, view MonitoringView) RelatedObservation {
	out := RelatedObservation{Source: view.Source, ID: id, Name: id, Kind: kind, Provenance: "DECLARED_MAPPING", CollectorWorkload: mapping.CollectorWorkload, State: "NOT_DEPLOYED", Configuration: "UNVERIFIED", From: view.From, To: view.To, Metrics: map[string]MetricObservation{}, Alerts: []prometheus.Alert{}, GrafanaURL: view.GrafanaURL}
	if out.GrafanaURL != "" {
		if u, e := url.Parse(out.GrafanaURL); e == nil {
			q := u.Query()
			q.Set("var-workload", mapping.CollectorWorkload)
			u.RawQuery = q.Encode()
			out.GrafanaURL = u.String()
		}
	}
	for _, workload := range view.Workloads {
		if workload.Workload != mapping.CollectorWorkload {
			continue
		}
		out.Configuration = workload.Configuration
		out.ProfileVersion = workload.ProfileVersion
		out.MetricsAt = workload.MetricsAt
		out.State = "RECEIVING"
		selected := map[string]bool{}
		for _, key := range mapping.MetricKeys {
			selected[key] = true
			metric, ok := workload.ApplicationMetrics[key]
			if !ok {
				metric = MetricObservation{State: "MISSING", Series: []prometheus.Series{}}
			}
			out.Metrics[key] = metric
			if metric.State != "RECEIVING" {
				out.State = "PARTIAL"
			}
		}
		if workload.State == "STALE" {
			out.State = "STALE"
		}
		if out.Configuration != "APPLIED" {
			out.State = "PARTIAL"
		}
		for _, alert := range view.Alerts {
			if alert.Labels["orchestrator_workload"] == mapping.CollectorWorkload && selected[alert.Labels["metric_key"]] {
				out.Alerts = append(out.Alerts, alert)
			}
		}
		break
	}
	return out
}
func sortedMappings(mappings map[string]monitoring.ObservationMapping) []string {
	keys := make([]string, 0, len(mappings))
	for key := range mappings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func (m *Monitoring) projectRelated(ctx context.Context, scope telemetry.Scope, b monitoring.Binding, out *MonitoringView) {
	out.Resources = []RelatedObservation{}
	out.Dependencies = []RelatedObservation{}
	for _, id := range sortedMappings(b.ResourceObservations) {
		item := projectObservation(id, "resource", b.ResourceObservations[id], *out)
		if m.ResourceIdentity == nil {
			item.State = "UNAVAILABLE"
		} else {
			resource, e := m.ResourceIdentity(ctx, scope, id)
			if e != nil || resource.ID != id || resource.Generation != b.Generation || resource.Status == "DESTROYED" {
				item.State = "UNAVAILABLE"
				item.Metrics = map[string]MetricObservation{}
				item.Alerts = []prometheus.Alert{}
			} else {
				item.Name = resource.Name
				item.Type = resource.Type
			}
		}
		if item.State == "UNAVAILABLE" {
			item.Metrics = map[string]MetricObservation{}
			item.Alerts = []prometheus.Alert{}
			item.GrafanaURL = ""
		}
		out.Resources = append(out.Resources, item)
	}
	for _, id := range sortedMappings(b.DependencyObservations) {
		out.Dependencies = append(out.Dependencies, projectObservation(id, "dependency", b.DependencyObservations[id], *out))
	}
}
func (m *Monitoring) ReadResource(ctx context.Context, scope telemetry.Scope, id string, p Params) (RelatedObservation, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out := RelatedObservation{ID: id, Kind: "resource", State: "NOT_CONFIGURED", Configuration: "UNVERIFIED", Metrics: map[string]MetricObservation{}, Alerts: []prometheus.Alert{}}
	if !ids.ValidUUID(id) {
		return out, persistence.ErrNotFound
	}
	if m.ResourceIdentity == nil || m.Repo == nil {
		return out, ErrUnavailable
	}
	resource, e := m.ResourceIdentity(ctx, scope, id)
	if e != nil {
		return out, e
	}
	if resource.ID != id || resource.Status == "DESTROYED" {
		return out, persistence.ErrNotFound
	}
	out.Name = resource.Name
	out.Type = resource.Type
	env, e := m.Store.GetEnvironment(ctx, scope.ApplicationKey, scope.EnvironmentKey)
	if e != nil {
		return out, e
	}
	if resource.Generation != env.TargetGeneration {
		return out, persistence.ErrNotFound
	}
	b, e := m.Repo.GetMonitoringBinding(ctx, scope.OrganizationKey, scope.ApplicationKey, scope.EnvironmentKey)
	if errors.Is(e, persistence.ErrNotFound) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	mapping, ok := b.ResourceObservations[id]
	if !ok || b.MetricsSource == "native" {
		return out, nil
	}
	p.Workload = mapping.CollectorWorkload
	view, e := m.Read(ctx, scope, p)
	if e != nil {
		return out, e
	}
	out = projectObservation(id, "resource", mapping, view)
	out.Name = resource.Name
	out.Type = resource.Type
	return out, nil
}
