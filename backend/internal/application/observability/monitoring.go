package observability

import (
	"context"
	"errors"
	"orchestrator/internal/adapters/prometheus"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
	"orchestrator/internal/ports/persistence"
	"strconv"
	"strings"
	"time"
)

// Monitoring keeps external reads independent of native stored log/history reads.
type Monitoring struct {
	Native      *Service
	Store       persistence.Store
	Repo        persistence.MonitoringRepository
	Credentials credentials.Store
}

func NewMonitoring(native *Service, st persistence.Store, creds credentials.Store) *Monitoring {
	repo, _ := st.(persistence.MonitoringRepository)
	return &Monitoring{Native: native, Store: st, Repo: repo, Credentials: creds}
}
func (m *Monitoring) client(ctx context.Context, c monitoring.Connection, endpoint string) (*prometheus.Client, error) {
	token := ""
	if c.CredentialRef != "" {
		if m.Credentials == nil {
			return nil, ErrUnavailable
		}
		b, e := m.Credentials.Get(ctx, c.Organization, c.Key, c.CredentialRef)
		if e != nil {
			return nil, ErrUnavailable
		}
		token = string(b)
	}
	return prometheus.New(prometheus.Config{Endpoint: endpoint, CA: []byte(c.CA), BearerToken: token})
}
func (m *Monitoring) Connections(ctx context.Context, org string) ([]monitoring.Connection, error) {
	if m.Repo == nil {
		return nil, ErrUnavailable
	}
	return m.Repo.ListMonitoringConnections(ctx, org)
}
func (m *Monitoring) Save(ctx context.Context, org string, c monitoring.Connection, expected int64, token string) (monitoring.Connection, error) {
	if m.Repo == nil {
		return c, ErrUnavailable
	}
	c.Organization = org
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 120 || c.Rules.Validate() != nil || c.ClusterUID == "" || len(c.CA) > 1<<20 || len(token) > 8192 {
		return c, monitoring.ErrInvalid
	}
	for _, endpoint := range []string{c.PrometheusURL, c.AlertmanagerURL, c.GrafanaURL} {
		if _, e := prometheus.Endpoint(endpoint); e != nil {
			return c, monitoring.ErrInvalid
		}
	}
	if expected == 0 {
		c.CredentialRef = ""
		c.ID = ids.New()
		c.Key = "monitor-" + c.ID
	} else {
		prior, e := m.Repo.GetMonitoringConnection(ctx, org, c.ID)
		if e != nil {
			return c, e
		}
		if prior.Revision != expected {
			return c, persistence.ErrVersionConflict
		}
		c.Key = prior.Key
		c.CredentialRef = prior.CredentialRef
	}
	c.Revision = expected + 1
	c.VerifiedAt = nil
	// Verification is scoped to endpoint/credential/cluster configuration revision.
	newCredential := ""
	if token != "" {
		if m.Credentials == nil || !m.Credentials.Durable() {
			return c, ErrUnavailable
		}
		ref, e := m.Credentials.Put(ctx, org, c.Key, []byte(token))
		if e != nil {
			return c, ErrUnavailable
		}
		c.CredentialRef = ref
		newCredential = ref
	}
	// On an uncertain commit keep private credentials for operator reconciliation.
	e := m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.PutMonitoringConnection(ctx, c, expected) })
	if e != nil {
		if newCredential != "" && !errors.Is(e, persistence.ErrCommitUncertain) {
			_ = m.Credentials.Delete(context.WithoutCancel(ctx), org, c.Key, newCredential)
		}
		return c, e
	}
	return c, nil
}

type Coverage struct {
	Connectivity bool   `json:"connectivity"`
	Alertmanager bool   `json:"alertmanager"`
	Metrics      bool   `json:"metrics"`
	Cluster      bool   `json:"cluster"`
	ConfigSync   string `json:"configSync"`
}

func (m *Monitoring) Test(ctx context.Context, org, id string) (Coverage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out := Coverage{ConfigSync: "UNVERIFIED"}
	if m.Repo == nil {
		return out, ErrUnavailable
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, org, id)
	if e != nil {
		return out, e
	}
	cl, e := m.client(ctx, c, c.PrometheusURL)
	if e != nil {
		return out, e
	}
	defer cl.Close()
	_, e = cl.Instant(ctx, "vector(1)", time.Now())
	if e != nil {
		return out, e
	}
	out.Connectivity = true
	am, e := m.client(ctx, c, c.AlertmanagerURL)
	if e != nil {
		return out, e
	}
	defer am.Close()
	if e = am.AlertmanagerStatus(ctx); e != nil {
		return out, e
	}
	out.Alertmanager = true
	series, e := cl.Instant(ctx, `count by (__name__) ({__name__=~"kube_pod_info|kube_pod_container_status_restarts_total|container_cpu_usage_seconds_total|container_memory_working_set_bytes"})`, time.Now())
	if e != nil {
		return out, e
	}
	out.Metrics = len(series) == 4
	// A setup-owned identity recording rule is required; never infer physical
	// identity from an endpoint hostname or namespace alone.
	identity, e := cl.Instant(ctx, `orchestrator_cluster_identity{cluster_uid=`+quote(c.ClusterUID)+`}`, time.Now())
	if e != nil {
		return out, e
	}
	out.Cluster = len(identity) == 1 && identity[0].Value != nil && identity[0].Value.Value != nil && *identity[0].Value.Value == 1
	if out.Metrics && out.Cluster {
		at := time.Now().UTC()
		c.VerifiedAt = &at
		c.Revision++
		e = m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.PutMonitoringConnection(ctx, c, c.Revision-1) })
	}
	return out, e
}
func quote(s string) string { // PromQL string escaping is JSON/Go compatible for identity strings.
	return strconv.Quote(s)
}
func (m *Monitoring) Bind(ctx context.Context, scope telemetry.Scope, id string, expected int64) (monitoring.Binding, error) {
	var out monitoring.Binding
	if m.Repo == nil || m.Native == nil {
		return out, ErrUnavailable
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, scope.OrganizationKey, id)
	if e != nil {
		return out, e
	}
	if c.VerifiedAt == nil {
		return out, monitoring.ErrInvalid
	}
	app, e := m.Store.GetApplication(ctx, scope.ApplicationKey)
	if e != nil || app.OrganizationKey != scope.OrganizationKey {
		return out, persistence.ErrNotFound
	}
	env, e := m.Store.GetEnvironment(ctx, scope.ApplicationKey, scope.EnvironmentKey)
	if e != nil {
		return out, e
	}
	targets, e := m.targets(ctx, scope)
	if e != nil {
		return out, e
	}
	if len(targets) == 0 {
		return out, monitoring.ErrInvalid
	}
	for _, t := range targets {
		if t.Scope.ClusterUID != c.ClusterUID {
			return out, monitoring.ErrInvalid
		}
	}
	out = monitoring.Binding{Organization: scope.OrganizationKey, Application: scope.ApplicationKey, Environment: scope.EnvironmentKey, ConnectionID: id, ConnectionRevision: c.Revision, ClusterUID: c.ClusterUID, Generation: env.TargetGeneration, Revision: expected + 1}
	e = m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.PutMonitoringBinding(ctx, out, expected) })
	return out, e
}
func (m *Monitoring) targets(ctx context.Context, scope telemetry.Scope) ([]monitoring.Target, error) {
	verifier, ok := m.Native.reader.(obs.TargetVerifier)
	if !ok {
		return nil, ErrUnavailable
	}
	sources, e := m.Native.discover(ctx)
	if e != nil {
		return nil, e
	}
	out := []monitoring.Target{}
	for _, s := range sources {
		if s.OrganizationKey != scope.OrganizationKey || s.ApplicationKey != scope.ApplicationKey || s.EnvironmentKey != scope.EnvironmentKey {
			continue
		}
		if s.MonitoringController == nil {
			continue
		}
		t, e := verifier.MonitoringTarget(ctx, s)
		if e != nil {
			return nil, ErrUnavailable
		}
		out = append(out, t)
	}
	return out, nil
}

type WorkloadObservation struct {
	Workload  string                         `json:"workload"`
	Kind      string                         `json:"kind"`
	UID       string                         `json:"uid"`
	State     string                         `json:"state"`
	MetricsAt *time.Time                     `json:"metricsAt,omitempty"`
	Metrics   map[string][]prometheus.Series `json:"metrics"`
}
type MonitoringView struct {
	Source     string                `json:"source"`
	State      string                `json:"state"`
	From       time.Time             `json:"from"`
	To         time.Time             `json:"to"`
	Workloads  []WorkloadObservation `json:"workloads"`
	Alerts     []prometheus.Alert    `json:"alerts"`
	GrafanaURL string                `json:"grafanaUrl,omitempty"`
}

func (m *Monitoring) Read(ctx context.Context, scope telemetry.Scope, p Params) (MonitoringView, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out := MonitoringView{Source: "prometheus", State: "NOT_CONFIGURED", Workloads: []WorkloadObservation{}}
	if m.Repo == nil || m.Native == nil {
		return out, ErrUnavailable
	}
	app, e := m.Store.GetApplication(ctx, scope.ApplicationKey)
	if e != nil || app.OrganizationKey != scope.OrganizationKey {
		return out, persistence.ErrNotFound
	}
	env, e := m.Store.GetEnvironment(ctx, scope.ApplicationKey, scope.EnvironmentKey)
	if e != nil {
		return out, e
	}
	b, e := m.Repo.GetMonitoringBinding(ctx, scope.OrganizationKey, scope.ApplicationKey, scope.EnvironmentKey)
	if errors.Is(e, persistence.ErrNotFound) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	if b.Generation != env.TargetGeneration {
		return out, monitoring.ErrInvalid
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, scope.OrganizationKey, b.ConnectionID)
	if e != nil {
		return out, e
	}
	if c.Revision != b.ConnectionRevision {
		return out, persistence.ErrVersionConflict
	}
	now := time.Now().UTC()
	out.From = now.Add(-time.Hour)
	out.To = now
	if p.From != "" {
		out.From, e = time.Parse(time.RFC3339, p.From)
		if e != nil {
			return out, &FieldError{"from"}
		}
	}
	if p.To != "" {
		out.To, e = time.Parse(time.RFC3339, p.To)
		if e != nil {
			return out, &FieldError{"to"}
		}
	}
	if !out.From.Before(out.To) || out.From.Before(now.Add(-15*24*time.Hour)) || out.To.After(now.Add(time.Minute)) {
		return out, &FieldError{"from"}
	}
	step := max(30*time.Second, time.Duration((out.To.Sub(out.From)/time.Second+238)/239)*time.Second)
	cl, e := m.client(ctx, c, c.PrometheusURL)
	if e != nil {
		return out, e
	}
	defer cl.Close()
	targets, e := m.targets(ctx, scope)
	if e != nil {
		return out, e
	}
	out.State = "NOT_DEPLOYED"
	for _, t := range targets {
		if t.Scope.ClusterUID != b.ClusterUID {
			return out, monitoring.ErrInvalid
		}
		if p.Workload != "" && p.Workload != t.Workload {
			continue
		}
		queries, e := prometheus.WorkloadQueries(t)
		if e != nil {
			return out, e
		}
		w := WorkloadObservation{Workload: t.Workload, Kind: t.Kind, UID: t.UID, State: "RECEIVING", Metrics: map[string][]prometheus.Series{}}
		for _, key := range []string{"cpu", "memory", "restarts", "ready", "desired", "oom"} {
			var data []prometheus.Series
			if key == "cpu" || key == "memory" {
				data, e = cl.Range(ctx, queries[key], out.From, out.To, step)
			} else {
				data, e = cl.Instant(ctx, queries[key], out.To)
			}
			if e != nil {
				return out, e
			}
			w.Metrics[key] = data
			if len(data) == 0 {
				w.State = "PARTIAL"
			}
			for _, series := range data {
				if series.Value != nil && series.Value.Timestamp.Before(now.Add(-90*time.Second)) {
					w.State = "STALE"
				}
			}
		}
		source, e := cl.Instant(ctx, queries["sourceTimestamp"], out.To)
		if e != nil {
			return out, e
		}
		if len(source) != 1 || source[0].Value == nil || source[0].Value.Value == nil {
			w.State = "PARTIAL"
		} else {
			stamp := time.Unix(int64(*source[0].Value.Value), 0).UTC()
			w.MetricsAt = &stamp
			if stamp.Before(now.Add(-90 * time.Second)) {
				w.State = "STALE"
			}
		}
		out.Workloads = append(out.Workloads, w)
		if out.State == "NOT_DEPLOYED" || w.State != "RECEIVING" {
			out.State = w.State
		}
	}
	out.Alerts, e = cl.Alerts(ctx, map[string]string{"orchestrator_org": scope.OrganizationKey, "orchestrator_app": scope.ApplicationKey, "orchestrator_env": scope.EnvironmentKey, "orchestrator_generation": strconv.FormatInt(b.Generation, 10), "orchestrator_cluster_uid": b.ClusterUID})
	if e != nil {
		return out, e
	}
	out.GrafanaURL = c.GrafanaURL
	return out, nil
}

// PlanRules adds desired app-owned rules to the existing fenced delivery request.
// Scope comes from deployment, never from Console selectors. Unbound apps retain
// their existing artifacts exactly.
func (m *Monitoring) PlanRules(ctx context.Context, org, app string, env environment.Environment, target execution.Target, workload string, manifests []execution.Manifest) ([]execution.Manifest, error) {
	if m.Repo == nil {
		return manifests, nil
	}
	b, e := m.Repo.GetMonitoringBinding(ctx, org, app, env.Key)
	if errors.Is(e, persistence.ErrNotFound) {
		return manifests, nil
	}
	if e != nil {
		return nil, e
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, org, b.ConnectionID)
	if e != nil {
		return nil, e
	}
	if c.Revision != b.ConnectionRevision || b.Generation != env.TargetGeneration || c.VerifiedAt == nil {
		return nil, monitoring.ErrInvalid
	}
	// A bound environment's existing physical controller is independently verified
	// before publishing rules. New controller support must establish the same pin.
	targets, e := m.targets(ctx, telemetry.Scope{OrganizationKey: org, ApplicationKey: app, EnvironmentKey: env.Key})
	if e != nil {
		return nil, e
	}
	var verified *monitoring.Target
	for i := range targets {
		if targets[i].Workload == workload && targets[i].Scope.Namespace == target.Namespace && targets[i].Scope.ClusterUID == b.ClusterUID {
			verified = &targets[i]
			break
		}
	}
	for _, manifest := range manifests {
		if manifest.Kind != "Deployment" && manifest.Kind != "StatefulSet" {
			continue
		}
		if verified == nil || verified.Kind != manifest.Kind || verified.Name != manifest.Name {
			return nil, monitoring.ErrInvalid
		}
		doc, e := prometheus.RuleDocument(*verified, c.Rules)
		if e != nil {
			return nil, e
		}
		md := doc["metadata"].(map[string]any)
		labels := map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "app.kubernetes.io/name": workload, "orchestrator.io/application": app, "orchestrator.io/environment": env.Key}
		for k, v := range c.RuleLabels {
			labels[k] = v
		}
		md["labels"] = labels
		manifests = append(manifests, execution.Manifest{APIVersion: "monitoring.coreos.com/v1", Kind: "PrometheusRule", Name: monitoring.RuleName(*verified), Namespace: target.Namespace, Object: doc})
	}
	return manifests, nil
}
