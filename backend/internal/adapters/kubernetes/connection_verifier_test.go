package kubernetes_test

import (
	"context"
	"os"
	"testing"

	"orchestrator/internal/adapters/kubernetes"
)

func TestKindConnectionVerifierReadOnly(t *testing.T) {
	if os.Getenv("ORCH_KIND_VERIFY") != "1" {
		t.Skip("set ORCH_KIND_VERIFY=1 for read-only kind verification")
	}
	verified, err := (kubernetes.ConnectionVerifier{}).Verify(context.Background(), "kind-idp-internal")
	if err != nil {
		t.Fatal(err)
	}
	if verified.Endpoint == "" || verified.Version == "" {
		t.Fatalf("incomplete verification: %#v", verified)
	}
}
