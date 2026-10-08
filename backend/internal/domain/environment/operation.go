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
	OpTransition  OperationKind = "TRANSITION"
	OpCleanup     OperationKind = "CLEANUP"
	OpRecovery    OperationKind = "RECOVERY"
	OpCredentials OperationKind = "CREDENTIALS"
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

// TransitionMode selects what a target change does with data.
type TransitionMode string

// Transition modes.
const (
	ModeDeployNew       TransitionMode = "DEPLOY_NEW"
	ModeMigratePostgres TransitionMode = "MIGRATE_POSTGRES"
)

// TransitionStage is the persisted stage of a target transition.
type TransitionStage string

// Transition stages in execution order (ADR-012).
const (
	StagePreflight    TransitionStage = "PREFLIGHT"
	StageQuiescing    TransitionStage = "QUIESCING"
	StageBackup       TransitionStage = "BACKUP"
	StageProvisioning TransitionStage = "PROVISIONING"
	StageRestoring    TransitionStage = "RESTORING"
	StageDeploying    TransitionStage = "DEPLOYING"
	StageVerifying    TransitionStage = "VERIFYING"
	StageCutover      TransitionStage = "CUTOVER"
	StageSucceeded    TransitionStage = "SUCCEEDED"
)

// TransitionStatus is the overall state of a transition record.
type TransitionStatus string

// Transition statuses.
const (
	TransitionRunning     TransitionStatus = "RUNNING"
	TransitionSucceeded   TransitionStatus = "SUCCEEDED"
	TransitionFailed      TransitionStatus = "FAILED"
	TransitionInterrupted TransitionStatus = "INTERRUPTED"
)

// SourceState says what is known about the source generation.
type SourceState string

// Source generation states.
const (
	SourceAuthoritative SourceState = "AUTHORITATIVE"
	SourceQuiesced      SourceState = "RETAINED_QUIESCED"
	SourceCleaned       SourceState = "CLEANED"
	SourceCleanupFailed SourceState = "CLEANUP_FAILED"
	// SourceNeedsAttention: compensation could not safely restore the source.
	SourceNeedsAttention SourceState = "NEEDS_ATTENTION"
)

// ResourceMapping pairs a source PostgreSQL resource with its destination.
type ResourceMapping struct {
	SourceDescriptor      string `json:"sourceDescriptor"`
	DestinationDescriptor string `json:"destinationDescriptor"`
}

// StageResult records one stage outcome without credentials or data bytes.
type StageResult struct {
	Stage      TransitionStage `json:"stage"`
	Status     string          `json:"status"`
	Message    string          `json:"message,omitempty"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

// BackupRecord is a private on-Pod archive handle: no bytes and no secrets.
type BackupRecord struct {
	SourceDescriptor string `json:"sourceDescriptor"`
	Pod              string `json:"pod"`
	Namespace        string `json:"namespace"`
	Dir              string `json:"dir"`
	SHA256           string `json:"sha256"`
	Bytes            int64  `json:"bytes"`
	Removed          bool   `json:"removed,omitempty"`
}

// Transition is the persisted target-change record and retained source.
type Transition struct {
	ID                 string            `json:"id"`
	OperationID        string            `json:"operationId"`
	ApplicationKey     string            `json:"applicationKey"`
	EnvironmentKey     string            `json:"environmentKey"`
	Mode               TransitionMode    `json:"mode"`
	Stage              TransitionStage   `json:"stage"`
	Status             TransitionStatus  `json:"status"`
	Source             Binding           `json:"source"`
	Destination        Binding           `json:"destination"`
	SourceRuntime      string            `json:"sourceRuntimeStatus,omitempty"`
	DestinationRuntime string            `json:"destinationRuntimeStatus,omitempty"`
	OldSetID           string            `json:"oldDeploymentSetId"`
	NewSetID           string            `json:"newDeploymentSetId,omitempty"`
	ConfigRevisionID   string            `json:"configRevisionId,omitempty"`
	SourceNamespace    string            `json:"sourceNamespace,omitempty"`
	DestNamespace      string            `json:"destinationNamespace,omitempty"`
	Replicas           map[string]int    `json:"sourceReplicas,omitempty"`
	Mappings           []ResourceMapping `json:"mappings,omitempty"`
	Backups            []BackupRecord    `json:"backups,omitempty"`
	DestinationRefs    []string          `json:"destinationResources,omitempty"`
	SourceTargets      []map[string]any  `json:"sourceTargets,omitempty"`
	// DestinationWorkloads names destination workloads this attempt started.
	DestinationWorkloads []string      `json:"destinationWorkloads,omitempty"`
	RoutesMoved          bool          `json:"routesMoved,omitempty"`
	SourceState          SourceState   `json:"sourceState"`
	Stages               []StageResult `json:"stages,omitempty"`
	Failure              string        `json:"failure,omitempty"`
	Compensation         []string      `json:"compensation,omitempty"`
	CompensationFailed   []string      `json:"compensationFailed,omitempty"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}
