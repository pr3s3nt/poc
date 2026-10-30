package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
)

var ErrOwnership = errors.New("fleet: controller ownership or incarnation mismatch")
var ErrController = errors.New("fleet: controller operation could not be confirmed")

// Manager writes only the exact Organization/Environment/epoch-scoped GitRepo.
// Namespace/resources created by Terraform are never submitted through this port.
type Manager struct {
	RecoveryGuard func(context.Context, port.Intent) error
	Verifier      *Verifier
	HelmPath      string
	Timeout       time.Duration
	Guard         func(context.Context, port.Intent) error
}

func (m *Manager) open(ctx context.Context, i port.Intent) (k8s.CLI, func(), Config, error) {
	if err := i.Validate(); err != nil {
		return k8s.CLI{}, func() {}, Config{}, err
	}
	cfg, err := ParseConfig(i.Binding.Config)
	if err != nil {
		return k8s.CLI{}, func() {}, cfg, err
	}
	if m.Verifier == nil || m.Verifier.Targets == nil || m.Verifier.Credentials == nil {
		return k8s.CLI{}, func() {}, cfg, ErrVerification
	}
	target, err := m.Verifier.Targets.ResolveManagementTarget(ctx, i.Binding)
	if err != nil {
		return k8s.CLI{}, func() {}, cfg, ErrVerification
	}
	cli, close, err := k8s.OpenCLI(ctx, m.Verifier.KubectlPath, m.Verifier.Credentials, target)
	return cli, close, cfg, err
}
func ownerLabels(i port.Intent, unit string) map[string]any {
	return map[string]any{"orchestrator.io/organization-id": i.Revision.Scope.OrganizationID, "orchestrator.io/environment-id": i.Revision.Scope.EnvironmentID, "orchestrator.io/delivery-unit": unit}
}
func owns(obj map[string]any, i port.Intent, unit string) bool {
	md, _ := obj["metadata"].(map[string]any)
	labels, _ := md["labels"].(map[string]any)
	for k, v := range ownerLabels(i, unit) {
		if labels[k] != v {
			return false
		}
	}
	return true
}
func (m *Manager) Submit(ctx context.Context, i port.Intent, p Package, git gitport.Receipt) (port.Receipt, error) {
	if m.Guard == nil {
		return port.Receipt{}, ErrOwnership
	}
	if git.IntentID != i.ID || git.TreeDigest != p.Publication.TreeDigest || git.Commit == "" {
		return port.Receipt{}, ErrOwnership
	}
	if err := m.Guard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	cli, close, cfg, err := m.open(ctx, i)
	if err != nil {
		return port.Receipt{}, err
	}
	defer close()
	if cfg.GitRepoMode == "EXISTING" {
		return m.submitExisting(ctx, i, p, git, cli, cfg)
	}
	current, exists, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", p.UnitName)
	if err != nil {
		return port.Receipt{}, ErrController
	}
	md := map[string]any{"name": p.UnitName, "namespace": cfg.Workspace, "labels": ownerLabels(i, p.UnitName), "annotations": map[string]any{"orchestrator.io/delivery-revision": i.Revision.ID, "orchestrator.io/delivery-digest": git.TreeDigest, "orchestrator.io/delivery-intent": i.ID}}
	spec := map[string]any{"repo": cfg.RepositoryURL, "revision": git.Commit, "paths": []any{p.Publication.Path}, "targets": []any{map[string]any{"clusterName": cfg.TargetCluster}}, "deleteNamespace": false, "keepResources": false}
	if cfg.CABundle != "" {
		spec["caBundle"] = []byte(cfg.CABundle)
	}
	if cfg.ReadSecret != "" {
		spec["clientSecretName"] = cfg.ReadSecret
	}
	obj := map[string]any{"apiVersion": "fleet.cattle.io/v1alpha1", "kind": "GitRepo", "metadata": md, "spec": spec}
	if exists {
		if !owns(current, i, p.UnitName) {
			return port.Receipt{}, ErrOwnership
		}
		old, _ := current["metadata"].(map[string]any)
		annotations, _ := old["annotations"].(map[string]any)
		observed, _ := annotations["orchestrator.io/delivery-revision"].(string)
		if observed != i.ExpectedRevision && observed != i.Revision.ID {
			return port.Receipt{}, ErrOwnership
		}
		md["resourceVersion"] = old["resourceVersion"]
	}
	payload, err := json.Marshal(obj)
	if err != nil {
		return port.Receipt{}, ErrController
	}
	if err := m.Guard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	verb := "create"
	if exists {
		verb = "replace"
	}
	if _, err = cli.Run(ctx, payload, verb, "-f", "-"); err != nil {
		return port.Receipt{}, ErrController
	}
	applied, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", p.UnitName)
	if err != nil || !found || !owns(applied, i, p.UnitName) {
		return port.Receipt{}, ErrController
	}
	appliedMD, _ := applied["metadata"].(map[string]any)
	uid, _ := appliedMD["uid"].(string)
	if uid == "" || controllerRevision(applied) != i.Revision.ID {
		return port.Receipt{}, ErrController
	}
	inventory, _ := json.Marshal(pkgObjects(p))
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: git.TreeDigest, Handle: map[string]string{"gitrepo": p.UnitName, "gitrepoUID": uid, "commit": git.Commit, "release": p.ReleaseName, "namespace": i.Target.Namespace, "releaseNamespace": cfg.ReleaseNamespace, "inventory": string(inventory)}, ObservedAt: time.Now().UTC()}, nil
}
func (m *Manager) Withdraw(ctx context.Context, i port.Intent, receipt port.Receipt) (port.WithdrawalProof, error) {
	proof := port.WithdrawalProof{Receipt: receipt}
	if m.Guard == nil || !label.MatchString(receipt.Handle["gitrepo"]) || receipt.Handle["gitrepoUID"] == "" || !label.MatchString(receipt.Handle["release"]) {
		return proof, ErrOwnership
	}
	if receipt.RevisionID != i.Revision.ID || receipt.Handle["namespace"] != i.Target.Namespace {
		return proof, ErrOwnership
	}
	cli, close, cfg, err := m.open(ctx, i)
	if err != nil {
		return proof, err
	}
	defer close()
	if receipt.Handle["releaseNamespace"] != cfg.ReleaseNamespace {
		return proof, ErrOwnership
	}
	if cfg.GitRepoMode == "EXISTING" {
		return m.withdrawExisting(ctx, i, receipt, cli, cfg)
	}
	current, exists, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", receipt.Handle["gitrepo"])
	if err != nil {
		return proof, ErrController
	}
	if exists {
		md, _ := current["metadata"].(map[string]any)
		if !owns(current, i, receipt.Handle["gitrepo"]) || md["uid"] != receipt.Handle["gitrepoUID"] {
			return proof, ErrOwnership
		}
		if err := m.Guard(ctx, i); err != nil {
			return proof, err
		}
		payload, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]any{"uid": md["uid"], "resourceVersion": md["resourceVersion"]}})
		api := "/apis/fleet.cattle.io/v1alpha1/namespaces/" + cfg.Workspace + "/gitrepos/" + receipt.Handle["gitrepo"]
		if _, err := cli.Run(ctx, payload, "delete", "--raw", api, "-f", "-"); err != nil {
			return proof, ErrController
		}
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_, found, err := cli.Get(waitCtx, cfg.Workspace, "gitrepos.fleet.cattle.io", receipt.Handle["gitrepo"])
		if err != nil {
			return proof, ErrController
		}
		if !found {
			break
		}
		select {
		case <-waitCtx.Done():
			return proof, ErrController
		case <-ticker.C:
		}
	}
	// Finalizers should have retired only this GitRepo's bundles. A missing GitRepo
	// alone never proves the downstream controller has relinquished ownership.
	raw, err := cli.Run(waitCtx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+receipt.Handle["gitrepo"], "-o", "json")
	var bundles struct {
		Items []json.RawMessage `json:"items"`
	}
	if err != nil || json.Unmarshal(raw, &bundles) != nil || len(bundles.Items) != 0 {
		return proof, ErrController
	}
	if err := m.waitReleaseGone(waitCtx, cli, cfg, i, receipt); err != nil {
		return proof, err
	}

	proof.ManagementStopped = true
	return proof, nil
}

// Reconcile never increments GitRepo forceSyncGeneration: that can rerun Job
// releases. Re-observation or publishing the admitted revision is coordinated by
// the application using the exact original receipt instead.
func (m *Manager) Reconcile(context.Context, port.Intent, port.Receipt) (port.Receipt, error) {
	return port.Receipt{}, errors.New("fleet: explicit force resync is unsupported")
}

func controllerRevision(obj map[string]any) string {
	md, _ := obj["metadata"].(map[string]any)
	a, _ := md["annotations"].(map[string]any)
	value, _ := a["orchestrator.io/delivery-revision"].(string)
	return strings.TrimSpace(value)
}

func pkgObjects(p Package) []execution.ObjectRef { return p.Objects }
