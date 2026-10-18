package planning

import (
	"fmt"
	"sort"
)

// schedule validates the COMPLETE graph, workload nodes included, and returns
// provider-first execution batches of the resource nodes plus, per resource,
// the workloads it transitively depends on (UC-06 MS-07/BR-08, UC-08 BR-01).
//
// UC-08 executes resources only, so the batches list resources. A resource that
// depends on a workload (an is_dependent companion of the workload) is listed in
// afterWorkloads: lifecycle orchestration (UC-06) provisions it only after that
// workload has been applied. A cycle anywhere, including through a workload or
// a self edge, fails here, before any side effect.
func schedule(g Graph) (batches [][]string, afterWorkloads map[string][]string, err error) {
	known := map[string]Node{}
	for _, n := range g.Nodes {
		known[n.Descriptor] = n
	}
	providers := map[string]map[string]bool{}
	for descriptor := range known {
		providers[descriptor] = map[string]bool{}
	}
	for _, e := range g.Edges {
		if _, ok := known[e.Consumer]; !ok {
			continue
		}
		if _, ok := known[e.Provider]; !ok {
			continue
		}
		if e.Consumer == e.Provider {
			return nil, nil, fmt.Errorf("planning: %s depends on itself", e.Consumer)
		}
		providers[e.Consumer][e.Provider] = true
	}

	layer := map[string]int{}
	remaining := len(known)
	for remaining > 0 {
		var ready []string
		for descriptor := range known {
			if _, done := layer[descriptor]; done {
				continue
			}
			depth, ok := 0, true
			for provider := range providers[descriptor] {
				l, done := layer[provider]
				if !done {
					ok = false
					break
				}
				if l+1 > depth {
					depth = l + 1
				}
			}
			if ok {
				layer[descriptor] = depth
				ready = append(ready, descriptor)
			}
		}
		if len(ready) == 0 {
			var stuck []string
			for descriptor := range known {
				if _, done := layer[descriptor]; !done {
					stuck = append(stuck, descriptor)
				}
			}
			sort.Strings(stuck)
			return nil, nil, fmt.Errorf("planning: graph has a cycle involving %v", stuck)
		}
		remaining -= len(ready)
	}

	byLayer := map[int][]string{}
	maxLayer := -1
	for descriptor, l := range layer {
		if known[descriptor].Kind != NodeResource {
			continue
		}
		byLayer[l] = append(byLayer[l], descriptor)
		if l > maxLayer {
			maxLayer = l
		}
	}
	batches = [][]string{}
	for l := 0; l <= maxLayer; l++ {
		if batch := byLayer[l]; len(batch) > 0 {
			sort.Strings(batch)
			batches = append(batches, batch)
		}
	}

	// Transitive workload ancestors of every resource.
	memo := map[string]map[string]bool{}
	var ancestors func(string) map[string]bool
	ancestors = func(descriptor string) map[string]bool {
		if got, ok := memo[descriptor]; ok {
			return got
		}
		out := map[string]bool{}
		for provider := range providers[descriptor] {
			if known[provider].Kind == NodeWorkload {
				out[provider] = true
			}
			for w := range ancestors(provider) {
				out[w] = true
			}
		}
		memo[descriptor] = out
		return out
	}
	for descriptor, n := range known {
		if n.Kind != NodeResource {
			continue
		}
		if set := ancestors(descriptor); len(set) > 0 {
			if afterWorkloads == nil {
				afterWorkloads = map[string][]string{}
			}
			list := make([]string, 0, len(set))
			for w := range set {
				list = append(list, w)
			}
			sort.Strings(list)
			afterWorkloads[descriptor] = list
		}
	}
	return batches, afterWorkloads, nil
}
