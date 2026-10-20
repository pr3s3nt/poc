package environment

import "time"

// OperationKind names what owns an Environment.
type OperationKind string

// Operation kinds. One active operation may own an Environment at a time.
const (
	OpDeploy      OperationKind = "DEPLOY"
	OpRemove      OperationKind = "REMOVE"
	OpRoutes      OperationKind = "ROUTES"
	OpStoreCopy   OperationKind = "STORE_COPY"
	OpCleanup     OperationKind = "CLEANUP"
	OpRecovery    OperationKind = "RECOVERY"
	OpCredentials OperationKind = "CREDENTIALS"
	// OpResourceCleanup destroys an explicitly confirmed set of Terraform
	// resource instances (ADR-014). It is claimed only together with its
	// persisted authorization and selected set.
	OpResourceCleanup OperationKind = "RESOURCE_CLEANUP"
)

// OperationStatus is the persisted state of an operation claim.
type OperationStatus string

// Operation statuses. ACTIVE, INTERRUPTED and RECOVERING all hold the claim;
// only an explicit release moves an operation to a terminal status.
const (
	OpActive      OperationStatus = "ACTIVE"
	OpInterrupted OperationStatus = "INTERRUPTED"
	OpRecovering  OperationStatus = "RECOVERING"
	OpSucceeded   OperationStatus = "SUCCEEDED"
	OpFailed      OperationStatus = "FAILED"
	OpRecovered   OperationStatus = "RECOVERED"
)

// Holds reports whether the status keeps the Environment claimed.
func (s OperationStatus) Holds() bool {
	return s == OpActive || s == OpInterrupted || s == OpRecovering
}

// Operation is the persisted Environment claim. It carries no credentials.
type Operation struct {
	ID             string        `json:"id"`
	ApplicationKey string        `json:"applicationKey"`
	EnvironmentKey string        `json:"environmentKey"`
	Kind           OperationKind `json:"kind"`
	Owner          string        `json:"owner"`
	// Fence increments each time recovery takes the claim over; owner-only
	// writes and release must present the current (Owner, Fence) pair.
	Fence       int64           `json:"fence"`
	Status      OperationStatus `json:"status"`
	Stage       string          `json:"stage,omitempty"`
	Pins        OperationPins   `json:"pins"`
	Detail      map[string]any  `json:"detail,omitempty"`
	Failure     string          `json:"failure,omitempty"`
	StartedAt   time.Time       `json:"startedAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	HeartbeatAt time.Time       `json:"heartbeatAt"`
	Deadline    time.Time       `json:"deadline"`
	FinishedAt  *time.Time      `json:"finishedAt,omitempty"`
}

// OperationPins are the versions and identities verified in the claim
// transaction. A Check flag selects the corresponding field.
type OperationPins struct {
	CheckEnvVersion    bool    `json:"checkEnvVersion,omitempty"`
	EnvVersion         int64   `json:"envVersion,omitempty"`
	CheckDraftVersion  bool    `json:"checkDraftVersion,omitempty"`
	DraftVersion       int64   `json:"draftVersion,omitempty"`
	CheckConfigVersion bool    `json:"checkConfigVersion,omitempty"`
	ConfigVersion      int64   `json:"configVersion,omitempty"`
	CheckSet           bool    `json:"checkSet,omitempty"`
	CurrentSetID       string  `json:"currentSetId,omitempty"`
	CheckRevision      bool    `json:"checkRevision,omitempty"`
	DesiredRevisionID  string  `json:"desiredRevisionId,omitempty"`
	CheckBinding       bool    `json:"checkBinding,omitempty"`
	Binding            Binding `json:"binding,omitempty"`
	CheckStore         bool    `json:"checkStore,omitempty"`
	SecretStoreKey     string  `json:"secretStoreKey,omitempty"`
}
