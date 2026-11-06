// Package observability defines the read-only cluster collection boundary.
package observability

import (
	"context"
	"orchestrator/internal/domain/monitoring"
	"orchestrator/internal/domain/telemetry"
	"orchestrator/internal/ports/execution"
)

type Source struct {
	telemetry.Source
	Target               execution.Target
	Controller           *execution.ObjectRef
	MonitoringController *execution.ObjectRef
	DeploymentID         string
}

type Reader interface {
	Collect(context.Context, Source, telemetry.State) (telemetry.Batch, error)
}

// TargetVerifier independently proves the physical cluster and controller UID.
type TargetVerifier interface {
	MonitoringTarget(context.Context, Source) (monitoring.Target, error)
}

// ClusterVerifier proves the credential-scoped destination before first deployment.
type ClusterVerifier interface {
	MonitoringCluster(context.Context, execution.Target) (string, error)
}

// ConfigurationVerifier compares only platform-owned monitoring artifacts.
// Empty expected requires their absence before detaching a live workload.
type ConfigurationVerifier interface {
	MonitoringConfiguration(context.Context, Source, []execution.Manifest) (string, error)
}
