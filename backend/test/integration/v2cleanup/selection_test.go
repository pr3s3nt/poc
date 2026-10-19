package main

import (
	"orchestrator/internal/domain/tfresource"
	"testing"
)

func TestSelectFailedIDsRequiresExactFailedOwnedGenerationAndUniqueDescriptor(t *testing.T) {
	probe := tfresource.Instance{ID: "probe-id", Identity: tfresource.Identity{Descriptor: "redis.probe#shared.probe", TargetGeneration: 0}, CreatedByRunID: "pending-shared", Status: tfresource.InstanceFailed}
	healthy := tfresource.Instance{ID: "healthy-id", Identity: tfresource.Identity{Descriptor: "redis.default#shared.healthy", TargetGeneration: 0}, CreatedByRunID: probe.CreatedByRunID, Status: tfresource.InstanceReady}
	owned := map[string]bool{probe.CreatedByRunID: true}
	if ids, err := selectFailedIDs([]tfresource.Instance{probe, healthy}, []string{probe.Identity.Descriptor}, owned, 0); err != nil || len(ids) != 1 || ids[0] != probe.ID {
		t.Fatalf("same-run exact subset %v %v", ids, err)
	}
	for name, names := range map[string][]string{"unknown": {"missing"}, "healthy": {healthy.Identity.Descriptor}, "duplicate": {probe.Identity.Descriptor, probe.Identity.Descriptor}, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			if _, err := selectFailedIDs([]tfresource.Instance{probe, healthy}, names, owned, 0); err == nil {
				t.Fatal("accepted invalid selection")
			}
		})
	}
	if _, err := selectFailedIDs([]tfresource.Instance{probe}, []string{probe.Identity.Descriptor}, map[string]bool{}, 0); err == nil {
		t.Fatal("accepted unrecorded run")
	}
	if _, err := selectFailedIDs([]tfresource.Instance{probe}, []string{probe.Identity.Descriptor}, owned, 1); err == nil {
		t.Fatal("accepted wrong generation")
	}
	if _, err := selectFailedIDs([]tfresource.Instance{probe, probe}, []string{probe.Identity.Descriptor}, owned, 0); err == nil {
		t.Fatal("accepted ambiguous descriptor")
	}
}

func TestPreApplyPendingRequiresCompleteOwnedTerminalNoEffectHistory(t *testing.T) {
	inst := tfresource.Instance{ID: "pending-id", CreatedByRunID: "run", Status: tfresource.InstancePending, Identity: tfresource.Identity{Descriptor: "pg.probe#shared.preapply"}}
	owned := map[string]bool{"run": true}
	good := tfresource.Attempt{InstanceID: inst.ID, RunID: "run", Action: tfresource.ActionCreate, Ownership: tfresource.CreatedByRun, Stage: "PLANNING", Status: tfresource.AttemptFailed}
	if !provenPreApplyFailure(inst, []tfresource.Attempt{good}, owned) {
		t.Fatal("valid pre-apply history refused")
	}
	if _, err := selectFailedIDs([]tfresource.Instance{inst}, []string{inst.Identity.Descriptor}, owned, 0); err == nil {
		t.Fatal("pending admitted without proof")
	}
	if _, err := selectFailedIDs([]tfresource.Instance{inst}, []string{inst.Identity.Descriptor}, owned, 0, map[string]bool{inst.ID: true}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*tfresource.Attempt){
		"foreign-instance": func(a *tfresource.Attempt) { a.InstanceID = "other" },
		"foreign-run":      func(a *tfresource.Attempt) { a.RunID = "other" },
		"update":           func(a *tfresource.Attempt) { a.Action = tfresource.ActionUpdate },
		"preexisting":      func(a *tfresource.Attempt) { a.Ownership = tfresource.Preexisting },
		"applying":         func(a *tfresource.Attempt) { a.Stage = "APPLYING" },
		"running":          func(a *tfresource.Attempt) { a.Status = tfresource.AttemptRunning },
		"succeeded":        func(a *tfresource.Attempt) { a.Status = tfresource.AttemptSucceeded },
		"observed-object":  func(a *tfresource.Attempt) { a.ObservedPhysicalIDs = []string{"object"} },
		"state":            func(a *tfresource.Attempt) { n := int64(0); a.StateSerial = &n },
		"recovery":         func(a *tfresource.Attempt) { a.RecoveryHandle = "retained" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := good
			change(&a)
			if provenPreApplyFailure(inst, []tfresource.Attempt{good, a}, owned) {
				t.Fatal("unproven history accepted")
			}
		})
	}
	if provenPreApplyFailure(inst, nil, owned) {
		t.Fatal("empty history accepted")
	}
}
