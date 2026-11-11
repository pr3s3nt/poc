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
