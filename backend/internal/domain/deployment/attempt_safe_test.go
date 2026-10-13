package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	leakCred = "AKIAPRIVATECREDENTIAL"
	leakPath = "/var/lib/orchestrator/recovery/PRIVATE"
)

func goodAttempt() ResourceAttempt {
	return ResourceAttempt{Descriptor: "redis.in-cluster#shared.cache", ScopeType: "SHARED", Number: 1, Action: "UPDATE", Ownership: "PREEXISTING", Stage: "APPLYING", Status: "FAILED", SafeErrorCode: "TIMEOUT", ModuleID: "redis-k8s", ModuleRevision: "v3-2026.10.13"}
}

// A persisted value is free text to the database; the API boundary keeps known
// codes and replaces everything else with a generic marker.
func TestResourceAttemptSafeKeepsKnownValues(t *testing.T) {
	if got := goodAttempt().Safe(); got != goodAttempt() {
		t.Fatalf("known values changed: %+v", got)
	}
}

func TestResourceAttemptSafeNeverLeaksUnknownPersistedText(t *testing.T) {
	bad := ResourceAttempt{Descriptor: "x#" + leakPath, ScopeType: leakCred, Number: 2, Action: leakPath, Ownership: leakCred, Stage: "APPLYING " + leakCred, Status: leakPath, SafeErrorCode: "AUTH:" + leakCred, ModuleID: leakPath, ModuleRevision: leakCred + " " + leakPath}
	got := bad.Safe()
	raw, _ := json.Marshal(got)
	for _, secret := range []string{leakCred, leakPath, "PRIVATE", "/var/"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("leaks %q: %s", secret, raw)
		}
	}
	if got.Stage != AttemptUnknown || got.Status != AttemptUnknown || got.Action != AttemptUnknown || got.Ownership != AttemptUnknown || got.ScopeType != AttemptUnknown || got.SafeErrorCode != AttemptUnknownError || got.Descriptor != AttemptUnknown || got.ModuleID != AttemptUnknown || got.ModuleRevision != AttemptUnknown || got.Number != 2 {
		t.Fatalf("generic markers expected: %+v", got)
	}
	empty := goodAttempt()
	empty.SafeErrorCode = ""
	if empty.Safe().SafeErrorCode != "" {
		t.Fatal("no error stays no error")
	}
}
