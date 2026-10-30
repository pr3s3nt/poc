package persistence

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"time"
)

// Read-only leases cannot authorize Submit, apply, withdrawal or state writes.
// IntentID pins the admitted revision; stale workers cannot overwrite its successor.
type DeliveryPollLease struct {
	OrganizationID string
	UnitID         string
	IntentID       string
	Owner          string
	Fence          int64
	LeaseUntil     time.Time
	NextPollAt     time.Time
	Sequence       int64
}
type DeliveryObservation struct {
	ID          string             `json:"id"`
	UnitID      string             `json:"unitId"`
	WorkloadID  string             `json:"workloadId"`
	Sequence    int64              `json:"sequence"`
	Observation domain.Observation `json:"observation"`
	ReasonCode  string             `json:"reasonCode,omitempty"`
	StoredAt    time.Time          `json:"storedAt"`
}
type DeliveryObservationRepository interface {
	ClaimDeliveryPoll(context.Context, DeliveryPollLease, time.Time) (DeliveryPollLease, error)
	SaveDeliveryObservation(context.Context, DeliveryPollLease, DeliveryObservation, time.Time) error
	ListDeliveryObservations(context.Context, string, string, int64, int64, string, int) ([]DeliveryObservation, error)
}
