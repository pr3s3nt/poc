package planner

import (
	"sort"
	"strings"
)

// pendingRef records a Resource Reference so its output can be checked once
// every provider has been matched.
type pendingRef struct {
	From   string
	To     string
	Output string
	Path   string
}

// dependentRule remembers a match_dependents co-provision rule so every
// consumer of the parent gets an edge to the child, including consumers that
// only appear in a later expansion round.
type dependentRule struct {
	Parent string
	Child  string
}

// expandGraph matches Definitions and expands Resource References and
// co-provisioned resources until the graph reaches a fixed point.
func expandGraph(c *LoadedCase, g *ResourceGraph) ([]pendingRef, error) {
	var refs []pendingRef
	var dependents []dependentRule

	for {
		progress := false

		for _, key := range g.descriptors() {
			node := g.Nodes[key]
			if node.Definition != nil {
				continue
			}
			if err := matchDefinition(node, c.ResourceDefinitions, c.Context); err != nil {
				return nil, err
			}
			progress = true
		}

		for _, key := range g.descriptors() {
			node := g.Nodes[key]
			if node.expanded || node.Definition == nil {
				continue
			}
			node.expanded = true
			progress = true

			found, err := expandReferences(g, node)
			if err != nil {
				return nil, err
			}
			refs = append(refs, found...)

			provisioned, rules, err := expandProvision(g, node)
			if err != nil {
				return nil, err
			}
			refs = append(refs, provisioned...)
			dependents = append(dependents, rules...)
		}

		if applyMatchDependents(g, dependents) {
			progress = true
		}

		if !progress {
			break
		}
	}
	return refs, nil
}

// scanReferences walks string leaves for ${resources[...].outputs.X}
// placeholders. current supplies the class/ID inherited by "@" descriptors and
// owns the resulting edges.
func scanReferences(g *ResourceGraph, current *Node, value any, base string) ([]pendingRef, error) {
	var refs []pendingRef
	err := walkStrings(value, base, func(path, text string) error {
		for _, ph := range scanPlaceholders(text) {
			ref, ok := parseResourceReference(ph.Body)
			if !ok {
				if looksLikeResourceReference(ph.Body) {
					return newError("graph", "INVALID_RESOURCE_REFERENCE", path)
				}
				continue
			}
			target, err := parseDescriptor(ref.Raw, current, path)
			if err != nil {
				return err
			}
			g.ensure(target, "resource-reference", Document{})
			g.addEdge(current.Descriptor(), target.String(), "resource-reference", path)
			refs = append(refs, pendingRef{
				From: current.Descriptor(), To: target.String(), Output: ref.Output, Path: path,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// expandReferences covers the driver inputs of the node's own Definition.
func expandReferences(g *ResourceGraph, node *Node) ([]pendingRef, error) {
	def := node.Definition
	return scanReferences(g, node, def.DriverInputs, pointer("definitions", def.ID, "driver_inputs"))
}

// expandProvision realises entity.provision rules of the node's Definition.
// References inside a rule's params belong to the co-provisioned child, so they
// inherit the child's class/ID and produce child -> provider edges.
func expandProvision(g *ResourceGraph, node *Node) ([]pendingRef, []dependentRule, error) {
	def := node.Definition
	var refs []pendingRef
	var rules []dependentRule

	for _, key := range sortedKeys(def.Provision) {
		rule := asDocument(def.Provision[key])
		path := pointer("definitions", def.ID, "provision", key)
		target, err := parseDescriptor(key, node, path)
		if err != nil {
			return nil, nil, err
		}
		child := g.ensure(target, "co-provision", entryParams(rule))

		if dependent, _ := rule["is_dependent"].(bool); dependent {
			g.addEdge(child.Descriptor(), node.Descriptor(), "co-provision-is-dependent", "")
		}
		if matchDependents, _ := rule["match_dependents"].(bool); matchDependents {
			rules = append(rules, dependentRule{Parent: node.Descriptor(), Child: child.Descriptor()})
		}

		found, err := scanReferences(g, child, rule["params"], path+"/params")
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, found...)
	}
	return refs, rules, nil
}

// applyMatchDependents re-links every current consumer of a parent to its
// co-provisioned child and reports whether it added an edge.
func applyMatchDependents(g *ResourceGraph, rules []dependentRule) bool {
	added := false
	for _, rule := range rules {
		for _, consumer := range g.consumersOf(rule.Parent) {
			if consumer == rule.Child {
				continue
			}
			if g.addEdge(consumer, rule.Child, "co-provision-match-dependents", "") {
				added = true
			}
		}
	}
	return added
}

// looksLikeResourceReference reports a placeholder that opens a resource
// reference but does not parse, so it can be rejected instead of ignored.
func looksLikeResourceReference(body string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(body), "resources")
	if !ok || rest == "" {
		return false
	}
	return rest[0] == '.' || rest[0] == '['
}

// validateReferenceOutputs checks every Resource Reference against the
// provider's Terraform outputs, or its Echo Driver values.
func validateReferenceOutputs(g *ResourceGraph, refs []pendingRef, tc *terraformCache) error {
	ordered := append([]pendingRef(nil), refs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].From != ordered[j].From {
			return ordered[i].From < ordered[j].From
		}
		return ordered[i].Path < ordered[j].Path
	})

	for _, ref := range ordered {
		provider, ok := g.Nodes[ref.To]
		if !ok || provider.Definition == nil {
			continue
		}
		var outputs []string
		switch provider.Definition.DriverType {
		case "humanitec/echo":
			outputs = sortedKeys(mapAt(provider.Definition.DriverInputs, "values"))
		default:
			module, err := tc.moduleFor(provider.Definition, provider.Descriptor())
			if err != nil {
				// Source problems belong to the terraform phase.
				continue
			}
			outputs = module.Outputs
		}
		if !contains(outputs, ref.Output) {
			return newError("graph", "UNKNOWN_OUTPUT", ref.Path)
		}
	}
	return nil
}
