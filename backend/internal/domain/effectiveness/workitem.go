// Package effectiveness holds the UC-17 measurement contract: execution runs,
// logical work items, the metric catalog and versioned baselines. Everything
// here is pure; the history it reads is persisted by UC-06/07.
package effectiveness

import (
	"sort"
	"time"
)

// RunKind names the persisted execution a run comes from.
type RunKind string

// Run kinds.
const (
	KindDeployment      RunKind = "DEPLOYMENT"
	KindResourceCleanup RunKind = "RESOURCE_CLEANUP"
)

// ActionResourceCleanup is the action of every cleanup run.
const ActionResourceCleanup = "RESOURCE_CLEANUP"

// Outcome is the normalized result of one run.
type Outcome string

// Run outcomes. Running covers every non-terminal or still-held state.
const (
	OutcomeSucceeded Outcome = "SUCCEEDED"
	OutcomeFailed    Outcome = "FAILED"
	OutcomeRunning   Outcome = "RUNNING"
)

// ActorKind tells a resolved User Account from any other actor reference.
type ActorKind string

// Actor kinds.
const (
	ActorUser       ActorKind = "USER"
	ActorAutomation ActorKind = "AUTOMATION"
)

// Actor is who started a run. Key is the User Account ID for a user, so the
// same person counts once whichever reference form the run stored.
type Actor struct {
	Key  string
	Name string
	Kind ActorKind
}

// Run is one execution the platform performed (UC-17 "lượt chạy").
type Run struct {
	ID             string
	Kind           RunKind
	ApplicationKey string
	EnvironmentKey string
	// WorkloadID is set for Deployments, ResourceID (root) for cleanups.
	WorkloadID string
	ResourceID string
	Action     string
	Outcome    Outcome
	Actor      Actor
	StartedAt  time.Time
	FinishedAt *time.Time
}

// ItemState is the state of a logical work item.
type ItemState string

// Work item states.
const (
	ItemCompleted  ItemState = "COMPLETED"
	ItemRunning    ItemState = "RUNNING"
	ItemFailedOpen ItemState = "FAILED_OPEN"
)

// WorkItem is a chain of runs with the same key ending at the first success
// (UC-17 "công việc logic"). Runs are ordered by start time.
type WorkItem struct {
	Runs []Run
}

func (w WorkItem) first() Run { return w.Runs[0] }
func (w WorkItem) last() Run  { return w.Runs[len(w.Runs)-1] }

// ID is the stable identity of the item: the ID of its first run.
func (w WorkItem) ID() string { return w.first().ID }

// Kind, ApplicationKey, EnvironmentKey, WorkloadID, ResourceID and Action are
// shared by every run of the chain.
func (w WorkItem) Kind() RunKind          { return w.first().Kind }
func (w WorkItem) ApplicationKey() string { return w.first().ApplicationKey }
func (w WorkItem) EnvironmentKey() string { return w.first().EnvironmentKey }
func (w WorkItem) WorkloadID() string     { return w.first().WorkloadID }
func (w WorkItem) ResourceID() string     { return w.first().ResourceID }
func (w WorkItem) Action() string         { return w.first().Action }

// LastRunID is the run a detail link opens.
func (w WorkItem) LastRunID() string { return w.last().ID }

// StartedAt is the start of the first run.
func (w WorkItem) StartedAt() time.Time { return w.first().StartedAt }

// State reports completed, running or failed-and-not-yet-retried.
func (w WorkItem) State() ItemState {
	switch w.last().Outcome {
	case OutcomeSucceeded:
		return ItemCompleted
	case OutcomeRunning:
		return ItemRunning
	default:
		return ItemFailedOpen
	}
}

// CompletedAt is the finish of the successful run, nil when not completed or
// when the success has no recorded finish.
func (w WorkItem) CompletedAt() *time.Time {
	if w.State() != ItemCompleted {
		return nil
	}
	return w.last().FinishedAt
}

// Duration is first start to successful finish (UC-17 "thời gian thực hiện").
func (w WorkItem) Duration() (time.Duration, bool) {
	done := w.CompletedAt()
	if done == nil {
		return 0, false
	}
	return done.Sub(w.StartedAt()), true
}

// FailedRuns counts the failed runs of the chain.
func (w WorkItem) FailedRuns() int {
	n := 0
	for _, r := range w.Runs {
		if r.Outcome == OutcomeFailed {
			n++
		}
	}
	return n
}

// chainKey groups runs into retry chains: Deployments by Environment,
// workload and action; cleanups by Environment and root resource.
func chainKey(r Run) string {
	scope := string(r.Kind) + "\x00" + r.ApplicationKey + "\x00" + r.EnvironmentKey
	if r.Kind == KindResourceCleanup {
		return scope + "\x00" + r.ResourceID
	}
	return scope + "\x00" + r.WorkloadID + "\x00" + r.Action
}

// BuildWorkItems groups runs into logical work items. A chain ends at its
// first successful run; the next run of the same key opens a new item. The
// result is ordered by item start, then ID.
func BuildWorkItems(runs []Run) []WorkItem {
	ordered := append([]Run(nil), runs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].StartedAt.Equal(ordered[j].StartedAt) {
			return ordered[i].StartedAt.Before(ordered[j].StartedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	open := map[string]int{}
	var items []WorkItem
	for _, r := range ordered {
		key := chainKey(r)
		if index, ok := open[key]; ok {
			items[index].Runs = append(items[index].Runs, r)
		} else {
			open[key] = len(items)
			items = append(items, WorkItem{Runs: []Run{r}})
		}
		if r.Outcome == OutcomeSucceeded {
			delete(open, key)
		}
	}
	return items
}
