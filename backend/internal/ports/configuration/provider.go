// Package configuration defines the provider boundary for Application values.
package configuration

import "context"

// Provider stores values outside orchestrator state and returns opaque refs.
// ReadValue is an internal-only operation; delivery must never expose secrets.
type Provider interface {
	WriteValue(ctx context.Context, applicationKey, environmentKey, value string) (string, error)
	ReadValue(ctx context.Context, ref string) (string, error)
}

// WorkloadAccess is the immutable, least-privilege Vault identity for one
// workload/revision. It contains no value or token bytes.
type WorkloadAccess struct {
	Role           string
	Address        string
	ServiceAccount string
}

type WorkloadAccessPreparer interface {
	PrepareWorkloadAccess(ctx context.Context, app, env, workload, namespace, revisionID string, refs []string) (WorkloadAccess, error)
}
