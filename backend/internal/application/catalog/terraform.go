package catalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/persistence"
)

// Field paths of Terraform registration errors. They are fixed server paths;
// no submitted value is echoed.
const (
	sourcePath   = "definition.driverInputs.values.source"
	mappingPath  = "definition.driverInputs.values"
	bindingsPath = "definition.driverInputs.values.provider_bindings"
	accountPath  = "definition.driverAccount"
	regionPath   = "definition.driverInputs.values.variables.region"
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
		// UC-03 V02-03: an AWS module names its scoped READY AWS Driver
		// Account and a literal region; nothing is read from the Environment.
		if def.DriverAccount == "" {
			return tfmodule.Bundle{}, invalidField(accountPath, fmt.Errorf("%w: an AWS Terraform definition needs a Driver Account", ErrInvalid))
		}
		if err := s.requireAccount(ctx, org, def.DriverAccount, application.ConnectionAWS, "the AWS module requires an AWS Driver Account"); err != nil {
			return tfmodule.Bundle{}, err
		}
		region, literal := inputs.Variables["region"].(string)
		if !literal || !resource.ValidAWSRegion(region) {
			return tfmodule.Bundle{}, invalidField(regionPath, fmt.Errorf("%w: region must be a literal AWS region such as ap-southeast-1", ErrInvalid))
		}
		if _, mapped := inputs.InputMapping["region"]; mapped {
			// A Resource Type param could otherwise override the literal that
			// the provider pin and the plan hash cover.
			return tfmodule.Bundle{}, invalidField(mappingPath, fmt.Errorf("%w: region is a Definition literal and cannot be mapped to a Resource Type input", ErrInvalid))
		}
	} else if def.DriverAccount != "" {
		// A Kubernetes resource binding inherits its cluster access; a second
		// account would be redundant and ambiguous.
		return tfmodule.Bundle{}, invalidField(accountPath, fmt.Errorf("%w: this module takes no Driver Account; its cluster access comes from the binding", ErrInvalid))
	}
	if err := checkBindings(bundle, inputs.ProviderBindings, types); err != nil {
		return tfmodule.Bundle{}, invalidField(bindingsPath, fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if err := checkInputs(bundle, typ, inputs); err != nil {
		return tfmodule.Bundle{}, invalidField(mappingPath, fmt.Errorf("%w: %v", ErrInvalid, err))
	}
	if err := checkSecretReferences(inputs, types); err != nil {
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

// requireAccount checks that key is a READY Driver Account of kind in this
// Organization. Messages never echo credentials or the private reference.
func (s *Service) requireAccount(ctx context.Context, org, key string, kind application.ConnectionKind, kindMessage string) error {
	conn, err := s.store.GetConnection(ctx, org, key)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return &internalError{err: err}
	}
	if err != nil || conn.OrganizationKey != org || conn.Status != application.ConnectionReady {
		return invalidField(accountPath, fmt.Errorf("driver account %q is not READY in this organization", key))
	}
	if conn.Kind != kind {
		return invalidField(accountPath, errors.New(kindMessage))
	}
	return nil
}

// checkBindings requires exactly one binding per configurable provider of
// the bundle: AWS binds to the Definition's Driver Account; Kubernetes to a
// cluster resource descriptor or, until V03, the legacy Environment target.
// Unconfigured providers take none.
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
			if binding.Target != tfmodule.TargetAccount {
				return fmt.Errorf("%s binds only to the Definition's Driver Account ({target: account})", name)
			}
		case tfmodule.BindingKubernetes:
			if binding.Target == tfmodule.TargetAccount {
				return fmt.Errorf("%s inherits its cluster access; {target: account} is only for AWS", name)
			}
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
		if name == publicAccessCIDRsVariable {
			if err := checkPublicAccessCIDRs(variablesPath+"."+name, inputs.Variables[name]); err != nil {
				return err
			}
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
		if typ.Key == resource.ClusterTargetType && target == resource.ClusterTargetOutput {
			// ADR-017: the typed target is published by the platform from the completed cluster identity.
			return fmt.Errorf("output_mapping.%s: the %q output of a cluster is produced by the platform and cannot be mapped from a module output", name, target)
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
		if typ.Key == resource.ClusterTargetType && field.Name == resource.ClusterTargetOutput {
			continue // platform-owned (ADR-017)
		}
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

// checkSecretReferences rejects a Definition value that references an output
// the Resource Type classifies secret, wherever the placeholder sits (nested
// in a list or map, or inside a concatenated string). Definition variables
// are nonsecret module inputs persisted in attempt snapshots; module-mapping
// classification of the producer is only known once a Definition matches and
// is checked by planning and again at runtime.
func checkSecretReferences(inputs tfmodule.DriverInputs, types []resource.Type) error {
	ctx := planning.Context{App: application.Application{Key: "sample-app"}, Env: environment.Environment{Key: "staging"}, Connection: application.Connection{Key: "sample-connection"}}
	current := &planning.Node{Descriptor: "k8s-namespace.default#sample", Class: "default"}
	for _, name := range sortedKeys(inputs.Variables) {
		err := placeholder.WalkStrings(inputs.Variables[name], "", func(_, text string) error {
			refs, err := placeholder.Refs(text)
			if err != nil {
				return nil // reported by the placeholder checks
			}
			for _, ref := range refs {
				if ref.Kind != placeholder.KindResource {
					continue
				}
				descriptor, err := planning.ParseDescriptorText(ref.Resource, current, ctx)
				if err != nil {
					continue
				}
				for _, typ := range types {
					if field, ok := typ.Output(ref.OutputKey); typ.Key == descriptor.Type && ok && field.Secret {
						return fmt.Errorf("%s.%s binds the secret output %q of resource type %q; a secret output cannot feed a nonsecret module input", variablesPath, name, ref.OutputKey, typ.Key)
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// publicAccessCIDRsVariable is the EKS API allowlist input (module contract,
// "Provider bindings và dependency"): explicit IPv4 hosts only.
const publicAccessCIDRsVariable = "public_access_cidrs"

var ipv4Host32 = regexp.MustCompile(`^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})/32$`)

// checkPublicAccessCIDRs rejects a literal entry that is not one specific
// IPv4 /32 host, a wider prefix, 0.0.0.0, or a duplicate. Whole placeholders
// are checked when they resolve; nothing is ever added automatically (the
// executor's own IP is never appended) and there is no default.
func checkPublicAccessCIDRs(path string, value any) error {
	items, ok := value.([]any)
	if !ok {
		return nil // a whole placeholder; resolved and re-checked at execution
	}
	if len(items) == 0 || len(items) > 40 {
		return fmt.Errorf("%s must list 1 to 40 explicit IPv4 /32 hosts", path)
	}
	seen := map[string]bool{}
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return fmt.Errorf("%s[%d] must be a string", path, i)
		}
		if full, _ := classifyPlaceholders(text); full {
			continue
		}
		match := ipv4Host32.FindStringSubmatch(text)
		if match == nil || text == "0.0.0.0/32" {
			return fmt.Errorf("%s[%d] must be one specific IPv4 host written as a.b.c.d/32 (never a wider prefix or 0.0.0.0)", path, i)
		}
		for _, octet := range match[1:] {
			if n, _ := strconv.Atoi(octet); n > 255 {
				return fmt.Errorf("%s[%d] must be one specific IPv4 host written as a.b.c.d/32", path, i)
			}
		}
		if seen[text] {
			return fmt.Errorf("%s[%d] repeats an address", path, i)
		}
		seen[text] = true
	}
	return nil
}
