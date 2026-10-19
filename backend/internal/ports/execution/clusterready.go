package execution

import (
	"context"
	"errors"
)

// Cluster readiness failure categories (ADR-017). A cluster that reports ACTIVE is not yet usable by
// consumers: the executor must authenticate, be authorised, reach the API through the network and find
// usable Ready node capacity. Each failure is one fixed category; no endpoint, token or raw text.
var (
	ErrClusterUnreachable = errors.New("the cluster API is not reachable from the executor network")
	ErrClusterAuth        = errors.New("the cluster rejected the executor authentication")
	ErrClusterForbidden   = errors.New("the executor identity may not create the objects consumers need")
	ErrClusterNoCapacity  = errors.New("the cluster has no usable Ready node capacity")
	ErrClusterNotReady    = errors.New("the cluster did not become ready before the deadline")
)

// ClusterReadiness waits, bounded by ctx, until a completed opaque cluster target is usable by consumers.
// Every attempt derives fresh credentials from the target's original scoped account; nothing is cached or
// persisted. The error is one of the fixed categories above.
type ClusterReadiness interface {
	WaitClusterReady(ctx context.Context, target Target) error
}
