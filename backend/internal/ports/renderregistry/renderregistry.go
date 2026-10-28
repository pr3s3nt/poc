// Package renderregistry is the port of the verified Git render bundle
// registry (ADR-021): inspection of a credential-free HTTPS Git source into
// an immutable Organization-scoped record and runtime availability of a pin.
package renderregistry

import (
	"context"
	"errors"

	"orchestrator/internal/domain/renderbundle"
)

// Errors with stable public codes (contract RB-C07). Messages never quote a
// URL, ref, file content or template value.
var (
	// ErrSourceInvalid: URL/path/ref syntax (RENDER_SOURCE_INVALID, 400).
	ErrSourceInvalid = errors.New("renderregistry: the Git source is not valid")
	// ErrBundleInvalid: the selected tree breaks version 1 (RENDER_BUNDLE_INVALID, 400).
	ErrBundleInvalid = errors.New("renderregistry: the render bundle is not valid")
	// ErrContractUnsupported: unsupported API/engine/renderer (RENDER_CONTRACT_UNSUPPORTED, 400).
	ErrContractUnsupported = errors.New("renderregistry: the render bundle contract is not supported")
	// ErrSourceUnavailable: Git could not be resolved or fetched (RENDER_SOURCE_UNAVAILABLE, 503).
	ErrSourceUnavailable = errors.New("renderregistry: the Git source is not reachable")
	// ErrBundleUnavailable: cache/runtime cannot serve the pin (RENDER_BUNDLE_UNAVAILABLE, 503).
	ErrBundleUnavailable = errors.New("renderregistry: the render bundle is not available")
	// ErrDrift: different content for an identical immutable pin (RENDER_BUNDLE_DRIFT, 409).
	ErrDrift = errors.New("renderregistry: the render bundle content differs from its pin")
	// ErrRenderFailed: a bundle stage failed or broke an invariant (RENDER_FAILED, 422).
	ErrRenderFailed = errors.New("renderregistry: rendering failed")
)

// Inspection is one inspect request: HTTPS URL, directory and branch/tag/commit.
type Inspection struct {
	URL  string `json:"url"`
	Path string `json:"path"`
	Ref  string `json:"ref"`
}

// Registry inspects Git sources and reports runtime availability.
type Registry interface {
	// Inspect resolves the ref once, fetches the exact commit, validates and
	// caches the whole selected directory and records the verified pin for
	// the Organization. It creates no Definition and touches no provider.
	Inspect(ctx context.Context, organizationKey string, req Inspection) (renderbundle.Bundle, error)
	// Available reports whether the runtime can render the record from the
	// verified cache with its engine and binary. It never fetches Git.
	Available(ctx context.Context, b renderbundle.Bundle) error
}

// ZIPInspection uses the same on-disk bundle format as Git.
type ZIPInspection struct {
	Path string
	Data []byte
}
type ZIPInspector interface {
	InspectZIP(context.Context, string, ZIPInspection) (renderbundle.Bundle, error)
}
