package deployment

import (
	"encoding/json"
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
)

func TestDeltaDocument_EmptyBranchesAreOmitted(t *testing.T) {
	doc := DeltaDocument{Modules: &ModuleDelta{Add: map[string]environment.Module{}}, Shared: []JSONPatchOperation{}}.Normalized()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != "{}" {
		t.Fatalf("expected {}, got %s", b)
	}
}

func TestJSONPatchOperation_ValueIsWrittenOnlyForAddAndReplace(t *testing.T) {
	cases := map[string]JSONPatchOperation{
		`{"op":"remove","path":"/a"}`:            {Op: PatchRemove, Path: "/a", Value: "ignored"},
		`{"op":"add","path":"/a","value":null}`:  {Op: PatchAdd, Path: "/a"},
		`{"op":"replace","path":"/a","value":0}`: {Op: PatchReplace, Path: "/a", Value: 0},
	}
	for want, op := range cases {
		b, err := json.Marshal(op)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(b) != want {
			t.Fatalf("got %s, want %s", b, want)
		}
	}
}

func TestDeltaDocument_ValidateRejectsAmbiguousDocuments(t *testing.T) {
	cases := map[string]DeltaDocument{
		"add and remove same module": {Modules: &ModuleDelta{Add: map[string]environment.Module{"api": {}}, Remove: []string{"api"}}},
		"unsorted remove":            {Modules: &ModuleDelta{Remove: []string{"b", "a"}}},
		"empty update list":          {Modules: &ModuleDelta{Update: map[string][]JSONPatchOperation{"api": {}}}},
		"unsupported op":             {Shared: []JSONPatchOperation{{Op: "move", Path: "/a"}}},
		"relative path without /":    {Shared: []JSONPatchOperation{{Op: PatchRemove, Path: "a"}}},
	}
	for name, doc := range cases {
		if err := doc.Validate(); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func TestDeploymentDeltaSnapshot_HashIsDeterministicAndSurvivesReload(t *testing.T) {
	doc := DeltaDocument{
		Modules: &ModuleDelta{Update: map[string][]JSONPatchOperation{
			"api": {{Op: PatchReplace, Path: "/spec/replicas", Value: 2}},
		}},
		Shared: []JSONPatchOperation{{Op: PatchAdd, Path: "/db", Value: map[string]any{"type": "postgres", "class": "default"}}},
	}
	meta := DeltaSnapshotMetadata{ActorRef: "dev", Action: ActionUpdate, WorkloadID: "api"}
	at := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	first, err := NewDeploymentDeltaSnapshot("snap-1", "app", doc, meta, at)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	second, err := NewDeploymentDeltaSnapshot("snap-2", "app", doc, meta, at)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if first.DocumentHash == "" || first.DocumentHash != second.DocumentHash {
		t.Fatalf("document hash must be deterministic: %q vs %q", first.DocumentHash, second.DocumentHash)
	}
	b, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reloaded DeploymentDeltaSnapshot
	if err := json.Unmarshal(b, &reloaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("reloaded snapshot must validate: %v", err)
	}
	reloaded.Document.Shared = nil
	if err := reloaded.Validate(); err == nil {
		t.Fatal("a snapshot whose content no longer matches its hash must fail validation")
	}
}

func TestNewDeploymentDeltaSnapshot_RequiresIdentity(t *testing.T) {
	if _, err := NewDeploymentDeltaSnapshot("", "app", DeltaDocument{}, DeltaSnapshotMetadata{}, time.Time{}); err == nil {
		t.Fatal("expected an id error")
	}
	if _, err := NewDeploymentDeltaSnapshot("id", "", DeltaDocument{}, DeltaSnapshotMetadata{}, time.Time{}); err == nil {
		t.Fatal("expected an application key error")
	}
}
