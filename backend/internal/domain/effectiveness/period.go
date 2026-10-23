package effectiveness

import (
	"errors"
	"time"
)

// TimezoneName is the report timezone of UC-17. Viet Nam observes no daylight
// saving, so a fixed UTC+7 zone is exact and needs no tz database.
const TimezoneName = "Asia/Ho_Chi_Minh"

// Location is the report timezone.
var Location = time.FixedZone(TimezoneName, 7*60*60)

// Period bounds.
const (
	DefaultDays = 30
	MaxDays     = 366
	dateLayout  = "2006-01-02"
)

// ErrInvalidDate is returned for a value that is not a YYYY-MM-DD calendar date.
var ErrInvalidDate = errors.New("effectiveness: not a calendar date")

// Date is a report-timezone calendar date in YYYY-MM-DD form. The canonical
// form sorts lexically in date order.
type Date string

// ParseDate accepts only a canonical existing calendar date.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation(dateLayout, s, Location)
	if err != nil || t.Format(dateLayout) != s {
		return "", ErrInvalidDate
	}
	return Date(s), nil
}

// DateOf returns the report-timezone date of an instant.
func DateOf(t time.Time) Date { return Date(t.In(Location).Format(dateLayout)) }

func (d Date) start() time.Time {
	t, _ := time.ParseInLocation(dateLayout, string(d), Location)
	return t
}

// AddDays returns the date n days later (earlier when negative).
func (d Date) AddDays(n int) Date { return DateOf(d.start().AddDate(0, 0, n)) }

// Period is the inclusive date range [From, To] in the report timezone.
type Period struct {
	From, To Date
}

// Start is the first instant of From.
func (p Period) Start() time.Time { return p.From.start() }

// End is the first instant after To (exclusive bound).
func (p Period) End() time.Time { return p.To.start().AddDate(0, 0, 1) }

// Days is the number of calendar days in the period.
func (p Period) Days() int {
	return int(p.To.start().Sub(p.From.start()).Hours()/24+0.5) + 1
}

// Contains reports whether t falls inside the period.
func (p Period) Contains(t time.Time) bool {
	return !t.Before(p.Start()) && t.Before(p.End())
}

// PeriodError names the invalid period parameter.
type PeriodError struct{ Field string }

func (e *PeriodError) Error() string { return "effectiveness: invalid period " + e.Field }

// NewPeriod validates the request bounds. An empty bound takes its default:
// To is today, From is To minus DefaultDays-1 days.
func NewPeriod(from, to string, now time.Time) (Period, error) {
	var p Period
	if to == "" {
		p.To = DateOf(now)
	} else {
		d, err := ParseDate(to)
		if err != nil {
			return Period{}, &PeriodError{Field: "to"}
		}
		p.To = d
	}
	if from == "" {
		p.From = p.To.AddDays(-(DefaultDays - 1))
	} else {
		d, err := ParseDate(from)
		if err != nil {
			return Period{}, &PeriodError{Field: "from"}
		}
		p.From = d
	}
	if p.From > p.To || p.Days() > MaxDays {
		return Period{}, &PeriodError{Field: "from"}
	}
	return p, nil
}

// Bucket is the trend granularity.
type Bucket string

// Trend buckets: daily up to two weeks, otherwise Monday-start weeks.
const (
	BucketDay  Bucket = "DAY"
	BucketWeek Bucket = "WEEK"
)

// BucketOf returns the trend granularity of the period.
func (p Period) BucketOf() Bucket {
	if p.Days() <= 14 {
		return BucketDay
	}
	return BucketWeek
}

// BucketStarts returns the start date of every trend bucket covering the
// period. Weekly buckets start on the Monday on or before From.
func (p Period) BucketStarts() []Date {
	step, first := 1, p.From
	if p.BucketOf() == BucketWeek {
		step = 7
		offset := (int(first.start().Weekday()) + 6) % 7
		first = first.AddDays(-offset)
	}
	var out []Date
	for d := first; d <= p.To; d = d.AddDays(step) {
		out = append(out, d)
	}
	return out
}
