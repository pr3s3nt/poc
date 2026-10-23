package effectiveness

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC) // 10:00 in the report timezone

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

func finished(minutes int) *time.Time { v := at(minutes); return &v }

func user(key string) Actor { return Actor{Key: key, Name: key, Kind: ActorUser} }

func deploy(id, app, env, workload, action string, outcome Outcome, start, end int, actor Actor) Run {
	r := Run{ID: id, Kind: KindDeployment, ApplicationKey: app, EnvironmentKey: env, WorkloadID: workload, Action: action, Outcome: outcome, Actor: actor, StartedAt: at(start)}
	if outcome != OutcomeRunning {
		r.FinishedAt = finished(end)
	}
	return r
}

func october() Period { return Period{From: "2026-10-01", To: "2026-10-30"} }

func result(t *testing.T, results []Result, id string) Result {
	t.Helper()
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("metric %s missing", id)
	return Result{}
}

// BR-04: retries of one change are one logical item; every run stays counted.
func TestRetriesFormOneWorkItemEndingAtTheFirstSuccess(t *testing.T) {
	runs := []Run{
		deploy("d3", "shop", "production", "api", "UPDATE", OutcomeSucceeded, 30, 40, user("u1")),
		deploy("d1", "shop", "production", "api", "UPDATE", OutcomeFailed, 0, 5, user("u1")),
		deploy("d2", "shop", "production", "api", "UPDATE", OutcomeFailed, 10, 15, user("u2")),
		deploy("d4", "shop", "production", "api", "UPDATE", OutcomeFailed, 60, 65, user("u1")),
		deploy("d5", "shop", "production", "api", "REMOVE", OutcomeSucceeded, 70, 71, user("u1")),
		deploy("d6", "shop", "staging", "api", "UPDATE", OutcomeRunning, 80, 0, user("u1")),
	}
	items := BuildWorkItems(runs)
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4 (retry chain, open retry, other action, other environment)", len(items))
	}
	first := items[0]
	if first.ID() != "d1" || len(first.Runs) != 3 || first.State() != ItemCompleted || first.FailedRuns() != 2 || first.LastRunID() != "d3" {
		t.Fatalf("retry chain = %+v", first)
	}
	if d, ok := first.Duration(); !ok || d != 40*time.Minute {
		t.Fatalf("duration = %v %v, want first start to successful finish", d, ok)
	}
	if items[1].ID() != "d4" || items[1].State() != ItemFailedOpen {
		t.Fatalf("run after success must open a new item: %+v", items[1])
	}
	if items[3].State() != ItemRunning {
		t.Fatalf("running chain state %s", items[3].State())
	}
	results := Compute(Input{Period: october(), Items: items, ApplicationKeys: []string{"shop"}})
	if v := *result(t, results, "completed_work").Value; v != 2 {
		t.Fatalf("completed work = %v, want 2", v)
	}
	attempts := result(t, results, "execution_attempts")
	if *attempts.Value != 6 || attempts.Details["failedRuns"] != 3 || attempts.Details["runningRuns"] != 1 {
		t.Fatalf("attempts = %+v", attempts)
	}
	if *attempts.Numerator != 4 || *attempts.Denominator != 2 || attempts.Details["attemptsPerItem"] != 2 {
		t.Fatalf("attempts per completed item = %+v", attempts)
	}
	if v := *result(t, results, "deployment_frequency").Value; v != 1 {
		t.Fatalf("production deployments = %v, want 1 (REMOVE excluded, retries once)", v)
	}
	if open := result(t, results, "completed_work").Details["openWork"]; open != 2 {
		t.Fatalf("open work = %v, want 2", open)
	}
}

// Period boundaries follow Asia/Ho_Chi_Minh, not UTC.
func TestPeriodUsesTheReportTimezone(t *testing.T) {
	p, err := NewPeriod("2026-10-02", "2026-10-02", t0)
	if err != nil {
		t.Fatal(err)
	}
	inside := time.Date(2026, 10, 1, 17, 0, 0, 0, time.UTC)   // 00:00 on 10-02 in UTC+7
	outside := time.Date(2026, 10, 1, 16, 59, 0, 0, time.UTC) // 23:59 on 10-01
	if !p.Contains(inside) || p.Contains(outside) || p.Days() != 1 {
		t.Fatalf("timezone boundary wrong: %v %v %d", p.Contains(inside), p.Contains(outside), p.Days())
	}
	def, err := NewPeriod("", "", time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC))
	if err != nil || def.To != "2026-10-24" || def.Days() != DefaultDays {
		t.Fatalf("default period = %+v %v", def, err)
	}
	for _, bad := range [][2]string{{"2026-10-05", "2026-10-01"}, {"2026-13-01", ""}, {"2025-01-01", "2026-10-01"}, {"", "10/01/2026"}} {
		if _, err := NewPeriod(bad[0], bad[1], t0); err == nil {
			t.Fatalf("period %v accepted", bad)
		}
	}
	weeks := Period{From: "2026-10-01", To: "2026-10-30"}
	starts := weeks.BucketStarts()
	if weeks.BucketOf() != BucketWeek || starts[0] != "2026-09-28" || len(starts) != 5 {
		t.Fatalf("weekly buckets = %v", starts)
	}
}

// BR-05: a person is counted once across applications and reference forms.
func TestActiveUsersAreDistinctAndAutomationIsSeparate(t *testing.T) {
	runs := []Run{
		deploy("a", "shop", "production", "api", "DEPLOY", OutcomeSucceeded, 0, 1, user("u1")),
		deploy("b", "blog", "staging", "web", "DEPLOY", OutcomeSucceeded, 0, 1, user("u1")),
		deploy("c", "blog", "staging", "web", "UPDATE", OutcomeSucceeded, 5, 6, Actor{Key: "ci", Name: "ci", Kind: ActorAutomation}),
		{ID: "op", Kind: KindResourceCleanup, ApplicationKey: "blog", EnvironmentKey: "staging", ResourceID: "r1", Action: ActionResourceCleanup, Outcome: OutcomeSucceeded, Actor: user("u2"), StartedAt: at(8), FinishedAt: finished(9)},
	}
	items := BuildWorkItems(runs)
	users := result(t, Compute(Input{Period: october(), Items: items, ApplicationKeys: []string{"shop", "blog"}}), "active_users")
	if *users.Value != 2 || users.Details["automationRuns"] != 1 {
		t.Fatalf("active users = %+v", users)
	}
	shop := Summarize(items[:1], october())
	blog := Summarize(items[1:], october())
	if shop.ActiveUsers+blog.ActiveUsers == int(*users.Value) {
		t.Fatal("test precondition: child rows must overlap")
	}
}

// BR-06/BR-07: parent values are recomputed from records, not from child rows.
func TestParentRatesAndMediansAreRecomputed(t *testing.T) {
	var runs []Run
	add := func(id, app string, minutes int) {
		runs = append(runs, deploy(id, app, "production", id, "DEPLOY", OutcomeSucceeded, 0, minutes, user("u")))
	}
	add("a1", "a", 10)
	add("b1", "b", 20)
	add("b2", "b", 30)
	add("b3", "b", 100)
	items := BuildWorkItems(runs)
	all := result(t, Compute(Input{Period: october(), Items: items, ApplicationKeys: []string{"a", "b"}}), "work_duration")
	if *all.Value != 25*60 || all.Coverage.Covered != 4 || all.Details["p90Seconds"] != 100*60 {
		t.Fatalf("organization median = %+v", all)
	}
	// Averaging child medians (10 and 30 minutes) would give 20 minutes.
	apps := result(t, Compute(Input{Period: october(), Items: items, ApplicationKeys: []string{"a", "b", "idle"}}), "active_applications")
	if *apps.Numerator != 2 || *apps.Denominator != 3 {
		t.Fatalf("active applications = %+v", apps)
	}
}

// ALT-02 and BR-03: no data is not zero, and sources not collected stay explicit.
func TestMissingDataIsNeverZero(t *testing.T) {
	results := Compute(Input{Period: october(), ApplicationKeys: []string{"shop"}})
	for _, id := range []string{"work_duration", "work_duration_vs_baseline", "active_users"} {
		if r := result(t, results, id); r.Status != StatusNoData || r.Value != nil {
			t.Fatalf("%s = %+v, want NO_DATA without value", id, r)
		}
	}
	for _, id := range []string{"change_lead_time", "change_fail_rate", "failed_deployment_recovery_time", "deployment_rework_rate", "request_lead_time", "self_service_rate", "ops_effort", "onboarding_time", "labor_hours_saved"} {
		r := result(t, results, id)
		if r.Status != StatusNotCollected || r.Value != nil || r.RequiredSource == "" {
			t.Fatalf("%s = %+v, want NOT_COLLECTED with its required source", id, r)
		}
	}
	freq := result(t, results, "deployment_frequency")
	if freq.Status != StatusCollected || *freq.Value != 0 || freq.Details["bandNone"] != 1 {
		t.Fatalf("frequency with a source and no deployments = %+v", freq)
	}
}

func TestFrequencyBands(t *testing.T) {
	cases := []struct {
		count, days int
		want        FrequencyBand
	}{{30, 30, BandDaily}, {5, 30, BandWeekly}, {1, 30, BandMonthly}, {1, 90, BandRare}, {0, 7, BandNone}, {1, 7, BandWeekly}}
	for _, c := range cases {
		if got := BandOf(c.count, c.days); got != c.want {
			t.Fatalf("BandOf(%d,%d) = %s, want %s", c.count, c.days, got, c.want)
		}
	}
}

// BR-09/BR-10: versions apply from their effective date; differences keep sign.
func TestBaselineVersionsApplyByEffectiveDate(t *testing.T) {
	v1 := Baseline{Version: 1, ValueSeconds: 3600, EffectiveFrom: "2026-09-01"}
	v2 := Baseline{Version: 2, ValueSeconds: 600, EffectiveFrom: "2026-10-15"}
	v3 := Baseline{Version: 3, Retired: true, EffectiveFrom: "2026-11-01"}
	versions := []Baseline{v3, v2, v1}
	if b := EffectiveBaseline(versions, "2026-10-10"); b == nil || b.Version != 1 {
		t.Fatalf("older period uses %+v, want version 1", b)
	}
	if b := EffectiveBaseline(versions, "2026-10-30"); b == nil || b.Version != 2 {
		t.Fatalf("current period uses %+v, want version 2", b)
	}
	if b := EffectiveBaseline(versions, "2026-11-05"); b != nil {
		t.Fatalf("retired baseline still applies: %+v", b)
	}
	if CurrentVersion(versions) != 3 || CurrentVersion(nil) != 0 {
		t.Fatal("current version")
	}
	items := BuildWorkItems([]Run{deploy("a", "shop", "production", "api", "DEPLOY", OutcomeSucceeded, 0, 20, user("u"))})
	diff := result(t, Compute(Input{Period: october(), Items: items, ApplicationKeys: []string{"shop"}, Baseline: &v2}), "work_duration_vs_baseline")
	if *diff.Value != -600 || diff.Details["baselineVersion"] != 2 {
		t.Fatalf("slower than baseline must stay negative: %+v", diff)
	}
}

func TestBaselineValidation(t *testing.T) {
	size, zero := 12, 0
	from, to := Date("2026-09-01"), Date("2026-09-30")
	ok := Baseline{Metric: MetricWorkDuration, Statistic: StatisticMedian, ValueSeconds: 600, Source: "Khảo sát 12 yêu cầu", EffectiveFrom: "2026-10-01", SampleSize: &size, MeasuredFrom: &from, MeasuredTo: &to}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	retired := Baseline{Metric: MetricWorkDuration, Retired: true, Source: "Không còn phù hợp", EffectiveFrom: "2026-10-01"}
	if err := retired.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(b *Baseline){
		"metric":        func(b *Baseline) { b.Metric = "change_lead_time" },
		"valueSeconds":  func(b *Baseline) { b.ValueSeconds = 0 },
		"source":        func(b *Baseline) { b.Source = " padded " },
		"sampleSize":    func(b *Baseline) { b.SampleSize = &zero },
		"measuredFrom":  func(b *Baseline) { b.MeasuredTo = nil },
		"effectiveFrom": func(b *Baseline) { b.EffectiveFrom = "2026-02-30" },
	}
	for field, mutate := range bad {
		b := ok
		mutate(&b)
		err := b.Validate()
		fe, isField := err.(*FieldError)
		if !isField || fe.Field != field {
			t.Fatalf("%s: error %v", field, err)
		}
	}
	retired.ValueSeconds = 5
	if retired.Validate() == nil {
		t.Fatal("retired version with a value accepted")
	}
}

func TestTrendPlacesCompletionsByLocalDate(t *testing.T) {
	late := time.Date(2026, 10, 1, 17, 30, 0, 0, time.UTC) // 00:30 on 10-02 locally
	r := Run{ID: "x", Kind: KindDeployment, ApplicationKey: "shop", EnvironmentKey: "production", WorkloadID: "api", Action: "DEPLOY", Outcome: OutcomeSucceeded, StartedAt: late.Add(-time.Minute), FinishedAt: &late}
	p := Period{From: "2026-10-01", To: "2026-10-07"}
	bucket, points := Trend(BuildWorkItems([]Run{r}), p)
	if bucket != BucketDay || len(points) != 7 || points[1].ProductionDeployments != 1 || points[0].CompletedWork != 0 {
		t.Fatalf("trend = %s %+v", bucket, points)
	}
}
