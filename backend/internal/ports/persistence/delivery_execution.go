package persistence

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"time"
)

// DeliveryExecution keeps replayable artifact handles and external receipts out
// of public WorkloadInstance JSON while preserving the original target/binding.
type DeliveryExecution struct {
	UnitID         string
	EnvironmentID  string
	OrganizationID string
	WorkloadID     string
	Revision       domain.Revision
	Mutation       domain.Mutation
	Target         execution.Target
	Receipt        port.Receipt
	Inventory      []DeliveryObject
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
type DeliveryObject = port.AppliedObject

// DeliveryExecutionRepository writes only under an active matching Environment
// owner/fence. Observations have their own read-only lease protocol.
type DeliveryExecutionRepository interface {
	PrepareDelivery(context.Context, DeliveryExecution) (DeliveryExecution, error)
	UpdateDeliveryExecution(context.Context, DeliveryExecution, int64) (DeliveryExecution, error)
	GetDeliveryExecution(context.Context, string, string) (DeliveryExecution, error)
	ListDeliveryExecutions(context.Context, string, string) ([]DeliveryExecution, error)
}

// CipherArtifact is encrypted with the existing state cipher primitives. Its AAD
// binds Organization, purpose, owner and artifact ID; no plaintext is persisted.
type CipherArtifact struct {
	ID             string
	OrganizationID string
	OwnerID        string
	Purpose        string
	KeyID          string
	Nonce          []byte
	Ciphertext     []byte
	AADVersion     int64
	CreatedAt      time.Time
}
type DeliveryArtifactRepository interface {
	PutDeliveryArtifact(context.Context, CipherArtifact) error
	GetDeliveryArtifact(context.Context, string, string) (CipherArtifact, error)
}

// GitPublicationLease has no automatic mutation takeover on expiry. Release is
// explicit after the caller proves its remote operation terminal.
type GitPublicationLease struct {
	Destination    string
	OrganizationID string
	Owner          Owner
}
type GitPublicationRepository interface {
	ClaimGitPublication(context.Context, GitPublicationLease) error
	CheckGitPublication(context.Context, GitPublicationLease) error
	ReleaseGitPublication(context.Context, GitPublicationLease) error
}

// DeliveryRecoveryRepository is used only after explicit confirmation that the
// prior executor stopped and BeginRecovery fenced it. Only transfer adoption /
// retained withdrawal are replayable through incarnation-aware adapter methods;
// an unknown normal Submit is never reset by this protocol.
type DeliveryRecoveryRepository interface {
	// RebindDeliverySubmission keeps RUNNING/OUTCOME_UNKNOWN unchanged; it
	// never resets a Submit to PENDING or authorizes replay of Submit.
	RebindDeliverySubmission(context.Context, string, string, int64) (DeliveryExecution, error)
	ResumeDeliveryExecution(context.Context, string, string, int64) (DeliveryExecution, error)
	RecoverGitPublication(context.Context, string, string) error
}
