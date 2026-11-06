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

func TestWorkloadDefaultsPolicyAndFalseZeroSurviveJSONBReopen(t *testing.T) {
	ctx := context.Background()
	st, url := openFresh(t)
	opts := seed.Defaults()
	opts.EnvironmentKey = "staging"
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	env, err := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	no := false
	policy := &environment.WorkloadExecution{Version: 1, Kind: "Deployment", Defaults: &environment.DefaultsPolicy{Version: "workload-defaults-v1", Environment: "staging"}, AutomountServiceAccountToken: &no, RevisionHistoryLimit: &zero, PodDisruptionBudget: &environment.PodDisruptionBudget{Enabled: false}}
	doc := environment.NewDocument()
	doc.Modules["web"] = environment.Module{Profile: environment.ModuleProfile, Spec: environment.ModuleSpec{Execution: policy, Containers: map[string]environment.Container{"main": {Image: "example:v1", SecurityContext: &environment.ContainerSecurityContext{RunAsNonRoot: &no}}}}}
	set := environment.DeploymentSet{ID: ids.New(), EnvironmentID: env.ID, EnvironmentKey: opts.ApplicationKey + "/staging", Document: doc, DocumentHash: "defaults-snapshot", CreatedAt: time.Now().UTC()}
	if err := st.SaveDeploymentSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	st.Close()
	again, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got, err := again.GetDeploymentSet(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Document, doc) {
		t.Fatal("policy/false/zero changed across JSONB and runtime reopen")
	}
}
