package deployment

import (
	"encoding/json"
	"orchestrator/internal/ports/execution"
	"strings"
	"testing"
)

func TestRenderProgressCountsObjectsWithoutTheirContents(t *testing.T) {
	got := renderProgressSummary([]execution.Manifest{
		{Kind: "Deployment", Name: "PRIVATE", Object: map[string]any{"password": "PRIVATE"}},
		{Kind: "Service"}, {Kind: "Service"}, {Kind: "Secret", Secret: true},
		{Kind: "PRIVATE"},
	})
	raw, _ := json.Marshal(got)
	if got.Objects != 5 || len(got.Kinds) != 4 || strings.Contains(string(raw), "PRIVATE") {
		t.Fatalf("unsafe or incomplete summary: %s", raw)
	}
	if got.Kinds[3].Kind != "Service" || got.Kinds[3].Count != 2 {
		t.Fatalf("incorrect counts: %+v", got)
	}
	if empty := renderProgressSummary(nil); empty.Objects != 0 || empty.Kinds == nil {
		t.Fatal("zero render is not explicit")
	}
}
