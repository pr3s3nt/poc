package persistence

import (
	"context"
	"errors"
	"orchestrator/internal/domain/monitoring"
)

// MonitoringRepository writes participate in Store.Transact and Environment fencing.
type MonitoringRepository interface {
 MarkMonitoringDeleting(context.Context,string,string,int64) error
 ClearRetiredMonitoringCredentials(context.Context,string,string,int64) error
	ListMonitoringConnections(context.Context, string) ([]monitoring.Connection, error)
	GetMonitoringConnection(context.Context, string, string) (monitoring.Connection, error)
	PutMonitoringConnection(context.Context, monitoring.Connection, int64) error
	VerifyMonitoringConnection(context.Context, monitoring.Connection, int64) error
	DeleteMonitoringConnection(context.Context, string, string, int64) error
	GetMonitoringBinding(context.Context, string, string, string) (monitoring.Binding, error)
	PutMonitoringBinding(context.Context, monitoring.Binding, int64) error
	DeleteMonitoringBinding(context.Context, string, string, string, int64) error
}

// ReadMonitoringPin performs metadata reads only; callers use their existing
// consistent planning snapshot. No provider or credential read occurs here.
func ReadMonitoringPin(ctx context.Context, st Store, org, app, env string, generation int64) (monitoring.Pin, error) {
	repo, ok := st.(MonitoringRepository)
	if !ok {
		return monitoring.Pin{}, nil
	}
	b, e := repo.GetMonitoringBinding(ctx, org, app, env)
	if errors.Is(e, ErrNotFound) {
		return monitoring.Pin{}, nil
	}
	if e != nil {
		return monitoring.Pin{}, e
	}
	c, e := repo.GetMonitoringConnection(ctx, org, b.ConnectionID)
	if e != nil {
		return monitoring.Pin{}, e
	}
	return monitoring.NewPin(b, c, generation), nil
}
