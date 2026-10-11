// Package clock abstracts time so supervisor timers are testable.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

type Real struct{}

func (Real) Now() time.Time        { return time.Now().UTC() }
func (Real) Sleep(d time.Duration) { time.Sleep(d) }

// Fake is a manually advanced clock. Sleep advances time instead of blocking.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

func NewFake(start time.Time) *Fake { return &Fake{now: start.UTC()} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

func (f *Fake) Sleep(d time.Duration) { f.Advance(d) }
