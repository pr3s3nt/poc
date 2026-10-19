// Package catalog implements UC-02/03 resource catalog registration.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("catalog: invalid document")
var ErrDuplicate = errors.New("catalog: duplicate id")

// ErrInvalidID refines ErrInvalid for a registration ID that breaks the public
// ID rules; such errors also match ErrInvalid.
var ErrInvalidID = errors.New("catalog: invalid id")

// FieldError is an ErrInvalid at a fixed, server-defined field path (never a
// caller-supplied alias, key or value), so delivery can point at the failing
// part of a document without echoing its content.
type FieldError struct {
	Field string
	err   error
}

func (e *FieldError) Error() string        { return e.err.Error() }
func (e *FieldError) Unwrap() error        { return e.err }
func (e *FieldError) Is(target error) bool { return target == ErrInvalid }

func invalidField(field string, err error) error { return &FieldError{Field: field, err: err} }

type invalidIDError struct{ err error }

func (e *invalidIDError) Error() string        { return e.err.Error() }
func (e *invalidIDError) Unwrap() error        { return e.err }
func (e *invalidIDError) Is(target error) bool { return target == ErrInvalidID }

// internalError marks a platform-side failure found while validating a
// document (store, module inspector). It is returned unwrapped so it never
// becomes an ErrInvalid validation message.
type internalError struct{ err error }

func (e *internalError) Error() string { return e.err.Error() }
func (e *internalError) Unwrap() error { return e.err }

type Service struct {
	bundles map[string]resource.RenderBundle
	store   persistence.Store
	// modules is the verified Terraform module registry: registration may
	// verify (fetch/inspect) a pin; capability reads use metadata only.
	modules moduleregistry.Registry
}

// NewService returns the catalog service. Without a module registry,
// Terraform registration and capabilities fail closed.
func NewService(store persistence.Store, modules ...moduleregistry.Registry) *Service {
	s := &Service{store: store}
	if len(modules) > 0 {
		s.modules = modules[0]
	}
	return s
}

// RegisterResourceType validates and creates a contract in one Organization.
func (s *Service) RegisterResourceType(ctx context.Context, organizationKey string, typ resource.Type) (resource.Type, error) {
	if err := validatePublicID("resource type", typ.Key); err != nil {
		return resource.Type{}, &invalidIDError{err: invalidField("type.key", fmt.Errorf("%w: %v", ErrInvalid, err))}
	}
	if reservedTypeKeys[typ.Key] {
		return resource.Type{}, invalidField("type.key", fmt.Errorf("%w: resource type %q is a reserved virtual Score type", ErrInvalid, typ.Key))
	}
	if err := typ.Validate(); err != nil {
		return resource.Type{}, invalidField("type.schema", fmt.Errorf("%w: %v", ErrInvalid, err))
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
		// Insert-only: the list check gives a clear message; the repository
		// insert is the final duplicate guard (OC-03).
		return s.store.CreateResourceType(ctx, organizationKey, typ)
	})
	if errors.Is(err, persistence.ErrDuplicate) {
		return resource.Type{}, fmt.Errorf("%w: resource type %q", ErrDuplicate, typ.Key)
	}
	if err != nil {
		return resource.Type{}, err
	}
	return typ, nil
}

// ErrVersionConflict reports an edit based on a stale Definition version
// (HTTP 409). The stored winner is untouched.
var ErrVersionConflict = errors.New("catalog: stale resource definition version")

// ErrOutcomeUncertain means the database commit of a Definition write may have
// happened. The write is never replayed automatically; the caller reloads the
// Definition before deciding.
var ErrOutcomeUncertain = errors.New("catalog: the outcome is not known; reload the definition before trying again")

// ErrNotFound reports a Definition outside the Organization (HTTP 404).
var ErrNotFound = errors.New("catalog: resource definition not found")

// RegisterResourceDefinition validates and creates a Definition at version 1.
// Retired authoring fields are rejected by the strict public DTO; the version
// and source fingerprint are always server-owned.
func (s *Service) RegisterResourceDefinition(ctx context.Context, organizationKey string, def resource.Definition) (resource.Definition, error) {
	def.Version = 1
	prepared, bundle, err := s.prepareDefinition(ctx, organizationKey, def)
	if err != nil {
		return resource.Definition{}, err
	}
	err = s.store.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetOrganization(ctx, organizationKey); err != nil {
			return err
		}
		defs, err := s.store.ListResourceDefinitions(ctx, organizationKey)
		if err != nil {
			return err
		}
		for _, existing := range defs {
			if existing.Key == prepared.Key {
				return fmt.Errorf("%w: resource definition %q", ErrDuplicate, prepared.Key)
			}
		}
		if err := s.storeBundle(ctx, bundle); err != nil {
			return err
		}
		return s.store.CreateResourceDefinition(ctx, organizationKey, prepared)
	})
	if errors.Is(err, persistence.ErrDuplicate) {
		return resource.Definition{}, fmt.Errorf("%w: resource definition %q", ErrDuplicate, prepared.Key)
	}
	if errors.Is(err, persistence.ErrCommitUncertain) {
		return resource.Definition{}, ErrOutcomeUncertain
	}
	if err != nil {
		return resource.Definition{}, err
	}
	return prepared, nil
}

// UpdateResourceDefinition edits the whole configuration of an existing
// Definition except its ID and Resource Type (UC-03 V02-06). It runs the same
// validation as creation, then one transaction compares the expected version
// and replaces the Definition, its criteria and the verified bundle record.
// A lost race is ErrVersionConflict; an unknown commit outcome is returned as
// is and never replayed here.
func (s *Service) UpdateResourceDefinition(ctx context.Context, organizationKey, key string, expected int64, def resource.Definition) (resource.Definition, error) {
	if expected < 1 {
		return resource.Definition{}, invalidField("definition.version", fmt.Errorf("%w: expected version must be positive", ErrInvalid))
	}
	if def.Key != key {
		return resource.Definition{}, invalidField("definition.key", fmt.Errorf("%w: the definition ID is immutable", ErrInvalid))
	}
	stored, err := s.loadDefinition(ctx, organizationKey, key)
	if err != nil {
		return resource.Definition{}, err
	}
	if stored.ResourceTypeKey != def.ResourceTypeKey {
		return resource.Definition{}, invalidField("definition.resourceType", fmt.Errorf("%w: the resource type is immutable", ErrInvalid))
	}
	if stored.Version != expected {
		return resource.Definition{}, fmt.Errorf("%w: resource definition %q", ErrVersionConflict, key)
	}
	def.Version = expected
	prepared, bundle, err := s.prepareDefinition(ctx, organizationKey, def)
	if err != nil {
		return resource.Definition{}, err
	}
	var saved resource.Definition
	err = s.store.Transact(ctx, func(ctx context.Context) error {
		if err := s.storeBundle(ctx, bundle); err != nil {
			return err
		}
		var err error
		saved, err = s.store.UpdateResourceDefinition(ctx, organizationKey, expected, prepared)
		return err
	})
	switch {
	case errors.Is(err, persistence.ErrCommitUncertain):
		return resource.Definition{}, ErrOutcomeUncertain
	case errors.Is(err, persistence.ErrVersionConflict):
		return resource.Definition{}, fmt.Errorf("%w: resource definition %q", ErrVersionConflict, key)
	case errors.Is(err, persistence.ErrNotFound):
		return resource.Definition{}, fmt.Errorf("%w: resource definition %q", ErrNotFound, key)
	case err != nil:
		return resource.Definition{}, err
	}
	return saved, nil
}

func (s *Service) loadDefinition(ctx context.Context, organizationKey, key string) (resource.Definition, error) {
	defs, err := s.store.ListResourceDefinitions(ctx, organizationKey)
	if err != nil {
		return resource.Definition{}, err
	}
	for _, d := range defs {
		if d.Key == key && d.Key != planning.BuiltinClusterKey {
			return d, nil
		}
	}
	return resource.Definition{}, fmt.Errorf("%w: resource definition %q", ErrNotFound, key)
}

// storeBundle records the verified module bundle with the Definition. A bundle
// committed concurrently for the same revision path must be the identical
// verified content, contract and release (no drift).
func (s *Service) storeBundle(ctx context.Context, bundle *tfmodule.Bundle) error {
	if bundle == nil {
		return nil
	}
	stored, err := s.store.InsertModuleBundle(ctx, *bundle)
	if err != nil {
		return err
	}
	if !stored.SameVerifiedContent(*bundle) {
		return registryError(moduleregistry.ErrDrift)
	}
	return nil
}

// prepareDefinition is the shared create/edit validation pipeline: public ID
// and reserved-key rules, structural validation, Resource Type, driver
// (module registry, bindings, Driver Account and region) and references. It
// performs module fetch/inspection and account reads outside any write
// transaction.
func (s *Service) prepareDefinition(ctx context.Context, organizationKey string, def resource.Definition) (resource.Definition, *tfmodule.Bundle, error) {
	if def.Key == planning.BuiltinClusterKey {
		return resource.Definition{}, nil, invalidField("definition.key", fmt.Errorf("%w: resource definition key %q is reserved for the system", ErrInvalid, def.Key))
	}
	if err := validatePublicID("resource definition", def.Key); err != nil {
		return resource.Definition{}, nil, &invalidIDError{err: invalidField("definition.key", fmt.Errorf("%w: %v", ErrInvalid, err))}
	}
	if err := resource.ValidateName(def.Name); err != nil {
		return resource.Definition{}, nil, invalidField("definition.name", fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if err := def.Validate(); err != nil {
		return resource.Definition{}, nil, invalidField("definition.schema", fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	def.SourceFingerpr = "" // Server-owned metadata; never trust a submitted fingerprint.
	types, err := s.store.ListResourceTypes(ctx, organizationKey)
	if err != nil {
		return resource.Definition{}, nil, err
	}
	var typ resource.Type
	found := false
	for _, candidate := range types {
		if candidate.Key == def.ResourceTypeKey {
			typ, found = candidate, true
			break
		}
	}
	if !found {
		return resource.Definition{}, nil, invalidField("definition.resourceType", fmt.Errorf("%w: unknown resource type %q", ErrInvalid, def.ResourceTypeKey))
	}
	bundle, err := s.validateDriver(ctx, organizationKey, &def, typ, types)
	if err != nil {
		var internal *internalError
		if errors.As(err, &internal) {
			return resource.Definition{}, nil, internal.err
		}
		var fieldErr *FieldError
		if errors.As(err, &fieldErr) {
			return resource.Definition{}, nil, err
		}
		return resource.Definition{}, nil, invalidField("definition.driver", fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if err := validateReferences(def, types); err != nil {
		return resource.Definition{}, nil, invalidField("definition.references", fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	return def, bundle, nil
}

func (s *Service) validateDriver(ctx context.Context, org string, def *resource.Definition, typ resource.Type, types []resource.Type) (*tfmodule.Bundle, error) {
	variables, err := validateDriverInputShape(*def)
	if err != nil {
		return nil, err
	}
	switch def.DriverType {
	case resource.DriverExistingCluster:
		// ADR-016: a public existing-cluster Definition is a k8s-cluster whose
		// own READY Internal Driver Account supplies access. Authored inputs
		// carry no variables: identity and credentials come from the account.
		if def.ResourceTypeKey != resource.ClusterTargetType {
			return nil, invalidField("definition.resourceType", fmt.Errorf("%w: existing-cluster serves only the %s resource type", ErrInvalid, resource.ClusterTargetType))
		}
		if field, ok := typ.Output(resource.ClusterTargetOutput); !ok || field.Secret {
			return nil, invalidField("definition.resourceType", fmt.Errorf("%w: the %s resource type must declare a non-secret %q output", ErrInvalid, resource.ClusterTargetType, resource.ClusterTargetOutput))
		}
		if len(variables) != 0 {
			return nil, invalidField(variablesPath, fmt.Errorf("%w: existing-cluster takes no variables; its identity comes from the driver account", ErrInvalid))
		}
		if def.DriverAccount == "" {
			return nil, invalidField(accountPath, fmt.Errorf("%w: existing-cluster needs an Internal driver account", ErrInvalid))
		}
		if err := s.requireAccount(ctx, org, def.DriverAccount, application.ConnectionKubernetes, "existing-cluster requires an Internal driver account"); err != nil {
			return nil, err
		}
		return nil, nil
	case resource.DriverScoreK8s:
		if def.DriverAccount != "" {
			return nil, invalidField("definition.driverAccount", fmt.Errorf("%w: score-k8s rendering takes no driver account", ErrInvalid))
		}
		if err := resource.ValidateRenderDefinition(*def, typ, s.bundles, true); err != nil {
			return nil, err
		}
		bundle := s.bundles[variables["render_bundle"].(string)]
		def.SourceFingerpr = bundle.Digest
		return nil, nil
	case resource.DriverTerraform:
		bundle, err := s.validateTerraform(ctx, org, def, typ, types)
		if err != nil {
			return nil, err
		}
		return &bundle, nil
	default:
		// UC-03 BR-08: only registry-verified Terraform modules and score-k8s
		// rendering are registrable; the static Kubernetes driver and every
		// other driver are not accepted by the public API.
		return nil, invalidField("definition.driverType", fmt.Errorf("%w: driver %q is not registrable; use a verified Terraform module (Kubernetes resources included) or score-k8s for Workloads", ErrInvalid, def.DriverType))
	}
}

func validateReferences(def resource.Definition, types []resource.Type) error {
	byKey := map[string]resource.Type{}
	for _, typ := range types {
		byKey[typ.Key] = typ
	}
	context := planning.Context{App: application.Application{Key: "sample-app"}, Env: environment.Environment{Key: "staging"}, Connection: application.Connection{Key: "sample-connection"}}
	current := &planning.Node{Descriptor: def.ResourceTypeKey + ".default#sample", Class: "default"}
	// Errors name the field path only; the submitted value is never echoed
	// (UC-03 BR-13).
	check := func(path, value string) error {
		if _, err := classifyPlaceholders(value); err != nil {
			return fmt.Errorf("%s contains a malformed placeholder", path)
		}
		refs, err := placeholder.Refs(value)
		if err != nil {
			return fmt.Errorf("%s contains a malformed placeholder", path)
		}
		for _, ref := range refs {
			if ref.Kind != placeholder.KindResource {
				continue
			}
			descriptor, err := planning.ParseDescriptorText(ref.Resource, current, context)
			if err != nil {
				return fmt.Errorf("%s contains an invalid resource reference", path)
			}
			provider, ok := byKey[descriptor.Type]
			if !ok {
				return fmt.Errorf("%s references an unregistered provider resource type", path)
			}
			if _, ok := provider.Output(ref.OutputKey); !ok {
				return fmt.Errorf("%s references an output its provider resource type does not declare", path)
			}
		}
		return nil
	}
	if err := placeholder.WalkStrings(def.DriverInputs, "/driverInputs", check); err != nil {
		return err
	}
	for key, rule := range def.Provision {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("empty provision descriptor")
		}
		descriptor, err := planning.ParseDescriptorText(key, current, context)
		if err != nil {
			return fmt.Errorf("invalid provision descriptor %q: %w", key, err)
		}
		providerType := descriptor.Type
		if providerType == "workload" {
			return fmt.Errorf("workload rendering cannot be co-provisioned")
		}
		if _, ok := byKey[providerType]; !ok {
			return fmt.Errorf("provision descriptor %q has unknown type", key)
		}
		if err := placeholder.WalkStrings(rule.Params, "/provision/"+key, check); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SetRenderBundles(bundles map[string]resource.RenderBundle) {
	s.bundles = resource.CopyRenderBundles(bundles)
}

// InstalledRenderBundle is the safe, read-only view of an available bundle.
type InstalledRenderBundle struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// ListRenderBundles returns metadata of available installed bundles sorted by ID.
// It reads only the immutable in-process registry.
func (s *Service) ListRenderBundles() []InstalledRenderBundle {
	out := []InstalledRenderBundle{}
	for id, b := range s.bundles {
		if resource.RenderBundleAvailable(s.bundles, id) {
			out = append(out, InstalledRenderBundle{ID: b.ID, Version: b.Version, Status: "AVAILABLE"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
