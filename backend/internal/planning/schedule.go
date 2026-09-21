package planning

import (
	"fmt"
	"sort"
)

// scheduleBatches returns provider-first execution batches over the resource-only
// subgraph. Workload nodes are excluded: UC-08 executes resources only.
func scheduleBatches(g Graph) ([][]string, error) {
	resourceNodes := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Kind == NodeResource {
			resourceNodes[n.Descriptor] = true
		}
	}
	providers := map[string]map[string]bool{}
	consumers := map[string][]string{}
	for descriptor := range resourceNodes {
		providers[descriptor] = map[string]bool{}
	}
	for _, e := range g.Edges {
		if !resourceNodes[e.Consumer] || !resourceNodes[e.Provider] {
			continue
		}
		if providers[e.Consumer][e.Provider] {
			continue
		}
		providers[e.Consumer][e.Provider] = true
		consumers[e.Provider] = append(consumers[e.Provider], e.Consumer)
	}

	remaining := len(resourceNodes)
	var batches [][]string
	done := map[string]bool{}
	for remaining > 0 {
		var batch []string
		for descriptor := range resourceNodes {
			if done[descriptor] {
				continue
			}
			ready := true
			for provider := range providers[descriptor] {
				if !done[provider] {
					ready = false
					break
				}
			}
			if ready {
				batch = append(batch, descriptor)
			}
		}
		if len(batch) == 0 {
			var stuck []string
			for descriptor := range resourceNodes {
				if !done[descriptor] {
					stuck = append(stuck, descriptor)
				}
			}
			sort.Strings(stuck)
			return nil, fmt.Errorf("planning: resource graph has a cycle involving %v", stuck)
		}
		sort.Strings(batch)
		for _, descriptor := range batch {
			done[descriptor] = true
			remaining--
		}
		batches = append(batches, batch)
	}
	if batches == nil {
		batches = [][]string{}
	}
	return batches, nil
}
