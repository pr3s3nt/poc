package planner

import "sort"

// criteriaWeights follows the Humanitec matching specificity table.
var criteriaWeights = map[string]int{
	"env_type": 1,
	"app_id":   2,
	"env_id":   4,
	"res_id":   8,
	"class":    16,
}

func nodeMatchContext(node *Node, ctx Document) map[string]string {
	return map[string]string{
		"env_type": stringAt(ctx, "env_type"),
		"app_id":   stringAt(ctx, "app_id"),
		"env_id":   stringAt(ctx, "env_id"),
		"res_id":   node.ResourceID,
		"class":    node.Class,
	}
}

// criterionScore reports whether the criterion matches and its total weight.
func criterionScore(criterion Document, values map[string]string) (int, bool) {
	score := 0
	for _, field := range sortedKeys(criterion) {
		weight, known := criteriaWeights[field]
		if !known {
			return 0, false
		}
		declared, ok := criterion[field].(string)
		if !ok || declared != values[field] {
			return 0, false
		}
		score += weight
	}
	return score, true
}

// matchDefinition selects the highest scoring Resource Definition for a node.
func matchDefinition(node *Node, definitions []*Definition, ctx Document) error {
	values := nodeMatchContext(node, ctx)

	type candidate struct {
		def       *Definition
		criterion Document
		score     int
	}
	var best []candidate
	bestScore := -1

	ordered := append([]*Definition(nil), definitions...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	for _, def := range ordered {
		if def.Type != node.Type || len(def.Criteria) == 0 {
			continue
		}
		var bestCriterion Document
		defScore := -1
		for _, criterion := range def.Criteria {
			score, ok := criterionScore(criterion, values)
			if !ok || score <= defScore {
				continue
			}
			defScore, bestCriterion = score, criterion
		}
		if defScore < 0 {
			continue
		}
		if defScore > bestScore {
			bestScore, best = defScore, nil
		}
		if defScore == bestScore {
			best = append(best, candidate{def: def, criterion: bestCriterion, score: defScore})
		}
	}

	switch len(best) {
	case 0:
		return newError("match", "NO_MATCHING_DEFINITION", node.Descriptor())
	case 1:
		node.Definition = best[0].def
		node.Criterion = best[0].criterion
		node.Score = best[0].score
		return nil
	default:
		return newError("match", "AMBIGUOUS_DEFINITION", node.Descriptor())
	}
}
