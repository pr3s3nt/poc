package configuration

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/score"
)

func (f *fixture) deployWithProofs(t *testing.T, env string, scores map[string]map[string]any, proofs environment.LiteralProofs) {
	t.Helper()
	f.deploy(t, env, scores)
	ctx := context.Background()
	current, _ := f.st.GetEnvironment(ctx, appKey, env)
	set, err := f.st.GetDeploymentSet(ctx, current.CurrentDeploymentSetID)
	if err != nil {
		t.Fatal(err)
	}
	set.ID = "22222222-2222-2222-2222-222222222222"
	set.LiteralProofs = proofs
	if err := f.st.SaveDeploymentSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	current.CurrentDeploymentSetID = set.ID
	if err := f.st.SaveEnvironment(ctx, current); err != nil {
		t.Fatal(err)
	}
}

// DEVEX-ENV E02/E10: a reference rename keeps a proven ordinary literal
// verbatim and fails closed, changing nothing, on an unproven one.
func TestRenameKeepsProvenLiteralsAndRejectsUnprovenOnes(t *testing.T) {
	ctx := context.Background()
	scoreWith := func(literal string) map[string]any {
		return scoreOf("api", map[string]string{"A": "${resources.env.DB_URL}", "LOG": literal}, nil)
	}
	t.Run("proven", func(t *testing.T) {
		f := newFixture(t)
		seedKeys(t, f, "staging")
		proofs := environment.LiteralProofs{}
		proofs.Add("api", "web", "LOG", "  debug $${X}  ")
		f.deployWithProofs(t, "staging", map[string]map[string]any{"api": scoreWith("  debug $${X}  ")}, proofs)
		view, guards := f.guards(t, "staging")
		if _, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "DATABASE_URL", view.Version, guards); err != nil {
			t.Fatalf("rename with a proven literal: %v", err)
		}
		draft := draftByID(f.state(t, "staging").drafts)["api"]
		vars := variablesOf(t, draft.Score)
		if vars["A"] != "${resources.env.DATABASE_URL}" || vars["LOG"] != "  debug $${X}  " {
			t.Fatalf("draft variables: %v", vars)
		}
		if _, err := score.FromMap(draft.Score); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unproven", func(t *testing.T) {
		f := newFixture(t)
		seedKeys(t, f, "staging")
		f.deploy(t, "staging", map[string]map[string]any{"api": scoreWith("legacy-value")})
		before := f.state(t, "staging")
		view, guards := f.guards(t, "staging")
		_, err := f.svc.RenameKey(ctx, appKey, "staging", "DB_URL", "DATABASE_URL", view.Version, guards)
		if !errors.Is(err, ErrReferenceRewriteUnsafe) {
			t.Fatalf("want REFERENCE_REWRITE_UNSAFE, got %v", err)
		}
		after := f.state(t, "staging")
		if after.scope != before.scope || len(after.drafts) != 0 || after.env.DraftVersion != before.env.DraftVersion {
			t.Fatal("a failed rename changed state")
		}
	})
}
