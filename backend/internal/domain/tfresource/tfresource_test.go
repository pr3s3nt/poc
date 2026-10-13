package tfresource

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/resource"
)

func TestStateKeyIsStablePerIdentityAndDistinctAcrossScope(t *testing.T) {
	base := Identity{OrganizationID: "o", ApplicationID: "a", EnvironmentID: "e", TargetGeneration: 0, Descriptor: "redis.in-cluster#cache", Scope: resource.Scope{Type: resource.ScopeShared, ID: "cache"}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	if base.StateKey() != base.StateKey() || !ValidStateKey(base.StateKey()) {
		t.Fatal("state key unstable or malformed")
	}
	variants := map[string]Identity{}
	v := base
	v.OrganizationID = "o2"
	variants["org"] = v
	v = base
	v.EnvironmentID = "e2"
	variants["environment"] = v
	v = base
	v.TargetGeneration = 1
	variants["generation"] = v
	v = base
	v.Scope.Type = resource.ScopeWorkload
	variants["scope type"] = v
	v = base
	// Component boundaries are length-prefixed: shifting text between parts differs.
	v.ApplicationID, v.EnvironmentID = "ae", ""
	variants["boundary"] = v
	for name, id := range variants {
		if id.StateKey() == base.StateKey() {
			t.Errorf("%s aliases the same state", name)
		}
	}
	bad := base
	bad.Descriptor = "Redis.in-cluster#cache "
	if bad.Validate() == nil {
		t.Error("non-canonical descriptor accepted")
	}
}

func TestClassifyKeepsRetryOwnershipAndRefusesUnprovenCreation(t *testing.T) {
	created := CreatedByRun
	if err := Classify(CreatedByRun, nil, true, false); err != nil {
		t.Fatalf("new instance of this run: %v", err)
	}
	if err := Classify(CreatedByRun, nil, false, false); !errors.Is(err, ErrInvalid) {
		t.Error("instance of another run classified CREATED_BY_RUN")
	}
	if err := Classify(CreatedByRun, nil, true, true); !errors.Is(err, ErrInvalid) {
		t.Error("prior state classified CREATED_BY_RUN")
	}
	if err := Classify(UpdatedByRun, &created, true, true); !errors.Is(err, ErrInvalid) {
		t.Error("retry changed classification")
	}
	if err := Classify(CreatedByRun, &created, true, true); err != nil {
		t.Errorf("retry kept classification: %v", err)
	}
}

func TestDependencyHoldsUntilConsumerDestroyed(t *testing.T) {
	for _, s := range []InstanceStatus{InstanceReady, InstanceFailed, InstanceInterrupted, InstanceNeedsAttention, InstanceDestroying, InstanceUnreferenced} {
		if !(Dependency{ConsumerStatus: s}).Holds() {
			t.Errorf("%s consumer released its provider", s)
		}
	}
	if (Dependency{ConsumerStatus: InstanceDestroyed}).Holds() {
		t.Error("destroyed consumer still holds")
	}
}

func TestCleanupRequestIsExactExplicitSelection(t *testing.T) {
	item := func(id string) CleanupItem { return CleanupItem{InstanceID: id, Version: 1} }
	valid := CleanupRequest{Mode: CleanupUserConfirmed, ActorRef: "u", RootInstanceID: "r", CandidateIDs: []string{"d1", "d2"}, RequestHash: "h", ConfirmationName: "redis.in-cluster#cache",
		Scope: ConfirmedScope{SelectedResourceIDs: []string{"r", "d1"}, Items: []CleanupItem{item("r"), item("d1")}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(r *CleanupRequest){
		"root missing": func(r *CleanupRequest) {
			r.Scope.SelectedResourceIDs = []string{"d1"}
			r.Scope.Items = []CleanupItem{item("d1")}
		},
		"duplicate": func(r *CleanupRequest) { r.Scope.SelectedResourceIDs = []string{"r", "r"} },
		"outside candidates": func(r *CleanupRequest) {
			r.Scope.SelectedResourceIDs = []string{"r", "x"}
			r.Scope.Items = []CleanupItem{item("r"), item("x")}
		},
		"extra item added":   func(r *CleanupRequest) { r.Scope.Items = append(r.Scope.Items, item("d2")) },
		"item not selected":  func(r *CleanupRequest) { r.Scope.Items = []CleanupItem{item("r"), item("d2")} },
		"no confirmation":    func(r *CleanupRequest) { r.ConfirmationName = "" },
		"run without run id": func(r *CleanupRequest) { r.Mode = CleanupRunCleanup },
		"unknown mode":       func(r *CleanupRequest) { r.Mode = "AUTO" },
	}
	for name, mutate := range cases {
		r := valid
		r.Scope.SelectedResourceIDs = append([]string(nil), valid.Scope.SelectedResourceIDs...)
		r.Scope.Items = append([]CleanupItem(nil), valid.Scope.Items...)
		mutate(&r)
		if err := r.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: accepted", name)
		}
	}
	if !SameSelection([]string{"a", "b"}, []string{"b", "a"}) || SameSelection([]string{"a"}, []string{"a", "b"}) {
		t.Error("selection comparison")
	}
}
