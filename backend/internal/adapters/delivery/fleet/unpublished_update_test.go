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
