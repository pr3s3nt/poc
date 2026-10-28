package delivery

import "errors"

var ErrOutcomeUnknown = errors.New("delivery: prior mutation outcome must be resolved")
var ErrTransition = errors.New("delivery: invalid intent transition")

type Action string

const (
	Submit    Action = "SUBMIT"
	Reconcile Action = "RECONCILE"
	Withdraw  Action = "WITHDRAW"
	Adopt     Action = "ADOPT"
)

type IntentStatus string

const (
	IntentPending      IntentStatus = "PENDING"
	IntentRunning      IntentStatus = "RUNNING"
	IntentAcknowledged IntentStatus = "ACKNOWLEDGED"
	IntentFailed       IntentStatus = "FAILED"
	IntentUnknown      IntentStatus = "OUTCOME_UNKNOWN"
)

// Mutation records persisted authorization for a single external effect. Expired
// worker leases do not change its state or authorize another call.
type Mutation struct {
	ID            string       `json:"id"`
	Scope         Scope        `json:"scope"`
	RevisionID    string       `json:"revisionId"`
	OperationID   string       `json:"operationId"`
	RequestID     string       `json:"requestId"`
	RequestDigest string       `json:"requestDigest"`
	Action        Action       `json:"action"`
	Status        IntentStatus `json:"status"`
	Owner         string       `json:"owner"`
	Fence         int64        `json:"fence"`
	Version       int64        `json:"version"`
}

func (m Mutation) Validate() error {
	if m.Scope.Validate() != nil || m.ID == "" || m.RevisionID == "" || m.OperationID == "" || m.RequestID == "" || !digest(m.RequestDigest) || m.Owner == "" || m.Fence < 1 || m.Version < 1 {
		return ErrInvalid
	}
	switch m.Action {
	case Submit, Reconcile, Withdraw, Adopt:
	default:
		return ErrInvalid
	}
	switch m.Status {
	case IntentPending, IntentRunning, IntentAcknowledged, IntentFailed, IntentUnknown:
	default:
		return ErrInvalid
	}
	return nil
}

// Advance requires observation to resolve a running/unknown call, even after a
// process restart. External proof is gathered by the application coordinator;
// clients cannot supply this flag through a public mutation request.
func (m Mutation) Advance(next IntentStatus, outcomeObserved bool) (Mutation, error) {
	if err := m.Validate(); err != nil {
		return Mutation{}, err
	}
	if next == m.Status {
		return m, nil
	}
	allowed := false
	switch m.Status {
	case IntentPending:
		allowed = next == IntentRunning
	case IntentRunning:
		allowed = next == IntentAcknowledged || next == IntentFailed || next == IntentUnknown
		if next != IntentUnknown && !outcomeObserved {
			return Mutation{}, ErrOutcomeUnknown
		}
	case IntentUnknown:
		if !outcomeObserved {
			return Mutation{}, ErrOutcomeUnknown
		}
		allowed = next == IntentAcknowledged || next == IntentFailed
	}
	if !allowed {
		return Mutation{}, ErrTransition
	}
	m.Status = next
	m.Version++
	return m, nil
}

// SameRequest detects conflicting idempotency-key reuse in its exact scope.
func (m Mutation) SameRequest(other Mutation) bool {
	return m.Scope == other.Scope && m.OperationID == other.OperationID && m.RevisionID == other.RevisionID && m.Action == other.Action && m.RequestID == other.RequestID && m.RequestDigest == other.RequestDigest
}
