package effectiveness

// Summary is one breakdown row: the counts of a child scope, recomputed from
// its own items (never derived from other rows).
type Summary struct {
	ProductionDeployments int
	FrequencyBand         FrequencyBand
	CompletedWork         int
	OpenWork              int
	Attempts              int
	FailedAttempts        int
	MedianDurationSeconds *float64
	ActiveUsers           int
}

// Summarize computes the breakdown row of items over the period.
func Summarize(items []WorkItem, p Period) Summary {
	in := Input{Period: p, Items: items}
	var s Summary
	completed := completedIn(in)
	s.CompletedWork = len(completed)
	for _, w := range completed {
		if IsProductionDeployment(w) {
			s.ProductionDeployments++
		}
	}
	s.FrequencyBand = BandOf(s.ProductionDeployments, p.Days())
	for _, w := range items {
		if OpenAt(w, p.End()) {
			s.OpenWork++
		}
		for _, r := range w.Runs {
			if !p.Contains(r.StartedAt) {
				continue
			}
			s.Attempts++
			if r.Outcome == OutcomeFailed {
				s.FailedAttempts++
			}
		}
	}
	seconds, _ := Durations(completed)
	if median, ok := Median(seconds); ok {
		s.MedianDurationSeconds = num(round(median))
	}
	s.ActiveUsers, _ = ActiveUsers(items, p)
	return s
}

// TrendPoint is one trend bucket.
type TrendPoint struct {
	Start                 Date
	ProductionDeployments int
	CompletedWork         int
}

// Trend counts completions per bucket of the period. Completions are placed
// by the report-timezone date of their finish.
func Trend(items []WorkItem, p Period) (Bucket, []TrendPoint) {
	starts := p.BucketStarts()
	points := make([]TrendPoint, len(starts))
	for i, s := range starts {
		points[i].Start = s
	}
	for _, w := range completedIn(Input{Period: p, Items: items}) {
		day := DateOf(*w.CompletedAt())
		index := 0
		for i, s := range starts {
			if s <= day {
				index = i
			}
		}
		points[index].CompletedWork++
		if IsProductionDeployment(w) {
			points[index].ProductionDeployments++
		}
	}
	return p.BucketOf(), points
}
