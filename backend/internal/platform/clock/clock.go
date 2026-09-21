// Package clock isolates time access so tests stay deterministic.
package clock

import "time"

// Clock reports the current time.
type Clock interface {
	Now() time.Time
}

// System reads the wall clock.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed returns the same instant on every call.
type Fixed struct{ Instant time.Time }

// Now returns the configured instant.
func (f Fixed) Now() time.Time { return f.Instant.UTC() }
