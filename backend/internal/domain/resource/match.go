package resource

import (
	"regexp"
	"sort"
)

// Ranked is one Definition that matches a node context, with its best
// criterion and specificity score.
type Ranked struct {
	Definition Definition
	Criterion  Criterion
	Score      int
}

// Ranking is the outcome of the single weighted matcher of UC-03 V02-02, shared
// by the planner and the server matching preview. It filters only by Resource
// Type (and an optional caller predicate for renderer-only ranking); it never
// filters by profile, account or region.
type Ranking struct {
	// Matched holds every matching Definition ordered by key.
	Matched []Ranked
	// Top holds the Definitions that share the highest score, ordered by key.
	// One entry is a unique winner; several are a tie; none is no match.
	Top []Ranked
}

// Rank evaluates the five-field criteria of defs of one Resource Type against
// ctx. include (when non-nil) narrows the candidate set before scoring.
func Rank(defs []Definition, resourceType string, ctx MatchContext, include func(Definition) bool) Ranking {
	ordered := append([]Definition(nil), defs...)
	SortDefinitions(ordered)
	out := Ranking{}
	best := -1
	for _, def := range ordered {
		if def.ResourceTypeKey != resourceType || (include != nil && !include(def)) {
			continue
		}
		criterion, score, ok := def.BestCriterion(ctx)
		if !ok {
			continue
		}
		ranked := Ranked{Definition: def, Criterion: criterion, Score: score}
		out.Matched = append(out.Matched, ranked)
		switch {
		case score > best:
			best, out.Top = score, []Ranked{ranked}
		case score == best:
			out.Top = append(out.Top, ranked)
		}
	}
	sort.SliceStable(out.Top, func(i, j int) bool { return out.Top[i].Definition.Key < out.Top[j].Definition.Key })
	return out
}

var awsRegionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$`)

// ValidAWSRegion reports whether text is shaped like an AWS region name. It is
// a format check only: availability of a region is the provider's concern and
// never an account allowlist (UC-03 V02-03).
func ValidAWSRegion(text string) bool {
	return len(text) <= 32 && awsRegionPattern.MatchString(text)
}

// DefinitionRegion returns the literal AWS region of a Definition
// (`driverInputs.values.variables.region`) and whether it is a non-empty string.
func (d Definition) DefinitionRegion() (string, bool) {
	region, ok := d.Variables()["region"].(string)
	return region, ok && region != ""
}
