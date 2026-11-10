package fleet

import (
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	gitport "orchestrator/internal/ports/gitpublication"
	"testing"
)

func TestUnpublishedUpdateRequiresExactPriorControllerPin(t *testing.T) {
	i := port.Intent{ID: "new-intent", ExpectedRevision: "prior-revision", Revision: domain.Revision{ID: "new-revision", Scope: domain.Scope{EnvironmentID: "env"}}}
	pkg := Package{Publication: gitport.Intent{Path: "environments/staging/web/rollout"}}
	pin := unitPin{Path: pkg.Publication.Path, EnvironmentID: "env", Revision: "prior-revision", Intent: "prior-intent", Digest: "proof"}
	if !unpublishedPriorPin(pin, true, i, pkg) {
		t.Fatal("prior pin rejected")
	}
	for _, mutate := range []func(*unitPin){func(p *unitPin) { p.Removed = true }, func(p *unitPin) { p.Revision = i.Revision.ID }, func(p *unitPin) { p.Intent = i.ID }, func(p *unitPin) { p.EnvironmentID = "other" }, func(p *unitPin) { p.Path = "other" }, func(p *unitPin) { p.Digest = "" }} {
		p := pin
		mutate(&p)
		if unpublishedPriorPin(p, true, i, pkg) {
			t.Fatal("unverified prior controller accepted", p)
		}
	}
	if unpublishedPriorPin(pin, false, i, pkg) {
		t.Fatal("absent controller pin accepted")
	}
}

func TestFirstRolloutAbsenceRequiresItsOwnPrerequisitePin(t *testing.T) {
	i := port.Intent{ID: "new", Revision: domain.Revision{ID: "rollout", Phase: domain.Rollout, Scope: domain.Scope{OrganizationID: "org", EnvironmentID: "env", TargetGeneration: 2, Epoch: 3, UnitKey: "payment/ROLLOUT"}}}
	cfg := Config{Root: "environments/staging"}
	pkg := Package{Publication: gitport.Intent{Path: "environments/staging/payment/rollout"}}
	scope := i.Revision.Scope
	scope.UnitKey = "payment/PREREQUISITE"
	key := unitReleaseName(scope)
	prior := unitPin{EnvironmentID: "env", Path: "environments/staging/payment/prerequisite", Revision: "prior", Intent: "prior-intent", Digest: "digest"}
	if !hasPrerequisitePin(i, pkg, cfg, map[string]unitPin{key: prior}) {
		t.Fatal("valid prerequisite refused")
	}
	for _, mutate := range []func(*unitPin){func(p *unitPin) { p.Removed = true }, func(p *unitPin) { p.EnvironmentID = "other" }, func(p *unitPin) { p.Path = "environments/staging/other/prerequisite" }, func(p *unitPin) { p.Revision = "" }, func(p *unitPin) { p.Intent = i.ID }, func(p *unitPin) { p.Digest = "" }} {
		bad := prior
		mutate(&bad)
		if hasPrerequisitePin(i, pkg, cfg, map[string]unitPin{key: bad}) {
			t.Fatal("invalid prerequisite accepted")
		}
	}
	for _, mutate := range []func(*domain.Scope){func(s *domain.Scope) { s.Epoch++ }, func(s *domain.Scope) { s.TargetGeneration++ }, func(s *domain.Scope) { s.EnvironmentID = "other" }, func(s *domain.Scope) { s.UnitKey = "other/PREREQUISITE" }} {
		foreign := scope
		mutate(&foreign)
		if hasPrerequisitePin(i, pkg, cfg, map[string]unitPin{unitReleaseName(foreign): prior}) {
			t.Fatal("foreign prerequisite accepted")
		}
	}
}

// UC-06 CD-10: only an exact receipted published failure stands in for the
// acknowledged source inventory.
func TestFailedPriorRequiresTheReceiptedPinHolder(t *testing.T) {
	prior := &port.PriorSubmission{Failed: true, Prepared: port.Prepared{Revision: domain.Revision{ID: "failed"}}, Receipt: port.Receipt{IntentID: "failed-intent", RevisionID: "failed"}}
	i := port.Intent{ID: "new", ExpectedRevision: "failed", PriorSubmission: prior}
	if !failedPrior(i) {
		t.Fatal("receipted published failure refused")
	}
	for _, mutate := range []func(*port.PriorSubmission){
		func(p *port.PriorSubmission) { p.Failed = false },
		func(p *port.PriorSubmission) { p.Prepared.Revision.ID = "other" },
		func(p *port.PriorSubmission) { p.Receipt.RevisionID = "other" },
		func(p *port.PriorSubmission) { p.Receipt.IntentID = "" },
	} {
		bad := *prior
		mutate(&bad)
		if failedPrior(port.Intent{ID: "new", ExpectedRevision: "failed", PriorSubmission: &bad}) {
			t.Fatal("unproven failed prior accepted", bad)
		}
	}
	if failedPrior(port.Intent{ExpectedRevision: "failed"}) || failedPrior(port.Intent{ExpectedRevision: "other", PriorSubmission: prior}) {
		t.Fatal("missing or mismatched prior accepted")
	}
}

// Issue #48: Fleet status writes change resourceVersion; only a change of the
// controller itself may stop the first-Rollout absence proof.
func TestSameControllerIdentityIgnoresStatusWrites(t *testing.T) {
	repo := func() map[string]any {
		return map[string]any{
			"metadata": map[string]any{"uid": "u", "generation": float64(2), "resourceVersion": "10", "labels": map[string]any{"a": "1"}, "annotations": map[string]any{"orchestrator.io/unit-pins": "pins"}},
			"spec":     map[string]any{"revision": "c1", "paths": []any{"p"}},
			"status":   map[string]any{"commit": "c1"},
		}
	}
	before, after := repo(), repo()
	after["metadata"].(map[string]any)["resourceVersion"] = "11"
	after["status"] = map[string]any{"commit": "c1", "lastPollingTriggered": "now"}
	if !sameControllerIdentity(before, after) {
		t.Fatal("a status write was treated as a different controller")
	}
	for name, mutate := range map[string]func(map[string]any){
		"uid":        func(r map[string]any) { r["metadata"].(map[string]any)["uid"] = "other" },
		"generation": func(r map[string]any) { r["metadata"].(map[string]any)["generation"] = float64(3) },
		"unit pins": func(r map[string]any) {
			r["metadata"].(map[string]any)["annotations"] = map[string]any{"orchestrator.io/unit-pins": "changed"}
		},
		"labels":   func(r map[string]any) { r["metadata"].(map[string]any)["labels"] = map[string]any{"a": "2"} },
		"spec":     func(r map[string]any) { r["spec"] = map[string]any{"revision": "c2", "paths": []any{"p"}} },
		"deleting": func(r map[string]any) { r["metadata"].(map[string]any)["deletionTimestamp"] = "now" },
	} {
		changed := repo()
		mutate(changed)
		if sameControllerIdentity(before, changed) {
			t.Fatalf("%s change accepted as the same controller", name)
		}
	}
}
