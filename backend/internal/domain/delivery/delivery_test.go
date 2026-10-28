package delivery

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSelectionDoesNotEnableProviderForLegacyEnvironment(t *testing.T) {
	if got := (Selection{}).Effective(); got != (Selection{Mode: Direct}) {
		t.Fatal(got)
	}
	for _, s := range []Selection{{Mode: Direct, SelfHeal: true}, {Mode: Direct, BindingID: "b"}, {Mode: CD}, {Mode: CD, BindingID: "b", BindingVersion: -1}, {Mode: "FLEET"}} {
		if !errors.Is(s.Validate(), ErrInvalid) {
			t.Fatalf("accepted invalid selection: %+v", s)
		}
	}
}

func TestCapabilitiesDoNotInferSafeDeliveryFromProviderSelection(t *testing.T) {
	s := Selection{Mode: CD, BindingID: "fleet-a", BindingVersion: 1, SelfHeal: true}
	c := Capabilities{ContractVersion: "v1", ExactRevision: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true}
	if !errors.Is(c.ValidateSelection(s), ErrUnsupported) {
		t.Fatal("self heal accepted without verified support")
	}
	c.SelfHeal, c.DriftDetection = true, true
	if err := c.ValidateSelection(s); err != nil {
		t.Fatal(err)
	}
	c.GuardedPrune = false
	if !errors.Is(c.ValidateSelection(s), ErrUnsupported) {
		t.Fatal("accepted unsafe prune")
	}
}

func TestOccupiedEnvironmentRequiresTransferBothDirections(t *testing.T) {
	cd := Selection{Mode: CD, BindingID: "fleet-a", BindingVersion: 1}
	for _, pair := range [][2]Selection{{{}, cd}, {cd, {}}, {cd, {Mode: CD, BindingID: "fleet-b", BindingVersion: 1}}} {
		got, err := RequiresTransfer(pair[0], pair[1], true)
		if err != nil || !got {
			t.Fatalf("handoff omitted: %v %v", got, err)
		}
	}
	policy := cd
	policy.SelfHeal = true
	if got, err := RequiresTransfer(cd, policy, true); err != nil || got {
		t.Fatal("policy change transferred ownership", got, err)
	}
	if got, err := RequiresTransfer(Selection{}, cd, false); err != nil || got {
		t.Fatal("empty environment transferred ownership", got, err)
	}
}

func revision() Revision {
	return Revision{ID: "revision-1", Scope: Scope{OrganizationID: "org", EnvironmentID: "env", UnitKey: "app/rollout"}, Phase: Rollout, PayloadDigest: strings.Repeat("a", 64), ArtifactRef: "private-artifact", Selection: Selection{Mode: Direct}}
}
func TestReadinessRequiresExactFreshEvidence(t *testing.T) {
	r := revision()
	now := time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)
	o := Observation{Scope: r.Scope, IntendedRevision: r.ID, ObservedRevision: r.ID, PayloadDigest: r.PayloadDigest, Sync: InSync, Health: Healthy, Drift: NoDrift, Complete: true, ObservedAt: now}
	if !o.Ready(r, now, time.Second*30) {
		t.Fatal("complete observation refused")
	}
	for name, mutate := range map[string]func(*Observation){
		"old revision":     func(o *Observation) { o.ObservedRevision = "revision-0" },
		"different tree":   func(o *Observation) { o.PayloadDigest = strings.Repeat("b", 64) },
		"partial coverage": func(o *Observation) { o.Complete = false },
		"foreign target":   func(o *Observation) { o.Scope.EnvironmentID = "other" },
		"stale":            func(o *Observation) { o.ObservedAt = now.Add(-91 * time.Second) },
		"future":           func(o *Observation) { o.ObservedAt = now.Add(time.Second) },
		"healthy drift":    func(o *Observation) { o.Sync = OutOfSync; o.Drift = Detected },
	} {
		t.Run(name, func(t *testing.T) {
			changed := o
			mutate(&changed)
			if changed.Ready(r, now, time.Second*30) {
				t.Fatal("false success")
			}
		})
	}
	stale := o.Effective(now.Add(time.Minute*2), time.Second*30)
	if stale.ObservedAt != o.ObservedAt || stale.Sync != SyncUnknown || stale.Drift != DriftUnknown {
		t.Fatal("stale evidence misrepresented", stale)
	}
}

func TestJobRevisionCannotEnableAutonomousReplay(t *testing.T) {
	r := revision()
	r.Phase = Job
	r.Selection = Selection{Mode: CD, BindingID: "b", BindingVersion: 1, SelfHeal: true}
	if !errors.Is(r.Validate(), ErrUnsupported) {
		t.Fatal("job authorized for autonomous replay")
	}
}
