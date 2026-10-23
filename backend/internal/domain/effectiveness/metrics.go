package effectiveness

import (
	"math"
	"sort"
	"time"
)

// Group is the report section of a metric.
type Group string

// Metric groups.
const (
	GroupDORA        Group = "DORA"
	GroupUsage       Group = "USAGE"
	GroupSelfService Group = "SELF_SERVICE"
	GroupValue       Group = "VALUE"
)

// Unit is the base unit of a metric value.
type Unit string

// Units. RATIO is 0..1.
const (
	UnitCount   Unit = "COUNT"
	UnitRatio   Unit = "RATIO"
	UnitSeconds Unit = "SECONDS"
)

// Status is the collection state of one metric result.
type Status string

// Metric statuses (UC-17 danh mục chỉ số).
const (
	StatusCollected    Status = "COLLECTED"
	StatusNoData       Status = "NO_DATA"
	StatusNotCollected Status = "NOT_COLLECTED"
)

// Required sources of the metrics that are not collected yet.
const (
	SourceCommitLink      = "COMMIT_LINK"
	SourceIncidents       = "INCIDENTS"
	SourceRequests        = "REQUESTS"
	SourceSupportRecords  = "SUPPORT_RECORDS"
	SourceEffortRecords   = "EFFORT_RECORDS"
	SourceOnboardingStart = "ONBOARDING_START"
)

// ProductionEnvironment is the only Environment DORA metrics read (BR-11).
const ProductionEnvironment = "production"

// Coverage is how many of the eligible records carry the data a value needs.
type Coverage struct {
	Covered int
	Total   int
}

// Result is the computed value of one catalog entry.
type Result struct {
	ID             string
	Group          Group
	Unit           Unit
	Status         Status
	Value          *float64
	Numerator      *float64
	Denominator    *float64
	Coverage       *Coverage
	Details        map[string]float64
	RequiredSource string
}

// Input is everything a metric reads for one scope and period. Items are
// already limited to the scope; ApplicationKeys are the scope's Applications.
type Input struct {
	Period          Period
	Items           []WorkItem
	ApplicationKeys []string
	Baseline        *Baseline
}

// Definition is one catalog entry. A nil Compute marks a metric whose source
// is not collected yet; adding a source means adding Compute and Records.
type Definition struct {
	ID             string
	Group          Group
	Unit           Unit
	RequiredSource string
	Compute        func(Input) Result
	// Records selects the work items that are the evidence of the value.
	Records func(Input) []WorkItem
}

// Catalog is the ordered UC-17 metric catalog. IDs are stable public names.
var Catalog = []Definition{
	{ID: "deployment_frequency", Group: GroupDORA, Unit: UnitCount, Compute: deploymentFrequency, Records: productionDeployments},
	{ID: "change_lead_time", Group: GroupDORA, Unit: UnitSeconds, RequiredSource: SourceCommitLink},
	{ID: "change_fail_rate", Group: GroupDORA, Unit: UnitRatio, RequiredSource: SourceIncidents},
	{ID: "failed_deployment_recovery_time", Group: GroupDORA, Unit: UnitSeconds, RequiredSource: SourceIncidents},
	{ID: "deployment_rework_rate", Group: GroupDORA, Unit: UnitRatio, RequiredSource: SourceIncidents},
	{ID: "active_applications", Group: GroupUsage, Unit: UnitCount, Compute: activeApplications, Records: completedIn},
	{ID: "active_users", Group: GroupUsage, Unit: UnitCount, Compute: activeUsers, Records: startedIn},
	{ID: "completed_work", Group: GroupUsage, Unit: UnitCount, Compute: completedWork, Records: completedIn},
	{ID: "execution_attempts", Group: GroupUsage, Unit: UnitCount, Compute: executionAttempts, Records: startedIn},
	{ID: "work_duration", Group: GroupSelfService, Unit: UnitSeconds, Compute: workDuration, Records: completedIn},
	{ID: "work_duration_vs_baseline", Group: GroupValue, Unit: UnitSeconds, Compute: workDurationVsBaseline, Records: completedIn},
	{ID: "request_lead_time", Group: GroupSelfService, Unit: UnitSeconds, RequiredSource: SourceRequests},
	{ID: "self_service_rate", Group: GroupSelfService, Unit: UnitRatio, RequiredSource: SourceSupportRecords},
	{ID: "ops_effort", Group: GroupSelfService, Unit: UnitSeconds, RequiredSource: SourceEffortRecords},
	{ID: "onboarding_time", Group: GroupSelfService, Unit: UnitSeconds, RequiredSource: SourceOnboardingStart},
	{ID: "labor_hours_saved", Group: GroupValue, Unit: UnitSeconds, RequiredSource: SourceEffortRecords},
}

// Lookup returns the catalog entry with this ID.
func Lookup(id string) (Definition, bool) {
	for _, d := range Catalog {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// Compute evaluates every catalog entry for the input.
func Compute(in Input) []Result {
	out := make([]Result, 0, len(Catalog))
	for _, d := range Catalog {
		if d.Compute == nil {
			out = append(out, Result{ID: d.ID, Group: d.Group, Unit: d.Unit, Status: StatusNotCollected, RequiredSource: d.RequiredSource})
			continue
		}
		r := d.Compute(in)
		r.ID, r.Group, r.Unit = d.ID, d.Group, d.Unit
		out = append(out, r)
	}
	return out
}

// RecordsOf returns the evidence items of a metric; nil for a not collected one.
func RecordsOf(d Definition, in Input) []WorkItem {
	if d.Records == nil {
		return nil
	}
	return d.Records(in)
}

// AllRecords returns every item relevant to the period: completed in it, with
// a run started in it, or still open at its end.
func AllRecords(in Input) []WorkItem {
	var out []WorkItem
	for _, w := range in.Items {
		if completedInPeriod(w, in.Period) || RunsStartedIn(w, in.Period) > 0 || OpenAt(w, in.Period.End()) {
			out = append(out, w)
		}
	}
	return out
}

func num(v float64) *float64 { return &v }

func completedInPeriod(w WorkItem, p Period) bool {
	done := w.CompletedAt()
	return done != nil && p.Contains(*done)
}

// RunsStartedIn counts the runs of the item that started inside the period.
func RunsStartedIn(w WorkItem, p Period) int {
	n := 0
	for _, r := range w.Runs {
		if p.Contains(r.StartedAt) {
			n++
		}
	}
	return n
}

// OpenAt reports whether the item had started and was not completed at t.
func OpenAt(w WorkItem, t time.Time) bool {
	if !w.StartedAt().Before(t) {
		return false
	}
	if w.State() != ItemCompleted {
		return true
	}
	done := w.CompletedAt()
	return done != nil && !done.Before(t)
}

func completedIn(in Input) []WorkItem {
	var out []WorkItem
	for _, w := range in.Items {
		if completedInPeriod(w, in.Period) {
			out = append(out, w)
		}
	}
	return out
}

func startedIn(in Input) []WorkItem {
	var out []WorkItem
	for _, w := range in.Items {
		if RunsStartedIn(w, in.Period) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// IsProductionDeployment reports a delivered production change: a Deployment
// DEPLOY or UPDATE on the production Environment. REMOVE is decommissioning.
func IsProductionDeployment(w WorkItem) bool {
	return w.Kind() == KindDeployment && w.EnvironmentKey() == ProductionEnvironment && (w.Action() == "DEPLOY" || w.Action() == "UPDATE")
}

func productionDeployments(in Input) []WorkItem {
	var out []WorkItem
	for _, w := range completedIn(in) {
		if IsProductionDeployment(w) {
			out = append(out, w)
		}
	}
	return out
}

// FrequencyBand classifies a production deployment count over a period.
type FrequencyBand string

// Frequency bands, from the DORA reporting categories.
const (
	BandDaily   FrequencyBand = "DAILY"
	BandWeekly  FrequencyBand = "WEEKLY"
	BandMonthly FrequencyBand = "MONTHLY"
	BandRare    FrequencyBand = "RARE"
	BandNone    FrequencyBand = "NONE"
)

// BandOf returns the band of count deployments over days.
func BandOf(count, days int) FrequencyBand {
	switch rate := float64(count) / float64(days); {
	case count == 0:
		return BandNone
	case rate >= 1:
		return BandDaily
	case rate*7 >= 1:
		return BandWeekly
	case rate*30 >= 1:
		return BandMonthly
	default:
		return BandRare
	}
}

var bandDetail = map[FrequencyBand]string{
	BandDaily: "bandDaily", BandWeekly: "bandWeekly", BandMonthly: "bandMonthly", BandRare: "bandRare", BandNone: "bandNone",
}

func deploymentFrequency(in Input) Result {
	items := productionDeployments(in)
	days := in.Period.Days()
	count := float64(len(items))
	details := map[string]float64{"perWeek": round(count / float64(days) * 7)}
	perApp := map[string]int{}
	for _, w := range items {
		perApp[w.ApplicationKey()]++
	}
	for _, band := range bandDetail {
		details[band] = 0
	}
	for _, key := range in.ApplicationKeys {
		details[bandDetail[BandOf(perApp[key], days)]]++
	}
	return Result{Status: StatusCollected, Value: num(count), Numerator: num(count), Denominator: num(float64(days)), Details: details}
}

func activeApplications(in Input) Result {
	total := len(in.ApplicationKeys)
	if total == 0 {
		return Result{Status: StatusNoData}
	}
	active := map[string]bool{}
	for _, w := range completedIn(in) {
		active[w.ApplicationKey()] = true
	}
	n := float64(len(active))
	return Result{Status: StatusCollected, Value: num(n), Numerator: num(n), Denominator: num(float64(total))}
}

// ActiveUsers counts distinct User Accounts and runs by other actors among
// runs started in the period.
func ActiveUsers(items []WorkItem, p Period) (users int, automationRuns int) {
	seen := map[string]bool{}
	for _, w := range items {
		for _, r := range w.Runs {
			if !p.Contains(r.StartedAt) {
				continue
			}
			if r.Actor.Kind == ActorUser {
				seen[r.Actor.Key] = true
			} else {
				automationRuns++
			}
		}
	}
	return len(seen), automationRuns
}

func activeUsers(in Input) Result {
	users, automation := ActiveUsers(in.Items, in.Period)
	if users == 0 && automation == 0 {
		return Result{Status: StatusNoData}
	}
	return Result{Status: StatusCollected, Value: num(float64(users)), Details: map[string]float64{"automationRuns": float64(automation)}}
}

func completedWork(in Input) Result {
	items := completedIn(in)
	details := map[string]float64{"deploy": 0, "update": 0, "remove": 0, "resourceCleanup": 0, "production": 0, "staging": 0, "openWork": 0}
	for _, w := range items {
		switch w.Action() {
		case "DEPLOY":
			details["deploy"]++
		case "UPDATE":
			details["update"]++
		case "REMOVE":
			details["remove"]++
		case ActionResourceCleanup:
			details["resourceCleanup"]++
		}
		if w.EnvironmentKey() == ProductionEnvironment {
			details["production"]++
		} else {
			details["staging"]++
		}
	}
	for _, w := range in.Items {
		if OpenAt(w, in.Period.End()) {
			details["openWork"]++
		}
	}
	return Result{Status: StatusCollected, Value: num(float64(len(items))), Details: details}
}

func executionAttempts(in Input) Result {
	started, failed, running := 0, 0, 0
	for _, w := range in.Items {
		for _, r := range w.Runs {
			if !in.Period.Contains(r.StartedAt) {
				continue
			}
			started++
			switch r.Outcome {
			case OutcomeFailed:
				failed++
			case OutcomeRunning:
				running++
			}
		}
	}
	details := map[string]float64{"failedRuns": float64(failed), "runningRuns": float64(running)}
	res := Result{Status: StatusCollected, Value: num(float64(started)), Details: details}
	completed := completedIn(in)
	if len(completed) > 0 {
		runs := 0
		for _, w := range completed {
			runs += len(w.Runs)
		}
		res.Numerator, res.Denominator = num(float64(runs)), num(float64(len(completed)))
		details["attemptsPerItem"] = round(float64(runs) / float64(len(completed)))
	}
	return res
}

// Durations returns the execution durations, in seconds, of the items with a
// recorded successful finish, and how many items were eligible.
func Durations(items []WorkItem) (seconds []float64, eligible int) {
	for _, w := range items {
		eligible++
		if d, ok := w.Duration(); ok && d >= 0 {
			seconds = append(seconds, d.Seconds())
		}
	}
	return seconds, eligible
}

// Percentile is the nearest-rank percentile; Median averages the two middle
// values. Both return false for an empty sample.
func Percentile(values []float64, p float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	return sorted[rank], true
}

// Median returns the median of values.
func Median(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid], true
	}
	return (sorted[mid-1] + sorted[mid]) / 2, true
}

func workDuration(in Input) Result {
	seconds, eligible := Durations(completedIn(in))
	median, ok := Median(seconds)
	if !ok {
		return Result{Status: StatusNoData, Coverage: &Coverage{Covered: 0, Total: eligible}}
	}
	p90, _ := Percentile(seconds, 90)
	return Result{Status: StatusCollected, Value: num(round(median)), Coverage: &Coverage{Covered: len(seconds), Total: eligible}, Details: map[string]float64{"p90Seconds": round(p90)}}
}

func workDurationVsBaseline(in Input) Result {
	current := workDuration(in)
	if in.Baseline == nil || current.Status != StatusCollected {
		return Result{Status: StatusNoData, Coverage: current.Coverage}
	}
	base := float64(in.Baseline.ValueSeconds)
	return Result{Status: StatusCollected, Value: num(round(base - *current.Value)), Coverage: current.Coverage,
		Details: map[string]float64{"baselineSeconds": base, "currentSeconds": *current.Value, "baselineVersion": float64(in.Baseline.Version)}}
}

func round(v float64) float64 { return math.Round(v*100) / 100 }
