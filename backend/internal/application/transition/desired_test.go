package transition_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/transition"
	configdomain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
)

func (h *harness) saveDraft(workload string, state environment.DraftState, score map[string]any) {
	h.t.Helper()
	err := h.app.Store.SaveWorkloadDraft(context.Background(), h.env().DraftVersion, environment.WorkloadDraft{
		ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey, WorkloadID: workload, State: state, Score: score})
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) impact(p transition.Preview) map[string]string {
	out := map[string]string{}
	for _, item := range p.Workloads {
		out[item.WorkloadID] = item.Action
	}
	return out
}

func extraWorkload(image string) map[string]any {
	return map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "extra"},
		"containers": map[string]any{"main": map[string]any{"image": image}}}
}

// The destination is the COMPLETE DESIRED Environment (ADR-012): the current
// Set merged with the pinned drafts and the desired configuration revision,
// previewed for review, with every workload redeployed.
func TestTransitionDeploysTheCompleteDesiredEnvironmentAndConsumesOnlyPinnedDrafts(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedJobs(2)
	if _, err := h.app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{OrganizationKey: h.opts.OrganizationKey, ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey,
		WorkloadID: "extra", ScoreAfter: extraWorkload("example.invalid/extra:1"), Actor: "test", RunID: "run-extra"}); err != nil {
		t.Fatal(err)
	}
	// Newer configuration (desired revision is ahead of what runs) and drafts:
	// one update, one addition and one deletion.
	if _, err := h.app.Configurations.Put(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey, "MODE", configdomain.Variable, "blue", 0); err != nil {
		t.Fatal(err)
	}
	updated := extraWorkload("example.invalid/extra:2")
	h.saveDraft("extra", environment.DraftUpsert, updated)
	added := extraWorkload("example.invalid/extra2:1")
	added["metadata"] = map[string]any{"name": "extra2"}
	h.saveDraft("extra2", environment.DraftUpsert, added)
	h.saveDraft("worker", environment.DraftDelete, nil)
	draftVersion := h.env().DraftVersion

	p := h.preview(environment.ModeMigratePostgres)
	got := h.impact(p)
	want := map[string]string{"backend": "UNCHANGED", "frontend": "UNCHANGED", "extra": "UPDATED", "extra2": "ADDED", "worker": "REMOVED"}
	for id, action := range want {
		if got[id] != action {
			t.Fatalf("preview impact %s = %q, want %q (all: %v)", id, got[id], action, got)
		}
	}
	if len(p.Workloads) != 5 {
		t.Fatalf("workloads: %+v", p.Workloads)
	}
	sourceWorkerNS := h.env().Namespace()

	detail, err := h.execute(p, environment.ModeMigratePostgres, true)
	if err != nil || detail.Status != environment.TransitionSucceeded {
		t.Fatalf("%v %+v", err, detail)
	}
	env := h.env()
	set, _ := h.app.Store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
	if _, ok := set.Document.Modules["worker"]; ok || set.Document.Modules["extra2"].Spec.Containers == nil || set.Document.Modules["extra"].Spec.Containers["main"].Image != "example.invalid/extra:2" {
		t.Fatalf("committed Set is not the desired one: %v", set.Document.ModuleIDs())
	}
	// Pinned drafts are consumed like a normal Deploy; nothing is pending.
	drafts, _ := h.app.Store.ListWorkloadDrafts(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if len(drafts) != 0 || env.DraftVersion != draftVersion+3 {
		t.Fatalf("drafts after cutover: %d version %d (was %d)", len(drafts), env.DraftVersion, draftVersion)
	}
	preview, err := h.app.Pending.Preview(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if err != nil || len(preview.Changes) != 0 {
		t.Fatalf("a completed transition must leave nothing pending: %+v %v", preview.Changes, err)
	}
	// New generation instances report the desired revision they actually run.
	scope, _ := h.app.Store.GetConfigurationScope(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	instances, _ := h.app.Store.ListWorkloadInstances(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey)
	if len(instances) != 4 {
		t.Fatalf("destination instances: %+v", instances)
	}
	for _, instance := range instances {
		if instance.Generation != 1 || instance.AppliedConfigRevisionID != scope.DesiredRevisionID {
			t.Fatalf("destination instance revision: %+v want %s", instance, scope.DesiredRevisionID)
		}
	}
	// The deleted workload is only absent at the destination; the source keeps running data.
	old, _ := h.app.Store.ListWorkloadInstancesFor(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey, 0)
	hasWorker := false
	for _, instance := range old {
		hasWorker = hasWorker || instance.WorkloadID == "worker" && instance.Status == "READY"
	}
	if !hasWorker {
		t.Fatal("the source worker record was touched by a destination-side deletion")
	}
	for _, removed := range h.app.FakeDeploy.Removed {
		if removed == "worker" {
			t.Fatal("a draft delete must not remove anything from the source")
		}
	}
	_ = sourceWorkerNS
}

func TestTransitionFailurePreservesDraftsAndSourceSet(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedJobs(2)
	h.saveDraft("extra", environment.DraftUpsert, extraWorkload("example.invalid/extra:1"))
	before := h.env()
	p := h.preview(environment.ModeMigratePostgres)
	h.cluster.Inject("restore", errors.New("boom"))
	detail, err := h.execute(p, environment.ModeMigratePostgres, true)
	if err != nil || detail.Status != environment.TransitionFailed {
		t.Fatalf("%v %+v", err, detail)
	}
	after := h.env()
	drafts, _ := h.app.Store.ListWorkloadDrafts(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if len(drafts) != 1 || after.DraftVersion != before.DraftVersion || after.CurrentDeploymentSetID != before.CurrentDeploymentSetID {
		t.Fatalf("failure must preserve the pending edit and the source Set: drafts=%d %+v", len(drafts), after)
	}
}

func TestStalePreviewAfterDraftOrConfigEditIsRejectedBeforeSideEffects(t *testing.T) {
	for name, edit := range map[string]func(*harness){
		"draft": func(h *harness) {
			h.saveDraft("extra", environment.DraftUpsert, extraWorkload("example.invalid/extra:1"))
		},
		"config": func(h *harness) {
			if _, err := h.app.Configurations.Put(context.Background(), h.opts.ApplicationKey, h.opts.EnvironmentKey, "MODE", configdomain.Variable, "x", 0); err != nil {
				h.t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.seedJobs(1)
			p := h.preview(environment.ModeMigratePostgres)
			edit(h)
			calls := len(h.cluster.Calls())
			if _, err := h.execute(p, environment.ModeMigratePostgres, true); !errors.Is(err, transition.ErrStaleToken) {
				t.Fatalf("stale preview: %v", err)
			}
			for _, line := range h.cluster.Calls()[calls:] {
				if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") {
					t.Fatalf("side effect before rejection: %s", line)
				}
			}
		})
	}
}

func TestDesiredRemovalOfEveryDatabaseConsumerFailsBeforeQuiesce(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(2)
	h.saveDraft("backend", environment.DraftDelete, nil)
	h.saveDraft("worker", environment.DraftDelete, nil)
	calls := len(h.cluster.Calls())
	_, err := h.service.Preview(context.Background(), h.request(environment.ModeMigratePostgres))
	var unsupported *transition.UnsupportedError
	if !errors.As(err, &unsupported) || !strings.Contains(err.Error(), "no destination database") {
		t.Fatalf("an unmapped durable source must fail unsupported: %v", err)
	}
	for _, line := range h.cluster.Calls()[calls:] {
		if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") {
			t.Fatalf("data could be dropped silently: %s", line)
		}
	}
}
