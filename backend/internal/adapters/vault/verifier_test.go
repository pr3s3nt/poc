package vault

import (
	"context"
	"errors"
	"strings"
	"testing"

	configport "orchestrator/internal/ports/configuration"
)

func verifyAgainst(fv *fakeVault, token string) (map[string]any, error) {
	return Verifier{}.Verify(context.Background(), configport.VerifyRequest{BackendAddress: fv.server.URL, Mount: "kv", AuthMount: "kubernetes", Token: token})
}

func TestVerifierProvesKV2CapabilitiesAndRemovesItsProbe(t *testing.T) {
	fv := newFakeVault(t, "good")
	result, err := verifyAgainst(fv, "good")
	if err != nil {
		t.Fatal(err)
	}
	if result["verified"] != true || result["kvVersion"] != float64(2) || result["probe"] != "REMOVED" || result["kubernetesAuth"] != "CONFIGURED" {
		t.Fatalf("result: %+v", result)
	}
	if fv.valueCount() != 0 || fv.writes != 1 {
		t.Fatalf("probe was not written exactly once and removed: values=%d writes=%d", fv.valueCount(), fv.writes)
	}
	for _, text := range []string{"good"} {
		raw := strings.Join(fv.requests, "\n")
		if strings.Contains(raw, text) {
			t.Fatal("token must travel only in a header")
		}
	}
}

func TestVerifierRejectsWithFixedCategories(t *testing.T) {
	fv := newFakeVault(t, "good")
	if _, err := verifyAgainst(fv, "wrong"); !errors.Is(err, configport.ErrTokenRejected) {
		t.Fatalf("bad token: %v", err)
	}
	fv.notKV2 = true
	if _, err := verifyAgainst(fv, "good"); !errors.Is(err, configport.ErrNotKV2) {
		t.Fatalf("kv1: %v", err)
	}
	fv.notKV2 = false
	fv.capabilities = map[string][]string{"sys/policies": {"read"}}
	_, err := verifyAgainst(fv, "good")
	if !errors.Is(err, configport.ErrCapability) || !strings.Contains(err.Error(), "workload policies") {
		t.Fatalf("missing policy capability: %v", err)
	}
	fv.capabilities = map[string][]string{"/role/": {"read"}}
	if _, err := verifyAgainst(fv, "good"); !errors.Is(err, configport.ErrCapability) || !strings.Contains(err.Error(), "Kubernetes auth roles") {
		t.Fatalf("missing role capability: %v", err)
	}
	fv.capabilities = map[string][]string{"/metadata/": {"read"}}
	if _, err := verifyAgainst(fv, "good"); !errors.Is(err, configport.ErrCapability) {
		t.Fatalf("missing delete capability: %v", err)
	}
	if fv.writes != 0 {
		t.Fatal("a failed capability check must not write a probe")
	}
	_, err = Verifier{}.Verify(context.Background(), configport.VerifyRequest{BackendAddress: "http://127.0.0.1:1", Mount: "kv", AuthMount: "kubernetes", Token: "x"})
	if !errors.Is(err, configport.ErrUnreachable) {
		t.Fatalf("unreachable: %v", err)
	}
}

func TestVerifierFailsWhenProbeCleanupFails(t *testing.T) {
	fv := newFakeVault(t, "good")
	fv.noDelete = true
	_, err := verifyAgainst(fv, "good")
	if !errors.Is(err, configport.ErrProbeCleanup) {
		t.Fatalf("cleanup failure must fail registration: %v", err)
	}
}

func TestVerifierRecordsMissingWorkloadAuthWithoutClaimingIt(t *testing.T) {
	fv := newFakeVault(t, "good")
	fv.authMissing = true
	result, err := verifyAgainst(fv, "good")
	if err != nil || result["kubernetesAuth"] != "ABSENT" {
		t.Fatalf("auth presence must be reported honestly: %+v %v", result, err)
	}
}

func TestVerifierProvesRemovalOfAProbeWhoseWriteReportedFailure(t *testing.T) {
	fv := newFakeVault(t, "good")
	fv.failWrite = true
	if _, err := verifyAgainst(fv, "good"); !errors.Is(err, configport.ErrProbe) || fv.valueCount() != 0 {
		t.Fatalf("a failed write must clean its probe: %v values=%d", err, fv.valueCount())
	}
	fv.noDelete = true
	if _, err := verifyAgainst(fv, "good"); !errors.Is(err, configport.ErrProbeCleanup) {
		t.Fatalf("a failed write with failed cleanup must be a visible cleanup failure: %v", err)
	}
}
