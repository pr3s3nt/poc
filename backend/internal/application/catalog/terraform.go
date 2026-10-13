package catalog

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
)

// Field paths of Terraform registration errors. They are fixed server paths;
// no submitted value is echoed.
const (
	sourcePath   = "definition.driverInputs.values.source"
	mappingPath  = "definition.driverInputs.values"
	bindingsPath = "definition.driverInputs.values.provider_bindings"
)

// validateTerraform checks a Terraform Definition against the verified module
// registry (UC-03 MS-05/MS-07, BR-08/09/11/12/14). The registrar may fetch and
// inspect an allowlisted revision here, outside any database transaction. The
// returned bundle is persisted with the Definition in one transaction.
func (s *Service) validateTerraform(ctx context.Context, org string, def *resource.Definition, typ resource.Type, types []resource.Type) (tfmodule.Bundle, error) {
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return tfmodule.Bundle{}, invalidField(mappingPath, fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if s.modules == nil {
		return tfmodule.Bundle{}, &internalError{err: errors.New("catalog: the Terraform module registry is unavailable")}
	}
	bundle, err := s.modules.Verify(ctx, inputs.Source)
	if err != nil {
		return tfmodule.Bundle{}, registryError(err)
	}
	if !bundle.ServesType(def.ResourceTypeKey) {
		return tfmodule.Bundle{}, invalidField("definition.resourceType", fmt.Errorf("%w: module %q is not registered for this Resource Type", ErrInvalid, inputs.Source.ModuleID))
	}
	usesAWS := false
	for _, provider := range bundle.Providers {
		usesAWS = usesAWS || provider.Binding == tfmodule.BindingAWS
	}
	if usesAWS {
		if def.ExecutionProfile != "aws-eks" {
			return tfmodule.Bundle{}, invalidField("definition.executionProfile", fmt.Errorf("%w: an AWS Terraform module requires the aws-eks profile", ErrInvalid))
		}
		// UC-03 BR-08: AWS modules always run on the Environment AWS
		// Connection; no Definition-level account/region override.
		if def.ConnectionKey != "" {
			return tfmodule.Bundle{}, invalidField("definition.connectionKey", fmt.Errorf("%w: AWS Terraform definitions use the Environment AWS connection; leave connectionKey empty", ErrInvalid))
		}
	} else if def.ConnectionKey != "" {
		if err := s.requireConnection(ctx, org, def.ConnectionKey, application.ConnectionKubernetes, "the module's Kubernetes provider binding requires a cluster connection"); err != nil {
			return tfmodule.Bundle{}, err
		}
	}
	if err := checkBindings(bundle, inputs.ProviderBindings, types); err != nil {
		return tfmodule.Bundle{}, invalidField(bindingsPath, fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if err := checkInputs(bundle, typ, inputs); err != nil {
		return tfmodule.Bundle{}, invalidField(mappingPath, fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if err := checkOutputs(bundle, typ, inputs.OutputMapping); err != nil {
		return tfmodule.Bundle{}, invalidField(mappingPath, fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	def.SourceFingerpr = "sha256:" + bundle.Ref.ContentSHA256
	return bundle, nil
}

// registryError maps registry failures onto safe registration errors.
func registryError(err error) error {
	switch {
	case errors.Is(err, moduleregistry.ErrNotAllowed):
		return invalidField(sourcePath, fmt.Errorf("%w: the module source is not allowlisted or the revision is not an exact pinned revision", ErrInvalid))
	case errors.Is(err, moduleregistry.ErrDrift):
		return invalidField(sourcePath, fmt.Errorf("%w: the module content does not match the pinned digest or the digest already verified for this revision", ErrInvalid))
	case errors.Is(err, moduleregistry.ErrReleaseMismatch):
		return invalidField(sourcePath, fmt.Errorf("%w: the module was verified for another platform release; register a new pin", ErrInvalid))
	case errors.Is(err, moduleregistry.ErrRejected):
		return invalidField(sourcePath, fmt.Errorf("%w: %v", ErrInvalid, err))
	case errors.Is(err, moduleregistry.ErrUnavailable):
		return invalidField(sourcePath, fmt.Errorf("%w: the pinned revision could not be fetched from the allowlisted source", ErrInvalid))
	}
	return &internalError{err: errors.New("catalog: the Terraform module registry failed")}
}

func (s *Service) requireConnection(ctx context.Context, org, key string, kind application.ConnectionKind, kindMessage string) error {
	conn, err := s.store.GetConnection(ctx, org, key)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return &internalError{err: err}
	}
	if err != nil || conn.OrganizationKey != org || conn.Status != application.ConnectionReady {
		return invalidField("definition.connectionKey", fmt.Errorf("connection %q is not READY in this organization", key))
	}
	if conn.Kind != kind {
		return invalidField("definition.connectionKey", errors.New(kindMessage))
	}
	return nil
}

// checkBindings requires exactly one binding per configurable provider of
// the bundle: AWS binds to the Environment; Kubernetes to the Environment or
// to a cluster resource descriptor. Unconfigured providers take none.
func checkBindings(bundle tfmodule.Bundle, bindings map[string]tfmodule.Binding, types []resource.Type) error {
	for _, name := range sortedKeys(bindings) {
		provider, ok := bundle.Providers[name]
		if !ok || provider.Binding == tfmodule.BindingNone {
			return fmt.Errorf("%s is not a provider requirement of the module that takes a binding", name)
		}
	}
	for _, name := range sortedKeys(bundle.Providers) {
		provider := bundle.Providers[name]
		if provider.Binding == tfmodule.BindingNone {
			continue
		}
		binding, ok := bindings[name]
		if !ok {
			return fmt.Errorf("%s needs a provider binding", name)
		}
		switch provider.Binding {
		case tfmodule.BindingAWS:
			if binding.Target != tfmodule.TargetEnvironment {
				return fmt.Errorf("%s binds only to the Environment AWS connection", name)
			}
		case tfmodule.BindingKubernetes:
			if binding.Target == tfmodule.TargetResource {
				if err := checkClusterRef(binding.ResourceRef, types); err != nil {
					return fmt.Errorf("%s.resource_ref %v", name, err)
				}
			}
		}
	}
	return nil
}

func checkClusterRef(ref string, types []resource.Type) error {
	ctx := planning.Context{App: application.Application{Key: "sample-app"}, Env: environment.Environment{Key: "staging"}, Connection: application.Connection{Key: "sample-connection"}}
	current := &planning.Node{Descriptor: "k8s-namespace.default#sample", Class: "default"}
	descriptor, err := planning.ParseDescriptorText(ref, current, ctx)
	if err != nil {
		return errors.New("is not a valid resource descriptor")
	}
	if descriptor.Type != planning.TypeCluster {
		return errors.New("must reference a Kubernetes cluster resource")
	}
	for _, typ := range types {
		if typ.Key == descriptor.Type {
			return nil
		}
	}
	return errors.New("references an unregistered resource type")
}

// checkInputs validates variables and the input mapping against the
// inspected module inputs and the Resource Type inputs.
func checkInputs(bundle tfmodule.Bundle, typ resource.Type, inputs tfmodule.DriverInputs) error {
	moduleInputs := bundle.Contract.Inputs
	owned := map[string]bool{}
	for _, name := range bundle.Contract.ExecutorOwned {
		owned[name] = true
	}
	authorable := func(path, name string) (tfmodule.Input, error) {
		input, ok := moduleInputs[name]
		switch {
		case credentialVariables[name] || owned[name]:
			return input, fmt.Errorf("%s.%s is executor-owned or a credential and cannot be a Driver Input", path, name)
		case !ok:
			return input, fmt.Errorf("%s.%s is not declared by module %q", path, name, bundle.Ref.ModuleID)
		case input.Sensitive:
			return input, fmt.Errorf("%s.%s is a sensitive module variable and cannot be a Driver Input", path, name)
		}
		return input, nil
	}
	for _, name := range sortedKeys(inputs.Variables) {
		input, err := authorable(variablesPath, name)
		if err != nil {
			return err
		}
		want, err := tfmodule.ParseType(input.Type)
		if err != nil {
			return fmt.Errorf("%s.%s has an unsupported module type", variablesPath, name)
		}
		if err := checkVariableValue(variablesPath+"."+name, want, inputs.Variables[name]); err != nil {
			return err
		}
	}
	mappedParams := map[string]string{}
	for _, name := range sortedKeys(inputs.InputMapping) {
		input, err := authorable("input_mapping", name)
		if err != nil {
			return err
		}
		param := inputs.InputMapping[name]
		field, ok := typ.Input(param)
		if !ok {
			return fmt.Errorf("input_mapping.%s maps an input the Resource Type does not declare", name)
		}
		if prior, dup := mappedParams[param]; dup {
			return fmt.Errorf("input_mapping.%s maps the same Resource Type input as input_mapping.%s", name, prior)
		}
		mappedParams[param] = name
		if !tfmodule.InputCompatible(field.Type, input.Type) {
			return fmt.Errorf("input_mapping.%s maps a %s input to a %s module variable", name, contractType(field.Type), input.Type)
		}
	}
	for _, field := range typ.Inputs {
		if _, ok := mappedParams[field.Name]; !ok {
			return fmt.Errorf("input_mapping does not map Resource Type input %q to a module variable", field.Name)
		}
	}
	for _, name := range sortedKeys(moduleInputs) {
		input := moduleInputs[name]
		if !input.Required || owned[name] {
			continue
		}
		if _, ok := inputs.Variables[name]; ok {
			continue
		}
		if param, ok := inputs.InputMapping[name]; ok {
			if field, _ := typ.Input(param); field.Required {
				continue
			}
		}
		return fmt.Errorf("module %q requires variable %q: map a required Resource Type input or set a Definition value", bundle.Ref.ModuleID, name)
	}
	return nil
}

// checkOutputs validates the output mapping: module output names that exist,
// distinct Resource Type outputs, compatible types and every required Type
// output covered. Secret classification is monotonic (EffectiveSecret).
func checkOutputs(bundle tfmodule.Bundle, typ resource.Type, mapping map[string]string) error {
	available := map[string]bool{}
	for _, name := range bundle.OutputNames() {
		available[name] = true
	}
	targets := map[string]string{}
	for _, name := range sortedKeys(mapping) {
		target := mapping[name]
		if !available[name] {
			return fmt.Errorf("output_mapping.%s is not an output of module %q", name, bundle.Ref.ModuleID)
		}
		field, ok := typ.Output(target)
		if !ok {
			return fmt.Errorf("output_mapping.%s maps to an output the Resource Type does not declare", name)
		}
		if prior, dup := targets[target]; dup {
			return fmt.Errorf("output_mapping.%s maps to the same Resource Type output as output_mapping.%s", name, prior)
		}
		targets[target] = name
		if !tfmodule.OutputCompatible(field.Type, bundle.Contract.Outputs[name].Type) {
			return fmt.Errorf("output_mapping.%s maps a %s module output to a %s Resource Type output", name, bundle.Contract.Outputs[name].Type, contractType(field.Type))
		}
	}
	for _, field := range typ.Outputs {
		if _, ok := targets[field.Name]; !ok && field.Required {
			return fmt.Errorf("output_mapping does not provide required Resource Type output %q", field.Name)
		}
	}
	return nil
}

func contractType(t string) string {
	if t == "" {
		return "any"
	}
	return t
}
