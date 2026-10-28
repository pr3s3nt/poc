package delivery

import (
	"errors"
	"strings"
	"testing"
)

func TestUnknownMutationCannotBeReplayedByLeaseTakeover(t *testing.T) {
	m := Mutation{ID: "intent", Scope: revision().Scope, RevisionID: "r", OperationID: "op", RequestID: "request", RequestDigest: strings.Repeat("a", 64), Action: Submit, Status: IntentPending, Owner: "worker", Fence: 1, Version: 1}
	running, err := m.Advance(IntentRunning, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = running.Advance(IntentAcknowledged, false); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal("acknowledged without external proof", err)
	}
	unknown, err := running.Advance(IntentUnknown, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = unknown.Advance(IntentRunning, false); !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal("replayed unknown external call", err)
	}
	done, err := unknown.Advance(IntentAcknowledged, true)
	if err != nil || done.Version != 4 {
		t.Fatal(done, err)
	}
	if _, err = done.Advance(IntentRunning, true); !errors.Is(err, ErrTransition) {
		t.Fatal("replayed acknowledged external call", err)
	}
	other := m
	other.RequestDigest = strings.Repeat("b", 64)
	if m.SameRequest(other) {
		t.Fatal("accepted idempotency key with a different body")
	}
	other = m
	other.Scope.OrganizationID = "foreign"
	if m.SameRequest(other) {
		t.Fatal("mixed tenant requests")
	}
}
