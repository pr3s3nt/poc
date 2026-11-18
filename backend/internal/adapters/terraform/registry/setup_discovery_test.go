package registry

import (
	"context"
	"encoding/json"
	access "orchestrator/internal/ports/gitaccess"
	"os"
	"strings"
	"testing"
)

func TestSetupDiscoveryRecognizesContentAndPinsMovedBranch(t *testing.T) {
	server := newGitServer(t)
	first := server.commit(t, map[string]string{"catalog/arbitrary/one.yaml": "key: queue\ninputs: []\noutputs: []\n", "catalog/two.json": `{"key":"queue-definition","resourceType":"queue","driverType":"template"}`, "catalog/notes.yaml": "description: not a product document\n"}, nil, nil)
	g, err := NewGitCLI("git", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(server.caFile)
	if err != nil {
		t.Fatal(err)
	}
	g.Access = draftAccess{repository: server.url, value: access.Access{CAPEM: string(ca)}}
	read := func(ref string) map[string]any {
		t.Helper()
		v, err := g.DiscoverSetup(context.Background(), "org", server.url, ref, "catalog")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(v)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	result := read("main")
	if result["revision"] != first || len(result["documents"].([]any)) != 2 || result["skipped"] != float64(1) {
		t.Fatal(result)
	}
	server.commit(t, map[string]string{"catalog/arbitrary/one.yaml": "key: changed\noutputs: []\n"}, nil, nil)
	pinned := read(first)
	if pinned["revision"] != first || !strings.Contains(string(mustJSON(pinned)), "queue") {
		t.Fatal("pin moved")
	}
	if _, err := g.DiscoverSetup(context.Background(), "org", server.url, "main", "../catalog"); err == nil {
		t.Fatal("accepted traversal")
	}
}
func mustJSON(v any) []byte { data, _ := json.Marshal(v); return data }
