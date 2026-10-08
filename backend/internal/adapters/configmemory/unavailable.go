package configmemory

import (
	"context"
	"errors"

	configport "orchestrator/internal/ports/configuration"
)

var ErrUnavailable = errors.New("configuration provider is not configured")

// Unavailable keeps non-UC-12 API available when no store registry exists.
type Unavailable struct{}

func (Unavailable) Provider(context.Context, string, string) (configport.Provider, error) {
	return nil, ErrUnavailable
}

func (Unavailable) PrepareBundle(context.Context, configport.BundleRequest) (configport.WorkloadBundle, error) {
	return configport.WorkloadBundle{}, ErrUnavailable
}

func (Unavailable) PrepareAccess(context.Context, configport.AccessRequest) (configport.WorkloadAccess, error) {
	return configport.WorkloadAccess{}, ErrUnavailable
}

func (Unavailable) CheckWorkloadAuth(context.Context, string, string) error { return ErrUnavailable }

var _ configport.Registry = Unavailable{}
