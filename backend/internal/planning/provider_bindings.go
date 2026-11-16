package planning

import (
	"fmt"
	"sort"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

// OriginProviderBinding marks a node that exists because a Terraform module's
// Kubernetes provider binding names it.
const OriginProviderBinding Origin = "provider-binding"

// driverBindings parses the Definition's provider bindings without failing
// the plan: unparsable Definitions are reported by the contract inspection.
func (b *graphBuilder) driverBindings(def resource.Definition) (tfmodule.DriverInputs, ModuleContract, bool) {
	if def.DriverType != resource.DriverTerraform || b.inspector == nil {
		return tfmodule.DriverInputs{}, ModuleContract{}, false
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return tfmodule.DriverInputs{}, ModuleContract{}, false
	}
	contract, err := b.inspector.Contract(def)
	if err != nil {
		return tfmodule.DriverInputs{}, ModuleContract{}, false
	}
	return inputs, contract, true
}

// providerResource resolves the authored cluster or namespace dependency.
// Product Environment targets are retired; read-only planner conformance may
// name its synthetic registered cluster. No target is inferred from variables.
func (b *graphBuilder) providerResource(node *Node, binding tfmodule.Binding) (resource.Descriptor, error) {
	if binding.Target == tfmodule.TargetResource {
		target, err := ParseDescriptorText(binding.ResourceRef, node, b.ctx)
		if err != nil {
			return resource.Descriptor{}, fmt.Errorf("resource_ref is not a valid resource descriptor")
		}
		if target.Type != TypeCluster && target.Type != TypeNamespace {
			return resource.Descriptor{}, fmt.Errorf("resource_ref must reference a Kubernetes cluster or namespace resource")
		}
		return target, nil
	}
	if b.ctx.DefinitionPlaced() {
		return resource.Descriptor{}, fmt.Errorf("the Environment provider target is retired; bind the provider to a cluster resource (target resource)")
	}
	if b.ctx.ReferenceCluster { // planner reference mode only; a product Environment never reaches here
		return InternalClusterDescriptor(b.ctx.Connection.Key)
	}
	return resource.Descriptor{}, fmt.Errorf("no resource-bound cluster target is available")
}

// clusterProvider follows a namespace's own selected provider, never a display
// name, Environment account or arbitrary cluster found elsewhere in the graph.
func (b *graphBuilder) clusterProvider(node *Node, def resource.Definition, binding tfmodule.Binding, matches map[string]Match) (resource.Descriptor, error) {
	target, err := b.providerResource(node, binding)
	if err != nil || target.Type == TypeCluster {
		return target, err
	}
	if node.ResourceType == TypeNamespace {
		return resource.Descriptor{}, fmt.Errorf("namespace provider must reference a cluster directly")
	}
	ns := b.nodes[target.String()]
	if ns == nil {
		return resource.Descriptor{}, fmt.Errorf("provider namespace is missing")
	}
	input, ok := def.Variables()[resource.VariableNamespace].(string)
	ref, output, whole := resource.WorkloadReference(input)
	selected, parseErr := ParseDescriptorText(ref, node, b.ctx)
	if !ok || !whole || output != "name" || parseErr != nil || selected.String() != target.String() {
		return resource.Descriptor{}, fmt.Errorf("namespace input must reference the provider namespace name output")
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return resource.Descriptor{}, err
	}
	if _, mapped := inputs.InputMapping[resource.VariableNamespace]; mapped {
		return resource.Descriptor{}, fmt.Errorf("provider namespace cannot be overridden by a resource input")
	}
	match, matched := matches[ns.Descriptor]
	nsDef, found := b.definition(match.DefinitionKey)
	nsInputs, contract, valid := b.driverBindings(nsDef)
	if !matched || !found || !valid {
		return resource.Descriptor{}, fmt.Errorf("namespace has no verified Kubernetes provider")
	}
	var cluster resource.Descriptor
	count := 0
	for _, name := range sortedKeys(nsInputs.ProviderBindings) {
		if contract.ProviderKinds[name] != tfmodule.BindingKubernetes {
			continue
		}
		resolved, err := b.providerResource(ns, nsInputs.ProviderBindings[name])
		if err != nil || resolved.Type != TypeCluster {
			return resource.Descriptor{}, fmt.Errorf("namespace provider must reference a cluster directly")
		}
		if count > 0 && cluster.String() != resolved.String() {
			return resource.Descriptor{}, fmt.Errorf("namespace has ambiguous cluster providers")
		}
		cluster = resolved
		count++
	}
	if count == 0 {
		return resource.Descriptor{}, fmt.Errorf("namespace has no Kubernetes cluster provider")
	}
	return cluster, nil
}

// expandProviderBindings adds the deterministic dependency edge of every
// Kubernetes provider binding of a Terraform node, so the cluster runs first
// even when no variable placeholder mentions it.
func (b *graphBuilder) expandProviderBindings(node *Node, def resource.Definition) error {
	inputs, contract, ok := b.driverBindings(def)
	if !ok {
		return nil
	}
	for _, name := range sortedKeys(inputs.ProviderBindings) {
		if contract.ProviderKinds[name] != tfmodule.BindingKubernetes {
			continue
		}
		provider, err := b.providerResource(node, inputs.ProviderBindings[name])
		if err != nil {
			return fmt.Errorf("planning: %s provider binding %s: %w", node.Descriptor, name, err)
		}
		if _, err := b.ensure(provider, NodeResource, OriginProviderBinding, nil); err != nil {
			return err
		}
		b.addEdge(node.Descriptor, provider.String(), ReasonProviderBinding, pointer("definitions", def.Key, "driver_inputs", "values", "provider_bindings", name))
		b.bind(node, "provider."+name, provider.String())
	}
	return nil
}

// awsAccountPin resolves the Driver Account and literal region an AWS binding
// of def uses (UC-03 V02-03). Environment identity is never a fallback and the
// account's verification regions are not an allowlist.
func (b *graphBuilder) awsAccountPin(def resource.Definition) (application.Connection, string, error) {
	if def.DriverAccount == "" {
		return application.Connection{}, "", fmt.Errorf("%w: definition %q has no driver account", ErrAccountConflict, def.Key)
	}
	account, ok := b.catalog.Accounts[def.DriverAccount]
	if !ok || account.OrganizationKey != b.ctx.OrganizationKey || account.Kind != application.ConnectionAWS || account.Status != application.ConnectionReady {
		return application.Connection{}, "", fmt.Errorf("%w: definition %q needs a READY AWS driver account of this organization", ErrAccountConflict, def.Key)
	}
	region, ok := def.DefinitionRegion()
	if !ok || !resource.ValidAWSRegion(region) {
		return application.Connection{}, "", fmt.Errorf("planning: definition %q has no literal AWS region in its driver inputs", def.Key)
	}
	return account, region, nil
}

// providerPins resolves and pins the scoped target metadata of every
// provider binding of one Terraform node. It checks the target exists and has
// the required kind: AWS needs the Definition's READY AWS Driver Account and
// literal region; Kubernetes needs a cluster node already in the graph. A
// configurable provider without a binding is refused.
func (b *graphBuilder) providerPins(node *Node, def resource.Definition, contract ModuleContract, matches map[string]Match) ([]tfmodule.ProviderPin, error) {
	if contract.ProviderKinds == nil {
		return nil, nil // planner-reference fixtures declare no providers
	}
	inputs, err := tfmodule.ParseDriverInputs(def.DriverValues())
	if err != nil {
		return nil, fmt.Errorf("planning: definition %q: %w", def.Key, err)
	}
	env := b.ctx.Env
	names := sortedKeys(contract.ProviderKinds)
	pins := []tfmodule.ProviderPin{}
	for _, name := range names {
		kind := contract.ProviderKinds[name]
		if kind == tfmodule.BindingNone {
			continue
		}
		binding, bound := inputs.ProviderBindings[name]
		if !bound {
			return nil, fmt.Errorf("planning: %s has no provider binding for %s", node.Descriptor, name)
		}
		pin := tfmodule.ProviderPin{Name: name, Kind: kind, Target: binding.Target, Generation: env.TargetGeneration}
		switch kind {
		case tfmodule.BindingAWS:
			if _, mapped := inputs.InputMapping["region"]; mapped {
				return nil, fmt.Errorf("planning: %s provider binding %s: the AWS region cannot come from a Resource Type input", node.Descriptor, name)
			}
			account, region, err := b.awsAccountPin(def)
			if err != nil {
				return nil, fmt.Errorf("planning: %s provider binding %s: %w", node.Descriptor, name, err)
			}
			if binding.Target != tfmodule.TargetAccount {
				return nil, fmt.Errorf("planning: %s provider binding %s must target the definition's driver account", node.Descriptor, name)
			}
			pin.ConnectionKey, pin.ConnectionKind, pin.Region, pin.AccountVersion = account.Key, string(application.ConnectionAWS), region, account.Version
		case tfmodule.BindingKubernetes:
			provider, err := b.clusterProvider(node, def, binding, matches)
			if err != nil {
				return nil, fmt.Errorf("planning: %s provider binding %s: %w", node.Descriptor, name, err)
			}
			if _, present := b.nodes[provider.String()]; !present {
				return nil, fmt.Errorf("planning: %s provider binding %s names a cluster %s that is not part of the plan", node.Descriptor, name, provider)
			}
			pin.Provider = provider.String()
			if authored, err := b.providerResource(node, binding); err == nil && authored.Type == TypeNamespace {
				pin.Namespace = authored.String()
			}
			if b.ctx.DefinitionPlaced() {
				src, err := b.clusterSourceOf(b.nodes[provider.String()], matches)
				if err != nil {
					return nil, fmt.Errorf("planning: %s provider binding %s: %w", node.Descriptor, name, err)
				}
				switch src.Provider {
				case ClusterEKS:
					// No Kubernetes Connection supplies credentials: execution derives fresh short-lived
					// authentication from the producing cluster's original AWS account (ADR-017).
					pin.ClusterProvider, pin.ClusterAccount, pin.AccountVersion, pin.Region = ClusterEKS, src.Account, src.AccountVersion, src.Region
				default:
					pin.ConnectionKey, pin.ConnectionKind, pin.AccountVersion = src.Account, string(application.ConnectionKubernetes), src.AccountVersion
				}
			}
		}
		pins = append(pins, pin)
	}
	for _, name := range sortedKeys(inputs.ProviderBindings) {
		if _, declared := contract.ProviderKinds[name]; !declared {
			return nil, fmt.Errorf("planning: %s binds provider %s which the module does not require", node.Descriptor, name)
		}
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].Name < pins[j].Name })
	return pins, nil
}
