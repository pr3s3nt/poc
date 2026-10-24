package pending_test

import (
	"context"
	"testing"

	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/environment"
)

// literalScore is a workload whose container carries ordinary strings only:
// plain text, an empty value, surrounding whitespace and an escaped placeholder.
func literalScore(name string, values map[string]string) map[string]any {
	score := scoreFor(name, "example.invalid/"+name+":v1", false)
	variables := map[string]any{}
	for key, value := range values {
		variables[key] = value
	}
	score["containers"].(map[string]any)["main"].(map[string]any)["variables"] = variables
	return score
}

var acceptedLiterals = map[string]string{
	"LOG_LEVEL": "debug",
	"EMPTY":     "",
	"PADDED":    "  two spaces  ",
	"TEMPLATE":  "$${NOT_A_REFERENCE}",
}

func liveScore(t *testing.T, workloads *workloadconfig.Service, app, env, id string) map[string]any {
	t.Helper()
	view, err := workloads.List(context.Background(), app, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Workloads {
		if item.ID == id {
			return item.Score
		}
	}
	t.Fatalf("workload %s not listed", id)
	return nil
}

func variablesOf(score map[string]any) map[string]any {
	main := score["containers"].(map[string]any)["main"].(map[string]any)
	vars, _ := main["variables"].(map[string]any)
	return vars
}

// DEVEX-ENV E02/E10: an ordinary value accepted by UC-16 Save survives Preview,
// Deploy and reopen exactly; the same value written by a direct Score deploy
// never becomes readable.
func TestAcceptedLiteralsRoundTripAndDirectDeployStaysHidden(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)

	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", literalScore("api", acceptedLiterals), 0); err != nil {
		t.Fatalf("save ordinary literals: %v", err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(preview.Changes) != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}

	got := variablesOf(liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "api"))
	for key, want := range acceptedLiterals {
		if value, ok := got[key]; !ok || value != want {
			t.Fatalf("variable %s = %q (present %v), want %q", key, value, ok, want)
		}
	}

	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	set, err := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if err != nil || set.LiteralProofs.Empty() {
		t.Fatalf("current set records no proof: %+v %v", set.LiteralProofs, err)
	}

	// A second UC-16 edit of the deployed workload starts from the proven value.
	edited := literalScore("api", map[string]string{"LOG_LEVEL": "info", "EMPTY": "", "PADDED": "  two spaces  ", "TEMPLATE": "$${NOT_A_REFERENCE}"})
	view, _ := workloads.List(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", edited, view.DraftVersion); err != nil {
		t.Fatal(err)
	}
	preview, err = svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(preview.Changes) != 1 {
		t.Fatalf("second preview: %+v %v", preview, err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("second deploy: %+v %v", report, err)
	}
	if value := variablesOf(liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "api"))["LOG_LEVEL"]; value != "info" {
		t.Fatalf("edited value = %v", value)
	}

	// A direct Score deploy that changes the value invalidates the proof.
	direct := literalScore("api", map[string]string{"LOG_LEVEL": "trace", "EMPTY": "", "PADDED": "  two spaces  ", "TEMPLATE": "$${NOT_A_REFERENCE}"})
	if _, err := app.Deployments.DeployWorkload(ctx, deployCommand(opts, "api", edited, direct)); err != nil {
		t.Fatalf("direct deploy: %v", err)
	}
	if score := liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "api"); score != nil {
		t.Fatalf("direct-API literal became readable: %v", variablesOf(score))
	}

	// A brand-new workload deployed directly never gains a proof either.
	deploy(t, app, opts, "legacy", literalScore("legacy", map[string]string{"TOKEN": "looks-like-a-secret"}))
	if score := liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "legacy"); score != nil {
		t.Fatalf("unproven legacy literal is readable: %v", score)
	}
}

// A UC-16 deploy of one workload proves only that workload; a literal of another
// workload keeps its own proof state and is not promoted by the deploy.
func TestLiteralProofIsPerWorkload(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	deploy(t, app, opts, "legacy", literalScore("legacy", map[string]string{"TOKEN": "unproven"}))
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", literalScore("api", map[string]string{"LOG_LEVEL": "debug"}), 0); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); err != nil || report.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", report, err)
	}
	if variablesOf(liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "api"))["LOG_LEVEL"] != "debug" {
		t.Fatal("accepted workload lost its value")
	}
	if score := liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "legacy"); score != nil {
		t.Fatalf("deploying another workload promoted a legacy literal: %v", score)
	}
	env, _ := app.Store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	set, _ := app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if set.LiteralProofs.Proves("legacy", "main", "TOKEN", "unproven") {
		t.Fatal("legacy literal was proven")
	}
	if !set.LiteralProofs.Proves("api", "main", "LOG_LEVEL", "debug") || set.LiteralProofs.Proves("api", "main", "LOG_LEVEL", "other") {
		t.Fatal("proof must match the exact accepted value only")
	}
}

func TestProveLiteralsCarriesOnlyIdenticalValues(t *testing.T) {
	doc := environment.NewDocument()
	doc.Modules["a"] = environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{
		"main": {Image: "i", Variables: map[string]string{"KEEP": "x", "CHANGED": "new", "REF": "${externals.db.host}"}},
	}}}
	base := environment.LiteralProofs{}
	base.Add("a", "main", "KEEP", "x")
	base.Add("a", "main", "CHANGED", "old")
	got := workloadconfig.ProveLiterals(base, doc, "")
	if !got.Proves("a", "main", "KEEP", "x") || got.Proves("a", "main", "CHANGED", "new") || got.Proves("a", "main", "REF", "${externals.db.host}") {
		t.Fatalf("unexpected proofs: %+v", got)
	}
	if workloadconfig.ProveLiterals(nil, doc, "") != nil {
		t.Fatal("no base proof and no accepted workload must prove nothing")
	}
}
