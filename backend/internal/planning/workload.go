package planning

import (
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

func placementErr(stage Stage, format string, args ...any) error {
	return &StageError{Stage: stage, Reason: ReasonPlacement, Err: fmt.Errorf("planning: "+format, args...)}
}

// expandWorkloads selects every workload Definition BEFORE the resource graph
// is expanded (ADR-016), then adds the references and companion resources the
// selected Definition declares: namespace/cluster output references, typed
// input references and provision rules. The returned references are checked
// against provider output contracts like any other reference.
func (b *graphBuilder) expandWorkloads(s *Service, matcher *definitionMatcher) ([]pendingRef, []dependentRule, error) {
	rm := renderMatcher(b.ctx, matcher)
	var refs []pendingRef
	var rules []dependentRule
	for _, key := range b.descriptors() {
		node := b.nodes[key]
		if node.Kind != NodeWorkload {
			continue
		}
		def, match, skipped, err := s.pickWorkload(b.ctx, rm, *node, b.catalog)
		if err != nil {
			return nil, nil, err
		}
		if skipped {
			return nil, nil, placementErr(StageCatalog, "workload %s has no rendering Definition", node.Descriptor)
		}
		if _, ok := def.Variables()[resource.VariableNamespace]; !ok {
			return nil, nil, placementErr(StageCatalog, "workload definition %q declares no namespace/cluster references", def.Key)
		}
		b.workloadDefs[node.Descriptor] = def
		b.workloadMatches[node.Descriptor] = match
		found, err := b.scanReferences(node, def.DriverInputs, pointer("definitions", def.Key, "driver_inputs"))
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, found...)
		// Provision rules keep the canonical resource semantics (UC-06 BR-08): neither flag
		// adds no edge; is_dependent makes the companion depend on this workload;
		// match_dependents makes every consumer of this workload depend on the companion.
		provisioned, provisionRules, err := b.expandProvision(node, def)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, provisioned...)
		rules = append(rules, provisionRules...)
	}
	return refs, rules, nil
}

// referenced returns the node a workload variable's whole reference points at.
func (b *graphBuilder) referenced(node *Node, def resource.Definition, variable string) (*Node, error) {
	text, _ := def.Variables()[variable].(string)
	descriptorText, _, ok := resource.WorkloadReference(text)
	if !ok {
		return nil, placementErr(StageGraph, "workload definition %q variable %q is not a whole output reference", def.Key, variable)
	}
	provider, ok := node.Bindings[descriptorText]
	if !ok {
		return nil, placementErr(StageGraph, "workload definition %q variable %q is unresolved", def.Key, variable)
	}
	target, ok := b.nodes[provider]
	if !ok {
		return nil, placementErr(StageGraph, "workload definition %q variable %q names a missing node", def.Key, variable)
	}
	return target, nil
}

// clusterAccountMatch returns the existing-cluster match that backs a cluster
// node, or a placement error when the node is not an account-backed cluster.
func clusterAccountMatch(node *Node, matches map[string]Match, defs *definitionMatcher) (Match, error) {
	match, ok := matches[node.Descriptor]
	if !ok || match.DriverType != resource.DriverExistingCluster || match.DriverAccount == "" || match.AccountVersion < 1 {
		return Match{}, placementErr(StageGraph, "cluster %s is not backed by an existing-cluster Definition with a READY driver account", node.Descriptor)
	}
	if node.ResourceType != TypeCluster {
		return Match{}, placementErr(StageGraph, "%s is not a Kubernetes cluster resource", node.Descriptor)
	}
	return match, nil
}

// resolvePlacements pins, per workload and per Kubernetes-bound Terraform
// resource, the cluster Definition/account the node runs on. A namespace's
// provider cluster and the workload's cluster reference must be the same
// cluster; missing, ambiguous or contradictory targets fail here, before any
// consumer runs. V03 admits one cluster per Candidate Set; the pins stay per node.
func (b *graphBuilder) resolvePlacements(matches map[string]Match, matcher *definitionMatcher, contracts []TerraformContract) (map[string]Placement, error) {
	if !b.ctx.DefinitionPlaced() {
		return nil, nil
	}
	out := map[string]Placement{}
	place := func(node, clusterDescriptor, namespace, source string) error {
		cluster, ok := b.nodes[clusterDescriptor]
		if !ok {
			return placementErr(StageGraph, "%s names cluster %s which is not part of the plan", node, clusterDescriptor)
		}
		match, err := clusterAccountMatch(cluster, matches, matcher)
		if err != nil {
			return err
		}
		out[node] = Placement{
			Node: node, Cluster: clusterDescriptor, ClusterDefinition: match.DefinitionKey, ClusterDefinitionVersion: match.DefinitionVersion,
			Account: match.DriverAccount, AccountVersion: match.AccountVersion, Namespace: namespace, Source: source,
		}
		return nil
	}
	for _, contract := range contracts {
		for _, pin := range contract.ProviderBindings {
			if pin.Kind != tfmodule.BindingKubernetes {
				continue
			}
			if pin.Provider == "" {
				return nil, placementErr(StageContract, "%s has a Kubernetes provider binding without a cluster resource", contract.Descriptor)
			}
			if err := place(contract.Descriptor, pin.Provider, "", PlacementProviderBinding); err != nil {
				return nil, err
			}
		}
	}
	// A resource that references a cluster output directly runs on that cluster.
	for _, key := range b.descriptors() {
		node := b.nodes[key]
		if node.Kind != NodeResource || node.ResourceType == TypeCluster {
			continue
		}
		if _, placed := out[key]; placed {
			continue
		}
		for _, bound := range sortedKeys(node.Bindings) {
			if target, ok := b.nodes[node.Bindings[bound]]; ok && target.ResourceType == TypeCluster {
				if err := place(key, target.Descriptor, "", PlacementReference); err != nil {
					return nil, err
				}
				break
			}
		}
	}
	for _, key := range b.descriptors() {
		node := b.nodes[key]
		if node.Kind != NodeWorkload {
			continue
		}
		def := b.workloadDefs[key]
		ns, err := b.referenced(node, def, resource.VariableNamespace)
		if err != nil {
			return nil, err
		}
		cluster, err := b.referenced(node, def, resource.VariableCluster)
		if err != nil {
			return nil, err
		}
		if ns.ResourceType != TypeNamespace {
			return nil, placementErr(StageGraph, "workload %s namespace reference is not a Kubernetes namespace", key)
		}
		nsPlacement, ok := out[ns.Descriptor]
		if !ok {
			return nil, placementErr(StageGraph, "namespace %s has no Kubernetes provider cluster", ns.Descriptor)
		}
		if nsPlacement.Cluster != cluster.Descriptor {
			return nil, placementErr(StageGraph, "workload %s cluster %s contradicts the provider cluster %s of its namespace", key, cluster.Descriptor, nsPlacement.Cluster)
		}
		if err := place(key, cluster.Descriptor, ns.Descriptor, PlacementWorkloadReference); err != nil {
			return nil, err
		}
	}
	clusters := map[string]bool{}
	for _, placement := range out {
		clusters[placement.Cluster] = true
	}
	if len(clusters) > 1 {
		names := make([]string, 0, len(clusters))
		for name := range clusters {
			names = append(names, name)
		}
		sort.Strings(names)
		return nil, &StageError{Stage: StageGraph, Reason: ReasonMultiCluster, Err: fmt.Errorf("planning: the Candidate Set resolves %d clusters %v; V03 supports one", len(names), names)}
	}
	return out, nil
}

var _ = errors.Is
