package deployment

import (
	"sort"

	domain "orchestrator/internal/domain/deployment"
)

// Blocked-dependency codes a view may show for a failed provider (ADR-017).
// A Terraform provider reports its persisted safe attempt code; a provider
// rejected before its module ran (missing or mistyped output, secret input,
// unusable target) has no attempt and reports RejectedBeforeRun.
const RejectedBeforeRun = "REJECTED_BEFORE_RUN"

// BlockedView says why one consumer did not run: the provider it depends on
// failed, the graph edge reason that makes it a provider, and a safe code for
// the provider's failure. It carries descriptors and codes only: never an
// error text, output value or secret reference.
type BlockedView struct {
	Consumer string `json:"consumer"`
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
	Code     string `json:"code"`
}

type graphEdge struct{ consumer, provider, reason string }

// deriveBlocked computes the consumers held back by a provider that FAILED in
// this Deployment. It is derived from the persisted plan graph and progress, so
// it stays truthful after a restart and needs no extra stored state. A consumer
// that itself reached READY was not held back.
func deriveBlocked(graph any, resources []ResourceView, attempts []domain.ResourceAttempt) []BlockedView {
	edges := edgesOf(graph)
	if len(edges) == 0 {
		return nil
	}
	status := map[string]string{}
	for _, r := range resources {
		status[r.Descriptor] = r.Status
	}
	code := map[string]string{}
	for _, persisted := range attempts {
		// Persisted text is untrusted even when its field is named SafeErrorCode.
		// Use the same allowlisted projection as ResourceProgress before it can
		// enter the public blocked-dependency view.
		a := persisted.Safe()
		if a.Status == "FAILED" || a.Status == "NEEDS_ATTENTION" || a.Status == "INTERRUPTED" {
			if a.SafeErrorCode != "" {
				code[a.Descriptor] = a.SafeErrorCode
			} else if code[a.Descriptor] == "" {
				code[a.Descriptor] = "FAILED"
			}
		}
	}
	var out []BlockedView
	for _, e := range edges {
		if status[e.consumer] == string(domain.ResourceReady) || status[e.provider] != string(domain.ResourceFailed) {
			continue
		}
		c := code[e.provider]
		if c == "" {
			c = RejectedBeforeRun
		}
		out = append(out, BlockedView{Consumer: e.consumer, Provider: e.provider, Reason: e.reason, Code: c})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Consumer != out[j].Consumer {
			return out[i].Consumer < out[j].Consumer
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func edgesOf(graph any) []graphEdge {
	m, ok := graph.(map[string]any)
	if !ok {
		return nil
	}
	list, _ := m["edges"].([]any)
	text := func(v map[string]any, key string) string { s, _ := v[key].(string); return s }
	var out []graphEdge
	for _, item := range list {
		if e, ok := item.(map[string]any); ok {
			out = append(out, graphEdge{consumer: text(e, "consumer"), provider: text(e, "provider"), reason: text(e, "reason")})
		}
	}
	return out
}
