// Package oidc defines the identity-provider boundary without provider SDK types.
package oidc

import (
	"context"
	"time"

	"orchestrator/internal/domain/identity"
)

// Grant contains credentials only in backend memory, never in public DTOs/logs.
type Grant struct {
	Subject, Username, RefreshToken string
	Role                            identity.Role
	ExpiresAt                       time.Time
}

type Provider interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(context.Context, string, string, string) (Grant, error)
	Refresh(context.Context, string) (Grant, error)
	Revoke(context.Context, string) error
}
