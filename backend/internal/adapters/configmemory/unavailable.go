package configmemory

import (
	"context"
	"errors"
	configport "orchestrator/internal/ports/configuration"
)

var ErrUnavailable = errors.New("configuration provider is not configured")

// Unavailable keeps non-UC-12 API available when a real Vault token is absent.
type Unavailable struct{}

func (Unavailable) WriteValue(context.Context, string, string, string) (string, error) {
	return "", ErrUnavailable
}

func (Unavailable) ReadValue(context.Context, string) (string, error) {
	return "", ErrUnavailable
}

func (Unavailable) PrepareWorkloadAccess(context.Context, string, string, string, string, string, []string) (configport.WorkloadAccess, error) {
	return configport.WorkloadAccess{}, ErrUnavailable
}
