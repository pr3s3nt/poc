package catalog

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/moduleregistry"
)

func (s *Service) InspectTerraformModule(ctx context.Context, org string, request moduleregistry.GitInspection) (Capability, error) {
	// Check scoped Type before any outbound source request.
	types, err := s.store.ListResourceTypes(ctx, org)
	if err != nil {
		return Capability{}, &internalError{err: err}
	}
	var typ resource.Type
	found := false
	for _, candidate := range types {
		if candidate.Key == request.ResourceType {
			typ = candidate
			found = true
			break
		}
	}
	if !found {
		return Capability{}, invalidField("resourceType", fmt.Errorf("%w: unknown Resource Type", ErrInvalid))
	}
	inspector, ok := s.modules.(moduleregistry.GitInspector)
	if !ok {
		return Capability{}, &internalError{err: errors.New("module inspection is unavailable")}
	}
	bundle, err := inspector.InspectGit(ctx, request)
	if err != nil {
		return Capability{}, registryError(err)
	}
	capability, err := terraformCapability(bundle, typ, request.ResourceType)
	if err != nil {
		return Capability{}, &internalError{err: err}
	}
	return capability, nil
}
