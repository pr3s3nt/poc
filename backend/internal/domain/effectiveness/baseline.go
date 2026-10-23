package effectiveness

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Baseline metric and statistic of đợt 1.
const (
	MetricWorkDuration = "work_duration"
	StatisticMedian    = "MEDIAN"
	maxBaselineSeconds = 365 * 24 * 60 * 60
	maxSourceRunes     = 2000
)

// Baseline is one immutable version of a comparison value (UC-17 BR-09). An
// empty ApplicationKey is the Organization scope. A retired version stops the
// baseline from its EffectiveFrom date without rewriting older periods.
type Baseline struct {
	ID              string `json:"id"`
	OrganizationKey string `json:"organizationKey"`
	ApplicationKey  string `json:"applicationKey,omitempty"`
	Metric          string `json:"metric"`
	Version         int64  `json:"version"`
	Retired         bool   `json:"retired,omitempty"`
	Statistic       string `json:"statistic,omitempty"`
	ValueSeconds    int64  `json:"valueSeconds,omitempty"`
	Source          string `json:"source"`
	SampleSize      *int   `json:"sampleSize,omitempty"`
	MeasuredFrom    *Date  `json:"measuredFrom,omitempty"`
	MeasuredTo      *Date  `json:"measuredTo,omitempty"`
	EffectiveFrom   Date   `json:"effectiveFrom"`
	// CreatedBy is the User Account ID of the author.
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

// FieldError names the invalid baseline field (a public request field name).
type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "effectiveness: invalid baseline " + e.Field }

// Validate checks the content of a new version. Version, ID and timestamps are
// assigned by the repository.
func (b Baseline) Validate() error {
	if b.Metric != MetricWorkDuration {
		return &FieldError{Field: "metric"}
	}
	if b.Retired {
		if b.ValueSeconds != 0 || b.Statistic != "" {
			return &FieldError{Field: "valueSeconds"}
		}
	} else {
		if b.Statistic != StatisticMedian {
			return &FieldError{Field: "statistic"}
		}
		if b.ValueSeconds <= 0 || b.ValueSeconds > maxBaselineSeconds {
			return &FieldError{Field: "valueSeconds"}
		}
	}
	source := strings.TrimSpace(b.Source)
	if source == "" || source != b.Source || utf8.RuneCountInString(source) > maxSourceRunes {
		return &FieldError{Field: "source"}
	}
	for _, r := range source {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return &FieldError{Field: "source"}
		}
	}
	if b.SampleSize != nil && *b.SampleSize <= 0 {
		return &FieldError{Field: "sampleSize"}
	}
	if (b.MeasuredFrom == nil) != (b.MeasuredTo == nil) {
		return &FieldError{Field: "measuredFrom"}
	}
	if b.MeasuredFrom != nil && *b.MeasuredFrom > *b.MeasuredTo {
		return &FieldError{Field: "measuredFrom"}
	}
	if _, err := ParseDate(string(b.EffectiveFrom)); err != nil {
		return &FieldError{Field: "effectiveFrom"}
	}
	return nil
}

// EffectiveBaseline returns the version that applies to a period ending at to:
// among versions effective on or before to, the latest EffectiveFrom (ties:
// highest version). A retired winner means no baseline applies.
func EffectiveBaseline(versions []Baseline, to Date) *Baseline {
	var winner *Baseline
	for i := range versions {
		v := &versions[i]
		if v.EffectiveFrom > to {
			continue
		}
		if winner == nil || v.EffectiveFrom > winner.EffectiveFrom || (v.EffectiveFrom == winner.EffectiveFrom && v.Version > winner.Version) {
			winner = v
		}
	}
	if winner == nil || winner.Retired {
		return nil
	}
	return winner
}

// CurrentVersion is the highest stored version, 0 when there is none.
func CurrentVersion(versions []Baseline) int64 {
	var current int64
	for _, v := range versions {
		if v.Version > current {
			current = v.Version
		}
	}
	return current
}
