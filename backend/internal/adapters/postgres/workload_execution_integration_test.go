package postgres

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

func TestWorkloadExecutionJSONBSurvivesRuntimeReopen(t *testing.T) {
	ctx := context.Background()
	store, url := openFresh(t)
	opts := seed.Defaults()
	if err := seedtest.Apply(ctx, store, opts); err != nil {
		t.Fatal(err)
	}
	env, err := store.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	policy := &environment.WorkloadExecution{Version: 1, Kind: "CronJob", Template: environment.TemplatePin{ID: "scheduled", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}, Schedule: &environment.Schedule{Expression: "0 0 * * *", TimeZone: "UTC", ConcurrencyPolicy: "Forbid", BackoffLimit: &zero}, SecretFiles: []environment.SecretFile{{Ref: "${context.uc12.GPG}", Container: "main", Path: "/keys/server", Mode: 0400}}}
	document := environment.NewDocument()
	document.Modules["backup"] = environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": {Image: "example:v1"}}, Execution: policy}}
	set := environment.DeploymentSet{ID: ids.New(), EnvironmentID: env.ID, EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, Document: document, DocumentHash: "execution-pin", CreatedAt: time.Now().UTC()}
	if err = store.SaveDeploymentSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	store.Close()
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetDeploymentSet(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Document.Modules["backup"].Spec.Execution, policy) {
		t.Fatalf("execution lost across runtime reopen: %#v", got.Document.Modules["backup"].Spec.Execution)
	}
}
