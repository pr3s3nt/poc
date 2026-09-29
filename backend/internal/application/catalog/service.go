// Package catalog implements UC-02/03 resource catalog registration.
package catalog

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("catalog: invalid document")
var ErrDuplicate = errors.New("catalog: duplicate id")

type Service struct{ store persistence.Store }

func NewService(store persistence.Store) *Service { return &Service{store: store} }

// RegisterResourceType validates and creates a contract in one Organization.
func (s *Service) RegisterResourceType(ctx context.Context, organizationKey string, typ resource.Type) (resource.Type, error) {
	if err := typ.Validate(); err != nil {
		return resource.Type{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetOrganization(ctx, organizationKey); err != nil {
			return err
		}
		types, err := s.store.ListResourceTypes(ctx, organizationKey)
		if err != nil {
			return err
		}
		for _, existing := range types {
			if existing.Key == typ.Key {
				return fmt.Errorf("%w: resource type %q", ErrDuplicate, typ.Key)
			}
		}
		return s.store.SaveResourceType(ctx, organizationKey, typ)
	})
	if err != nil {
		return resource.Type{}, err
	}
	return typ, nil
}
