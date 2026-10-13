package tfresource

import (
	"sort"
	"strings"
)

// CleanupMode is how a RESOURCE_CLEANUP operation was authorized.
type CleanupMode string

// Cleanup authorization modes (schema cleanup persistence).
const (
	CleanupUserConfirmed CleanupMode = "USER_CONFIRMED"
	CleanupRunCleanup    CleanupMode = "RUN_CLEANUP"
)

// CleanupItem is the pinned record of one selected instance: its version and
// the original target, module and state references that cleanup must use.
type CleanupItem struct {
	InstanceID     string         `json:"instanceId"`
	Version        int64          `json:"version"`
	StateKey       string         `json:"stateKey"`
	ModuleBundleID string         `json:"moduleBundleId"`
	TargetRef      map[string]any `json:"targetRef"`
}

// ConfirmedScope is exactly the selected set that was checked, with the root
// always included. It never grows after admission.
type ConfirmedScope struct {
	SelectedResourceIDs []string          `json:"selectedResourceIds"`
	Items               []CleanupItem     `json:"items"`
	RetainedReasons     map[string]string `json:"retainedReasons,omitempty"`
}

// CleanupRequest is the persisted authorization of one resource cleanup.
// RequestHash identifies the preview token per actor; a replay with the same
// hash returns the admitted operation only when its selection is identical,
// so the stored selection binds the request scope. CandidateIDs are the IDs pinned by the preview token; they bound what may be
// selected and never authorize deletion by themselves.
type CleanupRequest struct {
	Mode             CleanupMode
	ActorRef         string
	RootInstanceID   string
	CandidateIDs     []string
	Scope            ConfirmedScope
	RequestHash      string
	ConfirmationName string
	ScopeSnapshot    map[string]any
	RunID            string
}

// Validate enforces the explicit-selection rules that do not need persisted
// state: the root is selected, no ID repeats, a user selection stays inside
// the pinned candidates, every selected ID has exactly one pinned item and
// nothing is added beyond the selection.
func (r CleanupRequest) Validate() error {
	if strings.TrimSpace(r.ActorRef) == "" || r.RootInstanceID == "" {
		return invalid("cleanup needs an actor and a root")
	}
	selected := map[string]bool{}
	for _, id := range r.Scope.SelectedResourceIDs {
		if id == "" || selected[id] {
			return invalid("cleanup selection has an empty or repeated id")
		}
		selected[id] = true
	}
	if !selected[r.RootInstanceID] {
		return invalid("cleanup selection must include the root")
	}
	if len(r.Scope.Items) != len(selected) {
		return invalid("cleanup items must match the selection exactly")
	}
	for _, item := range r.Scope.Items {
		if !selected[item.InstanceID] {
			return invalid("cleanup items must match the selection exactly")
		}
	}
	seen := map[string]bool{}
	for _, item := range r.Scope.Items {
		if seen[item.InstanceID] {
			return invalid("cleanup items must match the selection exactly")
		}
		seen[item.InstanceID] = true
	}
	switch r.Mode {
	case CleanupUserConfirmed:
		candidates := map[string]bool{r.RootInstanceID: true}
		for _, id := range r.CandidateIDs {
			candidates[id] = true
		}
		for id := range selected {
			if !candidates[id] {
				return invalid("cleanup selection is outside the pinned candidates")
			}
		}
		if r.RequestHash == "" || r.ConfirmationName == "" {
			return invalid("user cleanup needs a request hash and the confirmation name")
		}
	case CleanupRunCleanup:
		if strings.TrimSpace(r.RunID) == "" {
			return invalid("run cleanup needs a run id")
		}
	default:
		return invalid("cleanup authorization mode is unknown")
	}
	return nil
}

// SameSelection reports whether two selections name the same set of IDs.
func SameSelection(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
