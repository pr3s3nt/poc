package planning

import (
	"testing"

	"orchestrator/internal/domain/resource"
)

// TestDestroyedActiveResourceIsCreatedAgain: a verified-destroyed Active
// Resource is a tombstone, not an existing resource (UC-05 BR-10). The same
// descriptor classifies as NEW (CREATE intent), while every live status keeps
// classifying as existing.
func TestDestroyedActiveResourceIsCreatedAgain(t *testing.T) {
	const descriptor = "redis.in-cluster#shared.cache"
	d, err := resource.ParseDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	scope := resource.Scope{Type: resource.ScopeShared, ID: "shop.staging"}
	ctx := Context{OrganizationKey: "org"}
	graph := Graph{Nodes: []Node{{Kind: NodeResource, Descriptor: descriptor, Scope: scope}}}
	for _, status := range []resource.Status{resource.StatusReady, resource.StatusUnreferenced, resource.StatusFailed, resource.StatusProvisioning, resource.StatusDestroying} {
		got := classify(ctx, graph, []resource.ActiveResource{{OrganizationKey: "org", Descriptor: d, Scope: scope, Status: status}})
		if len(got.Existing) != 1 || len(got.New) != 0 {
			t.Fatalf("%s must stay existing: %+v", status, got)
		}
	}
	got := classify(ctx, graph, []resource.ActiveResource{{OrganizationKey: "org", Descriptor: d, Scope: scope, Status: resource.StatusDestroyed}})
	if len(got.New) != 1 || len(got.Existing) != 0 || got.New[0] != descriptor {
		t.Fatalf("a DESTROYED tombstone must be created again: %+v", got)
	}
	// And it is never marked UNREFERENCED again (only READY ones are).
	if len(got.Unreferenced) != 0 {
		t.Fatalf("unreferenced: %+v", got)
	}
}
