package planning

import (
	"fmt"
	"sort"

	"orchestrator/internal/domain/resource"
)

// definitionMatcher selects the most specific Resource Definition for each
// resource node (UC-03 BR-02 and BR-06, UC-06 MS-07).
type definitionMatcher struct {
	ctx  Context
	defs map[string]resource.Definition
	list []resource.Definition
}

func newDefinitionMatcher(ctx Context, catalog Catalog) (*definitionMatcher, error) {
	m := &definitionMatcher{ctx: ctx, defs: map[string]resource.Definition{}}
	defs := append([]resource.Definition(nil), catalog.Definitions...)
	resource.SortDefinitions(defs)
	for _, d := range defs {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		if _, ok := catalog.Types[d.ResourceTypeKey]; !ok {
			return nil, fmt.Errorf("planning: definition %q references unknown resource type %q", d.Key, d.ResourceTypeKey)
		}
		if _, exists := m.defs[d.Key]; exists {
			return nil, fmt.Errorf("planning: duplicate resource definition key %q", d.Key)
		}
		m.defs[d.Key] = d
	}
	m.list = defs
	return m, nil
}

func (m *definitionMatcher) definition(key string) (resource.Definition, bool) {
	d, ok := m.defs[key]
	return d, ok
}

// match returns the winning definition for one node.
func (m *definitionMatcher) match(node *Node) (Match, error) {
	ctx := resource.MatchContext{
		EnvironmentType: m.ctx.Env.Type,
		ApplicationID:   m.ctx.App.Key,
		EnvironmentID:   m.ctx.Env.Key,
		ResourceID:      descriptorID(node.Descriptor),
		Class:           node.Class,
	}

	type candidate struct {
		def       resource.Definition
		criterion resource.Criterion
		score     int
	}
	var best []candidate
	bestScore := -1

	for _, def := range m.list {
		if def.ResourceTypeKey != node.ResourceType {
			continue
		}
		criterion, score, ok := def.BestCriterion(ctx)
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore, best = score, nil
		}
		if score == bestScore {
			best = append(best, candidate{def: def, criterion: criterion, score: score})
		}
	}

	switch len(best) {
	case 0:
		return Match{}, fmt.Errorf("planning: no resource definition matches %s (type %q, class %q)",
			node.Descriptor, node.ResourceType, node.Class)
	case 1:
		def := best[0].def
		return Match{
			Descriptor:    node.Descriptor,
			DefinitionKey: def.Key,
			DriverType:    def.DriverType,
			ConnectionKey: def.ConnectionKey,
			Specificity:   best[0].score,
			Criterion:     best[0].criterion,
		}, nil
	default:
		keys := make([]string, 0, len(best))
		for _, c := range best {
			keys = append(keys, c.def.Key)
		}
		sort.Strings(keys)
		return Match{}, fmt.Errorf("planning: ambiguous match for %s between %v", node.Descriptor, keys)
	}
}
