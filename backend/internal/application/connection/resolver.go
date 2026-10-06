package connection

import (
	"context"
	"errors"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/execution"
)

// ErrCredentialUnavailable is returned when a credential-backed target cannot
// be resolved. Execution fails closed; no host credential is ever used instead.
var ErrCredentialUnavailable = errors.New("connection: the target connection credential is unavailable; the connection must be READY in this organization")

type connectionReader interface {
	GetConnection(ctx context.Context, organizationKey, key string) (application.Connection, error)
}

// CredentialResolver resolves the credential of a KUBECONFIG Connection for
// one adapter operation, after checking Organization, kind, authentication
// type, status, selected context and reference scope.
type CredentialResolver struct {
	connections connectionReader
	store       credentials.Store
}

// NewCredentialResolver returns the execution-time resolver. A nil store
// makes every credential-backed target fail closed.
func NewCredentialResolver(connections connectionReader, store credentials.Store) *CredentialResolver {
	return &CredentialResolver{connections: connections, store: store}
}

// ResolveKubeconfig implements execution.KubeconfigSource.
func (r *CredentialResolver) ResolveKubeconfig(ctx context.Context, target execution.Target) ([]byte, error) {
	conn, err := r.Connection(ctx, target.Organization, target.Connection)
	if err != nil {
		return nil, err
	}
	// The Connection's selected context is authoritative: a target cannot
	// redirect the credential to another context.
	if target.Context != "" && target.Context != conn.ConfigString("kubeContext") {
		return nil, ErrCredentialUnavailable
	}
	if r.store == nil {
		return nil, ErrCredentialUnavailable
	}
	value, err := r.store.Get(ctx, conn.OrganizationKey, conn.Key, conn.SecretRef)
	if err != nil || len(value) == 0 {
		return nil, ErrCredentialUnavailable
	}
	return value, nil
}

// Connection returns a READY KUBECONFIG Connection of the Organization.
func (r *CredentialResolver) Connection(ctx context.Context, org, key string) (application.Connection, error) {
	if org == "" || key == "" || r.connections == nil {
		return application.Connection{}, ErrCredentialUnavailable
	}
	conn, err := r.connections.GetConnection(ctx, org, key)
	if err != nil {
		return application.Connection{}, ErrCredentialUnavailable
	}
	if conn.OrganizationKey != org || conn.Key != key || conn.Kind != application.ConnectionKubernetes ||
		conn.AuthenticationType != application.AuthKubeconfig || conn.Status != application.ConnectionReady ||
		conn.SecretRef == "" || conn.ConfigString("kubeContext") == "" {
		return application.Connection{}, ErrCredentialUnavailable
	}
	return conn, nil
}

var _ execution.KubeconfigSource = (*CredentialResolver)(nil)
