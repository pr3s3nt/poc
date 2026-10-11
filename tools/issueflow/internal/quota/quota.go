// Package quota classifies agent failures and computes resume times.
package quota

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Class string

const (
	None      Class = ""
	Quota     Class = "quota"
	Context   Class = "context"
	Auth      Class = "auth"
	Transient Class = "transient"
)

type Result struct {
	Class   Class
	Match   string
	ResetAt time.Time // zero when unknown
}

type Classifier struct {
	quota, ctx, auth []*regexp.Regexp
	MarginSeconds    int
	Backoff          []int
	Loc              *time.Location
}

func compile(ps []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range ps {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

func New(quotaP, ctxP, authP []string, margin int, backoff []int) *Classifier {
	return &Classifier{quota: compile(quotaP), ctx: compile(ctxP), auth: compile(authP), MarginSeconds: margin, Backoff: backoff, Loc: time.Local}
}

// Classify inspects output text. Auth wins over quota wins over context so a
// login problem is never hidden behind a timer.
func (c *Classifier) Classify(text string, now time.Time) Result {
	for _, set := range []struct {
		res []*regexp.Regexp
		cl  Class
	}{{c.auth, Auth}, {c.quota, Quota}, {c.ctx, Context}} {
		for _, re := range set.res {
			loc := re.FindStringIndex(text)
			if loc == nil {
				continue
			}
			r := Result{Class: set.cl, Match: strings.TrimSpace(text[loc[0]:loc[1]])}
			if set.cl == Quota {
				// Look at the surrounding window for a reset time.
				end := loc[1] + 240
				if end > len(text) {
					end = len(text)
				}
				start := loc[0] - 120
				if start < 0 {
					start = 0
				}
				if t, ok := ParseReset(text[start:end], now, c.Loc); ok {
					r.ResetAt = t
				}
			}
			return r
		}
	}
	return Result{}
}

// WakeAt returns the resume time: known reset + margin, otherwise backoff[attempt].
func (c *Classifier) WakeAt(r Result, attempt int, now time.Time) (time.Time, string) {
	if !r.ResetAt.IsZero() && r.ResetAt.After(now.Add(-time.Hour)) {
		w := r.ResetAt.Add(time.Duration(c.MarginSeconds) * time.Second)
		if w.After(now) {
			return w.UTC(), "reset"
		}
	}
	i := attempt
	if i >= len(c.Backoff) {
		i = len(c.Backoff) - 1
	}
	return now.Add(time.Duration(c.Backoff[i]) * time.Second).UTC(), "backoff"
}

var (
	reClock = regexp.MustCompile(`(?i)(?:resets?|try again|available again)[^0-9\n]{0,24}?(\d{1,2})(?::(\d{2}))?\s*([ap]m)(?:\s*\(([A-Za-z_]+(?:/[A-Za-z_+-]+)*)\))?`)
	reIn    = regexp.MustCompile(`(?i)\bin\s+(?:(\d+)\s*(?:h|hours?|hrs?))?\s*(?:(\d+)\s*(?:m|mins?|minutes?))?`)
	reISO   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)
)

// ParseReset understands "resets 3pm (Asia/Ho_Chi_Minh)", "in 2h 10m" and RFC3339.
func ParseReset(text string, now time.Time, def *time.Location) (time.Time, bool) {
	if m := reISO.FindString(text); m != "" {
		if t, err := time.Parse(time.RFC3339Nano, m); err == nil {
			return t.UTC(), true
		}
	}
	if m := reClock.FindStringSubmatch(text); m != nil {
		loc := def
		if m[4] != "" {
			if l, err := time.LoadLocation(m[4]); err == nil {
				loc = l
			}
		}
		h, _ := strconv.Atoi(m[1])
		min := 0
		if m[2] != "" {
			min, _ = strconv.Atoi(m[2])
		}
		pm := strings.EqualFold(m[3], "pm")
		if h == 12 {
			h = 0
		}
		if pm {
			h += 12
		}
		ln := now.In(loc)
		t := time.Date(ln.Year(), ln.Month(), ln.Day(), h, min, 0, 0, loc)
		if !t.After(ln) {
			t = t.AddDate(0, 0, 1)
		}
		return t.UTC(), true
	}
	for _, m := range reIn.FindAllStringSubmatch(text, -1) {
		if m[1] == "" && m[2] == "" {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		return now.Add(time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute).UTC(), true
	}
	return time.Time{}, false
}
