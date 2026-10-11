package quota

import (
	"testing"
	"time"

	"issueflow/internal/config"
)

func classifier() *Classifier {
	c := config.Config{}
	c.Defaults()
	q := New(c.Quota.QuotaPatterns, c.Quota.ContextPatterns, c.Quota.AuthPatterns, 90, []int{900, 1800, 3600})
	q.Loc = time.UTC
	return q
}

var now = time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC)

func TestParseResetForms(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	cases := []struct {
		name, text string
		want       time.Time
	}{
		{"relative hours and minutes", "limit reached. resets in 2h 10m", now.Add(2*time.Hour + 10*time.Minute)},
		{"relative minutes only", "try again in 45 minutes", now.Add(45 * time.Minute)},
		{"rfc3339", "usage limit reached, try again at 2026-10-11T12:30:00Z", time.Date(2026, 10, 11, 12, 30, 0, 0, time.UTC)},
		{"clock later today utc", "resets 3pm", time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC)},
		{"clock already passed rolls to tomorrow", "resets 7am", time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC)},
		{"clock with zone", "resets 9pm (Asia/Ho_Chi_Minh)", time.Date(2026, 10, 11, 21, 0, 0, 0, loc).UTC()},
		{"clock with minutes", "resets at 11:30pm", time.Date(2026, 10, 11, 23, 30, 0, 0, time.UTC)},
		{"noon", "resets 12pm", time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseReset(c.text, now, time.UTC)
			if !ok || !got.Equal(c.want) {
				t.Fatalf("got %v ok=%v want %v", got, ok, c.want)
			}
		})
	}
	if _, ok := ParseReset("nothing useful", now, time.UTC); ok {
		t.Fatal("garbage parsed")
	}
}

func TestClassifyDistinguishesQuotaContextAuthAndNothing(t *testing.T) {
	q := classifier()
	cases := map[string]Class{
		"Claude usage limit reached. resets in 1h": Quota,
		"You've hit your limit":                    Quota,
		"Error: prompt is too long":                Context,
		"Context window is full":                   Context,
		"Invalid API key · Please run /login":      Auth,
		"All tests passed. Wrote the limit helper": None,
		"": None,
	}
	for text, want := range cases {
		if got := q.Classify(text, now).Class; got != want {
			t.Errorf("%q: got %q want %q", text, got, want)
		}
	}
	// auth wins over quota text in the same output
	if got := q.Classify("usage limit reached\nPlease run /login", now).Class; got != Auth {
		t.Fatalf("auth must not hide behind a timer: %q", got)
	}
}

func TestWakeUsesResetPlusMarginElseBackoff(t *testing.T) {
	q := classifier()
	r := q.Classify("usage limit reached. resets in 1h 0m", now)
	at, how := q.WakeAt(r, 0, now)
	if how != "reset" || !at.Equal(now.Add(time.Hour+90*time.Second)) {
		t.Fatalf("%v %s", at, how)
	}
	r = q.Classify("usage limit reached", now)
	for i, want := range []time.Duration{900 * time.Second, 1800 * time.Second, 3600 * time.Second, 3600 * time.Second} {
		at, how := q.WakeAt(r, i, now)
		if how != "backoff" || !at.Equal(now.Add(want)) {
			t.Fatalf("attempt %d: %v %s", i, at, how)
		}
	}
	// a reset time in the past must not produce an instant wake loop
	r = Result{Class: Quota, ResetAt: now.Add(-5 * time.Minute)}
	at, how = q.WakeAt(r, 0, now)
	if how != "backoff" || !at.After(now) {
		t.Fatalf("stale reset: %v %s", at, how)
	}
}
