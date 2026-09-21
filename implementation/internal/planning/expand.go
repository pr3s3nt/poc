package planning

import (
	"fmt"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/platform/canon"
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
// consumer of the parent also gets an edge to the child, including consumers
// that appear only in a later expansion round.
type dependentRule struct {
	Parent string
	Child  string
}

const maxExpansionRounds = 32

// expand matches Definitions and expands Resource References and
// co-provisioned resources until the graph reaches a fixed point
// (UC-06 MS-07, BR-08).
func (b *graphBuilder) expand(matcher *definitionMatcher, matches map[string]Match) ([]pendingRef, error) {
	var refs []pendingRef
	var dependents []dependentRule

	for round := 0; round < maxExpansionRounds; round++ {
		progress := false

		for _, key := range b.descriptors() {
			node := b.nodes[key]
			if node.Kind != NodeResource {
				continue
			}
			if _, done := matches[key]; done {
				continue
			}
			match, err := matcher.match(node)
			if err != nil {
				return nil, err
			}
			matches[key] = match
			progress = true
		}

		for _, key := range b.descriptors() {
			node := b.nodes[key]
			if node.Kind != NodeResource || b.expanded[key] {
				continue
			}
			match, ok := matches[key]
			if !ok {
				continue
			}
			def, ok := matcher.definition(match.DefinitionKey)
			if !ok {
				continue
			}
			b.expanded[key] = true
			progress = true

			found, err := b.expandReferences(node, def)
			if err != nil {
				return nil, err
			}
			refs = append(refs, found...)

			provisioned, rules, err := b.expandProvision(node, def)
			if err != nil {
				return nil, err
			}
			refs = append(refs, provisioned...)
			dependents = append(dependents, rules...)
		}

		if b.applyMatchDependents(dependents) {
			progress = true
		}
		if !progress {
			return refs, nil
		}
	}
	return nil, fmt.Errorf("planning: graph expansion did not reach a fixed point")
}

// expandReferences covers the driver inputs of the node's own Definition.
func (b *graphBuilder) expandReferences(node *Node, def resource.Definition) ([]pendingRef, error) {
	return b.scanReferences(node, def.DriverInputs, pointer("definitions", def.Key, "driver_inputs"))
}

// expandProvision realises the provision rules of the node's Definition.
// References inside a rule's params belong to the co-provisioned child, so they
// inherit the child's class/ID and produce child -> provider edges.
func (b *graphBuilder) expandProvision(node *Node, def resource.Definition) ([]pendingRef, []dependentRule, error) {
	var refs []pendingRef
	var rules []dependentRule

	keys := make([]string, 0, len(def.Provision))
	for key := range def.Provision {
		keys = append(keys, key)
	}
	sortStrings(keys)

	for _, key := range keys {
		rule := def.Provision[key]
		path := pointer("definitions", def.Key, "provision", key)
		target, err := ParseDescriptorText(key, node, b.ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("planning: %s: %w", path, err)
		}
		child, err := b.ensure(target, NodeResource, OriginProvision, copyParams(rule.Params))
		if err != nil {
			return nil, nil, err
		}
		if rule.IsDependent {
			b.addEdge(child.Descriptor, node.Descriptor, ReasonDependent, "")
		}
		if rule.MatchDependents {
			rules = append(rules, dependentRule{Parent: node.Descriptor, Child: child.Descriptor})
		}
		found, err := b.scanReferences(child, rule.Params, path+"/params")
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, found...)
	}
	return refs, rules, nil
}

// scanReferences walks string leaves for ${resources[...].outputs.X}
// placeholders. current supplies the class/ID inherited by "@" descriptors and
// owns the resulting edges.
func (b *graphBuilder) scanReferences(current *Node, value any, base string) ([]pendingRef, error) {
	if value == nil {
		return nil, nil
	}
	tree, err := canon.Clone(value)
	if err != nil {
		return nil, err
	}
	var refs []pendingRef
	err = placeholder.WalkStrings(tree, base, func(path, text string) error {
		for _, m := range placeholder.Pattern.FindAllStringSubmatch(text, -1) {
			if len(m) < 2 || m[0][:2] == "$$" {
				continue
			}
			body := m[1]
			if !placeholder.LooksLikeResourceReference(body) {
				continue
			}
			ref, err := placeholder.Parse(body)
			if err != nil {
				return fmt.Errorf("planning: %s: %w", path, err)
			}
			target, err := ParseDescriptorText(ref.Resource, current, b.ctx)
			if err != nil {
				return fmt.Errorf("planning: %s: %w", path, err)
			}
			if _, err := b.ensure(target, NodeResource, OriginReference, nil); err != nil {
				return err
			}
			b.addEdge(current.Descriptor, target.String(), ReasonReference, path)
			b.bind(current, ref.Resource, target.String())
			refs = append(refs, pendingRef{
				From: current.Descriptor, To: target.String(), Output: ref.OutputKey, Path: path,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// applyMatchDependents re-links every current consumer of a parent to its
// co-provisioned child and reports whether it added an edge.
func (b *graphBuilder) applyMatchDependents(rules []dependentRule) bool {
	added := false
	for _, rule := range rules {
		for _, consumer := range b.consumersOf(rule.Parent) {
			if consumer == rule.Child {
				continue
			}
			if b.addEdge(consumer, rule.Child, ReasonMatchDependent, "") {
				added = true
			}
		}
	}
	return added
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}
