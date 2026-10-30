package persistence

import (
	"context"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"time"
)

// DeliveryBindingRepository scopes all lookups by Organization ID. Writer refs
// are private adapter data; HTTP projections must never serialize them.
type DeliveryBindingRepository interface {
	CreateDeliveryBinding(context.Context, delivery.BindingVersion) error
	GetDeliveryBinding(context.Context, string, string, int64) (delivery.BindingVersion, error)
	ListDeliveryBindings(context.Context, string) ([]delivery.BindingVersion, error)
	VerifyDeliveryBinding(context.Context, string, string, int64, int64, delivery.BindingStatus, delivery.Capabilities, time.Time, string) (delivery.BindingVersion, error)
}

// DeliverySelectionCommand saves desired intent only; applied selection is an
// execution result owned by the deployment/transfer coordinator.
type DeliverySelectionCommand struct {
	OrganizationID                string
	ApplicationKey                string
	EnvironmentKey                string
	ExpectedEnvironmentVersion    int64
	ExpectedConfigurationRevision int64
	Selection                     delivery.Selection
}
type DeliverySelectionRepository interface {
	SelectDelivery(context.Context, DeliverySelectionCommand) (environment.Environment, error)
}

type DeliveryActivation struct {
	ApplicationKey        string
	EnvironmentKey        string
	ConfigurationRevision int64
	Selection             delivery.Selection
	CheckEpoch            bool
	ExpectedEpoch         int64
	NextEpoch             int64
}
type DeliveryActivationRepository interface {
	ActivateDelivery(context.Context, DeliveryActivation) error
}
