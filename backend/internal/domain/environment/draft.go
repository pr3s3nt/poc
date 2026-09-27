package environment

// DraftState distinguishes a pending workload upsert from pending deletion.
type DraftState string

const (
	DraftUpsert DraftState = "PENDING_UPSERT"
	DraftDelete DraftState = "PENDING_DELETE"
)

// WorkloadDraft is desired UC-16 state, separate from the current Deployment Set.
// Score contains references only, never copied Application secret values.
type WorkloadDraft struct {
	ApplicationKey string         `json:"applicationKey"`
	EnvironmentKey string         `json:"environmentKey"`
	WorkloadID     string         `json:"workloadId"`
	State          DraftState     `json:"state"`
	Score          map[string]any `json:"score,omitempty"`
}
