package pending_test

import (
	"context"
	"testing"
)

func TestSharedResourceAliasesSurviveDeployAndNoopPreview(t *testing.T) {
	ctx := context.Background()
	app, workloads, svc, opts := fixture(t)
	defer app.Close()
	raw := scoreFor("api", "example.invalid/api:v1", false)
	raw["resources"] = map[string]any{"original_db": map[string]any{"type": "postgres", "id": "shared-db", "params": map[string]any{"database": "appdb", "username": "appuser"}}}
	raw["containers"].(map[string]any)["main"].(map[string]any)["variables"] = map[string]any{"PGHOST": "${resources.original_db.host}"}
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", raw, 0); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := svc.Deploy(ctx, opts.ApplicationKey, opts.EnvironmentKey, "test", preview.Token); err != nil || result.Status != "SUCCEEDED" {
		t.Fatalf("deploy: %+v %v", result, err)
	}
	reopened := liveScore(t, workloads, opts.ApplicationKey, opts.EnvironmentKey, "api")
	if reopened == nil || variablesOf(reopened)["PGHOST"] != "${resources.original_db.host}" || reopened["resources"].(map[string]any)["original_db"] == nil {
		t.Fatalf("deployed Score lost its shared alias: %+v", reopened)
	}
	if _, err := workloads.Save(ctx, opts.ApplicationKey, opts.EnvironmentKey, "api", reopened, preview.DraftVersion+1); err != nil {
		t.Fatal(err)
	}
	unchanged, err := svc.Preview(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || len(unchanged.Changes) != 0 {
		t.Fatalf("preserving display aliases changed deployment semantics: %+v %v", unchanged.Changes, err)
	}
}
