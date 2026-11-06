package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
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
func (m *Monitoring) client(ctx context.Context, c monitoring.Connection, provider string) (*prometheus.Client, error) {
	endpoint := ""
	switch provider {
	case "prometheus":
		endpoint = c.PrometheusURL
	case "alertmanager":
		endpoint = c.AlertmanagerURL
	case "loki":
		endpoint = c.LokiURL
	default:
		return nil, monitoring.ErrInvalid
	}
	token := ""
	key, ref := c.Credential(provider)
	if ref != "" {
		if m.Credentials == nil {
			return nil, ErrUnavailable
		}
		b, e := m.Credentials.Get(ctx, c.Organization, key, ref)
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
	return m.SaveWithTokens(ctx, org, c, expected, token, nil)
}
func (m *Monitoring) SaveWithTokens(ctx context.Context, org string, c monitoring.Connection, expected int64, token string, tokens map[string]string) (monitoring.Connection, error) {
	if m.Repo == nil {
		return c, ErrUnavailable
	}
	c.Organization = org
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 120 || c.Rules.Validate() != nil || c.Profile.Validate() != nil || c.ClusterUID == "" || (c.PrometheusURL == "" && c.LokiURL == "") || monitoring.ValidateRuleLabels(c.RuleLabels) != nil || len(c.CA) > 1<<20 || len(token) > 8192 {
		return c, monitoring.ErrInvalid
	}
	for _, endpoint := range []string{c.PrometheusURL, c.AlertmanagerURL, c.GrafanaURL, c.LokiURL} {
		if endpoint == "" {
			continue
		}
		if _, e := prometheus.Endpoint(endpoint); e != nil {
			return c, monitoring.ErrInvalid
		}
	}
	if token != "" && len(tokens) > 0 {
		return c, monitoring.ErrInvalid
	}
	for provider, value := range tokens {
		if (provider != "prometheus" && provider != "alertmanager" && provider != "loki") || len(value) > 8192 || strings.ContainsAny(value, "\r\n") {
			return c, monitoring.ErrInvalid
		}
	}
	if expected == 0 {
		c.CredentialRef = ""
		c.CredentialRefs = map[string]string{}
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
		c.CredentialRefs = monitoring.CloneConnection(prior).CredentialRefs
		if c.CredentialRefs == nil {
			c.CredentialRefs = map[string]string{}
		}
	}
	c.Revision = expected + 1
	c.VerifiedAt = nil
	c.Verification = monitoring.Verification{Revision: c.Revision, ConfigSync: "UNVERIFIED"}
	// Verification is scoped to endpoint/credential/cluster configuration revision.
	newCredential := ""
	if token != "" {
		if m.Credentials == nil || !m.Credentials.Durable() {
			return c, ErrUnavailable
		}
		ref, e := m.Credentials.Put(ctx, org, c.Key, []byte(token))
		if e != nil {
			if ref != "" {
				_ = m.Credentials.Delete(context.WithoutCancel(ctx), org, c.Key, ref)
			}
			return c, ErrUnavailable
		}
		c.CredentialRef = ref
		newCredential = ref
	}
	type privateRef struct{ key, ref string }
	created := []privateRef{}
	for provider, value := range tokens {
		if value == "" {
			c.CredentialRefs[provider] = ""
			continue
		}
		if m.Credentials == nil || !m.Credentials.Durable() {
			return c, ErrUnavailable
		}
		providerKey := c.Key + "-" + provider
		ref, e := m.Credentials.Put(ctx, org, providerKey, []byte(value))
		if e != nil {
			if ref != "" {
				_ = m.Credentials.Delete(context.WithoutCancel(ctx), org, providerKey, ref)
			}
			for _, item := range created {
				_ = m.Credentials.Delete(context.WithoutCancel(ctx), org, item.key, item.ref)
			}
			return c, ErrUnavailable
		}
		c.CredentialRefs[provider] = ref
		created = append(created, privateRef{providerKey, ref})
	}
	// On an uncertain commit keep private credentials for operator reconciliation.
	e := m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.PutMonitoringConnection(ctx, c, expected) })
	if e != nil {
		if !errors.Is(e, persistence.ErrCommitUncertain) {
			for _, item := range created {
				_ = m.Credentials.Delete(context.WithoutCancel(ctx), org, item.key, item.ref)
			}
		}
		if newCredential != "" && !errors.Is(e, persistence.ErrCommitUncertain) {
			_ = m.Credentials.Delete(context.WithoutCancel(ctx), org, c.Key, newCredential)
		}
		return c, e
	}
	return c, nil
}

type Coverage struct {
	Connectivity bool                    `json:"connectivity"`
	Alertmanager bool                    `json:"alertmanager"`
	Metrics      bool                    `json:"metrics"`
	Cluster      bool                    `json:"cluster"`
	ConfigSync   string                  `json:"configSync"`
	Verification monitoring.Verification `json:"verification"`
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
	v := monitoring.Verification{Revision: c.Revision, Connectivity: map[string]bool{}, Identity: map[string]bool{}, Coverage: map[string]bool{}, ConfigSync: "UNVERIFIED"}
	allConnected := true
	identityCount := 0
	allIdentity := true
	if c.PrometheusURL != "" {
		cl, err := m.client(ctx, c, "prometheus")
		if err == nil {
			defer cl.Close()
			_, err = cl.Instant(ctx, "vector(1)", time.Now())
			v.Connectivity["prometheus"] = err == nil
			if err == nil {
				facts, readErr := cl.Instant(ctx, `count by (__name__) ({__name__=~"kube_pod_info|kube_pod_container_status_restarts_total|container_cpu_usage_seconds_total|container_memory_working_set_bytes",cluster_uid=`+quote(c.ClusterUID)+`})`, time.Now())
				v.Coverage["kubernetes"] = readErr == nil && kubernetesCoverage(facts)
				identity, readErr := cl.Instant(ctx, `orchestrator_cluster_identity`, time.Now())
				v.Identity["prometheus"] = readErr == nil && len(identity) == 1 && identity[0].Labels["cluster_uid"] == c.ClusterUID && identity[0].Value != nil && identity[0].Value.Value != nil && *identity[0].Value.Value == 1
			}
		}
		allConnected = allConnected && v.Connectivity["prometheus"]
		allIdentity = allIdentity && v.Identity["prometheus"]
		identityCount++
	}
	if c.AlertmanagerURL != "" {
		cl, err := m.client(ctx, c, "alertmanager")
		if err == nil {
			defer cl.Close()
			err = cl.AlertmanagerStatus(ctx)
			v.Connectivity["alertmanager"] = err == nil
		}
		allConnected = allConnected && v.Connectivity["alertmanager"]
	}
	if c.LokiURL != "" {
		cl, err := m.client(ctx, c, "loki")
		if err == nil {
			defer cl.Close()
			var result struct {
				Status string   `json:"status"`
				Data   []string `json:"data"`
			}
			err = cl.ReadJSON(ctx, "/loki/api/v1/label/cluster_uid/values", nil, &result)
			v.Connectivity["loki"] = err == nil && result.Status == "success"
			if v.Connectivity["loki"] {
				for _, uid := range result.Data {
					if uid == c.ClusterUID {
						v.Identity["loki"] = true
						v.Coverage["logs"] = true
					}
				}
			}
		}
		allConnected = allConnected && v.Connectivity["loki"]
		allIdentity = allIdentity && v.Identity["loki"]
		identityCount++
	}
	now := time.Now().UTC()
	v.CheckedAt = &now
	c.Verification = v
	c.VerifiedAt = nil
	if allConnected && allIdentity && identityCount > 0 {
		c.VerifiedAt = &now
	}
	e = m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.VerifyMonitoringConnection(ctx, c, c.Revision) })
	out.Connectivity = allConnected
	out.Metrics = v.Coverage["kubernetes"]
	out.Cluster = allIdentity && identityCount > 0
	out.Alertmanager = v.Connectivity["alertmanager"]
	out.Verification = v
	return out, e
}

func quote(s string) string { // PromQL string escaping is JSON/Go compatible for identity strings.
	return strconv.Quote(s)
}
func (m *Monitoring) Bind(ctx context.Context, scope telemetry.Scope, id string, expected int64) (monitoring.Binding, error) {
	return m.BindSources(ctx, scope, id, expected, "", "")
}
func (m *Monitoring) BindSources(ctx context.Context, scope telemetry.Scope, id string, expected int64, metricsSource, logsSource string) (monitoring.Binding, error) {
	return m.BindProfiles(ctx, scope, id, expected, metricsSource, logsSource, nil)
}
func (m *Monitoring) BindProfiles(ctx context.Context, scope telemetry.Scope, id string, expected int64, metricsSource, logsSource string, profiles map[string]monitoring.WorkloadProfile) (monitoring.Binding, error) {
	var out monitoring.Binding
	if m.Repo == nil || m.Native == nil {
		return out, ErrUnavailable
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, scope.OrganizationKey, id)
	if e != nil {
		return out, e
	}
	if metricsSource == "" {
		metricsSource = "native"
		if c.PrometheusURL != "" {
			metricsSource = "prometheus"
		}
	}
	if logsSource == "" {
		logsSource = "native"
	}
	if (metricsSource != "native" && metricsSource != "prometheus") || (logsSource != "native" && logsSource != "loki") {
		return out, monitoring.ErrInvalid
	}
	if metricsSource == "prometheus" && !sourceVerified(c, "prometheus") {
		return out, monitoring.ErrInvalid
	}
	if logsSource == "loki" && !sourceVerified(c, "loki") {
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
	for _, t := range targets {
		if t.Scope.ClusterUID != c.ClusterUID {
			return out, monitoring.ErrInvalid
		}
	}
	if len(profiles) > 32 {
		return out, monitoring.ErrInvalid
	}
	for workload, profile := range profiles {
		if !workloadShape.MatchString(workload) || profile.Validate() != nil {
			return out, monitoring.ErrInvalid
		}
	}
	out = monitoring.Binding{ProfileVersion: c.Profile.Version, WorkloadProfiles: profiles, Organization: scope.OrganizationKey, Application: scope.ApplicationKey, Environment: scope.EnvironmentKey, ConnectionID: id, ConnectionRevision: c.Revision, ClusterUID: c.ClusterUID, Generation: env.TargetGeneration, Revision: expected + 1, MetricsSource: metricsSource, LogsSource: logsSource}
	e = m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.PutMonitoringBinding(ctx, out, expected) })
	return out, e
}
func (m *Monitoring) targets(ctx context.Context, scope telemetry.Scope) ([]monitoring.Target, error) {
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
		verifier, ok := m.Native.reader.(obs.TargetVerifier)
		if !ok {
			return nil, ErrUnavailable
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
	Workload           string                         `json:"workload"`
	Kind               string                         `json:"kind"`
	UID                string                         `json:"uid"`
	State              string                         `json:"state"`
	MetricsAt          *time.Time                     `json:"metricsAt,omitempty"`
	Metrics            map[string][]prometheus.Series `json:"metrics"`
	Configuration      string                         `json:"configuration"`
	ProfileVersion     string                         `json:"profileVersion,omitempty"`
	ApplicationMetrics map[string]MetricObservation   `json:"applicationMetrics"`
}
type MetricObservation struct {
	Unit   string              `json:"unit"`
	State  string              `json:"state"`
	Series []prometheus.Series `json:"series"`
}
type MonitoringView struct {
	AlertmanagerState string                         `json:"alertmanagerState"`
	Notifications     []prometheus.NotificationState `json:"notifications"`
	Source            string                         `json:"source"`
	State             string                         `json:"state"`
	From              time.Time                      `json:"from"`
	To                time.Time                      `json:"to"`
	Workloads         []WorkloadObservation          `json:"workloads"`
	Alerts            []prometheus.Alert             `json:"alerts"`
	GrafanaURL        string                         `json:"grafanaUrl,omitempty"`
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
	if b.MetricsSource == "native" || (b.MetricsSource == "" && c.PrometheusURL == "") {
		out.Source = "native"
		out.State = "METRICS_NATIVE"
		return out, nil
	}
	if !sourceVerified(c, "prometheus") {
		return out, ErrUnavailable
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
	cl, e := m.client(ctx, c, "prometheus")
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
		w.Configuration = "UNVERIFIED"
		w.ProfileVersion = c.Profile.Version
		w.ApplicationMetrics = map[string]MetricObservation{}
		profile := c.Profile.Defaults
		if override, ok := b.WorkloadProfiles[t.Workload]; ok {
			profile = &override
		}
		if profile != nil {
			for _, metric := range profile.Metrics {
				data, err := cl.Range(ctx, prometheus.MetricQuery(t, metric), out.From, out.To, step)
				if err != nil {
					return out, err
				}
				state := "RECEIVING"
				if len(data) == 0 {
					state = "MISSING"
				}
				w.ApplicationMetrics[metric.Key] = MetricObservation{Unit: metric.Unit, State: state, Series: data}
			}
		}
		if verifier, ok := m.Native.reader.(obs.ConfigurationVerifier); ok {
			sources, err := m.Native.discover(ctx)
			if err != nil {
				return out, err
			}
			for _, source := range sources {
				if source.OrganizationKey == scope.OrganizationKey && source.ApplicationKey == scope.ApplicationKey && source.EnvironmentKey == scope.EnvironmentKey && source.WorkloadID == t.Workload {
					desired, err := monitoringObjects(c, b, t)
					if err != nil {
						return out, err
					}
					w.Configuration, err = verifier.MonitoringConfiguration(ctx, source, desired)
					if err != nil {
						w.Configuration = "UNAVAILABLE"
					}
					break
				}
			}
		}
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
	if len(out.Workloads) == 0 {
		return out, nil
	}
	out.Alerts, e = cl.Alerts(ctx, map[string]string{"orchestrator_org": scope.OrganizationKey, "orchestrator_app": scope.ApplicationKey, "orchestrator_env": scope.EnvironmentKey, "orchestrator_generation": strconv.FormatInt(b.Generation, 10), "orchestrator_cluster_uid": b.ClusterUID})
	if e != nil {
		return out, e
	}
	out.AlertmanagerState = "NOT_CONFIGURED"
	if c.AlertmanagerURL != "" {
		out.AlertmanagerState = "UNAVAILABLE"
		am, err := m.client(ctx, c, "alertmanager")
		if err == nil {
			defer am.Close()
			out.Notifications, err = am.AlertmanagerAlerts(ctx, map[string]string{"orchestrator_org": scope.OrganizationKey, "orchestrator_app": scope.ApplicationKey, "orchestrator_env": scope.EnvironmentKey, "orchestrator_generation": strconv.FormatInt(b.Generation, 10), "orchestrator_cluster_uid": b.ClusterUID})
			if err == nil {
				out.AlertmanagerState = "OBSERVED"
			}
		}
	}
	out.GrafanaURL = scopedGrafanaURL(c, scope, b.Generation, out.From, out.To, p.Workload)
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
	if c.Revision != b.ConnectionRevision || b.Generation != env.TargetGeneration || b.ClusterUID != c.ClusterUID {
		return nil, monitoring.ErrInvalid
	}
	externalMetrics := b.MetricsSource != "native" && c.PrometheusURL != ""
	if !externalMetrics && b.LogsSource != "loki" {
		return manifests, nil
	}
	if (externalMetrics && !sourceVerified(c, "prometheus")) || (b.LogsSource == "loki" && !sourceVerified(c, "loki")) || m.Native == nil || target.Organization != org {
		return nil, monitoring.ErrInvalid
	}
	verifier, ok := m.Native.reader.(obs.ClusterVerifier)
	if !ok {
		return nil, ErrUnavailable
	}
	clusterUID, e := verifier.MonitoringCluster(ctx, target)
	if e != nil {
		return nil, ErrUnavailable
	}
	if clusterUID != b.ClusterUID {
		return nil, monitoring.ErrInvalid
	}
	// Clone objects before applying server-owned pod identity to the delivery artifact.
	original := manifests
	manifests = make([]execution.Manifest, 0, len(original)+2)
	for _, manifest := range original {
		if manifest.Namespace != "" && manifest.Namespace != target.Namespace {
			return nil, monitoring.ErrInvalid
		}
		raw, err := json.Marshal(manifest.Object)
		if err != nil {
			return nil, monitoring.ErrInvalid
		}
		if json.Unmarshal(raw, &manifest.Object) != nil {
			return nil, monitoring.ErrInvalid
		}
		pod := podTemplate(manifest)
		if pod != nil {
			md, ok := pod["metadata"].(map[string]any)
			if !ok {
				md = map[string]any{}
				pod["metadata"] = md
			}
			labels, ok := md["labels"].(map[string]any)
			if !ok {
				labels = map[string]any{}
				md["labels"] = labels
			}
			for k, v := range map[string]string{"orchestrator.io/organization": org, "orchestrator.io/application": app, "orchestrator.io/environment": env.Key, "orchestrator.io/generation": strconv.FormatInt(env.TargetGeneration, 10), "app.kubernetes.io/name": workload} {
				labels[k] = v
			}
		}
		manifests = append(manifests, manifest)
		if !externalMetrics || (manifest.Kind != "Deployment" && manifest.Kind != "StatefulSet") {
			continue
		}
		verified := monitoring.Target{Scope: monitoring.Scope{Organization: org, Application: app, Environment: env.Key, Generation: env.TargetGeneration, ClusterUID: clusterUID, Namespace: target.Namespace}, Workload: workload, Kind: manifest.Kind, Name: manifest.Name}
		labels := map[string]string{"app.kubernetes.io/managed-by": "orchestrator", "app.kubernetes.io/name": workload, "orchestrator.io/application": app, "orchestrator.io/environment": env.Key}
		for k, v := range c.RuleLabels {
			labels[k] = v
		}
		profile := c.Profile.Defaults
		if override, ok := b.WorkloadProfiles[workload]; ok {
			profile = &override
		}
		doc, err := prometheus.RuleDocument(verified, c.Rules)
		if err != nil {
			return nil, err
		}
		if profile != nil {
			if !hasMetricsPort(pod, profile.Port) {
				return nil, monitoring.ErrInvalid
			}
			monitor, err := prometheus.PodMonitorDocument(verified, *profile, labels)
			if err != nil {
				return nil, err
			}
			manifests = append(manifests, execution.Manifest{APIVersion: "monitoring.coreos.com/v1", Kind: "PodMonitor", Name: monitoring.RuleName(verified) + "-scrape", Namespace: target.Namespace, Object: monitor})
			prometheus.AddMetricRules(doc, verified, *profile, c.Profile.Version)
		}
		doc["metadata"].(map[string]any)["labels"] = labels
		manifests = append(manifests, execution.Manifest{APIVersion: "monitoring.coreos.com/v1", Kind: "PrometheusRule", Name: monitoring.RuleName(verified), Namespace: target.Namespace, Object: doc})
	}
	return manifests, nil
}

func (m *Monitoring) Unbind(ctx context.Context, scope telemetry.Scope, expected int64) error {
	if m.Repo == nil {
		return ErrUnavailable
	}
	app, e := m.Store.GetApplication(ctx, scope.ApplicationKey)
	if e != nil || app.OrganizationKey != scope.OrganizationKey {
		return persistence.ErrNotFound
	}
	env, e := m.Store.GetEnvironment(ctx, scope.ApplicationKey, scope.EnvironmentKey)
	if e != nil {
		return e
	}
	if env.Busy() {
		return persistence.Busy(scope.ApplicationKey, scope.EnvironmentKey)
	}
	instances, e := m.Store.ListWorkloadInstancesFor(ctx, scope.ApplicationKey+"/"+scope.EnvironmentKey, env.TargetGeneration)
	if e != nil {
		return e
	}
	for _, instance := range instances {
		if instance.Status != "REMOVED" {
			b, err := m.Repo.GetMonitoringBinding(ctx, scope.OrganizationKey, scope.ApplicationKey, scope.EnvironmentKey)
			if err != nil {
				return err
			}
			if b.MetricsSource != "native" || b.LogsSource != "native" {
				return monitoring.ErrInUse
			}
			verifier, ok := m.Native.reader.(obs.ConfigurationVerifier)
			if !ok {
				return monitoring.ErrInUse
			}
			sources, err := m.Native.discover(ctx)
			if err != nil {
				return err
			}
			verified := false
			for _, source := range sources {
				if source.OrganizationKey == scope.OrganizationKey && source.ApplicationKey == scope.ApplicationKey && source.EnvironmentKey == scope.EnvironmentKey && source.WorkloadID == instance.WorkloadID {
					state, err := verifier.MonitoringConfiguration(ctx, source, nil)
					if err != nil || state != "APPLIED" {
						return monitoring.ErrInUse
					}
					verified = true
				}
			}
			if !verified {
				return monitoring.ErrInUse
			}
		}
	}
	return m.Store.Transact(ctx, func(ctx context.Context) error {
		return m.Repo.DeleteMonitoringBinding(ctx, scope.OrganizationKey, scope.ApplicationKey, scope.EnvironmentKey, expected)
	})
}
func (m *Monitoring) Delete(ctx context.Context, org, id string, expected int64) error {
	if m.Repo == nil {
		return ErrUnavailable
	}
	c, e := m.Repo.GetMonitoringConnection(ctx, org, id)
	if e != nil {
		return e
	}
	if c.Revision != expected {
		return persistence.ErrVersionConflict
	}
	e = m.Store.Transact(ctx, func(ctx context.Context) error { return m.Repo.DeleteMonitoringConnection(ctx, org, id, expected) })
	if e != nil {
		return e
	}
	if m.Credentials != nil {
		if c.CredentialRef != "" {
			if err := m.Credentials.Delete(ctx, org, c.Key, c.CredentialRef); err != nil {
				return ErrUnavailable
			}
		}
		for provider, ref := range c.CredentialRefs {
			if ref != "" {
				if err := m.Credentials.Delete(ctx, org, c.Key+"-"+provider, ref); err != nil {
					return ErrUnavailable
				}
			}
		}
	}
	return nil
}

func sourceVerified(c monitoring.Connection, provider string) bool {
	if c.Verification.Revision == c.Revision {
		return c.Verification.Connectivity[provider] && c.Verification.Identity[provider]
	}
	return provider == "prometheus" && c.Verification.Revision == 0 && c.VerifiedAt != nil
}

func podTemplate(m execution.Manifest) map[string]any {
	spec, _ := m.Object["spec"].(map[string]any)
	if m.Kind == "CronJob" {
		job, _ := spec["jobTemplate"].(map[string]any)
		spec, _ = job["spec"].(map[string]any)
	}
	switch m.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
		pod, _ := spec["template"].(map[string]any)
		return pod
	}
	return nil
}
func hasMetricsPort(pod map[string]any, name string) bool {
	spec, _ := pod["spec"].(map[string]any)
	containers, _ := spec["containers"].([]any)
	for _, raw := range containers {
		c, _ := raw.(map[string]any)
		ports, _ := c["ports"].([]any)
		for _, raw := range ports {
			p, _ := raw.(map[string]any)
			if p["name"] == name {
				return true
			}
		}
	}
	return false
}

func monitoringObjects(c monitoring.Connection, b monitoring.Binding, t monitoring.Target) ([]execution.Manifest, error) {
	doc, err := prometheus.RuleDocument(t, c.Rules)
	if err != nil {
		return nil, err
	}
	profile := c.Profile.Defaults
	if override, ok := b.WorkloadProfiles[t.Workload]; ok {
		profile = &override
	}
	out := []execution.Manifest{}
	if profile != nil {
		prometheus.AddMetricRules(doc, t, *profile, c.Profile.Version)
		monitor, err := prometheus.PodMonitorDocument(t, *profile, c.RuleLabels)
		if err != nil {
			return nil, err
		}
		out = append(out, execution.Manifest{Kind: "PodMonitor", Object: monitor})
	}
	out = append(out, execution.Manifest{Kind: "PrometheusRule", Object: doc})
	return out, nil
}

func scopedGrafanaURL(c monitoring.Connection, scope telemetry.Scope, generation int64, from, to time.Time, workload string) string {
	if c.GrafanaURL == "" {
		return ""
	}
	u, err := url.Parse(c.GrafanaURL)
	if err != nil {
		return ""
	}
	if c.DashboardUID != "" {
		u.Path = strings.TrimRight(u.Path, "/") + "/d/" + url.PathEscape(c.DashboardUID)
	}
	q := u.Query()
	for k, v := range map[string]string{"var-cluster_uid": c.ClusterUID, "var-organization": scope.OrganizationKey, "var-application": scope.ApplicationKey, "var-environment": scope.EnvironmentKey, "var-generation": strconv.FormatInt(generation, 10), "var-workload": workload, "from": strconv.FormatInt(from.UnixMilli(), 10), "to": strconv.FormatInt(to.UnixMilli(), 10)} {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func kubernetesCoverage(facts []prometheus.Series) bool {
	expected := map[string]bool{"kube_pod_info": false, "kube_pod_container_status_restarts_total": false, "container_cpu_usage_seconds_total": false, "container_memory_working_set_bytes": false}
	for _, fact := range facts {
		if _, ok := expected[fact.Labels["__name__"]]; ok && fact.Value != nil && fact.Value.Value != nil && *fact.Value.Value > 0 {
			expected[fact.Labels["__name__"]] = true
		}
	}
	for _, present := range expected {
		if !present {
			return false
		}
	}
	return true
}
