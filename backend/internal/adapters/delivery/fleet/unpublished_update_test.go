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
