package terraformstate

import (
	"context"
	"time"

	"orchestrator/internal/domain/drift"
	"orchestrator/internal/ports/persistence"
)

// ExecutionBaseline is immutable acknowledgement evidence, never reconstructed
// from today's definition, draft or provider values. Its private artifact is
// accessible only through the State runtime and purpose-bound encryption.
type ExecutionBaseline struct {
	ID, OrganizationID, EnvironmentID, InstanceID, AttemptID, ModuleBundleID string
	InstanceVersion, StateVersion, Serial                                    int64
	Lineage, ArtifactID                                                      string
	Dependencies                                                             []BaselineDependency
	CreatedAt                                                                time.Time
}

type BaselineDependency struct {
	InstanceID            string
	Version, StateVersion int64
}

// BaselineRepository is deliberately separate from Console persistence. Writes
// require the current Environment owner/fence and a successful acknowledged
// instance/attempt; the artifact insert and baseline append are atomic.
type BaselineRepository interface {
	AppendExecutionBaseline(context.Context, ExecutionBaseline, persistence.CipherArtifact) (ExecutionBaseline, error)
	LatestExecutionBaseline(context.Context, string, string) (ExecutionBaseline, error)
	FindExecutionBaselineVersion(context.Context, string, string, int64, int64) (ExecutionBaseline, error)
	ReadBaselineArtifact(context.Context, string, string) (persistence.CipherArtifact, error)
}

// RepairArtifactRepository restricts saved plans/private comparison pins to the
// same State runtime as input baselines. Console stores only opaque handles.
type RepairArtifactRepository interface {
	PutRepairArtifact(context.Context, persistence.CipherArtifact) error
	ReadRepairArtifact(context.Context, string, string) (persistence.CipherArtifact, error)
}

type PrivateRepairReview struct {
	Review    drift.RepairReview
	Token     persistence.CipherArtifact
	TokenHash string
}

// Review rows and token ciphertext are State-private. Every write requires the
// current owner/fence; confirmation consumes READY exactly once under its claim.
type RepairReviewRepository interface {
	CreateRepairReview(context.Context, PrivateRepairReview) error
	UpdateRepairReview(context.Context, PrivateRepairReview, int64) error
	GetRepairReview(context.Context, string, string, string) (PrivateRepairReview, error)
	FindRepairReviewRequest(context.Context, string, string, string, string) (PrivateRepairReview, error)
	ConsumeRepairReview(context.Context, string, string, string, string, string, string, int64) (PrivateRepairReview, error)
}
