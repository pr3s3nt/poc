package fleet

import (
	"encoding/json"
	"errors"
	"testing"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
)

func TestGitRenderGenerationIsPerIntentAndNeverReplaysExistingJobs(t *testing.T) {
	i := packageIntent()
	i.ID = "new-intent"
	current := map[string]any{"metadata": map[string]any{"annotations": map[string]any{"orchestrator.io/delivery-intent": "old-intent"}}, "spec": map[string]any{"forceSyncGeneration": float64(7), "revision": "old-commit"}}
	if next, err := gitRenderGeneration(current, true, i, "new-commit"); err != nil || next != 8 {
		t.Fatal("new intent did not request one Git render", next, err)
	}
	current["metadata"].(map[string]any)["annotations"].(map[string]any)["orchestrator.io/delivery-intent"] = i.ID
	current["spec"].(map[string]any)["revision"] = "new-commit"
	if next, err := gitRenderGeneration(current, true, i, "new-commit"); err != nil || next != 7 {
		t.Fatal("replayed intent advanced render generation", next, err)
	}
	i.Revision.Phase = domain.Job
	i.ID = "another-job-intent"
	if _, err := gitRenderGeneration(current, true, i, "new-commit"); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal("existing Job admitted for render replay", err)
	}
	if generation, err := gitRenderGeneration(nil, false, i, "first-commit"); err != nil || generation != 0 {
		t.Fatal("first Job submission was blocked", err)
	}
}

func TestManagedRepoMetadataPreservesFleetOwnership(t *testing.T) {
	i := packageIntent()
	old := map[string]any{"metadata": map[string]any{"resourceVersion": "42", "uid": "owned-uid", "finalizers": []any{"fleet.cattle.io/gitjob"}, "annotations": map[string]any{"fleet.cattle.io/client-secret-hash": "digest", "orchestrator.io/delivery-revision": "old"}, "labels": map[string]any{"fleet-owned-label": "keep"}}}
	metadata := managedRepoMetadata(old, i, "unit", "tree-digest")
	annotations := metadata["annotations"].(map[string]any)
	if annotations["fleet.cattle.io/client-secret-hash"] != "digest" || annotations["orchestrator.io/delivery-revision"] != i.Revision.ID || metadata["finalizers"] == nil || metadata["uid"] != "owned-uid" {
		t.Fatal("controller metadata was discarded")
	}
	if old["metadata"].(map[string]any)["annotations"].(map[string]any)["orchestrator.io/delivery-revision"] != "old" {
		t.Fatal("source metadata mutated")
	}
}

func TestGitRenderRecoveryPatchIsReceiptScopedAndBounded(t *testing.T) {
	i := packageIntent()
	cfg := Config{RepositoryURL: "https://github.com/team/app.git", TargetCluster: "local", ReadSecret: "reader"}
	receipt := port.Receipt{Handle: map[string]string{"gitrepoUID": "controller-uid", "commit": "admitted-sha"}}
	annotations := map[string]any{"orchestrator.io/delivery-intent": i.ID, "orchestrator.io/delivery-revision": i.Revision.ID}
	repo := map[string]any{"metadata": map[string]any{"uid": "controller-uid", "resourceVersion": "42", "annotations": annotations}, "spec": map[string]any{"repo": cfg.RepositoryURL, "revision": "admitted-sha", "paths": []any{"environments/staging/manifests/unit"}, "targets": []any{map[string]any{"clusterName": "local"}}, "clientSecretName": "reader"}}
	raw, needed, err := gitRenderRecoveryPatch(repo, cfg, i, receipt, "environments/staging/manifests/unit")
	if err != nil || !needed {
		t.Fatal("bounded recovery not prepared", err)
	}
	var patch []map[string]any
	if json.Unmarshal(raw, &patch) != nil || len(patch) != 5 || patch[2]["path"] != "/spec/revision" || patch[2]["op"] != "test" {
		t.Fatal("missing receipt revision precondition")
	}
	for _, item := range patch {
		if item["path"] == "/status" || item["path"] == "/spec/targets" {
			t.Fatal("recovery forges status or retargets")
		}
	}
	annotations[gitRenderRecoveryAnnotation] = i.ID
	if _, needed, err := gitRenderRecoveryPatch(repo, cfg, i, receipt, "environments/staging/manifests/unit"); err != nil || needed {
		t.Fatal("recovery is not bounded to one request", err)
	}
	i.Revision.Phase = domain.Job
	if _, _, err := gitRenderRecoveryPatch(repo, cfg, i, receipt, "environments/staging/manifests/unit"); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal("Job recovery render was admitted", err)
	}
}

func TestSharedRenderRecoveryIsBoundedAndRejectsJobReplay(t *testing.T) {
	i := packageIntent()
	r := port.Receipt{PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"gitrepoUID": "uid", "release": "release", "path": "payment/prerequisite"}}
	pins := map[string]unitPin{"release": {EnvironmentID: i.Revision.Scope.EnvironmentID, Revision: i.Revision.ID, Digest: r.PayloadDigest, Path: r.Handle["path"]}}
	repo := map[string]any{"metadata": map[string]any{"uid": "uid", "resourceVersion": "42", "annotations": map[string]any{}}, "spec": map[string]any{"revision": "admitted"}}
	patch, needed, err := sharedGitRenderRecoveryPatch(repo, i, r, pins)
	if err != nil || !needed || len(patch) == 0 {
		t.Fatal(needed, err)
	}
	repo["metadata"].(map[string]any)["annotations"].(map[string]any)[gitRenderRecoveryAnnotation] = i.ID + "/admitted"
	if _, needed, err := sharedGitRenderRecoveryPatch(repo, i, r, pins); err != nil || needed {
		t.Fatal("unbounded retry", err)
	}
	pins["job"] = unitPin{Path: "worker/job"}
	if _, _, err := sharedGitRenderRecoveryPatch(repo, i, r, pins); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal("Job replay accepted", err)
	}
	delete(pins, "job")
	repo["metadata"].(map[string]any)["uid"] = "other"
	if _, _, err := sharedGitRenderRecoveryPatch(repo, i, r, pins); err == nil {
		t.Fatal("foreign controller accepted")
	}
}
