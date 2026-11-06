package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	publication "orchestrator/internal/adapters/gitpublication"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
)

// Kubernetes transport fixture only. Publication uses a real local Git remote;
// this is not evidence of installed Fleet behavior.
func TestApplicationFleetTransport(t *testing.T) {
	dir := os.Getenv("ORCH_APPLICATION_FLEET_FIXTURE")
	if dir == "" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	for len(args) > 0 && args[0] != "get" && args[0] != "create" && args[0] != "replace" && args[0] != "delete" {
		args = args[1:]
	}
	fail := func() { fmt.Fprintln(os.Stderr, "NotFound"); os.Exit(1) }
	emit := func(v any) { json.NewEncoder(os.Stdout).Encode(v); os.Exit(0) }
	read := func(name string) map[string]any {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			fail()
		}
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil {
			fail()
		}
		return v
	}
	switch args[0] {
	case "create", "replace":
		raw, _ := io.ReadAll(os.Stdin)
		var repo map[string]any
		if json.Unmarshal(raw, &repo) != nil {
			os.Exit(2)
		}
		md := repo["metadata"].(map[string]any)
		if args[0] == "replace" {
			if _, err := os.Stat(filepath.Join(dir, "reject-once")); err == nil {
				os.Remove(filepath.Join(dir, "reject-once"))
				old := read("repo.json")
				omd := old["metadata"].(map[string]any)
				omd["resourceVersion"] = omd["resourceVersion"].(string) + "1"
				raw, _ := json.Marshal(old)
				os.WriteFile(filepath.Join(dir, "repo.json"), raw, 0600)
				fmt.Fprintln(os.Stderr, "Error from server (Conflict): object has been modified")
				os.Exit(1)
			}
		}
		if args[0] == "create" {
			if _, err := os.Stat(filepath.Join(dir, "repo.json")); err == nil {
				os.Exit(2)
			}
			md["uid"] = "owned-repo-uid"
			md["resourceVersion"] = "1"
		} else {
			old := read("repo.json")
			omd := old["metadata"].(map[string]any)
			if md["uid"] != omd["uid"] || md["resourceVersion"] != omd["resourceVersion"] {
				os.Exit(2)
			}
			md["resourceVersion"] = md["resourceVersion"].(string) + "1"
		}
		spec := repo["spec"].(map[string]any)
		repo["status"] = map[string]any{"commit": spec["revision"]}
		raw, _ = json.Marshal(repo)
		os.WriteFile(filepath.Join(dir, "repo.json"), raw, 0600)
		if _, err := os.Stat(filepath.Join(dir, "lose-ack")); err == nil {
			os.Remove(filepath.Join(dir, "lose-ack"))
			os.Exit(2)
		}
		emit(repo)
	case "delete":
		repo := read("repo.json")
		raw, _ := io.ReadAll(os.Stdin)
		var opts struct{ Preconditions map[string]any }
		json.Unmarshal(raw, &opts)
		md := repo["metadata"].(map[string]any)
		if opts.Preconditions["uid"] != md["uid"] || opts.Preconditions["resourceVersion"] != md["resourceVersion"] {
			os.Exit(2)
		}
		os.Remove(filepath.Join(dir, "repo.json"))
		emit(map[string]any{})
	case "get":
		if args[1] == "--raw" {
			if args[2] == "/api/v1" {
				emit(map[string]any{"resources": []any{map[string]any{"name": "configmaps", "kind": "ConfigMap", "namespaced": true, "verbs": []any{"get", "list", "delete"}}}})
			}
			parts := strings.Split(args[2], "/")
			if len(parts) == 7 {
				emit(read("live-" + parts[4] + "-" + parts[6] + ".json"))
			}
			fail()
		}
		switch args[1] {
		case "namespace":
			emit(map[string]any{"metadata": map[string]any{"uid": "physical-cluster-uid"}})
		case "gitrepos.fleet.cattle.io":
			emit(read("repo.json"))
		case "clusters.fleet.cattle.io":
			emit(map[string]any{"metadata": map[string]any{"uid": "cluster-uid"}, "status": map[string]any{"namespace": "cluster-local"}})
		case "bundledeployments.fleet.cattle.io":
			if len(args) > 2 && args[2] != "-n" {
				repo := read("repo.json")
				spec := repo["spec"].(map[string]any)
				emit(map[string]any{"metadata": map[string]any{"labels": map[string]any{"fleet.cattle.io/commit": spec["revision"]}}, "spec": map[string]any{"deploymentID": "applied"}, "status": map[string]any{"appliedDeploymentID": "applied"}})
			}
			emit(map[string]any{"items": []any{}})
		case "bundles.fleet.cattle.io":
			items := []any{}
			if _, err := os.Stat(filepath.Join(dir, "repo.json")); err == nil {
				repo := read("repo.json")
				pins, err := readUnitPins(repo)
				if err != nil {
					os.Exit(2)
				}
				spec := repo["spec"].(map[string]any)
				for release, pin := range pins {
					if !pin.Removed {
						items = append(items, map[string]any{"metadata": map[string]any{"name": release, "labels": map[string]any{"fleet.cattle.io/commit": spec["revision"], "orchestrator.io/organization-id": "org", "orchestrator.io/environment-id": pin.EnvironmentID, "orchestrator.io/delivery-unit": release}}, "spec": map[string]any{"helm": map[string]any{"releaseName": release}}})
					}
				}
			}
			emit(map[string]any{"items": items})
		case "secrets":
			if _, err := os.Stat(filepath.Join(dir, "release-secret")); err == nil {
				fmt.Print("orphan-release-uid")
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func applicationFixture(t *testing.T) (*Provider, *publication.Writer, string, func(string, string) port.Intent) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	t.Setenv("ORCH_APPLICATION_FLEET_FIXTURE", dir)
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORCH_APPLICATION_FLEET_TEST_BIN", bin)
	cli := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nexec \"$ORCH_APPLICATION_FLEET_TEST_BIN\" -test.run=^TestApplicationFleetTransport$ -- \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		raw, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git fixture: %v %s", err, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	remote := filepath.Join(dir, "remote.git")
	clone := filepath.Join(dir, "writer")
	git("init", "--bare", "--initial-branch=main", remote)
	git("clone", remote, clone)
	os.WriteFile(filepath.Join(clone, "README.md"), []byte("Application delivery\n"), 0600)
	git("-C", clone, "add", "README.md")
	git("-C", clone, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-m", "init")
	git("-C", clone, "push", "origin", "main")
	makeIntent := func(env, workload string) port.Intent {
		i := packageIntent()
		i.Binding.Config["gitRepoMode"] = "MANAGED"
		i.Binding.Config["applicationID"] = "app-id"
		i.Binding.Config["environmentKey"] = env
		i.Binding.Config["root"] = "environments/" + env
		i.ID = env + "-" + workload + "-intent"
		i.Revision.ID = env + "-" + workload + "-revision"
		i.Revision.Scope.EnvironmentID = env + "-id"
		i.Revision.Scope.UnitKey = workload + "/ROLLOUT"
		i.Target.Connection = "task-cluster"
		i.Target.Namespace = "app-" + env
		return i
	}
	i := makeIntent("staging", "backend")
	dest, _ := (&Provider{}).PublicationDestination(i.Binding)
	writer, err := publication.New(clone, dest, nil)
	if err != nil {
		t.Fatal(err)
	}
	guard := func(context.Context, port.Intent) error { return nil }
	provider := &Provider{Manager: &Manager{Guard: guard, RecoveryGuard: guard, Timeout: time.Second, Verifier: &Verifier{KubectlPath: cli, Credentials: fixtureCredentials{[]byte("fixture")}, Targets: fixtureTargets{}}}, Publishers: fixturePublisher{writer}}
	return provider, writer, dir, makeIntent
}
func applicationPackage(t *testing.T, i port.Intent) (port.Intent, Package, execution.Manifest) {
	t.Helper()
	m := configMap()
	m.Name = strings.Split(i.Revision.Scope.UnitKey, "/")[0] + "-config"
	m.Object["metadata"].(map[string]any)["name"] = m.Name
	m.Namespace = i.Target.Namespace
	m.Object["metadata"].(map[string]any)["namespace"] = m.Namespace
	pkg, err := PackageUnit(i, []execution.Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	i.Revision.PayloadDigest = pkg.Publication.TreeDigest
	return i, pkg, m
}
func publishApplicationUnit(t *testing.T, p *Provider, w *publication.Writer, dir string, i port.Intent) (port.Intent, Package, port.Receipt) {
	t.Helper()
	i, pkg, m := applicationPackage(t, i)
	git, err := w.Publish(context.Background(), pkg.Publication, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := w.Publish(context.Background(), *pkg.Metadata, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	git.Commit = metadata.Commit
	r, err := p.submitApplication(context.Background(), i, pkg, git)
	if err != nil {
		t.Fatal(err)
	}
	md := m.Object["metadata"].(map[string]any)
	md["uid"] = "live-uid"
	md["annotations"] = map[string]any{"meta.helm.sh/release-name": r.Handle["release"], "orchestrator.io/delivery-revision": i.Revision.ID}
	raw, _ := json.Marshal(m.Object)
	if err := os.WriteFile(filepath.Join(dir, "live-"+m.Namespace+"-"+m.Name+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return i, pkg, r
}
func TestApplicationControllerSharesEnvironmentsAndPreservesSiblingOnWithdrawal(t *testing.T) {
	for _, name := range []string{"", "idp-nangcap-order-management-deploy-abc234"} {
		t.Run("repository="+name, func(t *testing.T) { testApplicationControllerLifecycle(t, name) })
	}
}

func testApplicationControllerLifecycle(t *testing.T, name string) {
	t.Helper()
	p, w, dir, makeIntent := applicationFixture(t)
	intent := func(env, workload string) port.Intent {
		i := makeIntent(env, workload)
		if name != "" {
			i.Binding.Config["applicationRepoName"] = name
		}
		return i
	}
	ctx := context.Background()
	staging, pkg, sr := publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	prod, _, pr := publishApplicationUnit(t, p, w, dir, intent("production", "backend"))
	worker, _, wr := publishApplicationUnit(t, p, w, dir, intent("staging", "worker"))
	if sr.Handle["gitrepo"] != pr.Handle["gitrepo"] || sr.Handle["gitrepoUID"] != wr.Handle["gitrepoUID"] {
		t.Fatal("controller not shared")
	}
	if name != "" && sr.Handle["gitrepo"] != name {
		t.Fatalf("Fleet name %q does not match Git repository %q", sr.Handle["gitrepo"], name)
	}
	if pkg.Publication.Path != "environments/staging/backend/rollout" || pkg.Metadata.Path != "metadata/staging/backend/rollout" {
		t.Fatal("unreadable publication layout")
	}
	for _, unit := range []struct {
		i port.Intent
		r port.Receipt
	}{{staging, sr}, {prod, pr}, {worker, wr}} {
		observed, err := p.Observe(ctx, unit.i, unit.r)
		if err != nil || !observed.Ready(unit.i.Revision, time.Now(), time.Minute) {
			t.Fatalf("unit at advanced shared SHA not ready: %+v %v", observed, err)
		}
	}
	removal := staging
	removal.ID = "withdraw-staging"
	proof, err := p.Withdraw(ctx, removal, sr)
	if err != nil || !proof.ManagementStopped {
		t.Fatal("unit withdrawal", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "repo.json")); err != nil {
		t.Fatal("sibling controller deleted")
	}
	observed, err := p.Observe(ctx, prod, pr)
	if err != nil || !observed.Ready(prod.Revision, time.Now(), time.Minute) {
		t.Fatal("production invalidated by staging withdrawal", err)
	}
	if proof, err := p.Withdraw(ctx, removal, sr); err != nil || !proof.ManagementStopped {
		t.Fatal("withdrawal not idempotent", err)
	}
	removal = worker
	removal.ID = "withdraw-worker"
	if _, err := p.Withdraw(ctx, removal, wr); err != nil {
		t.Fatal(err)
	}
	removal = prod
	removal.ID = "withdraw-production"
	if _, err := p.Withdraw(ctx, removal, pr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "repo.json")); !os.IsNotExist(err) {
		t.Fatal("last-unit controller not removed")
	}
	if _, err := p.Withdraw(ctx, removal, pr); err != nil {
		t.Fatal("last-unit cleanup replay", err)
	}
}
func TestApplicationControllerRejectsUnapprovedSiblingTrees(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	ctx := context.Background()
	_, pkg, _ := publishApplicationUnit(t, p, w, dir, intent("production", "backend"))
	tamper := pkg.Publication
	tamper.ID = "foreign"
	tamper.Files = map[string][]byte{"foreign.yaml": []byte("kind: ConfigMap\n")}
	tamper.TreeDigest = gitport.Digest(tamper.Files)
	if _, err := w.Publish(ctx, tamper, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	next, np, _ := applicationPackage(t, intent("staging", "worker"))
	git, err := w.Publish(ctx, np.Publication, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.submitApplication(ctx, next, np, git); err != ErrOwnership {
		t.Fatal("unapproved sibling admitted", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if string(before) != string(after) {
		t.Fatal("shared pointer changed after failed validation")
	}
}
func TestApplicationControllerConvergesAfterLostWriteAcknowledgement(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	ctx := context.Background()
	i, pkg, _ := applicationPackage(t, intent("staging", "backend"))
	git, err := w.Publish(ctx, pkg.Publication, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "lose-ack"), nil, 0600)
	if _, err := p.submitApplication(ctx, i, pkg, git); err != ErrController {
		t.Fatal("lost ack not reported", err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	raw, _ := json.Marshal(pkg)
	prepared := port.Prepared{Revision: i.Revision, Payload: raw}
	r, err := p.RecoverReceipt(ctx, i, prepared)
	if err != nil || r.Handle["gitrepoUID"] != "owned-repo-uid" {
		t.Fatal("lost acknowledgement not observable", err)
	}
	if _, err := p.RecoverSubmission(ctx, i, prepared, r, port.TransferSource{}); err != nil {
		t.Fatal("submission recovery", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if string(before) != string(after) {
		t.Fatal("recovery changed shared controller")
	}
}
func TestApplicationControllerIdentityAndLegacyBoundary(t *testing.T) {
	p, _, _, intent := applicationFixture(t)
	a, b := intent("staging", "backend"), intent("production", "worker")
	ca, _ := ParseConfig(a.Binding.Config)
	cb, _ := ParseConfig(b.Binding.Config)
	if applicationRepoName(a, ca) != applicationRepoName(b, cb) {
		t.Fatal("identity depends on phase/environment")
	}
	cb.ApplicationID = "other-app"
	if applicationRepoName(a, ca) == applicationRepoName(b, cb) {
		t.Fatal("cross-application collision")
	}
	if p.ValidateDeliveryReceipt(a.Binding, port.Receipt{Handle: map[string]string{}}) != ErrLegacyApplication {
		t.Fatal("legacy live layout silently adopted")
	}
}

func TestApplicationControllerAdvancesOnlyRequestedUnit(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	ctx := context.Background()
	prod, _, pr := publishApplicationUnit(t, p, w, dir, intent("production", "backend"))
	stage, _, sr := publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	stage.ExpectedRevision = stage.Revision.ID
	stage.ID += "-v2"
	stage.Revision.ID += "-v2"
	_, _, updated := publishApplicationUnit(t, p, w, dir, stage)
	if updated.Handle["gitrepoUID"] != sr.Handle["gitrepoUID"] || updated.Handle["commit"] == sr.Handle["commit"] {
		t.Fatal("controller identity or pointer incorrect")
	}
	same, err := w.VerifyTree(ctx, "main", pr.Handle["path"], updated.Handle["commit"], pr.PayloadDigest)
	if err != nil || !same {
		t.Fatal("production was modified", err)
	}
	observed, err := p.Observe(ctx, prod, pr)
	if err != nil || !observed.Ready(prod.Revision, time.Now(), time.Minute) {
		t.Fatal("production no longer ready", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	var repo map[string]any
	json.Unmarshal(raw, &repo)
	if _, forced := repo["spec"].(map[string]any)["forceSyncGeneration"]; forced {
		t.Fatal("unit update force-synced sibling Jobs")
	}
	// A replaced controller with the same name cannot be used for observation or deletion.
	repo["metadata"].(map[string]any)["uid"] = "foreign-uid"
	raw, _ = json.Marshal(repo)
	os.WriteFile(filepath.Join(dir, "repo.json"), raw, 0600)
	if _, err := p.Observe(ctx, prod, pr); err != ErrOwnership {
		t.Fatal("replaced controller accepted", err)
	}
	if _, err := p.Withdraw(ctx, prod, pr); err != ErrOwnership {
		t.Fatal("replaced controller deleted", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if string(after) != string(raw) {
		t.Fatal("foreign controller mutated")
	}
}

func TestApplicationSubmitPublishesManifestAndMetadataAndRejectsPathTakeover(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	ctx := context.Background()
	i, pkg, _ := applicationPackage(t, intent("staging", "backend"))
	raw, _ := json.Marshal(pkg)
	r, err := p.Submit(ctx, i, port.Prepared{Revision: i.Revision, Payload: raw})
	if err != nil {
		t.Fatal(err)
	}
	for _, publication := range []gitport.Intent{pkg.Publication, *pkg.Metadata} {
		same, err := w.VerifyTree(ctx, "main", publication.Path, r.Handle["commit"], publication.TreeDigest)
		if err != nil || !same {
			t.Fatal("common commit lacks admitted manifest or metadata", err)
		}
	}
	before, _ := exec.Command("git", "--git-dir", filepath.Join(dir, "remote.git"), "rev-parse", "main").Output()
	takeover := i
	takeover.ID += "-new-epoch"
	takeover.Revision.ID += "-new-epoch"
	takeover.Revision.Scope.Epoch++
	takeover, tpkg, _ := applicationPackage(t, takeover)
	raw, _ = json.Marshal(tpkg)
	if _, err := p.Submit(ctx, takeover, port.Prepared{Revision: takeover.Revision, Payload: raw}); err != ErrOwnership {
		t.Fatal("new epoch overwrote active readable path", err)
	}
	after, _ := exec.Command("git", "--git-dir", filepath.Join(dir, "remote.git"), "rev-parse", "main").Output()
	if string(before) != string(after) {
		t.Fatal("path conflict mutated Git before rejection")
	}
}

func TestApplicationControllerRetriesStatusWriteConflict(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	i, pkg, _ := applicationPackage(t, intent("production", "frontend"))
	git, err := w.Publish(context.Background(), pkg.Publication, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "reject-once"), nil, 0600)
	if _, err = p.submitApplication(context.Background(), i, pkg, git); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	var repo map[string]any
	json.Unmarshal(raw, &repo)
	pins, err := readUnitPins(repo)
	if err != nil || len(pins) != 2 {
		t.Fatal("lost sibling during retry", err)
	}
}
func TestApplicationRecoversPublishedButUnadmittedNewUnit(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	i, pkg, _ := applicationPackage(t, intent("production", "frontend"))
	ctx := context.Background()
	for _, pub := range []gitport.Intent{pkg.Publication, *pkg.Metadata} {
		if _, err := w.Publish(ctx, pub, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	headBefore, err := exec.Command("git", "-C", w.Dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pkg)
	prepared := port.Prepared{Revision: i.Revision, Payload: raw}
	r, err := p.RecoverReceipt(ctx, i, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if r.IntentID != i.ID || r.Handle["gitrepoUID"] != "owned-repo-uid" {
		t.Fatal("wrong recovery receipt", r)
	}
	headAfter, _ := exec.Command("git", "-C", w.Dir, "rev-parse", "HEAD").Output()
	if string(headBefore) != string(headAfter) {
		t.Fatal("recovery republished Git")
	}
	repoRaw, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	var repo map[string]any
	json.Unmarshal(repoRaw, &repo)
	pins, _ := readUnitPins(repo)
	if len(pins) != 2 {
		t.Fatal("recovery changed sibling")
	}
	// Reconstructing the now admitted acknowledgement is strictly read-only.
	if _, err = p.RecoverReceipt(ctx, i, prepared); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if string(again) != string(repoRaw) {
		t.Fatal("repeated recovery wrote controller")
	}
}
func TestApplicationRecoveryRequiresExactMetadataAndNoNativeAdoption(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	i, pkg, _ := applicationPackage(t, intent("production", "frontend"))
	ctx := context.Background()
	w.Publish(ctx, pkg.Publication, func(context.Context) error { return nil })
	raw, _ := json.Marshal(pkg)
	prepared := port.Prepared{Revision: i.Revision, Payload: raw}
	before, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if _, err := p.RecoverReceipt(ctx, i, prepared); err == nil {
		t.Fatal("missing metadata admitted")
	}
	w.Publish(ctx, *pkg.Metadata, func(context.Context) error { return nil })
	live := map[string]any{"metadata": map[string]any{"uid": "foreign"}}
	body, _ := json.Marshal(live)
	os.WriteFile(filepath.Join(dir, "live-app-production-frontend-config.json"), body, 0600)
	if _, err := p.RecoverReceipt(ctx, i, prepared); err == nil {
		t.Fatal("foreign native object adopted")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if string(before) != string(after) {
		t.Fatal("failed recovery changed controller")
	}
}

func TestApplicationAdmissionRequiresTheObservedControllerIncarnation(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	i, pkg, _ := applicationPackage(t, intent("production", "frontend"))
	git, err := w.Publish(context.Background(), pkg.Publication, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if _, err = p.submitApplicationAttempt(context.Background(), i, pkg, git, 0, "different-uid"); err != ErrOwnership {
		t.Fatal("foreign controller accepted", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	if string(before) != string(after) {
		t.Fatal("foreign controller changed")
	}
	os.Remove(filepath.Join(dir, "repo.json"))
	if _, err = p.submitApplicationAttempt(context.Background(), i, pkg, git, 0, "owned-repo-uid"); err != ErrOwnership {
		t.Fatal("missing controller recreated", err)
	}
	if _, err = os.Stat(filepath.Join(dir, "repo.json")); !os.IsNotExist(err) {
		t.Fatal("controller recreated during recovery")
	}
}

func TestApplicationWithdrawalRetriesStatusConflictWithoutRepublishing(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	i, _, r := publishApplicationUnit(t, p, w, dir, intent("staging", "backend"))
	publishApplicationUnit(t, p, w, dir, intent("production", "frontend"))
	beforeRaw, _ := exec.Command("git", "-C", w.Dir, "rev-list", "--count", "HEAD").Output()
	os.WriteFile(filepath.Join(dir, "reject-once"), nil, 0600)
	proof, err := p.Withdraw(context.Background(), i, r)
	if err != nil || !proof.ManagementStopped {
		t.Fatal("withdrawal conflict", err)
	}
	afterRaw, _ := exec.Command("git", "-C", w.Dir, "rev-list", "--count", "HEAD").Output()
	var before, after int
	fmt.Sscan(string(beforeRaw), &before)
	fmt.Sscan(string(afterRaw), &after)
	if after != before+1 {
		t.Fatalf("withdrawal republished Git: %d -> %d", before, after)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "repo.json"))
	var repo map[string]any
	json.Unmarshal(raw, &repo)
	pins, _ := readUnitPins(repo)
	if len(sharedPaths(pins)) != 1 || !pins[r.Handle["release"]].Removed {
		t.Fatal("withdrawal changed sibling pins")
	}
}

func TestApplicationFirstPrerequisiteAbsenceRecoveryIsReadOnlyAndConservative(t *testing.T) {
	p, w, dir, intent := applicationFixture(t)
	i := intent("staging", "backend")
	i.Revision.Phase = domain.Prerequisite
	i.Revision.Scope.UnitKey = "backend/PREREQUISITE"
	i, pkg, _ := applicationPackage(t, i)
	raw, _ := json.Marshal(pkg)
	prepared := port.Prepared{Revision: i.Revision, Payload: raw}
	ctx := context.Background()
	absent, err := p.ObserveSubmissionAbsent(ctx, i, prepared)
	if err != nil || !absent {
		t.Fatal("exact absent prerequisite", absent, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "repo.json")); !os.IsNotExist(err) {
		t.Fatal("absence observation created a controller")
	}
	os.WriteFile(filepath.Join(dir, "release-secret"), nil, 0600)
	if absent, err = p.ObserveSubmissionAbsent(ctx, i, prepared); err != nil || absent {
		t.Fatal("orphan release treated as absent", absent, err)
	}
	os.Remove(filepath.Join(dir, "release-secret"))
	w.Publish(ctx, pkg.Publication, func(context.Context) error { return nil })
	if absent, err = p.ObserveSubmissionAbsent(ctx, i, prepared); err != nil || absent {
		t.Fatal("published unit treated as absent", absent, err)
	}
	i.Revision.Phase = domain.Job
	if absent, err = p.ObserveSubmissionAbsent(ctx, i, prepared); err != nil || absent {
		t.Fatal("Job absence inferred", absent, err)
	}
}
