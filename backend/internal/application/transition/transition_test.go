package transition_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/transition"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

func TestPreviewPinsTheSnapshotAndRejectsUnsupportedBeforeAnySideEffect(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	before := len(h.cluster.Calls())

	p := h.preview(environment.ModeMigratePostgres)
	if p.Token == "" || !p.DowntimeRequired || p.Destination.Generation != 1 || p.Source.Generation != 0 || len(p.Workloads) != 3 || len(p.Mappings) != 1 {
		t.Fatalf("preview: %+v", p)
	}
	if p.Destination.Namespace == p.Source.Namespace || !generationSuffix.MatchString(p.Destination.Namespace) {
		t.Fatalf("destination namespace must be generation-isolated: %+v", p)
	}
	// Preview only inspects; it never scales, backs up or provisions.
	for _, line := range h.cluster.Calls()[before:] {
		if !strings.HasPrefix(line, "inspect") && !strings.HasPrefix(line, "probe") {
			t.Fatalf("preview performed a side effect: %s", line)
		}
	}

	t.Run("same destination", func(t *testing.T) {
		req := h.request(environment.ModeDeployNew)
		req.DestinationKey = h.env().ConnectionKey
		if _, err := h.service.Preview(ctx, req); !errors.Is(err, transition.ErrSameDestination) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("unknown or foreign destination", func(t *testing.T) {
		req := h.request(environment.ModeDeployNew)
		req.DestinationKey = "missing"
		if _, err := h.service.Preview(ctx, req); !errors.Is(err, transition.ErrDestinationFailed) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("old PostgreSQL major version blocks migration before quiesce", func(t *testing.T) {
		target, name := h.sourceDatabase()
		db, _ := h.cluster.Database(target, name)
		h.cluster.SeedDatabase(target, name, db.Tables)
		h.cluster.SetVersion(target, name, 150004)
		calls := len(h.cluster.Calls())
		_, err := h.service.Preview(ctx, h.request(environment.ModeMigratePostgres))
		var unsupported *transition.UnsupportedError
		if !errors.As(err, &unsupported) || !strings.Contains(err.Error(), "PostgreSQL 16") {
			t.Fatalf("want unsupported version, got %v", err)
		}
		for _, line := range h.cluster.Calls()[calls:] {
			if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") {
				t.Fatalf("unsupported capability must fail before quiesce: %s", line)
			}
		}
		h.cluster.SetVersion(target, name, 160004)
	})
	t.Run("unmapped or unknown mapping", func(t *testing.T) {
		bad := environment.ResourceMapping{SourceDescriptor: "postgres.default#shared.other", DestinationDescriptor: "postgres.default#shared.acceptance-db"}
		if _, err := h.service.Preview(ctx, h.request(environment.ModeMigratePostgres, bad)); !transition.IsInvalid(err) {
			t.Fatalf("unknown source: %v", err)
		}
		wrongDest := environment.ResourceMapping{SourceDescriptor: "postgres.default#shared.acceptance-db", DestinationDescriptor: "postgres.default#shared.nowhere"}
		if _, err := h.service.Preview(ctx, h.request(environment.ModeMigratePostgres, wrongDest)); !transition.IsInvalid(err) {
			t.Fatalf("unknown destination: %v", err)
		}
	})
	t.Run("fleet delivery is unsupported", func(t *testing.T) {
		h.service.SetDirectDelivery(false)
		defer h.service.SetDirectDelivery(true)
		_, err := h.service.Preview(ctx, h.request(environment.ModeMigratePostgres))
		var unsupported *transition.UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("busy environment", func(t *testing.T) {
		op, err := h.app.Store.ClaimEnvironment(ctx, persistence.OperationClaim{ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, Owner: "x", Kind: environment.OpDeploy})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = h.app.Store.ReleaseOperation(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, environment.OpSucceeded, "")
		}()
		if _, err := h.service.Preview(ctx, h.request(environment.ModeDeployNew)); !errors.Is(err, persistence.ErrEnvironmentBusy) {
			t.Fatalf("%v", err)
		}
	})
}

func TestExecuteRequiresFreshTokenAndDowntimeAcknowledgement(t *testing.T) {
	h := newHarness(t)
	p := h.preview(environment.ModeMigratePostgres)
	if _, err := h.execute(p, environment.ModeMigratePostgres, false); !errors.Is(err, transition.ErrAcknowledge) {
		t.Fatalf("missing acknowledgement: %v", err)
	}
	if _, err := h.service.Execute(context.Background(), transition.ExecuteRequest{Request: h.request(environment.ModeMigratePostgres), Token: "forged", AcknowledgeDowntime: true}); !errors.Is(err, transition.ErrStaleToken) {
		t.Fatalf("forged token: %v", err)
	}
	// A configuration edit between Preview and Execute invalidates the token
	// before anything is claimed or stopped.
	if _, err := h.app.Configurations.Put(context.Background(), h.opts.ApplicationKey, h.opts.EnvironmentKey, "MODE", "VARIABLE", "blue", 0); err != nil {
		t.Fatal(err)
	}
	calls := len(h.cluster.Calls())
	if _, err := h.execute(p, environment.ModeMigratePostgres, true); !errors.Is(err, transition.ErrStaleToken) {
		t.Fatalf("stale token: %v", err)
	}
	for _, line := range h.cluster.Calls()[calls:] {
		if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") || strings.HasPrefix(line, "restore") {
			t.Fatalf("a rejected execute performed a side effect: %s", line)
		}
	}
	if env := h.env(); env.Busy() || env.TargetGeneration != 0 {
		t.Fatalf("rejected execute changed the environment: %+v", env)
	}
}

func TestMigratePostgresRestoresRowsBeforeDestinationAppsAndKeepsSource(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedJobs(5)
	sourceEnv := h.env()
	sourceInstances, _ := h.app.Store.ListWorkloadInstances(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey)
	sourceDeployments, _ := h.app.Store.ListDeployments(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)

	p := h.preview(environment.ModeMigratePostgres)
	detail, err := h.execute(p, environment.ModeMigratePostgres, true)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != environment.TransitionSucceeded || detail.Stage != environment.StageSucceeded || detail.SourceState != environment.SourceQuiesced || detail.Failure != "" {
		t.Fatalf("transition: %+v", detail)
	}
	wantStages := []environment.TransitionStage{environment.StagePreflight, environment.StageQuiescing, environment.StageBackup, environment.StageProvisioning, environment.StageRestoring, environment.StageDeploying, environment.StageVerifying, environment.StageCutover}
	if len(detail.Stages) != len(wantStages) {
		t.Fatalf("stages: %+v", detail.Stages)
	}
	for i, stage := range detail.Stages {
		if stage.Stage != wantStages[i] || stage.Status != "SUCCEEDED" {
			t.Fatalf("stage %d: %+v", i, stage)
		}
	}

	// Destination database holds the same rows, restored before any destination app started.
	env := h.env()
	if env.TargetGeneration != 1 || env.ConnectionKey != destKey || env.Busy() || env.GenerationHigh != 1 {
		t.Fatalf("environment after cutover: %+v", env)
	}
	destTarget, destName := h.sourceDatabase()
	db, ok := h.cluster.Database(destTarget, destName)
	if !ok || db.Tables["public.jobs"] != 5 || db.Tables["public.results"] != 10 {
		t.Fatalf("destination database: %+v ok=%v", db, ok)
	}
	if destTarget.Namespace == sourceEnv.Namespace() || !generationSuffix.MatchString(destTarget.Namespace) {
		t.Fatalf("destination database namespace: %s", destTarget.Namespace)
	}
	if restore, firstApply := h.logIndex("restore"), h.logIndex("apply "+env.Namespace()); restore < 0 || firstApply >= 0 && firstApply < restore {
		t.Fatalf("restore must precede every destination application apply: restore=%d apply=%d\n%v", restore, firstApply, h.cluster.Calls())
	}
	if h.logIndex("scale ") > h.logIndex("backup") || h.logIndex("backup") > h.logIndex("restore") {
		t.Fatalf("quiesce -> backup -> restore order: %v", h.cluster.Calls())
	}

	// Source generation untouched and retained: instances, database, replicas.
	oldInstances, _ := h.app.Store.ListWorkloadInstancesFor(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey, 0)
	if len(oldInstances) != len(sourceInstances) {
		t.Fatalf("source instances changed: %d -> %d", len(sourceInstances), len(oldInstances))
	}
	for i, old := range oldInstances {
		if old.TargetRef["namespace"] != sourceInstances[i].TargetRef["namespace"] || old.LastDeploymentID != sourceInstances[i].LastDeploymentID || old.Status != deployment.InstanceReady {
			t.Fatalf("source instance overwritten: %+v vs %+v", old, sourceInstances[i])
		}
	}
	newInstances, _ := h.app.Store.ListWorkloadInstances(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey)
	if len(newInstances) != 3 {
		t.Fatalf("destination instances: %+v", newInstances)
	}
	for _, instance := range newInstances {
		if instance.Generation != 1 || instance.TargetRef["namespace"] != env.Namespace() || instance.Status != deployment.InstanceReady {
			t.Fatalf("destination instance: %+v", instance)
		}
	}
	srcTarget, srcName := h.sourceDatabaseAt(0)
	if rows, ok := h.cluster.Database(srcTarget, srcName); !ok || rows.Tables["public.jobs"] != 5 {
		t.Fatalf("source database must be retained intact: %+v", rows)
	}
	for _, workload := range []string{"backend", "worker", "frontend"} {
		if n, _ := h.cluster.ReplicasOf(h.sourceTarget(sourceEnv), workload); n != 0 {
			t.Fatalf("source %s is not quiesced: %d", workload, n)
		}
		if n, _ := h.cluster.ReplicasOf(h.sourceTarget(env), workload); n < 1 {
			t.Fatalf("destination %s is not running: %d", workload, n)
		}
	}
	// Routes: destination owns the host, source no longer does.
	if !h.hasRoute(env) || h.hasRoute(sourceEnv) {
		t.Fatalf("route ownership: destination=%v source=%v", h.hasRoute(env), h.hasRoute(sourceEnv))
	}
	// Resource identity: generation 0 resources keep their scope, generation 1 are new.
	active, _ := h.app.Store.ListActiveResources(ctx, h.opts.OrganizationKey)
	gen0, gen1 := 0, 0
	for _, a := range active {
		switch {
		case a.Scope.Type == resource.ScopeApplication:
		case a.Scope.ID == sourceEnv.ApplicationKey+"."+sourceEnv.Key || strings.HasPrefix(a.Scope.ID, sourceEnv.ApplicationKey+"."+sourceEnv.Key+"."):
			gen0++
			if a.Status != resource.StatusReady {
				t.Fatalf("source resource changed: %+v", a)
			}
		case strings.Contains(a.Scope.ID, "~g1"):
			gen1++
		}
	}
	if gen0 < 2 || gen1 < 2 {
		t.Fatalf("resource generations not isolated: gen0=%d gen1=%d", gen0, gen1)
	}
	// History records the actual target of each run.
	all, _ := h.app.Store.ListDeployments(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	for _, d := range sourceDeployments {
		stored, _ := h.app.Store.GetDeployment(ctx, d.ID)
		if stored.TargetGeneration != 0 {
			t.Fatalf("source deployment rewritten: %+v", stored)
		}
	}
	var provision, applied int
	for _, d := range all {
		if d.TargetGeneration == 1 {
			if d.ConnectionKey != destKey {
				t.Fatalf("destination deployment connection: %+v", d)
			}
			if d.Action == deployment.ActionProvision {
				provision++
			} else {
				applied++
			}
		}
	}
	if provision != 3 || applied != 3 {
		t.Fatalf("destination deployments: provision=%d apply=%d", provision, applied)
	}
	// Backups are private and removed after verification.
	stored, _ := h.app.Store.GetTransition(ctx, detail.ID)
	if len(stored.Backups) != 1 || !stored.Backups[0].Removed || stored.Backups[0].Dir == "" {
		t.Fatalf("backup handle: %+v", stored.Backups)
	}
	// The new current Set is what runs; no pending change is left behind.
	preview, err := h.app.Pending.Preview(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if err != nil || len(preview.Changes) != 0 {
		t.Fatalf("a completed transition must leave nothing pending: %+v %v", preview.Changes, err)
	}
}

// contextOfGeneration names the fake cluster of a generation: 0 is the seeded
// Connection, later generations the second logical Connection.
func (h *harness) contextOfGeneration(generation int64) string {
	if generation == 0 {
		return h.contextOf(environment.Environment{ConnectionKey: h.opts.ConnectionKey})
	}
	return h.contextOf(environment.Environment{ConnectionKey: destKey})
}

func (h *harness) hasRoute(env environment.Environment) bool {
	ok, _ := h.cluster.HasPublicRoute(context.Background(), h.sourceTarget(env), h.opts.ApplicationKey, h.opts.EnvironmentKey)
	return ok
}

// sourceDatabaseAt finds the database of one generation.
func (h *harness) sourceDatabaseAt(generation int64) (execution.Target, string) {
	h.t.Helper()
	active, _ := h.app.Store.ListActiveResources(context.Background(), h.opts.OrganizationKey)
	for _, a := range active {
		if a.Descriptor.Type == "postgres" && a.Scope.ID == environment.ScopeID(h.opts.ApplicationKey, h.opts.EnvironmentKey, generation) {
			name, _ := a.ExecutorState["name"].(string)
			namespace, _ := a.ExecutorState["namespace"].(string)
			return execution.Target{Context: h.contextOfGeneration(generation), Namespace: namespace}, name
		}
	}
	h.t.Fatalf("no database for generation %d", generation)
	return execution.Target{}, ""
}

func TestDeployNewKeepsSourceRunningUntilCutoverAndStartsEmpty(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(4)
	sourceEnv := h.env()
	p := h.preview(environment.ModeDeployNew)
	if p.DowntimeRequired || len(p.Mappings) != 0 {
		t.Fatalf("deploy-new preview: %+v", p)
	}
	detail, err := h.execute(p, environment.ModeDeployNew, false)
	if err != nil || detail.Status != environment.TransitionSucceeded {
		t.Fatalf("%v %+v", err, detail)
	}
	for _, line := range h.cluster.Calls() {
		if strings.HasPrefix(line, "backup") || strings.HasPrefix(line, "restore") {
			t.Fatalf("deploy-new must not transfer data: %s", line)
		}
	}
	env := h.env()
	destTarget, destName := h.sourceDatabase()
	db, _ := h.cluster.Database(destTarget, destName)
	if len(db.Tables) != 0 || env.TargetGeneration != 1 {
		t.Fatalf("destination must start empty: %+v env=%+v", db, env)
	}
	srcTarget, srcName := h.sourceDatabaseAt(0)
	if rows, _ := h.cluster.Database(srcTarget, srcName); rows.Tables["public.jobs"] != 4 {
		t.Fatalf("source data lost: %+v", rows)
	}
	// The full Environment was redeployed even though no Score changed.
	if n := len(h.app.FakeDeploy.Applied); n < 6 {
		t.Fatalf("expected the complete environment to be applied on both targets, applies=%d", n)
	}
	if n, _ := h.cluster.ReplicasOf(h.sourceTarget(sourceEnv), "backend"); n != 0 {
		t.Fatalf("source must be quiesced after cutover, replicas=%d", n)
	}
	if h.logIndex("scale ") < h.logIndex("route") {
		t.Fatalf("deploy-new quiesces the source only after the routes moved: %v", h.cluster.Calls())
	}
}

func TestFailuresBeforeCutoverKeepTheSourceAuthoritative(t *testing.T) {
	cases := []struct {
		name    string
		inject  string
		stage   environment.TransitionStage
		migrate bool
	}{
		{"backup fails", "backup", environment.StageBackup, true},
		{"restore fails", "restore", environment.StageRestoring, true},
		{"quiesce fails", "scale", environment.StageQuiescing, true},
		{"route cutover fails", "route", environment.StageCutover, true},
		{"route cutover fails deploy-new", "route", environment.StageCutover, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			h.seedJobs(3)
			mode := environment.ModeDeployNew
			if tc.migrate {
				mode = environment.ModeMigratePostgres
			}
			before := h.env()
			p := h.preview(mode)
			if tc.inject != "route" {
				h.cluster.Inject(tc.inject, errors.New("injected "+tc.inject+" failure with row 'secret-row-value'"))
			} else {
				// Fail only the destination route; restoring the source route must still work.
				h.cluster.InjectOnce("route", errors.New("injected route failure"), 2)
			}
			detail, err := h.execute(p, mode, true)
			if err != nil {
				t.Fatal(err)
			}
			h.cluster.Inject(tc.inject, nil)
			if detail.Status != environment.TransitionFailed || detail.Stage != tc.stage || detail.SourceState != environment.SourceAuthoritative {
				t.Fatalf("detail: %+v", detail)
			}
			if strings.Contains(detail.Failure, "secret-row-value") || strings.Contains(detail.Failure, "injected") || detail.Failure == "" {
				t.Fatalf("failure must be a fixed safe sentence: %q", detail.Failure)
			}
			env := h.env()
			if env.Busy() || env.ConnectionKey != before.ConnectionKey || env.TargetGeneration != 0 || env.CurrentDeploymentSetID != before.CurrentDeploymentSetID {
				t.Fatalf("binding or Set changed: %+v", env)
			}
			if env.GenerationHigh != 1 {
				t.Fatalf("the failed attempt must consume its generation: %+v", env)
			}
			for _, workload := range []string{"backend", "worker", "frontend"} {
				if n, _ := h.cluster.ReplicasOf(h.sourceTarget(before), workload); n < 1 {
					t.Fatalf("source %s was not restored: %d (compensation=%v failed=%v)", workload, n, detail.Compensation, detail.CompensationFailed)
				}
			}
			if !h.hasRoute(before) {
				t.Fatalf("source route was not restored: %v %v", detail.Compensation, detail.CompensationFailed)
			}
			if len(detail.CompensationFailed) > 0 {
				t.Fatalf("compensation reported failures: %v", detail.CompensationFailed)
			}
			srcTarget, srcName := h.sourceDatabaseAt(0)
			if rows, _ := h.cluster.Database(srcTarget, srcName); rows.Tables["public.jobs"] != 3 {
				t.Fatalf("source rows changed: %+v", rows)
			}
			if tc.migrate && tc.stage != environment.StageQuiescing {
				stored, _ := h.app.Store.GetTransition(ctx, detail.ID)
				for _, b := range stored.Backups {
					if !b.Removed {
						t.Fatalf("a failed transition must remove its private backup or report it: %+v", b)
					}
				}
			}
			ops, _ := h.app.Store.ListOperations(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey, 1)
			if ops[0].Status != environment.OpFailed {
				t.Fatalf("operation: %+v", ops[0])
			}
		})
	}
}

func TestFailedCompensationIsReportedNeverHidden(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(2)
	p := h.preview(environment.ModeMigratePostgres)
	h.cluster.Inject("backup", errors.New("boom"))
	h.cluster.Inject("scale-up", errors.New("cannot scale"))
	detail, err := h.execute(p, environment.ModeMigratePostgres, true)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != environment.TransitionFailed || len(detail.CompensationFailed) == 0 {
		t.Fatalf("a failed restore of the writers must be visible: %+v", detail)
	}
	joined := strings.Join(detail.CompensationFailed, ";")
	if !strings.Contains(joined, "restore source workload") {
		t.Fatalf("compensation failures: %v", detail.CompensationFailed)
	}
}

func TestConcurrentExecuteHasOneWinner(t *testing.T) {
	h := newHarness(t)
	h.service.SetAsync(false)
	p := h.preview(environment.ModeDeployNew)
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, err := h.execute(p, environment.ModeDeployNew, true)
			results <- err
		}()
	}
	wins := 0
	for i := 0; i < 4; i++ {
		err := <-results
		switch {
		case err == nil:
			wins++
		case errors.Is(err, persistence.ErrEnvironmentBusy), errors.Is(err, transition.ErrStaleToken):
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d", wins)
	}
	if env := h.env(); env.TargetGeneration != 1 || env.Busy() {
		t.Fatalf("%+v", env)
	}
}
