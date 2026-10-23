package deployment

import (
	"sort"
	"time"

	domain "orchestrator/internal/domain/deployment"
)

// projectNodeProgress uses only the plan and exact persisted run records. In
// particular APPLIED/OUTPUTS never mean readiness, and a later failed attempt
// cannot be hidden by an earlier successful attempt.
func projectNodeProgress(graph, review any, resources []ResourceView, attempts []domain.ResourceAttempt, workloads []domain.WorkloadSnapshot, run domain.Status, observed time.Time) []NodeProgress {
	root, _ := graph.(map[string]any)
	raw, _ := root["nodes"].([]any)
	if raw == nil {
		return []NodeProgress{}
	}
	resourceByID := map[string]ResourceView{}
	for _, r := range resources {
		resourceByID[r.Descriptor] = r
	}
	retained := map[string]bool{}
	if m, ok := review.(map[string]any); ok {
		if nodes, ok := m["nodes"].([]any); ok {
			for _, item := range nodes {
				if n, ok := item.(map[string]any); ok {
					id, _ := n["id"].(string)
					intent, _ := n["intent"].(string)
					retained[id] = intent == "RETAIN" || intent == "BIND"
				}
			}
		}
	}
	latest := map[string]domain.ResourceAttempt{}
	for _, a := range attempts {
		prior, ok := latest[a.Descriptor]
		if !ok || a.Number > prior.Number || (a.Number == prior.Number && a.UpdatedAt.After(prior.UpdatedAt)) {
			latest[a.Descriptor] = a.Safe()
		}
	}
	workByID := map[string]domain.WorkloadSnapshot{}
	for _, w := range workloads {
		workByID[w.WorkloadID] = w
	}
	deps := map[string][]string{}
	for _, e := range edgesOf(graph) {
		deps[e.consumer] = append(deps[e.consumer], e.provider)
	}
	out := make([]NodeProgress, 0, len(raw))
	states := map[string]string{}
	unobserved := map[string]bool{}
	for _, item := range raw {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := node["descriptor"].(string)
		if id == "" {
			continue
		}
		kind, _ := node["kind"].(string)
		p := NodeProgress{NodeID: id, State: "UNKNOWN", BlockingProviderIDs: []string{}, ObservedAt: observed}
		unobserved[id] = true
		if kind == "workload" {
			wid, _ := node["workloadId"].(string)
			if w, ok := workByID[wid]; ok {
				unobserved[id] = false
				p.Status = string(w.Status)
				t := w.ObservedAt
				p.UpdatedAt = &t
				switch w.Status {
				case domain.InstanceReady:
					p.State = "READY"
					p.FinishedAt = &t
				case domain.InstanceFailed:
					p.State = "FAILED"
					p.FinishedAt = &t
				case domain.InstanceApplying, domain.InstanceRemoving:
					p.State = "PROVISIONING"
				default:
					p.State = "UNKNOWN"
				}
			}
		} else {
			if r, ok := resourceByID[id]; ok {
				unobserved[id] = false
				p.Status = r.Status
				if !r.StartedAt.IsZero() {
					start := r.StartedAt
					p.StartedAt = &start
				}
				p.FinishedAt = r.FinishedAt
				switch r.Status {
				case string(domain.ResourceReady):
					p.State = "READY"
				case string(domain.ResourceFailed):
					p.State = "FAILED"
				case string(domain.ResourceRunning):
					p.State = "PROVISIONING"
				case string(domain.ResourcePending):
					p.State = "PENDING"
				default:
					p.State = "UNKNOWN"
				}
			}
			if a, ok := latest[id]; ok {
				unobserved[id] = false
				p.Attempt, p.Stage, p.Status, p.SafeErrorCode = a.Number, a.Stage, a.Status, a.SafeErrorCode
				start, update := a.StartedAt, a.UpdatedAt
				p.StartedAt, p.UpdatedAt = &start, &update
				switch a.Status {
				case "FAILED", "INTERRUPTED", "NEEDS_ATTENTION":
					p.State = "FAILED"
					p.FinishedAt = &update
				case "RUNNING":
					if a.Stage == "READINESS" || a.Stage == "CHECKING" {
						p.State = "CHECKING"
					} else {
						p.State = "PROVISIONING"
					}
				case "SUCCEEDED":
					if a.Stage == "READY" && p.State == "READY" {
						p.FinishedAt = &update
					} else if p.State != "READY" {
						p.State = "UNKNOWN"
					}
				default:
					p.State = "UNKNOWN"
				}
			}
			if p.State == "READY" && retained[id] {
				if _, hasAttempt := latest[id]; !hasAttempt {
					p.State = "RETAINED"
				}
			}
		}
		states[id] = p.State
		out = append(out, p)
	}
	// Dependencies can be listed after consumers. Resolve failed chains to a
	// fixed point; keep explicit observations above derived waiting/blocked.
	for changed := true; changed; {
		changed = false
		for i := range out {
			p := &out[i]
			if p.State != "PENDING" && p.State != "WAITING" && p.State != "BLOCKED" && !(p.State == "UNKNOWN" && unobserved[p.NodeID]) {
				continue
			}
			providers := []string{}
			failed := false
			for _, provider := range deps[p.NodeID] {
				state, known := states[provider]
				if !known || (state != "READY" && state != "RETAINED") {
					providers = append(providers, provider)
					if state == "FAILED" || state == "BLOCKED" {
						failed = true
					}
				}
			}
			sort.Strings(providers)
			next := p.State
			if failed {
				next = "BLOCKED"
			} else if len(providers) > 0 {
				if run != domain.StatusSucceeded && run != domain.StatusFailed {
					next = "WAITING"
				} else {
					next = "UNKNOWN"
				}
			} else if unobserved[p.NodeID] {
				next = "UNKNOWN"
			}
			if p.State != next {
				p.State = next
				states[p.NodeID] = next
				changed = true
			}
			p.BlockingProviderIDs = providers
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}
