package catalog

import (
	"context"
	"errors"
	"fmt"
	"orchestrator/internal/ports/gitaccess"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

func (s *Service) InspectTerraformModule(ctx context.Context, org string, request moduleregistry.GitInspection) (Capability, error) {
	return s.inspectTerraformSource(ctx, org, request.ResourceType, func() (tfmodule.Bundle, error) {
		inspector, ok := s.modules.(moduleregistry.GitInspector)
		if !ok {
			return tfmodule.Bundle{}, errors.New("module inspection is unavailable")
		}
		return inspector.InspectGit(gitaccess.WithOrganization(ctx, org), request)
	})
}

func (s *Service) UploadTerraformModule(ctx context.Context, org string, request moduleregistry.ZIPInspection) (Capability, error) {
	request.OrganizationKey = org
	return s.inspectTerraformSource(ctx, org, request.ResourceType, func() (tfmodule.Bundle, error) {
		inspector, ok := s.modules.(moduleregistry.ZIPInspector)
		if !ok {
			return tfmodule.Bundle{}, errors.New("module upload is unavailable")
		}
		return inspector.InspectZIP(ctx, request)
	})
}

func (s *Service) inspectTerraformSource(ctx context.Context, org, resourceType string, inspect func() (tfmodule.Bundle, error)) (Capability, error) {
	// Check scoped Type before any outbound source request.
	types, err := s.store.ListResourceTypes(ctx, org)
	if err != nil {
		return Capability{}, &internalError{err: err}
	}
	var typ resource.Type
	found := false
	for _, candidate := range types {
		if candidate.Key == resourceType {
			typ = candidate
			found = true
			break
		}
	}
	if !found {
		return Capability{}, invalidField("resourceType", fmt.Errorf("%w: unknown Resource Type", ErrInvalid))
	}
	bundle, err := inspect()
	if err != nil {
		return Capability{}, registryError(err)
	}
	capability, err := terraformCapability(bundle, typ, resourceType)
	if err != nil {
		return Capability{}, &internalError{err: err}
	}
	return capability, nil
}
