package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// UC-16 BR-17/23: private metadata rides in deployment_sets.document JSONB
// without a schema change, survives restart, and a row written before the
// feature reads as "no proof" (unknown), never as safe.
func TestDeploymentSetMetadataRoundTripRestartAndLegacyRows(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	doc := environment.NewDocument()
	doc.Modules["api"] = environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{
		"main": {ID: "main", Image: "example.invalid/api:v1", Variables: map[string]string{"LOG_LEVEL": "debug", "EMPTY": ""}},
	}}}
	proofs := environment.LiteralProofs{}
	proofs.Add("api", "main", "LOG_LEVEL", "debug")
	proofs.Add("api", "main", "EMPTY", "")
	withProofs := environment.DeploymentSet{ID: "33333333-3333-3333-3333-333333333333", EnvironmentID: env.ID, EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, Document: doc, DocumentHash: "h1", CreatedAt: time.Now().UTC().Truncate(time.Second), LiteralProofs: proofs}
	aliases := environment.ResourceAliases{"api": {Shared: map[string]string{"db": "shared-db"}, References: map[string]string{"/containers/main/variables/HOST": "${resources.db.host}"}}}
	withProofs.ResourceAliases = aliases
	legacy := withProofs
	legacy.ID, legacy.DocumentHash, legacy.LiteralProofs = "44444444-4444-4444-4444-444444444444", "h2", nil
	legacy.ResourceAliases = nil
	for _, set := range []environment.DeploymentSet{withProofs, legacy} {
		if err := st.SaveDeploymentSet(ctx, set); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()

	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetDeploymentSet(ctx, withProofs.ID)
	if err != nil || !reflect.DeepEqual(got.Document, doc) || !reflect.DeepEqual(got.LiteralProofs, proofs) || !reflect.DeepEqual(got.ResourceAliases, aliases) {
		t.Fatalf("proofs or document changed across restart: %+v %v", got, err)
	}
	if !got.LiteralProofs.Proves("api", "main", "LOG_LEVEL", "debug") || got.LiteralProofs.Proves("api", "main", "LOG_LEVEL", "info") {
		t.Fatal("restored proof is not bound to the exact value")
	}
	old, err := reopened.GetDeploymentSet(ctx, legacy.ID)
	if err != nil || old.LiteralProofs != nil || old.ResourceAliases != nil || !reflect.DeepEqual(old.Document, doc) {
		t.Fatalf("a set without provenance must read as no proof: %+v %v", old.LiteralProofs, err)
	}
}
