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

// clusterProvider resolves the cluster resource that supplies the Kubernetes
// provider target of a binding. A resource binding names it; an Environment
// binding means the Environment's own cluster: the registered internal cluster
// of internal-k8s or the EKS node of aws-eks. Never inferred from variables.
func (b *graphBuilder) clusterProvider(node *Node, binding tfmodule.Binding) (resource.Descriptor, error) {
	if binding.Target == tfmodule.TargetResource {
		target, err := ParseDescriptorText(binding.ResourceRef, node, b.ctx)
		if err != nil {
			return resource.Descriptor{}, fmt.Errorf("resource_ref is not a valid resource descriptor")
		}
		if target.Type != TypeCluster {
			return resource.Descriptor{}, fmt.Errorf("resource_ref must reference a Kubernetes cluster resource")
		}
		return target, nil
	}
	switch b.ctx.Env.Profile {
	case application.ProfileInternalK8s:
		return InternalClusterDescriptor(b.ctx.Env.ConnectionKey)
	case application.ProfileAWSEKS:
		return EKSDescriptor(b.ctx)
	}
	return resource.Descriptor{}, fmt.Errorf("the Environment execution profile has no cluster target")
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
		provider, err := b.clusterProvider(node, inputs.ProviderBindings[name])
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

// providerPins resolves and pins the scoped target metadata of every
// provider binding of one Terraform node. It checks the target exists and has
// the required kind: AWS needs an aws-eks Environment with its AWS
// Connection; Kubernetes needs a cluster node already in the graph. A
// configurable provider without a binding is refused.
func (b *graphBuilder) providerPins(node *Node, def resource.Definition, contract ModuleContract) ([]tfmodule.ProviderPin, error) {
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
		pin := tfmodule.ProviderPin{Name: name, Kind: kind, Target: binding.Target, Profile: string(env.Profile), Generation: env.TargetGeneration}
		switch kind {
		case tfmodule.BindingAWS:
			if env.Profile != application.ProfileAWSEKS || b.ctx.Connection.Kind != application.ConnectionAWS || b.ctx.Connection.Key != env.ConnectionKey {
				return nil, fmt.Errorf("planning: %s provider binding %s needs an aws-eks Environment with its AWS Connection", node.Descriptor, name)
			}
			pin.ConnectionKey, pin.ConnectionKind, pin.Region = env.ConnectionKey, string(application.ConnectionAWS), env.Region
		case tfmodule.BindingKubernetes:
			provider, err := b.clusterProvider(node, binding)
			if err != nil {
				return nil, fmt.Errorf("planning: %s provider binding %s: %w", node.Descriptor, name, err)
			}
			if _, present := b.nodes[provider.String()]; !present {
				return nil, fmt.Errorf("planning: %s provider binding %s names a cluster %s that is not part of the plan", node.Descriptor, name, provider)
			}
			pin.Provider = provider.String()
			if internal, err := InternalClusterDescriptor(env.ConnectionKey); err == nil && provider == internal {
				if env.Profile != application.ProfileInternalK8s || b.ctx.Connection.Kind != application.ConnectionKubernetes || b.ctx.Connection.Key != env.ConnectionKey {
					return nil, fmt.Errorf("planning: %s provider binding %s needs the Environment Kubernetes Connection", node.Descriptor, name)
				}
				pin.ConnectionKey, pin.ConnectionKind = env.ConnectionKey, string(application.ConnectionKubernetes)
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
