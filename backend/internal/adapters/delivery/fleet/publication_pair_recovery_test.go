package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	port "orchestrator/internal/ports/delivery"
	gitport "orchestrator/internal/ports/gitpublication"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRecoveryCompletesOnlyMissingUpdateMetadata(t *testing.T) {
	for _, test := range []string{"complete", "foreign-metadata", "foreign-native", "no-source", "changed-controller", "tampered-source"} {
		t.Run(test, func(t *testing.T) {
			p, w, dir, intent := applicationFixture(t)
			ctx := context.Background()
			old, oldPkg, receipt := publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
			if _, err := w.Publish(ctx, *oldPkg.Metadata, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			next := old
			next.ID += "-update"
			next.Revision.ID += "-update"
			next.ExpectedRevision = old.Revision.ID
			next, pkg, _ := applicationPackage(t, next)
			oldRaw, _ := json.Marshal(oldPkg)
			next.PriorSubmission = &port.PriorSubmission{Prepared: port.Prepared{Revision: old.Revision, Payload: oldRaw}, Receipt: receipt}
			ref := oldPkg.Objects[0]
			next.SourceInventory = []port.AppliedObject{{ObjectRef: ref, UID: "original-native", Release: oldPkg.ReleaseName}}
			cfg, _ := ParseConfig(old.Binding.Config)
			native := map[string]any{"metadata": map[string]any{"name": ref.Name, "namespace": old.Target.Namespace, "uid": "original-native", "annotations": map[string]any{"orchestrator.io/delivery-revision": old.Revision.ID, "meta.helm.sh/release-name": oldPkg.ReleaseName, "meta.helm.sh/release-namespace": cfg.ReleaseNamespace}, "labels": map[string]any{"orchestrator.io/managed-by": "orchestrator", "orchestrator.io/application": old.Target.Extra["application"], "orchestrator.io/environment": old.Target.Extra["environment"]}}}
			if test == "foreign-native" {
				native["metadata"].(map[string]any)["uid"] = "foreign"
			}
			body, _ := json.Marshal(native)
			os.WriteFile(filepath.Join(dir, "live-"+old.Target.Namespace+"-"+ref.Name+".json"), body, 0600)
			if test == "tampered-source" {
				next.PriorSubmission.Prepared.Revision.PayloadDigest = "tampered"
			}
			if test == "no-source" {
				next.PriorSubmission = nil
			}
			if test == "changed-controller" {
				next.PriorSubmission.Receipt.Handle = map[string]string{"gitrepoUID": "foreign"}
			}
			if _, err := w.Publish(ctx, pkg.Publication, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if test == "foreign-metadata" {
				foreign := *oldPkg.Metadata
				foreign.ID = "foreign"
				foreign.Files = map[string][]byte{"revision.json": []byte("foreign\n")}
				foreign.TreeDigest = gitport.Digest(foreign.Files)
				if _, err := w.Publish(ctx, foreign, func(context.Context) error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
			commitsBefore, _ := exec.Command("git", "-C", w.Dir, "rev-list", "--count", "HEAD").Output()
			headBefore, _ := exec.Command("git", "-C", w.Dir, "rev-parse", "HEAD").Output()
			raw, _ := json.Marshal(pkg)
			result, err := p.RecoverReceipt(ctx, next, port.Prepared{Revision: next.Revision, Payload: raw})
			if test != "complete" {
				if err == nil {
					t.Fatal("unproven recovery succeeded")
				}
				after, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
				headAfter, _ := exec.Command("git", "-C", w.Dir, "rev-parse", "HEAD").Output()
				if string(before) != string(after) || string(headBefore) != string(headAfter) {
					t.Fatal("refused recovery mutated Git/controller")
				}
				return
			}
			if err != nil || result.RevisionID != next.Revision.ID {
				t.Fatalf("recovery: %+v %v", result, err)
			}
			if _, found, err := w.ObservePublication(ctx, *pkg.Metadata); err != nil || !found {
				t.Fatal("paired metadata missing", err)
			}
			// A second request observes the admitted revision without another write.
			countBefore, _ := exec.Command("git", "-C", w.Dir, "rev-list", "--count", "HEAD").Output()
			var beforeCount, afterCount int
			fmt.Sscan(string(commitsBefore), &beforeCount)
			fmt.Sscan(string(countBefore), &afterCount)
			if afterCount != beforeCount+1 {
				t.Fatal("recovery must publish exactly one missing metadata commit")
			}
			if _, err := p.RecoverReceipt(ctx, next, port.Prepared{Revision: next.Revision, Payload: raw}); err != nil {
				t.Fatal(err)
			}
			countAfter, _ := exec.Command("git", "-C", w.Dir, "rev-list", "--count", "HEAD").Output()
			if string(countBefore) != string(countAfter) {
				t.Fatal("recovery repeated publication")
			}
		})
	}
}
