package drift

import "time"

type RepairStatus string

const (
	Preparing    RepairStatus = "PREPARING"
	Ready        RepairStatus = "READY"
	Blocked      RepairStatus = "BLOCKED"
	Expired      RepairStatus = "EXPIRED"
	Consumed     RepairStatus = "CONSUMED"
	RepairFailed RepairStatus = "FAILED"
)

type RepairInstance struct {
	InstanceID      string   `json:"instanceId"`
	InstanceVersion int64    `json:"instanceVersion"`
	BaselineID      string   `json:"baselineId"`
	Report          Report   `json:"report"`
	Blockers        []string `json:"blockers"`
	ArtifactID      string   `json:"-"`
}
type RepairReview struct {
	ID                    string           `json:"id"`
	OrganizationID        string           `json:"-"`
	EnvironmentID         string           `json:"environmentId"`
	ScanID                string           `json:"scanId"`
	ActorID               string           `json:"-"`
	RequestID             string           `json:"-"`
	RequestDigest         string           `json:"-"`
	EnvironmentVersion    int64            `json:"environmentVersion"`
	Status                RepairStatus     `json:"status"`
	Instances             []RepairInstance `json:"instances"`
	Blockers              []string         `json:"blockers"`
	ExpiresAt             time.Time        `json:"expiresAt"`
	CreatedAt             time.Time        `json:"createdAt"`
	OperationID           string           `json:"operationId"`
	ConsumedOperationID   string           `json:"consumedOperationId,omitempty"`
	ConfirmationRequestID string           `json:"-"`
	Version               int64            `json:"-"`
}
