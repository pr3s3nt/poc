package deployment

import (
	"testing"
	"time"

	domain "orchestrator/internal/domain/deployment"
)

func TestNodeProgressUsesReadinessAndLatestFailure(t *testing.T) {
	now := time.Now().UTC()
	graph := map[string]any{"nodes": []any{map[string]any{"descriptor": "db.postgres#environments.app.staging", "kind": "resource"}, map[string]any{"descriptor": "cache.redis#environments.app.staging", "kind": "resource"}, map[string]any{"descriptor": "workload.api#environments.app.staging", "kind": "workload", "workloadId": "api"}, map[string]any{"descriptor": "workload.web#environments.app.staging", "kind": "workload", "workloadId": "web"}}, "edges": []any{map[string]any{"consumer": "workload.api#environments.app.staging", "provider": "cache.redis#environments.app.staging", "reason": "resource-reference"}, map[string]any{"consumer": "workload.web#environments.app.staging", "provider": "workload.api#environments.app.staging", "reason": "workload-placeholder"}}}
	attempts := []domain.ResourceAttempt{{Descriptor: "db.postgres#environments.app.staging", Number: 1, Status: "RUNNING", Stage: "APPLIED", StartedAt: now, UpdatedAt: now}, {Descriptor: "cache.redis#environments.app.staging", Number: 1, Status: "SUCCEEDED", Stage: "READY", StartedAt: now, UpdatedAt: now}, {Descriptor: "cache.redis#environments.app.staging", Number: 2, Status: "FAILED", Stage: "READINESS", SafeErrorCode: "NOT_READY", StartedAt: now, UpdatedAt: now}}
	resources := []ResourceView{{Descriptor: "db.postgres#environments.app.staging", Status: "RUNNING"}, {Descriptor: "cache.redis#environments.app.staging", Status: "READY"}}
	out := projectNodeProgress(graph, nil, resources, attempts, nil, domain.StatusProvisioning, now)
	states := map[string]NodeProgress{}
	for _, p := range out {
		states[p.NodeID] = p
	}
	if states["db.postgres#environments.app.staging"].State != "PROVISIONING" {
		t.Fatal("APPLIED was treated as READY")
	}
	if states["cache.redis#environments.app.staging"].State != "FAILED" || states["cache.redis#environments.app.staging"].Attempt != 2 {
		t.Fatal("prior success masked current failure")
	}
	if states["workload.api#environments.app.staging"].State != "BLOCKED" || states["workload.web#environments.app.staging"].State != "BLOCKED" {
		t.Fatalf("failed dependency did not block transitive consumers: %#v", states)
	}
	if states["workload.web#environments.app.staging"].BlockingProviderIDs[0] != "workload.api#environments.app.staging" {
		t.Fatal("wrong blocking identity")
	}
}

func TestNodeProgressMissingFactsAreUnknownAfterTerminal(t *testing.T) {
	graph := map[string]any{"nodes": []any{map[string]any{"descriptor": "workload.api#environments.app.staging", "kind": "workload", "workloadId": "api"}}}
	out := projectNodeProgress(graph, nil, nil, nil, nil, domain.StatusSucceeded, time.Now())
	if len(out) != 1 || out[0].State != "UNKNOWN" {
		t.Fatalf("missing history presented as success: %#v", out)
	}
}

func TestNodeProgressMissingFactsAreUnknownWhileRunning(t *testing.T) {
	graph := map[string]any{"nodes": []any{map[string]any{"descriptor": "workload.api#environments.app.staging", "kind": "workload", "workloadId": "api"}}}
	out := projectNodeProgress(graph, nil, nil, nil, nil, domain.StatusDeploying, time.Now())
	if len(out) != 1 || out[0].State != "UNKNOWN" {
		t.Fatalf("absent facts presented as scheduled work: %#v", out)
	}
}

func TestNodeProgressRetainRequiresReadyRecordAndPinnedIntent(t *testing.T) {
	now := time.Now().UTC()
	graph := map[string]any{"nodes": []any{map[string]any{"descriptor": "cluster.internal#environments.app.staging", "kind": "resource"}}}
	review := map[string]any{"nodes": []any{map[string]any{"id": "cluster.internal#environments.app.staging", "intent": "RETAIN"}}}
	ready := []ResourceView{{Descriptor: "cluster.internal#environments.app.staging", Status: "READY"}}
	if out := projectNodeProgress(graph, review, ready, nil, nil, domain.StatusSucceeded, now); len(out) != 1 || out[0].State != "RETAINED" {
		t.Fatalf("verified retain lost: %#v", out)
	}
	if out := projectNodeProgress(graph, review, nil, nil, nil, domain.StatusSucceeded, now); len(out) != 1 || out[0].State != "UNKNOWN" {
		t.Fatalf("intent alone became retain: %#v", out)
	}
}
