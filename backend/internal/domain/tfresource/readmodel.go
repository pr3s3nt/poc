package tfresource

import "time"

// AttemptSummary is the safe, Console-readable summary of one execution
// attempt: no module or target snapshot, physical ID, state serial or
// recovery handle.
type AttemptSummary struct {
	Number        int
	Action        Action
	Ownership     Ownership
	Stage         string
	Status        AttemptStatus
	SafeErrorCode string
	UpdatedAt     time.Time
}

// ResourceRecord is the Console's safe projection of one resource instance
// (UC-09 RES-01). Instance.TargetRef and Instance.PhysicalIntent are never
// populated by a Console read: those columns belong to the State login.
type ResourceRecord struct {
	Instance       Instance
	ModuleID       string
	ModuleRevision string
	// Attempts counts every execution attempt; zero means no execution history.
	Attempts    int
	LastAttempt *AttemptSummary
}

// CleanupRecord is the Console's safe projection of one RESOURCE_CLEANUP
// operation: the admitted authorization and the selected IDs only, never the
// pinned items (which carry original target references).
type CleanupRecord struct {
	OperationID string
	Mode        CleanupMode
	ActorRef    string
	RootID      string
	SelectedIDs []string
	Status      string
	Stage       string
	Failure     string
	// Detail is the persisted operation detail document; callers project it
	// through an allowlist and never forward it raw.
	Detail     map[string]any
	StartedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}
