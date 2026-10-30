// Package delivery defines provider-neutral workload delivery identities and policy.
package delivery

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("delivery: invalid identity or selection")
	ErrUnsupported = errors.New("delivery: unsupported capability")
	ErrStale       = errors.New("delivery: observation does not match the admitted revision")
)

type Mode string

const (
	Direct Mode = "DIRECT"
	CD     Mode = "CD"
)

// Selection is intent only. Saving it does not change the applied manager.
type Selection struct {
	Mode           Mode   `json:"mode"`
	BindingID      string `json:"bindingId,omitempty"`
	BindingVersion int64  `json:"bindingVersion,omitempty"`
	SelfHeal       bool   `json:"selfHeal"`
}

// Effective preserves pre-feature records as direct delivery without selecting a CD.
func (s Selection) Effective() Selection {
	if s == (Selection{}) {
		return Selection{Mode: Direct}
	}
	return s
}
func (s Selection) Validate() error {
	s = s.Effective()
	switch s.Mode {
	case Direct:
		if s.BindingID != "" || s.BindingVersion != 0 || s.SelfHeal {
			return ErrInvalid
		}
	case CD:
		if s.BindingID == "" || s.BindingVersion < 1 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// Capabilities are verified facts, never inferred from provider kind.
type Capabilities struct {
	ContractVersion   string `json:"contractVersion"`
	ExactRevision     bool   `json:"exactRevision"`
	DriftDetection    bool   `json:"driftDetection"`
	SelfHeal          bool   `json:"selfHeal"`
	PhasedDelivery    bool   `json:"phasedDelivery"`
	RetainOnWithdraw  bool   `json:"retainOnWithdraw"`
	GuardedPrune      bool   `json:"guardedPrune"`
	OwnershipTransfer bool   `json:"ownershipTransfer"`
	Resync            bool   `json:"resync"`
}

func (c Capabilities) ValidateSelection(s Selection) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Effective().Mode == Direct {
		return nil
	}
	if c.ContractVersion == "" || !c.ExactRevision || !c.PhasedDelivery || !c.RetainOnWithdraw || !c.GuardedPrune {
		return ErrUnsupported
	}
	if s.SelfHeal && (!c.SelfHeal || !c.DriftDetection) {
		return ErrUnsupported
	}
	return nil
}

// Scope includes the delivery epoch independently of the infrastructure generation.
type Scope struct {
	OrganizationID   string `json:"organizationId"`
	EnvironmentID    string `json:"environmentId"`
	TargetGeneration int64  `json:"targetGeneration"`
	Epoch            int64  `json:"epoch"`
	UnitKey          string `json:"unitKey"`
}

func (s Scope) Validate() error {
	if s.OrganizationID == "" || s.EnvironmentID == "" || s.UnitKey == "" || s.TargetGeneration < 0 || s.Epoch < 0 {
		return ErrInvalid
	}
	return nil
}

type Phase string

const (
	Prerequisite Phase = "PREREQUISITE"
	Job          Phase = "JOB"
	Rollout      Phase = "ROLLOUT"
	Route        Phase = "ROUTE"
)

// Revision references the immutable artifact. Credentials and manifests are not public DTOs.
type Revision struct {
	ID            string    `json:"id"`
	Scope         Scope     `json:"scope"`
	Phase         Phase     `json:"phase"`
	PayloadDigest string    `json:"payloadDigest"`
	ArtifactRef   string    `json:"-"`
	Selection     Selection `json:"selection"`
}

func (r Revision) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.Selection.Validate(); err != nil {
		return err
	}
	if r.ID == "" || r.ArtifactRef == "" || !digest(r.PayloadDigest) {
		return ErrInvalid
	}
	switch r.Phase {
	case Prerequisite, Job, Rollout, Route:
	default:
		return ErrInvalid
	}
	// A replayable Job is never independently authorized for autonomous healing.
	if r.Selection.SelfHeal && r.Phase == Job {
		return ErrUnsupported
	}
	return nil
}
func digest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type Sync string
type Health string
type Drift string

const (
	InSync           Sync   = "IN_SYNC"
	OutOfSync        Sync   = "OUT_OF_SYNC"
	Pending          Sync   = "PENDING"
	SyncUnknown      Sync   = "UNKNOWN"
	SyncUnsupported  Sync   = "NOT_SUPPORTED"
	Healthy          Health = "HEALTHY"
	Progressing      Health = "PROGRESSING"
	Degraded         Health = "DEGRADED"
	HealthUnknown    Health = "UNKNOWN"
	NotApplicable    Health = "NOT_APPLICABLE"
	NoDrift          Drift  = "NONE"
	Detected         Drift  = "DETECTED"
	DriftUnknown     Drift  = "UNKNOWN"
	DriftUnsupported Drift  = "NOT_SUPPORTED"
)

type ObjectObservation struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	UID        string `json:"uid"`
}

type Observation struct {
	Objects          []ObjectObservation `json:"objects,omitempty"`
	Scope            Scope               `json:"scope"`
	IntendedRevision string              `json:"intendedRevision"`
	ObservedRevision string              `json:"observedRevision,omitempty"`
	PayloadDigest    string              `json:"payloadDigest,omitempty"`
	Sync             Sync                `json:"sync"`
	Health           Health              `json:"health"`
	Drift            Drift               `json:"drift"`
	Complete         bool                `json:"complete"`
	ObservedAt       time.Time           `json:"observedAt"`
}

// Validate rejects a provider's clean result without complete, exact revision evidence.
func (o Observation) Validate(r Revision) error {
	if o.Scope != r.Scope || o.IntendedRevision != r.ID {
		return ErrStale
	}
	switch o.Sync {
	case InSync, OutOfSync, Pending, SyncUnknown, SyncUnsupported:
	default:
		return ErrInvalid
	}
	switch o.Health {
	case Healthy, Progressing, Degraded, HealthUnknown, NotApplicable:
	default:
		return ErrInvalid
	}
	switch o.Drift {
	case NoDrift, Detected, DriftUnknown, DriftUnsupported:
	default:
		return ErrInvalid
	}
	if o.ObservedAt.IsZero() {
		return ErrInvalid
	}
	if o.Sync == InSync && (!o.Complete || o.ObservedRevision != r.ID || o.PayloadDigest != r.PayloadDigest) {
		return ErrStale
	}
	if o.Drift == NoDrift && (!o.Complete || o.ObservedRevision != r.ID || o.PayloadDigest != r.PayloadDigest) {
		return ErrStale
	}
	return nil
}

// Effective marks stale results unknown while retaining their original timestamp and revision.
func (o Observation) Effective(now time.Time, interval time.Duration) Observation {
	if interval <= 0 || o.ObservedAt.IsZero() || o.ObservedAt.After(now) || now.Sub(o.ObservedAt) > 3*interval {
		o.Sync, o.Health, o.Drift, o.Complete = SyncUnknown, HealthUnknown, DriftUnknown, false
	}
	return o
}

// Ready gates execution commit; health alone is never revision acknowledgement.
func (o Observation) Ready(r Revision, now time.Time, interval time.Duration) bool {
	o = o.Effective(now, interval)
	driftAccepted := o.Drift == NoDrift || (r.Selection.Effective().Mode == Direct && o.Drift == DriftUnsupported)
	return r.Validate() == nil && o.Validate(r) == nil && o.Sync == InSync && o.Health == Healthy && driftAccepted
}

// RequiresTransfer includes disabling CD, changing bindings and crossing binding versions.
func RequiresTransfer(applied, desired Selection, occupied bool) (bool, error) {
	if err := applied.Validate(); err != nil {
		return false, err
	}
	if err := desired.Validate(); err != nil {
		return false, err
	}
	applied, desired = applied.Effective(), desired.Effective()
	if !occupied {
		return false, nil
	}
	applied.SelfHeal, desired.SelfHeal = false, false
	return applied != desired, nil
}
