package persistence

import (
	"context"
	"orchestrator/internal/domain/monitoring"
)

// MonitoringRepository writes participate in Store.Transact and Environment fencing.
type MonitoringRepository interface {
	ListMonitoringConnections(context.Context, string) ([]monitoring.Connection, error)
	GetMonitoringConnection(context.Context, string, string) (monitoring.Connection, error)
	PutMonitoringConnection(context.Context, monitoring.Connection, int64) error
	DeleteMonitoringConnection(context.Context, string, string, int64) error
	GetMonitoringBinding(context.Context, string, string, string) (monitoring.Binding, error)
	PutMonitoringBinding(context.Context, monitoring.Binding, int64) error
}
