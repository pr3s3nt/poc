package http

import (
	"errors"
	"net/http"
	"time"

	effectivenessapp "orchestrator/internal/application/effectiveness"
	domain "orchestrator/internal/domain/effectiveness"
	identity2 "orchestrator/internal/domain/identity"
)

// effectivenessBodyLimit bounds one baseline version document.
const effectivenessBodyLimit = 16 << 10

func (s *Server) effectivenessRoutes() {
	s.mux.HandleFunc("GET /api/v1/effectiveness/report", s.handleEffectivenessReport)
	s.mux.HandleFunc("GET /api/v1/effectiveness/records", s.handleEffectivenessRecords)
	s.mux.HandleFunc("GET /api/v1/effectiveness/baselines", s.handleEffectivenessBaselines)
	s.mux.HandleFunc("POST /api/v1/effectiveness/baselines", s.handleAddEffectivenessBaseline)
}

func (s *Server) effectivenessQuery(r *http.Request, organizationKey string) effectivenessapp.Query {
	q := r.URL.Query()
	return effectivenessapp.Query{OrganizationKey: organizationKey, ApplicationKey: q.Get("application"), WorkloadID: q.Get("workload"),
		From: q.Get("from"), To: q.Get("to"), Metric: q.Get("metric")}
}

func (s *Server) writeEffectivenessError(w http.ResponseWriter, err error) {
	var field *effectivenessapp.FieldError
	switch {
	case errors.As(err, &field):
		writeCode(w, codeInvalidRequest, field.Field)
	case errors.Is(err, effectivenessapp.ErrBaselinesUnavailable):
		writeCode(w, codeServiceUnavailable, "")
	default:
		s.writeError(w, err)
	}
}

type effectivenessScopeView struct {
	Level           string `json:"level"`
	ApplicationKey  string `json:"applicationKey,omitempty"`
	ApplicationName string `json:"applicationName,omitempty"`
	WorkloadID      string `json:"workloadId,omitempty"`
}

type effectivenessPeriodView struct {
	From     domain.Date `json:"from"`
	To       domain.Date `json:"to"`
	Timezone string      `json:"timezone"`
	Days     int         `json:"days"`
}

type effectivenessSourceView struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}

type coverageView struct {
	Covered int `json:"covered"`
	Total   int `json:"total"`
}

type metricView struct {
	ID             string             `json:"id"`
	Group          domain.Group       `json:"group"`
	Unit           domain.Unit        `json:"unit"`
	Status         domain.Status      `json:"status"`
	Value          *float64           `json:"value,omitempty"`
	Numerator      *float64           `json:"numerator,omitempty"`
	Denominator    *float64           `json:"denominator,omitempty"`
	Coverage       *coverageView      `json:"coverage,omitempty"`
	Details        map[string]float64 `json:"details,omitempty"`
	RequiredSource string             `json:"requiredSource,omitempty"`
}

type breakdownRowView struct {
	Key                   string               `json:"key"`
	Name                  string               `json:"name"`
	ProductionDeployments int                  `json:"productionDeployments"`
	FrequencyBand         domain.FrequencyBand `json:"frequencyBand"`
	CompletedWork         int                  `json:"completedWork"`
	OpenWork              int                  `json:"openWork"`
	Attempts              int                  `json:"attempts"`
	FailedAttempts        int                  `json:"failedAttempts"`
	MedianDurationSeconds *float64             `json:"medianDurationSeconds"`
	ActiveUsers           int                  `json:"activeUsers"`
}

type breakdownView struct {
	Level string             `json:"level"`
	Rows  []breakdownRowView `json:"rows"`
}

type trendPointView struct {
	Start                 domain.Date `json:"start"`
	ProductionDeployments int         `json:"productionDeployments"`
	CompletedWork         int         `json:"completedWork"`
}

type trendView struct {
	Bucket domain.Bucket    `json:"bucket"`
	Points []trendPointView `json:"points"`
}

type baselineView struct {
	ID             string       `json:"id"`
	Metric         string       `json:"metric"`
	ApplicationKey string       `json:"applicationKey,omitempty"`
	Version        int64        `json:"version"`
	Retired        bool         `json:"retired"`
	Statistic      string       `json:"statistic,omitempty"`
	ValueSeconds   int64        `json:"valueSeconds,omitempty"`
	Source         string       `json:"source"`
	SampleSize     *int         `json:"sampleSize,omitempty"`
	MeasuredFrom   *domain.Date `json:"measuredFrom,omitempty"`
	MeasuredTo     *domain.Date `json:"measuredTo,omitempty"`
	EffectiveFrom  domain.Date  `json:"effectiveFrom"`
	CreatedBy      string       `json:"createdBy"`
	CreatedAt      time.Time    `json:"createdAt"`
}

type reportView struct {
	Scope       effectivenessScopeView    `json:"scope"`
	Period      effectivenessPeriodView   `json:"period"`
	DataOrigin  string                    `json:"dataOrigin"`
	GeneratedAt time.Time                 `json:"generatedAt"`
	Sources     []effectivenessSourceView `json:"sources"`
	Metrics     []metricView              `json:"metrics"`
	Breakdown   *breakdownView            `json:"breakdown"`
	Trend       trendView                 `json:"trend"`
	Baseline    *baselineView             `json:"baseline"`
}

func baselineViewOf(b domain.Baseline, author string) baselineView {
	return baselineView{ID: b.ID, Metric: b.Metric, ApplicationKey: b.ApplicationKey, Version: b.Version, Retired: b.Retired, Statistic: b.Statistic,
		ValueSeconds: b.ValueSeconds, Source: b.Source, SampleSize: b.SampleSize, MeasuredFrom: b.MeasuredFrom, MeasuredTo: b.MeasuredTo,
		EffectiveFrom: b.EffectiveFrom, CreatedBy: author, CreatedAt: b.CreatedAt}
}

func reportViewOf(r effectivenessapp.Report) reportView {
	v := reportView{
		Scope:       effectivenessScopeView{Level: r.Scope.Level, ApplicationKey: r.Scope.ApplicationKey, ApplicationName: r.Scope.ApplicationName, WorkloadID: r.Scope.WorkloadID},
		Period:      effectivenessPeriodView{From: r.Period.From, To: r.Period.To, Timezone: domain.TimezoneName, Days: r.Period.Days()},
		DataOrigin:  string(r.DataOrigin),
		GeneratedAt: r.GeneratedAt,
		Sources:     []effectivenessSourceView{},
		Metrics:     []metricView{},
		Trend:       trendView{Bucket: r.TrendBucket, Points: []trendPointView{}},
	}
	for _, src := range r.Sources {
		v.Sources = append(v.Sources, effectivenessSourceView{ID: src.ID, Available: src.Available})
	}
	for _, m := range r.Metrics {
		mv := metricView{ID: m.ID, Group: m.Group, Unit: m.Unit, Status: m.Status, Value: m.Value, Numerator: m.Numerator, Denominator: m.Denominator, Details: m.Details, RequiredSource: m.RequiredSource}
		if m.Coverage != nil {
			mv.Coverage = &coverageView{Covered: m.Coverage.Covered, Total: m.Coverage.Total}
		}
		v.Metrics = append(v.Metrics, mv)
	}
	if r.BreakdownLevel != "" {
		b := &breakdownView{Level: r.BreakdownLevel, Rows: []breakdownRowView{}}
		for _, row := range r.Breakdown {
			b.Rows = append(b.Rows, breakdownRowView{Key: row.Key, Name: row.Name, ProductionDeployments: row.ProductionDeployments, FrequencyBand: row.FrequencyBand,
				CompletedWork: row.CompletedWork, OpenWork: row.OpenWork, Attempts: row.Attempts, FailedAttempts: row.FailedAttempts,
				MedianDurationSeconds: row.MedianDurationSeconds, ActiveUsers: row.ActiveUsers})
		}
		v.Breakdown = b
	}
	for _, p := range r.Trend {
		v.Trend.Points = append(v.Trend.Points, trendPointView{Start: p.Start, ProductionDeployments: p.ProductionDeployments, CompletedWork: p.CompletedWork})
	}
	if r.Baseline != nil {
		b := baselineViewOf(*r.Baseline, r.BaselineAuthor)
		v.Baseline = &b
	}
	return v
}

func (s *Server) handleEffectivenessReport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	report, err := s.effectiveness.Report(r.Context(), s.effectivenessQuery(r, org))
	if err != nil {
		s.writeEffectivenessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reportViewOf(report))
}

type actorView struct {
	Name string           `json:"name"`
	Kind domain.ActorKind `json:"kind"`
}

type recordView struct {
	ID               string           `json:"id"`
	Kind             domain.RunKind   `json:"kind"`
	ApplicationKey   string           `json:"applicationKey"`
	ApplicationName  string           `json:"applicationName"`
	EnvironmentKey   string           `json:"environmentKey"`
	WorkloadID       string           `json:"workloadId,omitempty"`
	ResourceID       string           `json:"resourceId,omitempty"`
	Action           string           `json:"action"`
	State            domain.ItemState `json:"state"`
	Attempts         int              `json:"attempts"`
	FailedAttempts   int              `json:"failedAttempts"`
	AttemptsInPeriod int              `json:"attemptsInPeriod"`
	FirstStartedAt   time.Time        `json:"firstStartedAt"`
	CompletedAt      *time.Time       `json:"completedAt,omitempty"`
	DurationSeconds  *float64         `json:"durationSeconds,omitempty"`
	Actors           []actorView      `json:"actors"`
	LastRunID        string           `json:"lastRunId"`
}

func recordViewOf(rec effectivenessapp.Record) recordView {
	w := rec.Item
	v := recordView{ID: w.ID(), Kind: w.Kind(), ApplicationKey: w.ApplicationKey(), ApplicationName: rec.ApplicationName, EnvironmentKey: w.EnvironmentKey(),
		WorkloadID: w.WorkloadID(), ResourceID: w.ResourceID(), Action: w.Action(), State: w.State(), Attempts: len(w.Runs), FailedAttempts: w.FailedRuns(),
		AttemptsInPeriod: rec.AttemptsInPeriod, FirstStartedAt: w.StartedAt(), CompletedAt: w.CompletedAt(), Actors: []actorView{}, LastRunID: w.LastRunID()}
	if d, ok := w.Duration(); ok {
		seconds := d.Seconds()
		v.DurationSeconds = &seconds
	}
	seen := map[string]bool{}
	for _, run := range w.Runs {
		if run.Actor.Name == "" || seen[run.Actor.Key] {
			continue
		}
		seen[run.Actor.Key] = true
		v.Actors = append(v.Actors, actorView{Name: run.Actor.Name, Kind: run.Actor.Kind})
	}
	return v
}

func (s *Server) handleEffectivenessRecords(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	records, err := s.effectiveness.Records(r.Context(), s.effectivenessQuery(r, org))
	if err != nil {
		s.writeEffectivenessError(w, err)
		return
	}
	out := make([]recordView, 0, len(records))
	for _, rec := range records {
		out = append(out, recordViewOf(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": out})
}

func (s *Server) handleEffectivenessBaselines(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	list, err := s.effectiveness.Baselines(r.Context(), org, r.URL.Query().Get("application"))
	if err != nil {
		s.writeEffectivenessError(w, err)
		return
	}
	out := make([]baselineView, 0, len(list.Versions))
	for _, v := range list.Versions {
		out = append(out, baselineViewOf(v.Baseline, v.Author))
	}
	writeJSON(w, http.StatusOK, map[string]any{"currentVersion": list.CurrentVersion, "baselines": out})
}

type addBaselineRequest struct {
	Metric          string  `json:"metric"`
	ApplicationKey  string  `json:"applicationKey"`
	ExpectedVersion *int64  `json:"expectedVersion"`
	Retired         bool    `json:"retired"`
	ValueSeconds    *int64  `json:"valueSeconds"`
	Source          string  `json:"source"`
	EffectiveFrom   string  `json:"effectiveFrom"`
	SampleSize      *int    `json:"sampleSize"`
	MeasuredFrom    *string `json:"measuredFrom"`
	MeasuredTo      *string `json:"measuredTo"`
}

func optionalDate(value *string, field string) (*domain.Date, error) {
	if value == nil {
		return nil, nil
	}
	d, err := domain.ParseDate(*value)
	if err != nil {
		return nil, &effectivenessapp.FieldError{Field: field}
	}
	return &d, nil
}

func (s *Server) handleAddEffectivenessBaseline(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	if actor.Role != identity2.RolePlatformEngineer && actor.Role != identity2.RoleAdmin {
		writeCode(w, codeForbidden, "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, effectivenessBodyLimit)
	var req addBaselineRequest
	if err := decodeSingleObject(r, &req); err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return
	}
	if req.ExpectedVersion == nil {
		writeCode(w, codeInvalidRequest, "expectedVersion")
		return
	}
	b := domain.Baseline{Metric: req.Metric, ApplicationKey: req.ApplicationKey, Retired: req.Retired, Source: req.Source,
		EffectiveFrom: domain.Date(req.EffectiveFrom), SampleSize: req.SampleSize}
	if req.ValueSeconds != nil {
		if req.Retired {
			writeCode(w, codeInvalidRequest, "valueSeconds")
			return
		}
		b.ValueSeconds = *req.ValueSeconds
	}
	var err error
	if b.MeasuredFrom, err = optionalDate(req.MeasuredFrom, "measuredFrom"); err == nil {
		b.MeasuredTo, err = optionalDate(req.MeasuredTo, "measuredTo")
	}
	if err != nil {
		s.writeEffectivenessError(w, err)
		return
	}
	stored, err := s.effectiveness.AddBaselineVersion(r.Context(), effectivenessapp.AddBaseline{
		OrganizationKey: actor.OrganizationKey, AuthorID: actor.UserID, ExpectedVersion: *req.ExpectedVersion, Baseline: b,
	})
	if err != nil {
		s.writeEffectivenessError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, baselineViewOf(stored.Baseline, stored.Author))
}
