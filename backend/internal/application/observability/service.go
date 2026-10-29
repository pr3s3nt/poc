// Package observability collects from pinned workload targets and serves stored
// telemetry. Stored telemetry GET handlers never call a cluster reader.
package observability

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	deployapp "orchestrator/internal/application/deployment"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
	"orchestrator/internal/ports/persistence"
)

var ErrUnavailable = errors.New("observability: unavailable")

type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "observability: invalid query field" }

type Service struct {
	store  persistence.Store
	repo   persistence.TelemetryRepository
	reader obs.Reader
	now    func() time.Time
	owner  string
	sweep  sync.Mutex
	offset int
}

func New(st persistence.Store, reader obs.Reader) *Service {
	repo, _ := st.(persistence.TelemetryRepository)
	return &Service{store: st, repo: repo, reader: reader, now: func() time.Time { return time.Now().UTC() }, owner: ids.New()}
}
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Start returns a stop function that must run before closing database pools.
func (s *Service) Start(parent context.Context) func() {
	if s.reader == nil || s.repo == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(telemetry.Interval)
		defer ticker.Stop()
		for {
			round, stop := context.WithTimeout(ctx, 25*time.Second)
			if err := s.Sweep(round); err != nil && ctx.Err() == nil {
				slog.Warn("application telemetry collection round failed")
			}
			stop()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (s *Service) discover(ctx context.Context) ([]obs.Source, error) {
	apps, err := s.store.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	out := []obs.Source{}
	for _, app := range apps {
		// Collection stops when an Application deletion is admitted (UC-18 BR-10).
		if app.Lifecycle() != appdomain.StatusActive {
			continue
		}
		envs, err := s.store.ListEnvironments(ctx, app.Key)
		if err != nil {
			return nil, err
		}
		for _, env := range envs {
			instances, err := s.store.ListWorkloadInstancesFor(ctx, app.Key+"/"+env.Key, env.TargetGeneration)
			if err != nil {
				return nil, err
			}
			for _, instance := range instances {
				if instance.Status == deployment.InstanceRemoved || instance.Status == deployment.InstanceRemoving {
					continue
				}
				dep, err := s.store.GetDeployment(ctx, instance.LastDeploymentID)
				if err != nil || dep.OrganizationKey != app.OrganizationKey || dep.ApplicationKey != app.Key || dep.EnvironmentKey != env.Key || dep.WorkloadID != instance.WorkloadID || dep.TargetGeneration != env.TargetGeneration {
					continue
				}
				var target execution.Target
				if deployapp.RestoreTarget(&target, instance.TargetRef, app.OrganizationKey) != nil || !target.CredentialBacked() || target.Namespace == "" {
					continue
				}
				// Exclude optional private/provider delivery facts from identity.
				pin, _ := json.Marshal(struct{ Org, Account, Cluster, Namespace, Context, Provider, Region string }{target.Organization, target.Connection, target.ClusterName, target.Namespace, target.Context, target.Provider, target.Region})
				hash := telemetry.Hash(string(pin))
				scope := telemetry.Scope{OrganizationKey: app.OrganizationKey, ApplicationKey: app.Key, EnvironmentKey: env.Key, Generation: env.TargetGeneration}
				key := telemetry.Hash(app.OrganizationKey, app.Key, env.Key, strconv.FormatInt(env.TargetGeneration, 10), instance.WorkloadID, hash)
				source := obs.Source{Source: telemetry.Source{Scope: scope, Key: key, WorkloadID: instance.WorkloadID, TargetHash: hash}, Target: target}
				var objects []execution.ObjectRef
				raw, marshalErr := json.Marshal(instance.TargetRef["objects"])
				if marshalErr == nil && json.Unmarshal(raw, &objects) == nil {
					for _, ref := range objects {
						if ref.APIVersion == "batch/v1" && ref.Kind == "CronJob" && ref.Name == instance.WorkloadID {
							pin := ref
							source.Controller = &pin
							break
						}
					}
				}
				out = append(out, source)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Sweep is exposed for focused adapter/acceptance tests. Production calls it
// through Start. Rotation prevents a slow early source starving later apps.
func (s *Service) Sweep(ctx context.Context) error {
	if s.reader == nil || s.repo == nil {
		return ErrUnavailable
	}
	s.sweep.Lock()
	defer s.sweep.Unlock()
	if err := s.repo.PruneTelemetry(ctx, s.now()); err != nil {
		return err
	}
	sources, err := s.discover(ctx)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	start := s.offset % len(sources)
	s.offset = (start + 4) % len(sources)
	jobs := make(chan obs.Source)
	var wg sync.WaitGroup
	var failed atomic.Bool
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for source := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := s.collect(ctx, source); err != nil {
					failed.Store(true)
				}
			}
		}()
	}
	for i := 0; i < len(sources); i++ {
		select {
		case jobs <- sources[(start+i)%len(sources)]:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	if failed.Load() {
		return errors.New("observability: collection commit failed")
	}
	return nil
}

func (s *Service) collect(ctx context.Context, source obs.Source) error {
	state, acquired, err := s.repo.LeaseTelemetry(ctx, source.Source, s.owner, s.now())
	if err != nil || !acquired {
		return err
	}
	read, cancel := context.WithTimeout(ctx, telemetry.ReadTimeout)
	batch, readErr := s.reader.Collect(read, source, state)
	cancel()
	if readErr != nil {
		now := s.now()
		state.LogsCode = telemetry.Failed
		state.MetricsCode = telemetry.Failed
		state.ObservedAt = &now
		batch = telemetry.Batch{State: state}
	}
	batch.At = s.now()
	batch.State.Source = state.Source
	batch.State.Owner = state.Owner
	batch.State.Fence = state.Fence
	batch.State.LeaseUntil = state.LeaseUntil
	if batch.State.Checkpoints == nil {
		batch.State.Checkpoints = state.Checkpoints
	}
	if state.LastLogAt != nil && (batch.State.LastLogAt == nil || batch.State.LastLogAt.Before(*state.LastLogAt)) {
		batch.State.LastLogAt = state.LastLogAt
	}
	if state.LastMetricAt != nil && (batch.State.LastMetricAt == nil || batch.State.LastMetricAt.Before(*state.LastMetricAt)) {
		batch.State.LastMetricAt = state.LastMetricAt
	}
	commit, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.repo.CommitTelemetry(commit, batch)
}

type Params struct {
	From, To, Workload, Search, Level, Cursor string
	Limit                                     int
}
type SourceHealth struct {
	WorkloadID   string         `json:"workloadId"`
	LogsCode     telemetry.Code `json:"logsCode"`
	MetricsCode  telemetry.Code `json:"metricsCode"`
	ObservedAt   *time.Time     `json:"observedAt"`
	LastLogAt    *time.Time     `json:"lastLogAt"`
	LastMetricAt *time.Time     `json:"lastMetricAt"`
	Limited      bool           `json:"storageLimited"`
}
type View struct {
	telemetry.Data
	Sources     []SourceHealth `json:"sources"`
	State       string         `json:"state"`
	From        time.Time      `json:"from"`
	To          time.Time      `json:"to"`
	GeneratedAt time.Time      `json:"generatedAt"`
	StepSeconds int64          `json:"stepSeconds"`
	Workloads   []string       `json:"workloads"`
	SignalCount int64          `json:"signalCount"`
	LogsDays    int            `json:"logsRetentionDays"`
	MetricsDays int            `json:"metricsRetentionDays"`
	HasHistory  bool           `json:"hasHistory"`
}

var workloadShape = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

func (s *Service) Query(ctx context.Context, scope telemetry.Scope, kind string, p Params) (View, error) {
	var out View
	app, err := s.store.GetApplication(ctx, scope.ApplicationKey)
	if err != nil || app.OrganizationKey != scope.OrganizationKey {
		return out, persistence.ErrNotFound
	}
	env, err := s.store.GetEnvironment(ctx, scope.ApplicationKey, scope.EnvironmentKey)
	if err != nil {
		return out, err
	}
	scope.Generation = env.TargetGeneration
	if s.repo == nil {
		return out, ErrUnavailable
	}
	now := s.now()
	from, to := now.Add(-time.Hour), now.Add(time.Nanosecond)
	if p.From != "" {
		from, err = time.Parse(time.RFC3339Nano, p.From)
		if err != nil {
			return out, &FieldError{"from"}
		}
	}
	if p.To != "" {
		to, err = time.Parse(time.RFC3339Nano, p.To)
		if err != nil {
			return out, &FieldError{"to"}
		}
	}
	retention := telemetry.LogRetention
	if kind == "metrics" {
		retention = telemetry.MetricRetention
	}
	if !from.Before(to) || from.Before(now.Add(-retention-time.Minute)) || to.After(now.Add(time.Minute)) {
		return out, &FieldError{"from"}
	}
	if p.Workload != "" && !workloadShape.MatchString(p.Workload) {
		return out, &FieldError{"workload"}
	}
	p.Level = strings.ToUpper(p.Level)
	switch p.Level {
	case "", "ALL", "ERROR", "WARN", "INFO", "DEBUG", "UNKNOWN":
	default:
		return out, &FieldError{"level"}
	}
	if utf8.RuneCountInString(p.Search) > 200 || len(p.Search) > 800 {
		return out, &FieldError{"q"}
	}
	if p.Limit == 0 {
		p.Limit = 100
	}
	if p.Limit < 1 || p.Limit > telemetry.MaxPage {
		return out, &FieldError{"limit"}
	}
	if from.Before(now.Add(-retention)) {
		from = now.Add(-retention)
	}
	q := telemetry.Query{Scope: scope, Kind: kind, From: from, To: to, Workload: p.Workload, Search: p.Search, Level: p.Level, Limit: p.Limit}
	if p.Cursor != "" {
		q.Before, err = telemetry.DecodeCursor(p.Cursor)
		if err != nil || q.Before.Time.Before(from) || !q.Before.Time.Before(to) {
			return out, &FieldError{"cursor"}
		}
	}
	q.Step = max(telemetry.Interval, time.Duration((int64(to.Sub(from))+int64(240*telemetry.Interval)-1)/int64(240*telemetry.Interval))*telemetry.Interval)
	for (to.Sub(telemetry.BucketTime(from, q.Step))+q.Step-1)/q.Step > 240 {
		q.Step += telemetry.Interval
	}
	data, err := s.repo.ReadTelemetry(ctx, q)
	if err != nil {
		return out, err
	}
	instances, err := s.store.ListWorkloadInstancesFor(ctx, scope.ApplicationKey+"/"+scope.EnvironmentKey, scope.Generation)
	if err != nil {
		return out, err
	}
	known := map[string]bool{}
	active := 0
	for _, instance := range instances {
		known[instance.WorkloadID] = true
		if instance.Status != deployment.InstanceRemoved && (p.Workload == "" || p.Workload == instance.WorkloadID) {
			active++
		}
	}
	out = View{Data: data, Sources: []SourceHealth{}, From: from, To: to, GeneratedAt: now, StepSeconds: int64(q.Step / time.Second), Workloads: []string{}, LogsDays: 7, MetricsDays: 15}
	out.State = "RECEIVING"
	if len(data.Sources) == 0 {
		out.State = "WAITING"
	}
	rank := map[string]int{"RECEIVING": 0, "METRICS_UNAVAILABLE": 1, "WAITING": 2, "PARTIAL": 3, "STALE": 4, "STORAGE_LIMIT": 5}
	for _, state := range data.Sources {
		known[state.Source.WorkloadID] = true
		out.Sources = append(out.Sources, SourceHealth{state.Source.WorkloadID, telemetry.SafeCode(state.LogsCode), telemetry.SafeCode(state.MetricsCode), state.ObservedAt, state.LastLogAt, state.LastMetricAt, state.Limited})
		health := "RECEIVING"
		switch {
		case state.Limited:
			health = "STORAGE_LIMIT"
		case state.ObservedAt == nil:
			health = "WAITING"
		case state.ObservedAt.Before(now.Add(-3 * telemetry.Interval)):
			health = "STALE"
		case state.LogsCode == telemetry.Waiting && state.MetricsCode == telemetry.Waiting:
			health = "WAITING"
		case state.LogsCode != telemetry.OK:
			health = "PARTIAL"
		case state.MetricsCode != telemetry.OK:
			health = "METRICS_UNAVAILABLE"
		}
		if rank[health] > rank[out.State] {
			out.State = health
		}
	}
	if active == 0 {
		if len(data.Sources) == 0 {
			out.State = "NOT_DEPLOYED"
		} else {
			out.State = "INACTIVE"
		}
	}
	out.HasHistory = data.LogCount > 0 || len(data.Latest) > 0 || data.EventCount > 0
	if s.reader == nil {
		out.State = "UNAVAILABLE"
	}
	for workload := range known {
		out.Workloads = append(out.Workloads, workload)
	}
	sort.Strings(out.Workloads)
	for _, peak := range data.Peaks {
		if peak.Percent >= 80 {
			out.SignalCount++
		}
	}
	out.SignalCount += data.EventCount
	return out, nil
}
