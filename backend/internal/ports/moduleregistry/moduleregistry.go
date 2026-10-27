// Package moduleregistry defines the Terraform v2 module registry ports. The
// three roles are separate on purpose: only registration and platform
// preparation may fetch and inspect sources; planning, Preview and
// capability metadata read verified records only; execution re-verifies the
// cached artifact before any provider call (Terraform module contract).
package moduleregistry

import (
	"context"
	"errors"

	"orchestrator/internal/domain/tfmodule"
)

// Error kinds. Messages carry no URL, cache path, credential or diagnostic.
var (
	// ErrNotAllowed: the source/module/revision is outside the Admin policy
	// or the release, or the revision is mutable.
	ErrNotAllowed = errors.New("moduleregistry: the module source is not allowed")
	// ErrDrift: the content digest differs from the submitted pin or from
	// the digest already verified for the same revision path.
	ErrDrift = errors.New("moduleregistry: the module content does not match its pinned digest")
	// ErrNotVerified: no verified bundle exists for the pin (metadata reads
	// never fetch to create one).
	ErrNotVerified = errors.New("moduleregistry: the module pin is not a verified bundle")
	// ErrReleaseMismatch: the bundle was verified for another Terraform or
	// provider release; execution never upgrades implicitly.
	ErrReleaseMismatch = errors.New("moduleregistry: the bundle release pins differ from the platform release")
	// ErrRejected: static inspection refused the module content or contract
	// (provisioners, hooks, unsafe files, traversal, providers outside the
	// release, unsupported types). The message names constructs, never paths.
	ErrRejected = errors.New("moduleregistry: the module is not allowed")
	// ErrBinaryUnverified: no Terraform binary is configured for execution
	// verification, so no runnable artifact can be returned.
	ErrBinaryUnverified = errors.New("moduleregistry: the Terraform binary is not configured for verification; execution is refused")
	// ErrUnavailable: the pinned revision could not be fetched.
	ErrUnavailable = errors.New("moduleregistry: the module source revision could not be fetched")
)

// Registrar verifies a pin for registration: it may fetch from an
// legacy policy source, or reuse a directly inspected immutable pin. The
// returned bundle is persisted by the caller together with the Definition.
type Registrar interface {
	Verify(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error)
}

// Metadata reads verified bundles only. It never fetches, inspects, runs a
// provider or writes.
type Metadata interface {
	Bundle(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error)
	List(ctx context.Context) ([]tfmodule.Bundle, error)
}

// ExecutionVerifier re-verifies, immediately before execution and without
// fetching, the bundle's cached content against its recorded root and
// transitive digests, its Terraform/provider pins against the current
// release, and the configured Terraform binary against the release digest.
// Missing or drifted content, a release mismatch or an unconfigured binary
// fail closed; only a fully verified artifact is returned.
type ExecutionVerifier interface {
	VerifyForExecution(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.VerifiedArtifact, error)
	// VerifyHistoricalForExecution is the cleanup variant: it verifies an
	// already persisted bundle against the retained trusted release it was
	// verified under, never against the current release, and never fetches.
	VerifyHistoricalForExecution(ctx context.Context, bundle tfmodule.Bundle) (tfmodule.VerifiedArtifact, error)
}

// Registry is the complete registry adapter.
type Registry interface {
	Registrar
	Metadata
	ExecutionVerifier
}

// GitInspection is a source draft, never an execution pin or a credential.
type GitInspection struct {
	ResourceType string `json:"resourceType"`
	URL          string `json:"url"`
	Path         string `json:"path"`
	Ref          string `json:"ref"`
}

// GitInspector prepares verified metadata; it never creates a Definition or
// invokes Terraform. Kept separate from Metadata's pure read interface.
type GitInspector interface {
	InspectGit(ctx context.Context, request GitInspection) (tfmodule.Bundle, error)
}

// ZIPInspection is a bounded uploaded source, never an execution workspace.
type ZIPInspection struct {
	OrganizationKey string
	ResourceType    string
	Path            string
	Data            []byte
}
type ZIPInspector interface {
	InspectZIP(context.Context, ZIPInspection) (tfmodule.Bundle, error)
}
