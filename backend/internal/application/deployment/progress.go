package deployment

import (
	"orchestrator/internal/domain/progress"
	"orchestrator/internal/ports/execution"
	"sort"
)

// Only counts escape rendering; never object bodies, names or secret bindings.
func renderProgressSummary(manifests []execution.Manifest) *progress.RenderSummary {
	counts := map[string]int{}
	for _, m := range manifests {
		counts[progress.ObjectKind(m.Kind)]++
	}
	result := &progress.RenderSummary{Objects: len(manifests), Kinds: []progress.ObjectCount{}}
	for kind, count := range counts {
		result.Kinds = append(result.Kinds, progress.ObjectCount{Kind: kind, Count: count})
	}
	sort.Slice(result.Kinds, func(i, j int) bool { return result.Kinds[i].Kind < result.Kinds[j].Kind })
	return result
}
