package observability

import (
	"context"
	"time"

	"orchestrator/internal/ports/execution"
)

// JobHistory is live bounded inspection; Kubernetes history policy controls
// retention. It is not the persisted log/metric collector snapshot.
type JobHistory struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Code        string    `json:"code"`
	Jobs        []Job     `json:"jobs"`
	Truncated   bool      `json:"truncated"`
}
type Job struct {
	Name       string     `json:"name"`
	UID        string     `json:"uid"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	State      string     `json:"state"`
}
type JobReader interface {
	InspectJobs(context.Context, Source, execution.ObjectRef) JobHistory
}
