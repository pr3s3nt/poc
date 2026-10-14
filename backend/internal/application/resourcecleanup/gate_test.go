package resourcecleanup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// spies count every call that would plan, read private state, claim or run.
type spies struct{ reads, claims, states, engines atomic.Int64 }

type spyReader struct{ s *spies }

func (r spyReader) ListResourceRecords(context.Context, string, string) ([]tfresource.ResourceRecord, error) {
	r.s.reads.Add(1)
	return nil, nil
}
func (r spyReader) ListResourceEdges(context.Context, string, string) ([]tfresource.Dependency, error) {
	r.s.reads.Add(1)
	return nil, nil
}
func (r spyReader) GetCleanupRecord(context.Context, string, string, string) (tfresource.CleanupRecord, error) {
	r.s.reads.Add(1)
	return tfresource.CleanupRecord{}, persistence.ErrNotFound
}
func (r spyReader) ListCleanupRecords(context.Context, string, string, int) ([]tfresource.CleanupRecord, error) {
	r.s.reads.Add(1)
	return nil, nil
}
func (r spyReader) FindCleanupByRequest(context.Context, string, string, string, string) (tfresource.CleanupRecord, bool, error) {
	r.s.reads.Add(1)
	return tfresource.CleanupRecord{}, false, nil
}
func (r spyReader) ListOperationAttempts(context.Context, string, string, string) ([]tfresource.OperationAttempt, error) {
	r.s.reads.Add(1)
	return nil, nil
}

type spyClaims struct{ s *spies }

func (c spyClaims) ClaimResourceCleanup(context.Context, persistence.OperationClaim, tfresource.CleanupRequest, terraformstate.AdmissionCheck) (environment.Operation, bool, error) {
	c.s.claims.Add(1)
	return environment.Operation{}, false, errors.New("must not be called")
}
func (c spyClaims) CleanupRequest(context.Context, string) (tfresource.CleanupRequest, error) {
	c.s.claims.Add(1)
	return tfresource.CleanupRequest{}, errors.New("must not be called")
}

type spyState struct{ s *spies }

func (st spyState) GetInstance(context.Context, string) (tfresource.Instance, error) {
	st.s.states.Add(1)
	return tfresource.Instance{}, errors.New("must not be called")
}

type spyEngine struct{ s *spies }

func (e spyEngine) RunCleanup(context.Context, string, tfexec.CleanupSource, tfexec.StageReporter) (tfexec.CleanupResult, error) {
	e.s.engines.Add(1)
	return tfexec.CleanupResult{}, errors.New("must not be called")
}

func gateService(t *testing.T) (*Service, *spies) {
	t.Helper()
	st := store.New()
	ctx := context.Background()
	if err := st.SaveOrganization(ctx, application.Organization{ID: "org-id", Key: "acme", Name: "Acme"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveApplication(ctx, application.Application{ID: "app-id", Key: "shop", OrganizationKey: "acme", Name: "Shop", Subdomain: "shop", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ID: "env-id", Key: "staging", ApplicationID: "app-id", ApplicationKey: "shop", Name: "staging", Type: "staging", Version: 1}); err != nil {
		t.Fatal(err)
	}
	sp := &spies{}
	s := New(st, envops.NewManager(st))
	s.reader, s.claims, s.state, s.engine = spyReader{sp}, spyClaims{sp}, spyState{sp}, spyEngine{sp}
	return s, sp
}

func TestRoleGateComesBeforeAnyPlanningStateClaimOrEngine(t *testing.T) {
	s, sp := gateService(t)
	sc := Scope{OrganizationKey: "acme", ApplicationKey: "shop", EnvironmentKey: "staging"}
	developer := Actor{Username: "developer"}
	// Malformed shapes, an unknown root and a hostile token are all just 403.
	inputs := []Input{{}, {Token: "x", ConfirmationName: "n", Selected: nil}, {Token: "rd1.1.zz.zz", ConfirmationName: "n", Selected: []string{"", ""}}, {Token: "rd1.4102444800." + "0123456789abcdef." + string(make([]byte, 0)), ConfirmationName: "n", Selected: []string{"unknown"}}}
	for _, root := range []string{"", "not-an-id", "00000000-0000-4000-8000-000000000000"} {
		if _, err := s.PrepareDeletion(context.Background(), sc, developer, root); !errors.Is(err, ErrForbidden) {
			t.Errorf("preview %q: %v", root, err)
		}
		for _, in := range inputs {
			if _, err := s.CleanupResources(context.Background(), sc, developer, root, in); !errors.Is(err, ErrForbidden) {
				t.Errorf("deletion %q %+v: %v", root, in, err)
			}
		}
	}
	if sp.reads.Load()+sp.claims.Load()+sp.states.Load()+sp.engines.Load() != 0 {
		t.Fatalf("a Developer request reached the planner/state/claim/engine: %d %d %d %d", sp.reads.Load(), sp.claims.Load(), sp.states.Load(), sp.engines.Load())
	}
	// A caller outside the scope is 404 and equally reaches nothing.
	foreign := Scope{OrganizationKey: "globex", ApplicationKey: "shop", EnvironmentKey: "staging"}
	if _, err := s.PrepareDeletion(context.Background(), foreign, Actor{Username: "pe", MayDelete: true}, "x"); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("foreign preview: %v", err)
	}
	if _, err := s.CleanupResources(context.Background(), foreign, Actor{Username: "pe", MayDelete: true}, "x", Input{}); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("foreign deletion: %v", err)
	}
	if sp.reads.Load()+sp.claims.Load()+sp.states.Load()+sp.engines.Load() != 0 {
		t.Fatal("a foreign request reached the planner/state/claim/engine")
	}
}
