// Package catalog implements UC-02/03 resource catalog registration.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("catalog: invalid document")
var ErrDuplicate = errors.New("catalog: duplicate id")

// internalError marks a platform-side failure found while validating a
// document (store, module inspector). It is returned unwrapped so it never
// becomes an ErrInvalid validation message.
type internalError struct{ err error }

func (e *internalError) Error() string { return e.err.Error() }
func (e *internalError) Unwrap() error { return e.err }

type Service struct {
	bundles   map[string]resource.RenderBundle
	store     persistence.Store
	inspector planning.ModuleInspector
}

func NewService(store persistence.Store, inspector ...planning.ModuleInspector) *Service {
	s := &Service{store: store}
	if len(inspector) > 0 {
		s.inspector = inspector[0]
	}
	return s
}

// RegisterResourceType validates and creates a contract in one Organization.
func (s *Service) RegisterResourceType(ctx context.Context, organizationKey string, typ resource.Type) (resource.Type, error) {
	if err := validatePublicID("resource type", typ.Key); err != nil {
		return resource.Type{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if reservedTypeKeys[typ.Key] {
		return resource.Type{}, fmt.Errorf("%w: resource type %q is a reserved virtual Score type", ErrInvalid, typ.Key)
	}
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

// RegisterResourceDefinition accepts only runtime-supported driver/type pairs.
func (s *Service) RegisterResourceDefinition(ctx context.Context, organizationKey string, def resource.Definition) (resource.Definition, error) {
	if def.Key == planning.BuiltinClusterKey {
		return resource.Definition{}, fmt.Errorf("%w: resource definition key %q is reserved for the system", ErrInvalid, def.Key)
	}
	if err := validatePublicID("resource definition", def.Key); err != nil {
		return resource.Definition{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := def.Validate(); err != nil {
		return resource.Definition{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	def.SourceFingerpr = "" // Server-owned metadata; never trust a submitted fingerprint.
	types, err := s.store.ListResourceTypes(ctx, organizationKey)
	if err != nil {
		return resource.Definition{}, err
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
		return resource.Definition{}, fmt.Errorf("%w: unknown resource type %q", ErrInvalid, def.ResourceTypeKey)
	}
	if err := s.validateDriver(ctx, organizationKey, &def, typ); err != nil {
		var internal *internalError
		if errors.As(err, &internal) {
			return resource.Definition{}, internal.err
		}
		return resource.Definition{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := validateReferences(def, types); err != nil {
		return resource.Definition{}, fmt.Errorf("%w: %v", ErrInvalid, err)
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
			if existing.Key == def.Key {
				return fmt.Errorf("%w: resource definition %q", ErrDuplicate, def.Key)
			}
		}
		return s.store.CreateResourceDefinition(ctx, organizationKey, def)
	})
	if errors.Is(err, persistence.ErrDuplicate) {
		return resource.Definition{}, fmt.Errorf("%w: resource definition %q", ErrDuplicate, def.Key)
	}
	if err != nil {
		return resource.Definition{}, err
	}
	return def, nil
}

func (s *Service) validateDriver(ctx context.Context, org string, def *resource.Definition, typ resource.Type) error {
	variables, err := validateDriverInputShape(*def)
	if err != nil {
		return err
	}
	module := ""
	switch def.DriverType {
	case resource.DriverScoreK8s:
		if err := resource.ValidateRenderDefinition(*def, typ, s.bundles); err != nil {
			return err
		}
		bundle := s.bundles[variables["render_bundle"].(string)]
		def.SourceFingerpr = bundle.Digest
		return nil
	case resource.DriverTerraform:
		module, _ = def.Source()["module"].(string)
		valid := map[string]string{"vpc": "vpc", "eks": "k8s-cluster", "aurora": "postgres"}
		if module == "" || valid[module] != def.ResourceTypeKey || len(def.Source()) != 1 {
			return fmt.Errorf("unsupported Terraform module/source for %q", def.ResourceTypeKey)
		}
		if def.ExecutionProfile != "aws-eks" {
			return fmt.Errorf("Terraform definition requires aws-eks profile")
		}
	case resource.DriverKubernetes:
		if def.ResourceTypeKey != "postgres" && def.ResourceTypeKey != "k8s-namespace" {
			return fmt.Errorf("Kubernetes executor does not support %q", def.ResourceTypeKey)
		}
		if def.ExecutionProfile == "aws-eks" {
			return fmt.Errorf("Kubernetes definition cannot target aws-eks profile")
		}
		if len(def.Source()) > 0 {
			return fmt.Errorf("Kubernetes definition cannot declare Terraform source")
		}
	case resource.DriverExistingCluster:
		if def.ResourceTypeKey != "k8s-cluster" || def.ExecutionProfile != "internal-k8s" {
			return fmt.Errorf("existing-cluster requires k8s-cluster and internal-k8s profile")
		}
		if len(def.Source()) > 0 {
			return fmt.Errorf("existing-cluster cannot declare Terraform source")
		}
	default:
		return fmt.Errorf("driver %q is not supported for registration", def.DriverType)
	}
	if def.DriverType == resource.DriverTerraform || def.DriverType == resource.DriverExistingCluster {
		if def.ConnectionKey == "" {
			return fmt.Errorf("driver requires an explicit connection")
		}
	}
	if def.ConnectionKey != "" {
		conn, err := s.store.GetConnection(ctx, org, def.ConnectionKey)
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return &internalError{err: err}
		}
		if err != nil || conn.OrganizationKey != org || conn.Status != application.ConnectionReady {
			return fmt.Errorf("connection %q is not READY in this organization", def.ConnectionKey)
		}
		if def.DriverType == resource.DriverTerraform && conn.Kind != application.ConnectionAWS {
			return fmt.Errorf("Terraform requires AWS connection")
		}
		if def.DriverType != resource.DriverTerraform && conn.Kind != application.ConnectionKubernetes {
			return fmt.Errorf("Kubernetes driver requires cluster connection")
		}
	}
	if def.DriverType == resource.DriverTerraform {
		if s.inspector == nil {
			return &internalError{err: errors.New("catalog: Terraform contract inspector is unavailable")}
		}
		contract, err := s.inspector.Inspect(module)
		if err != nil {
			// Module inspection is a platform failure, not a problem with the
			// caller's document: never report it as validation (UC-03 mapping).
			return &internalError{err: err}
		}
		if err := validateTerraformVariables(contract, variables); err != nil {
			return err
		}
		provided := map[string]bool{}
		for name := range variables {
			provided[name] = true
		}
		for _, input := range typ.Inputs {
			if _, ok := contract.Variables[input.Name]; !ok {
				return fmt.Errorf("module %q does not declare resource input %q", module, input.Name)
			}
			provided[input.Name] = true
		}
		for _, name := range s.inspector.ExecutorVariables() {
			provided[name] = true
		}
		for name, variable := range contract.Variables {
			if !variable.HasDefault && !provided[name] {
				return fmt.Errorf("module %q requires variable %q", module, name)
			}
		}
		outputs := map[string]bool{}
		for _, name := range append(contract.Outputs, s.inspector.ExecutorOutputs(module)...) {
			outputs[name] = true
		}
		for _, output := range typ.Outputs {
			if !outputs[output.Name] {
				return fmt.Errorf("module %q does not provide output %q", module, output.Name)
			}
		}
		def.SourceFingerpr = contract.Fingerprint
	} else {
		if err := validateStaticVariables(*def, typ, variables); err != nil {
			return err
		}
		allowed := map[string]map[string]bool{
			"k8s-namespace": {"name": true},
			"postgres":      {"host": true, "port": true, "database": true, "username": true, "password": true},
			"k8s-cluster":   {"name": true, "endpoint": true, "kubeContext": true, "kubeconfig": true},
		}
		for _, output := range typ.Outputs {
			if !allowed[def.ResourceTypeKey][output.Name] {
				return fmt.Errorf("driver does not provide output %q", output.Name)
			}
		}
	}
	return nil
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
