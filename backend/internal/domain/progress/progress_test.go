package progress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProgressProjectionDropsUntrustedText(t *testing.T) {
	s := Step{ID: "step", Kind: "READINESS", Status: "raw-secret", Reason: "password=private", Pods: []Pod{{Name: "pod", Image: "https://user:password@registry/image?token=private", Reason: "token=private"}}, Events: []Event{{Object: "pod", Reason: "credential=private"}}}
	got := s.Safe()
	raw, _ := json.Marshal(got)
	for _, v := range []string{"password", "private", "token=", "credential="} {
		if strings.Contains(string(raw), v) {
			t.Fatalf("unsafe field %s: %s", v, raw)
		}
	}
	if got.Status != "PENDING" || got.Reason != "UNKNOWN" || got.Pods[0].Image != "" {
		t.Fatalf("unexpected projection: %+v", got)
	}
}
func TestPersistedReadRetainsStructuredFacts(t *testing.T) {
	source := Record{Version: 1, Steps: []Step{{ID: "wait", Kind: "READINESS", Status: "RUNNING", Pods: []Pod{{Name: "api-1", Image: "ghcr.io/acme/api:v1", Reason: "ImagePullBackOff", Restarts: 2}}}}}
	raw, _ := json.Marshal(source)
	var value any
	_ = json.Unmarshal(raw, &value)
	got := Read(value)
	if len(got.Steps) != 1 || got.Steps[0].Pods[0].Reason != "ImagePullBackOff" {
		t.Fatalf("lost persisted facts: %+v", got)
	}
}

func TestProgressNeverExposesCredentialsEmbeddedInAnImageURL(t *testing.T) {
	for _, value := range []string{"https://user:PRIVATE@registry/image", "user:PRIVATE@registry/image"} {
		got := Step{Pods: []Pod{{Name: "pod", Image: value}}}.Safe()
		if got.Pods[0].Image != "" {
			t.Fatal("credential-bearing image exposed")
		}
	}
	valid := "ghcr.io/acme/image@sha256:" + strings.Repeat("a", 64)
	if got := (Step{Pods: []Pod{{Name: "pod", Image: valid}}}.Safe()).Pods[0].Image; got != valid {
		t.Fatal("valid immutable image reference hidden")
	}
	if Identity("https://user:PRIVATE@host/path") != "" {
		t.Fatal("credential-bearing URL accepted as an identity")
	}
}

func TestPreparationFactsAreBoundedSafeAndSurvivePersistence(t *testing.T) {
	zero := 0
	source := Step{SecretBindings: &zero, Rendered: &RenderSummary{Objects: 2, Kinds: []ObjectCount{{Kind: "Secret", Count: 1}, {Kind: "PRIVATE", Count: 1}}}}
	raw, _ := json.Marshal(Record{Steps: []Step{source}})
	var value any
	_ = json.Unmarshal(raw, &value)
	got := Read(value).Steps[0]
	if got.SecretBindings == nil || *got.SecretBindings != 0 || got.Rendered.Objects != 2 || got.Rendered.Kinds[1].Kind != "Other" {
		t.Fatalf("lost safe facts: %+v", got)
	}
	if strings.Contains(string(mustJSON(got)), "PRIVATE") {
		t.Fatal("untrusted kind exposed")
	}
	negative := -5
	got = (Step{SecretBindings: &negative, Rendered: &RenderSummary{Objects: 200000, Kinds: []ObjectCount{{Kind: "Secret", Count: -4}}}}).Safe()
	if *got.SecretBindings != 0 || got.Rendered.Objects != 100000 || got.Rendered.Kinds[0].Count != 0 {
		t.Fatal("unbounded facts")
	}
	if Read(nil).Steps == nil {
		t.Fatal("legacy record changed")
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
