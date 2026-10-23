// Package effectiveness serves UC-17: the platform effectiveness report, its
// evidence records and the append-only baselines. It only reads persisted
// execution history; the single write is a new baseline version.
package effectiveness

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/deployment"
	domain "orchestrator/internal/domain/effectiveness"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
)

// DataOrigin labels the whole report (UC-17 BR-11).
type DataOrigin string

// Data origins.
const (
	OriginProduction       DataOrigin = "PRODUCTION"
	OriginTestInstallation DataOrigin = "TEST_INSTALLATION"
)

// OriginOf maps the installation seed profile to the report origin: only the
// production profile is production data.
func OriginOf(profile string) DataOrigin {
	if profile == "production" {
		return OriginProduction
	}
	return OriginTestInstallation
}

// Source IDs reported with their availability.
const (
	SourceDeployments      = "deployments"
	SourceResourceCleanups = "resource_cleanups"
)

// FieldError names the invalid request parameter.
type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "effectiveness: invalid " + e.Field }

// ErrBaselinesUnavailable means the store cannot persist baselines.
var ErrBaselinesUnavailable = errors.New("effectiveness: baseline persistence is unavailable")

// Service is the UC-17 control.
type Service struct {
	store  persistence.Store
	origin DataOrigin
	now    func() time.Time
}

// New builds the service over the product store.
func New(store persistence.Store, origin DataOrigin) *Service {
	return &Service{store: store, origin: origin, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Query selects the scope and period of a report or of its records.
type Query struct {
	OrganizationKey string
	ApplicationKey  string
	WorkloadID      string
	From, To        string
	// Metric filters records to the evidence of one catalog entry.
	Metric string
}

// Scope is the resolved report scope.
type Scope struct {
	Level           string
	ApplicationKey  string
	ApplicationName string
	WorkloadID      string
}

// Scope levels.
const (
	LevelOrganization = "ORGANIZATION"
	LevelApplication  = "APPLICATION"
	LevelWorkload     = "WORKLOAD"
)

// SourceState reports whether a history source could be read.
type SourceState struct {
	ID        string
	Available bool
}

// BreakdownRow is one child scope of the report.
type BreakdownRow struct {
	Key  string
	Name string
	domain.Summary
}

// Report is the UC-17 read model of one scope and period.
type Report struct {
	Scope          Scope
	Period         domain.Period
	DataOrigin     DataOrigin
	GeneratedAt    time.Time
	Sources        []SourceState
	Metrics        []domain.Result
	BreakdownLevel string
	Breakdown      []BreakdownRow
	TrendBucket    domain.Bucket
	Trend          []domain.TrendPoint
	Baseline       *domain.Baseline
	// BaselineAuthor is the username of the applied baseline's author.
	BaselineAuthor string
}

// Record is one evidence work item of a report.
type Record struct {
	Item             domain.WorkItem
	ApplicationName  string
	AttemptsInPeriod int
}

// loaded is the history of one Organization read in one snapshot.
type loaded struct {
	apps      map[string]string // key -> name
	appOrder  []string
	items     []domain.WorkItem
	sources   []SourceState
	baselines []domain.Baseline
	authors   map[string]string
}

func (s *Service) load(ctx context.Context, q Query) (loaded, Scope, domain.Period, error) {
	period, err := domain.NewPeriod(q.From, q.To, s.now())
	if err != nil {
		var pe *domain.PeriodError
		if errors.As(err, &pe) {
			return loaded{}, Scope{}, domain.Period{}, &FieldError{Field: pe.Field}
		}
		return loaded{}, Scope{}, domain.Period{}, err
	}
	if q.WorkloadID != "" && q.ApplicationKey == "" {
		return loaded{}, Scope{}, domain.Period{}, &FieldError{Field: "workload"}
	}
	var out loaded
	err = s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		apps, err := view.ListApplications(ctx)
		if err != nil {
			return err
		}
		out.apps = map[string]string{}
		for _, a := range apps {
			if a.OrganizationKey == q.OrganizationKey {
				out.apps[a.Key] = a.Name
				out.appOrder = append(out.appOrder, a.Key)
			}
		}
		if q.ApplicationKey != "" {
			if _, ok := out.apps[q.ApplicationKey]; !ok {
				return persistence.ErrNotFound
			}
		}
		actors := newActorResolver(view, q.OrganizationKey)
		var runs []domain.Run
		deployments, err := view.ListDeployments(ctx, "", "")
		if err != nil {
			return err
		}
		for _, d := range deployments {
			if _, ok := out.apps[d.ApplicationKey]; !ok || d.OrganizationKey != q.OrganizationKey {
				continue
			}
			runs = append(runs, deploymentRun(d, actors.resolve(ctx, d.ActorRef)))
		}
		out.sources = append(out.sources, SourceState{ID: SourceDeployments, Available: true})
		history, ok := view.(persistence.CleanupHistory)
		if !ok {
			history, ok = s.store.(persistence.CleanupHistory)
		}
		if ok {
			cleanups, err := history.ListCleanupRuns(ctx, q.OrganizationKey)
			if err != nil {
				return err
			}
			for _, c := range cleanups {
				if _, known := out.apps[c.ApplicationKey]; !known || c.Mode != string(tfresource.CleanupUserConfirmed) {
					continue
				}
				runs = append(runs, cleanupRun(c, actors.resolve(ctx, c.ActorRef)))
			}
		}
		out.sources = append(out.sources, SourceState{ID: SourceResourceCleanups, Available: ok})
		out.items = domain.BuildWorkItems(runs)
		if repo, ok := s.store.(persistence.EffectivenessBaselineRepository); ok && q.WorkloadID == "" {
			out.baselines, err = repo.ListEffectivenessBaselines(ctx, q.OrganizationKey, domain.MetricWorkDuration, q.ApplicationKey)
			if err != nil {
				return err
			}
		}
		out.authors = map[string]string{}
		for _, b := range out.baselines {
			out.authors[b.CreatedBy] = actors.username(ctx, b.CreatedBy)
		}
		return nil
	})
	if err != nil {
		return loaded{}, Scope{}, domain.Period{}, err
	}
	scope := Scope{Level: LevelOrganization}
	if q.ApplicationKey != "" {
		scope = Scope{Level: LevelApplication, ApplicationKey: q.ApplicationKey, ApplicationName: out.apps[q.ApplicationKey]}
		if q.WorkloadID != "" {
			scope.Level, scope.WorkloadID = LevelWorkload, q.WorkloadID
		}
	}
	return out, scope, period, nil
}

func inScope(w domain.WorkItem, application, workload string) bool {
	if application != "" && w.ApplicationKey() != application {
		return false
	}
	if workload != "" && (w.Kind() != domain.KindDeployment || w.WorkloadID() != workload) {
		return false
	}
	return true
}

func filter(items []domain.WorkItem, application, workload string) []domain.WorkItem {
	var out []domain.WorkItem
	for _, w := range items {
		if inScope(w, application, workload) {
			out = append(out, w)
		}
	}
	return out
}

func (l loaded) input(scope Scope, period domain.Period) domain.Input {
	keys := l.appOrder
	if scope.ApplicationKey != "" {
		keys = []string{scope.ApplicationKey}
	}
	in := domain.Input{Period: period, Items: filter(l.items, scope.ApplicationKey, scope.WorkloadID), ApplicationKeys: keys}
	if scope.Level != LevelWorkload {
		in.Baseline = domain.EffectiveBaseline(l.baselines, period.To)
	}
	return in
}

// Report computes the UC-17 report.
func (s *Service) Report(ctx context.Context, q Query) (Report, error) {
	l, scope, period, err := s.load(ctx, q)
	if err != nil {
		return Report{}, err
	}
	in := l.input(scope, period)
	r := Report{Scope: scope, Period: period, DataOrigin: s.origin, GeneratedAt: s.now().UTC(), Sources: l.sources, Metrics: domain.Compute(in), Baseline: in.Baseline}
	if in.Baseline != nil {
		r.BaselineAuthor = l.authors[in.Baseline.CreatedBy]
	}
	r.TrendBucket, r.Trend = domain.Trend(in.Items, period)
	switch scope.Level {
	case LevelOrganization:
		r.BreakdownLevel = LevelApplication
		for _, key := range l.appOrder {
			r.Breakdown = append(r.Breakdown, BreakdownRow{Key: key, Name: l.apps[key], Summary: domain.Summarize(filter(in.Items, key, ""), period)})
		}
		sort.SliceStable(r.Breakdown, func(i, j int) bool {
			return strings.ToLower(r.Breakdown[i].Name) < strings.ToLower(r.Breakdown[j].Name)
		})
	case LevelApplication:
		r.BreakdownLevel = LevelWorkload
		seen := map[string]bool{}
		var workloads []string
		for _, w := range in.Items {
			if w.Kind() == domain.KindDeployment && !seen[w.WorkloadID()] {
				seen[w.WorkloadID()] = true
				workloads = append(workloads, w.WorkloadID())
			}
		}
		sort.Strings(workloads)
		for _, id := range workloads {
			r.Breakdown = append(r.Breakdown, BreakdownRow{Key: id, Name: id, Summary: domain.Summarize(filter(in.Items, scope.ApplicationKey, id), period)})
		}
	}
	return r, nil
}

// Records returns the evidence work items of one metric, or every relevant
// item when no metric is given, newest first.
func (s *Service) Records(ctx context.Context, q Query) ([]Record, error) {
	var def domain.Definition
	if q.Metric != "" {
		var ok bool
		if def, ok = domain.Lookup(q.Metric); !ok {
			return nil, &FieldError{Field: "metric"}
		}
	}
	l, scope, period, err := s.load(ctx, q)
	if err != nil {
		return nil, err
	}
	in := l.input(scope, period)
	items := domain.AllRecords(in)
	if q.Metric != "" {
		items = domain.RecordsOf(def, in)
	}
	out := make([]Record, 0, len(items))
	for _, w := range items {
		out = append(out, Record{Item: w, ApplicationName: l.apps[w.ApplicationKey()], AttemptsInPeriod: domain.RunsStartedIn(w, period)})
	}
	sort.SliceStable(out, func(i, j int) bool { return recordTime(out[i].Item).After(recordTime(out[j].Item)) })
	return out, nil
}

func recordTime(w domain.WorkItem) time.Time {
	if done := w.CompletedAt(); done != nil {
		return *done
	}
	return w.Runs[len(w.Runs)-1].StartedAt
}

func deploymentRun(d deployment.Deployment, actor domain.Actor) domain.Run {
	outcome := domain.OutcomeRunning
	switch d.Status {
	case deployment.StatusSucceeded:
		outcome = domain.OutcomeSucceeded
	case deployment.StatusFailed:
		outcome = domain.OutcomeFailed
	}
	return domain.Run{ID: d.ID, Kind: domain.KindDeployment, ApplicationKey: d.ApplicationKey, EnvironmentKey: d.EnvironmentKey,
		WorkloadID: d.WorkloadID, Action: string(d.Action), Outcome: outcome, Actor: actor, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt}
}

func cleanupRun(c persistence.CleanupRun, actor domain.Actor) domain.Run {
	outcome := domain.OutcomeRunning
	switch environment.OperationStatus(c.Status) {
	case environment.OpSucceeded:
		outcome = domain.OutcomeSucceeded
	case environment.OpFailed, environment.OpRecovered:
		outcome = domain.OutcomeFailed
	}
	return domain.Run{ID: c.OperationID, Kind: domain.KindResourceCleanup, ApplicationKey: c.ApplicationKey, EnvironmentKey: c.EnvironmentKey,
		ResourceID: c.RootID, Action: domain.ActionResourceCleanup, Outcome: outcome, Actor: actor, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt}
}

// actorResolver maps a stored actor reference (a User Account ID from the
// Console, a username from cleanup, or free text from other callers) to a
// person of the Organization, once per reference.
type actorResolver struct {
	view  persistence.Store
	org   string
	cache map[string]domain.Actor
}

func newActorResolver(view persistence.Store, org string) *actorResolver {
	return &actorResolver{view: view, org: org, cache: map[string]domain.Actor{}}
}

func (a *actorResolver) resolve(ctx context.Context, ref string) domain.Actor {
	if actor, ok := a.cache[ref]; ok {
		return actor
	}
	actor := domain.Actor{Key: ref, Name: ref, Kind: domain.ActorAutomation}
	if ref == "" {
		actor.Name = ""
	} else if account, err := a.view.GetUserAccount(ctx, ref); err == nil && account.OrganizationKey == a.org {
		actor = domain.Actor{Key: account.ID, Name: account.Username, Kind: domain.ActorUser}
	} else if account, err := a.view.GetUserAccountByUsername(ctx, ref); err == nil && account.OrganizationKey == a.org {
		actor = domain.Actor{Key: account.ID, Name: account.Username, Kind: domain.ActorUser}
	}
	a.cache[ref] = actor
	return actor
}

func (a *actorResolver) username(ctx context.Context, id string) string {
	if account, err := a.view.GetUserAccount(ctx, id); err == nil {
		return account.Username
	}
	return ""
}
