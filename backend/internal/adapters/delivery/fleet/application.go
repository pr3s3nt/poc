package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	k8s "orchestrator/internal/adapters/kubernetes"
	"path"
	"sort"
	"strings"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	gitport "orchestrator/internal/ports/gitpublication"
)

const applicationLayout = "APPLICATION"
const unitPinsAnnotation = "orchestrator.io/unit-pins"

type unitPin struct {
	Path          string `json:"path"`
	EnvironmentID string `json:"environmentId"`
	Revision      string `json:"revision"`
	Intent        string `json:"intent"`
	Digest        string `json:"digest"`
	Removed       bool   `json:"removed,omitempty"`
}

func unitReleaseName(scope domain.Scope) string {
	raw, _ := json.Marshal(scope)
	sum := sha256.Sum256(raw)
	return "orch-" + hex.EncodeToString(sum[:])[:24]
}

func applicationRepoName(i port.Intent, cfg Config) string {
	if cfg.ApplicationRepoName != "" {
		return cfg.ApplicationRepoName
	}
	sum := sha256.Sum256([]byte(i.Binding.OrganizationID + "\x00" + cfg.ApplicationID))
	return "orch-app-" + hex.EncodeToString(sum[:12])
}
func sharedUnitPath(i port.Intent) (string, error) {
	parts := strings.Split(i.Revision.Scope.UnitKey, "/")
	if len(parts) == 2 && parts[0] == "_routes" {
		parts[0] = "routes"
	}
	if len(parts) != 2 || !validObjectName(parts[0]) || !strings.EqualFold(parts[1], string(i.Revision.Phase)) {
		return "", ErrConfig
	}
	return path.Join(parts[0], strings.ToLower(string(i.Revision.Phase))), nil
}
func applicationLabels(i port.Intent, cfg Config) map[string]any {
	return map[string]any{"orchestrator.io/organization-id": i.Binding.OrganizationID, "orchestrator.io/application-id": cfg.ApplicationID}
}
func applicationOwns(repo map[string]any, i port.Intent, cfg Config) bool {
	md, _ := repo["metadata"].(map[string]any)
	labels, _ := md["labels"].(map[string]any)
	for k, v := range applicationLabels(i, cfg) {
		if labels[k] != v {
			return false
		}
	}
	spec, _ := repo["spec"].(map[string]any)
	targets, _ := spec["targets"].([]any)
	if len(targets) != 1 {
		return false
	}
	target, _ := targets[0].(map[string]any)
	secret, _ := spec["clientSecretName"].(string)
	pins, err := readUnitPins(repo)
	if err != nil {
		return false
	}
	paths, _ := spec["paths"].([]any)
	if len(paths) == 0 || len(paths) != len(sharedPaths(pins)) {
		return false
	}
	wanted := map[any]bool{}
	for _, p := range sharedPaths(pins) {
		wanted[p] = true
	}
	for _, p := range paths {
		s, ok := p.(string)
		if !ok || !wanted[s] {
			return false
		}
		delete(wanted, s)
	}
	return md["name"] == applicationRepoName(i, cfg) && spec["repo"] == cfg.RepositoryURL && target["clusterName"] == cfg.TargetCluster && len(target) == 1 && secret == cfg.ReadSecret
}
func readUnitPins(repo map[string]any) (map[string]unitPin, error) {
	md, _ := repo["metadata"].(map[string]any)
	a, _ := md["annotations"].(map[string]any)
	raw, _ := a[unitPinsAnnotation].(string)
	pins := map[string]unitPin{}
	if raw == "" || json.Unmarshal([]byte(raw), &pins) != nil || pins == nil {
		return nil, ErrOwnership
	}
	return pins, nil
}
func sharedReceiptMatches(pins map[string]unitPin, i port.Intent, r port.Receipt) bool {
	pin, ok := pins[r.Handle["release"]]
	return ok && !pin.Removed && pin.Path == r.Handle["path"] && pin.EnvironmentID == i.Revision.Scope.EnvironmentID && pin.Revision == i.Revision.ID && pin.Digest == r.PayloadDigest
}
func sharedPaths(pins map[string]unitPin) []any {
	paths := []string{}
	for _, pin := range pins {
		if !pin.Removed {
			paths = append(paths, pin.Path)
		}
	}
	sort.Strings(paths)
	out := make([]any, len(paths))
	for n, p := range paths {
		out[n] = p
	}
	return out
}
func (p *Provider) verifySharedTrees(ctx context.Context, i port.Intent, commit string, pins map[string]unitPin) error {
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return err
	}
	trees := map[string]string{}
	for _, pin := range pins {
		if !pin.Removed {
			trees[pin.Path] = pin.Digest
		}
	}
	if len(trees) == 0 {
		return nil
	}
	if verifier, ok := publisher.(gitport.TreeSetVerifier); ok {
		same, err := verifier.VerifyTrees(ctx, i.Binding.Config["branch"], commit, trees)
		if err != nil {
			return err
		}
		if !same {
			return ErrOwnership
		}
		return nil
	}
	verifier, ok := publisher.(gitport.TreeVerifier)
	if !ok {
		return ErrConfig
	}
	for _, pin := range pins {
		if pin.Removed {
			continue
		}
		same, err := verifier.VerifyTree(ctx, i.Binding.Config["branch"], pin.Path, commit, pin.Digest)
		if err != nil {
			return err
		}
		if !same {
			return ErrOwnership
		}
	}
	return nil
}
func sharedMetadata(repo map[string]any, i port.Intent, cfg Config, pins map[string]unitPin) (map[string]any, error) {
	md := managedRepoMetadata(repo, i, applicationRepoName(i, cfg), i.Revision.PayloadDigest)
	labels, _ := md["labels"].(map[string]any)
	delete(labels, "orchestrator.io/environment-id")
	delete(labels, "orchestrator.io/delivery-unit")
	for k, v := range applicationLabels(i, cfg) {
		labels[k] = v
	}
	raw, err := json.Marshal(pins)
	if err != nil || len(raw) > 180000 {
		return nil, ErrConfig
	}
	a, _ := md["annotations"].(map[string]any)
	a[unitPinsAnnotation] = string(raw)
	md["namespace"] = cfg.Workspace
	return md, nil
}
func sharedSpec(cfg Config, commit string, pins map[string]unitPin) map[string]any {
	spec := map[string]any{"repo": cfg.RepositoryURL, "revision": commit, "paths": sharedPaths(pins), "targets": []any{map[string]any{"clusterName": cfg.TargetCluster}}, "deleteNamespace": false, "keepResources": false}
	if cfg.ReadSecret != "" {
		spec["clientSecretName"] = cfg.ReadSecret
	}
	if cfg.CABundle != "" {
		spec["caBundle"] = []byte(cfg.CABundle)
	}
	return spec
}
func (p *Provider) submitApplication(ctx context.Context, i port.Intent, pkg Package, git gitport.Receipt) (port.Receipt, error) {
	return p.submitApplicationAttempt(ctx, i, pkg, git, 0, "")
}
func (p *Provider) submitApplicationAttempt(ctx context.Context, i port.Intent, pkg Package, git gitport.Receipt, attempt int, requiredUID string) (port.Receipt, error) {
	m := p.Manager
	cli, close, cfg, err := m.open(ctx, i)
	if err != nil {
		return port.Receipt{}, err
	}
	defer close()
	if err := m.ensureReader(ctx, i, cli, cfg); err != nil {
		return port.Receipt{}, err
	}
	current, exists, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	if err != nil {
		return port.Receipt{}, ErrController
	}
	if requiredUID != "" {
		md, _ := current["metadata"].(map[string]any)
		if !exists || md["uid"] != requiredUID {
			return port.Receipt{}, ErrOwnership
		}
	}
	pins := map[string]unitPin{}
	if exists {
		if !applicationOwns(current, i, cfg) {
			return port.Receipt{}, ErrOwnership
		}
		pins, err = readUnitPins(current)
		if err != nil {
			return port.Receipt{}, err
		}
	}
	for release, pin := range pins {
		if !pin.Removed && pin.Path == pkg.Publication.Path && release != pkg.ReleaseName {
			return port.Receipt{}, ErrOwnership
		}
	}
	if old, ok := pins[pkg.ReleaseName]; ok {
		if (old.Removed && i.ExpectedRevision != "") || old.EnvironmentID != i.Revision.Scope.EnvironmentID || old.Path != pkg.Publication.Path || (!old.Removed && old.Revision != i.ExpectedRevision && old.Revision != i.Revision.ID) {
			return port.Receipt{}, ErrOwnership
		}
	}
	pins[pkg.ReleaseName] = unitPin{Path: pkg.Publication.Path, EnvironmentID: i.Revision.Scope.EnvironmentID, Revision: i.Revision.ID, Intent: i.ID, Digest: git.TreeDigest}
	if err := p.verifySharedTrees(ctx, i, git.Commit, pins); err != nil {
		return port.Receipt{}, err
	}
	md, err := sharedMetadata(current, i, cfg, pins)
	if err != nil {
		return port.Receipt{}, err
	}
	obj := map[string]any{"apiVersion": "fleet.cattle.io/v1alpha1", "kind": "GitRepo", "metadata": md, "spec": sharedSpec(cfg, git.Commit, pins)}
	payload, _ := json.Marshal(obj)
	if err := m.Guard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	verb := "create"
	if exists {
		verb = "replace"
	}
	if _, err := cli.Run(ctx, payload, verb, "-f", "-"); err != nil {
		if exists && errors.Is(err, k8s.ErrRequestRejected) && attempt < 2 {
			md, _ := current["metadata"].(map[string]any)
			uid, _ := md["uid"].(string)
			if uid == "" {
				return port.Receipt{}, ErrOwnership
			}
			return p.submitApplicationAttempt(ctx, i, pkg, git, attempt+1, uid)
		}
		return port.Receipt{}, ErrController
	}
	applied, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	if err != nil || !found || !applicationOwns(applied, i, cfg) {
		return port.Receipt{}, ErrController
	}
	appliedMD, _ := applied["metadata"].(map[string]any)
	uid, _ := appliedMD["uid"].(string)
	inventory, _ := json.Marshal(pkgObjects(pkg))
	r := port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: git.TreeDigest, Handle: map[string]string{"layout": applicationLayout, "gitrepo": applicationRepoName(i, cfg), "gitrepoUID": uid, "commit": git.Commit, "path": pkg.Publication.Path, "release": pkg.ReleaseName, "namespace": i.Target.Namespace, "releaseNamespace": cfg.ReleaseNamespace, "inventory": string(inventory)}, ObservedAt: time.Now().UTC()}
	confirmed, e := readUnitPins(applied)
	if e != nil || uid == "" || !sharedReceiptMatches(confirmed, i, r) {
		return port.Receipt{}, ErrController
	}
	return r, nil
}

// Remove only this Bundle publication. The repository-wide lease serializes
// pointer changes with both Environment writers; sibling trees are verified.
func (p *Provider) withdrawApplication(ctx context.Context, i port.Intent, r port.Receipt) (port.WithdrawalProof, error) {
	return p.withdrawApplicationAttempt(ctx, i, r, 0)
}
func (p *Provider) withdrawApplicationAttempt(ctx context.Context, i port.Intent, r port.Receipt, attempt int) (port.WithdrawalProof, error) {
	proof := port.WithdrawalProof{Receipt: r}
	m := p.Manager
	if m == nil || m.Guard == nil || p.Publishers == nil || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest || r.Handle["namespace"] != i.Target.Namespace {
		return proof, ErrOwnership
	}
	cli, close, cfg, err := m.open(ctx, i)
	if err != nil {
		return proof, err
	}
	defer close()
	relative, err := sharedUnitPath(i)
	if err != nil || r.Handle["path"] != path.Join(cfg.Root, relative) || r.Handle["gitrepo"] != applicationRepoName(i, cfg) || r.Handle["gitrepoUID"] == "" || r.Handle["releaseNamespace"] != cfg.ReleaseNamespace {
		return proof, ErrOwnership
	}
	current, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
	if err != nil {
		return proof, ErrController
	}
	if found {
		md, _ := current["metadata"].(map[string]any)
		if !applicationOwns(current, i, cfg) || md["uid"] != r.Handle["gitrepoUID"] {
			return proof, ErrOwnership
		}
		pins, err := readUnitPins(current)
		if err != nil {
			return proof, err
		}
		pin, ok := pins[r.Handle["release"]]
		if !ok || pin.Path != r.Handle["path"] || pin.EnvironmentID != i.Revision.Scope.EnvironmentID || pin.Revision != i.Revision.ID || pin.Digest != r.PayloadDigest {
			return proof, ErrOwnership
		}
		pin.Removed = true
		pins[r.Handle["release"]] = pin
		publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
		if err != nil {
			return proof, err
		}
		dest := sha256.Sum256([]byte(cfg.RepositoryURL + "\x00" + cfg.Branch))
		deletion := gitport.Intent{ID: i.ID, DestinationKey: hex.EncodeToString(dest[:]), Branch: cfg.Branch, Path: r.Handle["path"], Files: map[string][]byte{}, TreeDigest: gitport.Digest(nil), Delete: true}
		git, err := publisher.Publish(ctx, deletion, func(ctx context.Context) error { return m.Guard(ctx, i) })
		if err != nil {
			return proof, err
		}
		if err := p.verifySharedTrees(ctx, i, git.Commit, pins); err != nil {
			return proof, err
		}
		if err := m.Guard(ctx, i); err != nil {
			return proof, err
		}
		if len(sharedPaths(pins)) == 0 {
			payload, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]any{"uid": md["uid"], "resourceVersion": md["resourceVersion"]}})
			api := "/apis/fleet.cattle.io/v1alpha1/namespaces/" + cfg.Workspace + "/gitrepos/" + r.Handle["gitrepo"]
			if _, err := cli.Run(ctx, payload, "delete", "--raw", api, "-f", "-"); err != nil {
				if errors.Is(err, k8s.ErrRequestRejected) && attempt < 2 {
					return p.withdrawApplicationAttempt(ctx, i, r, attempt+1)
				}
				return proof, ErrController
			}
		} else {
			md, err := sharedMetadata(current, i, cfg, pins)
			if err != nil {
				return proof, err
			}
			payload, _ := json.Marshal(map[string]any{"apiVersion": "fleet.cattle.io/v1alpha1", "kind": "GitRepo", "metadata": md, "spec": sharedSpec(cfg, git.Commit, pins)})
			if _, err := cli.Run(ctx, payload, "replace", "-f", "-"); err != nil {
				if errors.Is(err, k8s.ErrRequestRejected) && attempt < 2 {
					return p.withdrawApplicationAttempt(ctx, i, r, attempt+1)
				}
				return proof, ErrController
			}
		}
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := m.Guard(wait, i); err != nil {
			return proof, err
		}
		repo, exists, err := cli.Get(wait, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
		if err != nil {
			return proof, ErrController
		}
		if exists {
			md, _ := repo["metadata"].(map[string]any)
			if md["uid"] != r.Handle["gitrepoUID"] || !applicationOwns(repo, i, cfg) {
				return proof, ErrOwnership
			}
		}
		raw, err := cli.Run(wait, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
		var bundles struct{ Items []map[string]any }
		if err != nil || json.Unmarshal(raw, &bundles) != nil {
			return proof, ErrController
		}
		live := false
		for _, b := range bundles.Items {
			if bundleRelease(b) == r.Handle["release"] {
				live = true
			}
		}
		// Final-unit deletion must finish its GitRepo finalizers as well.
		deleting := false
		if exists {
			md, _ := repo["metadata"].(map[string]any)
			deleting = md["deletionTimestamp"] != nil
		}
		if !live && !deleting {
			break
		}
		select {
		case <-wait.Done():
			return proof, ErrController
		case <-ticker.C:
		}
	}
	if err := m.waitReleaseGone(wait, cli, cfg, i, r); err != nil {
		return proof, err
	}
	proof.ManagementStopped = true
	return proof, nil
}

func selectRelease(bundles []map[string]any, release string) []map[string]any {
	out := []map[string]any{}
	for _, b := range bundles {
		if bundleRelease(b) == release {
			out = append(out, b)
		}
	}
	return out
}

// RecoverReceipt observes an admitted acknowledgement without writing. An exact
// remotely published first unit may finish admission under explicit recovery;
// neither path republishes Git or adopts a different controller incarnation.
func (p *Provider) RecoverReceipt(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	if p.Manager == nil || p.Manager.RecoveryGuard == nil || p.Publishers == nil {
		return port.Receipt{}, ErrOwnership
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil || pkg.Publication.ID != i.ID || pkg.Publication.TreeDigest != i.Revision.PayloadDigest || gitport.Digest(pkg.Publication.Files) != i.Revision.PayloadDigest {
		return port.Receipt{}, ErrOwnership
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return port.Receipt{}, err
	}
	defer close()
	if cfg.ApplicationID == "" {
		return port.Receipt{}, ErrOwnership
	}
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	if err != nil || !found {
		return port.Receipt{}, ErrController
	}
	if !applicationOwns(repo, i, cfg) {
		return port.Receipt{}, ErrOwnership
	}
	pins, err := readUnitPins(repo)
	if err != nil {
		return port.Receipt{}, err
	}
	pin, found := pins[pkg.ReleaseName]
	if !found {
		md, _ := repo["metadata"].(map[string]any)
		uid, _ := md["uid"].(string)
		if uid == "" {
			return port.Receipt{}, ErrOwnership
		}
		return p.recoverUnadmittedApplication(ctx, i, pkg, cfg, uid)
	}
	if pin.Removed || pin.Intent != i.ID || pin.Revision != i.Revision.ID || pin.EnvironmentID != i.Revision.Scope.EnvironmentID || pin.Path != pkg.Publication.Path || pin.Digest != i.Revision.PayloadDigest {
		return port.Receipt{}, ErrOwnership
	}
	spec, _ := repo["spec"].(map[string]any)
	commit, _ := spec["revision"].(string)
	if err := p.verifySharedTrees(ctx, i, commit, pins); err != nil {
		return port.Receipt{}, err
	}
	md, _ := repo["metadata"].(map[string]any)
	uid, _ := md["uid"].(string)
	if uid == "" {
		return port.Receipt{}, ErrOwnership
	}
	inventory, _ := json.Marshal(pkg.Objects)
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"layout": applicationLayout, "gitrepo": applicationRepoName(i, cfg), "gitrepoUID": uid, "commit": commit, "path": pkg.Publication.Path, "release": pkg.ReleaseName, "namespace": i.Target.Namespace, "releaseNamespace": cfg.ReleaseNamespace, "inventory": string(inventory)}, ObservedAt: time.Now().UTC()}, nil
}

// Validate the shared destination and live layout before any Git/native effect.
// Stable readable paths must never let a new epoch/generation overwrite a
// different still-active release without the existing explicit withdrawal.
func (p *Provider) validateApplicationSubmission(ctx context.Context, i port.Intent, pkg Package, cfg Config) error {
	relative, err := sharedUnitPath(i)
	if err != nil {
		return err
	}
	destination, err := p.PublicationDestination(i.Binding)
	if err != nil {
		return err
	}
	if pkg.UnitName != unitReleaseName(i.Revision.Scope) || pkg.ReleaseName != pkg.UnitName || pkg.Publication.Path != path.Join(cfg.Root, relative) || pkg.Publication.Branch != cfg.Branch || pkg.Publication.DestinationKey != destination || pkg.Metadata == nil || pkg.Metadata.Path != path.Join("metadata", cfg.EnvironmentKey, relative) || pkg.Metadata.Branch != cfg.Branch || pkg.Metadata.DestinationKey != destination {
		return ErrOwnership
	}
	cli, close, _, err := p.Manager.open(ctx, i)
	if err != nil {
		return err
	}
	defer close()
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	if err != nil {
		return ErrController
	}
	if !found {
		if i.ExpectedRevision != "" {
			return ErrOwnership
		}
		return nil
	}
	md, _ := repo["metadata"].(map[string]any)
	if !applicationOwns(repo, i, cfg) || md["deletionTimestamp"] != nil {
		return ErrOwnership
	}
	pins, err := readUnitPins(repo)
	if err != nil {
		return err
	}
	if pin, ok := pins[pkg.ReleaseName]; ok {
		if pin.EnvironmentID != i.Revision.Scope.EnvironmentID || pin.Path != pkg.Publication.Path || (pin.Removed && i.ExpectedRevision != "") || (!pin.Removed && pin.Revision != i.ExpectedRevision && pin.Revision != i.Revision.ID) {
			return ErrOwnership
		}
	} else if i.ExpectedRevision != "" {
		return ErrOwnership
	}
	for release, pin := range pins {
		if !pin.Removed && pin.Path == pkg.Publication.Path && release != pkg.ReleaseName {
			return ErrOwnership
		}
	}
	return nil
}

// Finish only the original new-unit admission, using remotely observed immutable
// input trees. Never push Git or overwrite an existing unit during recovery.
func (p *Provider) recoverUnadmittedApplication(ctx context.Context, i port.Intent, pkg Package, cfg Config, uid string) (port.Receipt, error) {
	if i.ExpectedRevision != "" || len(i.SourceInventory) != 0 || pkg.Metadata == nil {
		return port.Receipt{}, ErrOwnership
	}
	if err := p.validateApplicationSubmission(ctx, i, pkg, cfg); err != nil {
		return port.Receipt{}, err
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return port.Receipt{}, err
	}
	published, found, err := publisher.ObservePublication(ctx, pkg.Publication)
	if err != nil || !found {
		return port.Receipt{}, domain.ErrOutcomeUnknown
	}
	metadata, found, err := publisher.ObservePublication(ctx, *pkg.Metadata)
	if err != nil || !found || metadata.Commit != published.Commit {
		return port.Receipt{}, domain.ErrOutcomeUnknown
	}
	// Recovery may only admit a new release. Existing native objects cannot be
	// adopted, even when they carry matching labels or a coincidental release name.
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return port.Receipt{}, err
	}
	defer close()
	objects, err := k8s.InspectDeliveryObjects(ctx, cli, i.Target, pkg.Objects)
	if err != nil {
		return port.Receipt{}, err
	}
	if len(objects) != 0 {
		return port.Receipt{}, ErrOwnership
	}
	if err := p.preflight(ctx, i, pkg); err != nil {
		return port.Receipt{}, err
	}
	manager := *p.Manager
	manager.Guard = p.Manager.RecoveryGuard
	recovering := *p
	recovering.Manager = &manager
	return recovering.submitApplicationAttempt(ctx, i, pkg, published, 0, uid)
}

func (p *Provider) ObserveSubmissionAbsent(ctx context.Context, i port.Intent, prepared port.Prepared) (bool, error) {
	if p.Manager == nil || p.Manager.RecoveryGuard == nil || p.Publishers == nil {
		return false, ErrOwnership
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return false, err
	}
	if i.Revision.Phase != domain.Prerequisite || i.ExpectedRevision != "" || len(i.SourceInventory) != 0 {
		return false, nil
	}
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil || pkg.Publication.ID != i.ID || pkg.Publication.TreeDigest != i.Revision.PayloadDigest || gitport.Digest(pkg.Publication.Files) != i.Revision.PayloadDigest || pkg.Metadata == nil {
		return false, ErrOwnership
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return false, err
	}
	defer close()
	if cfg.ApplicationID == "" {
		return false, nil
	}
	_, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	if err != nil || found {
		return false, err
	}
	if err := p.validateApplicationSubmission(ctx, i, pkg, cfg); err != nil {
		return false, err
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return false, err
	}
	for _, publication := range []gitport.Intent{pkg.Publication, *pkg.Metadata} {
		_, found, err := publisher.ObservePublication(ctx, publication)
		if err != nil || found {
			return false, err
		}
	}
	target, done, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return false, err
	}
	defer done()
	objects, err := k8s.InspectDeliveryObjects(ctx, target, i.Target, pkg.Objects)
	if err != nil || len(objects) > 0 {
		return false, err
	}
	if err := p.preflight(ctx, i, pkg); err != nil {
		return false, err
	}
	// An orphan Bundle can still create a downstream manager after this read.
	bundlesRaw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+applicationRepoName(i, cfg), "-o", "json")
	if err != nil {
		return false, err
	}
	var bundles struct{ Items []map[string]any }
	if json.Unmarshal(bundlesRaw, &bundles) != nil {
		return false, ErrController
	}
	if len(selectRelease(bundles.Items, pkg.ReleaseName)) != 0 {
		return false, nil
	}
	// Neither an orphan Helm release nor a downstream pending manager may remain.
	secrets, err := target.Run(ctx, nil, "get", "secrets", "-n", cfg.ReleaseNamespace, "-l", "owner=helm,name="+pkg.ReleaseName, "-o", "jsonpath={.items[*].metadata.uid}")
	if err != nil || strings.TrimSpace(string(secrets)) != "" {
		return false, err
	}
	cluster, found, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !found {
		return false, err
	}
	md, _ := cluster["metadata"].(map[string]any)
	status, _ := cluster["status"].(map[string]any)
	ns, _ := status["namespace"].(string)
	if md["uid"] != cfg.TargetClusterUID || ns == "" {
		return false, ErrOwnership
	}
	raw, err := cli.Run(ctx, nil, "get", "bundledeployments.fleet.cattle.io", "-n", ns, "-o", "json")
	if err != nil {
		return false, err
	}
	var downstream struct {
		Items []struct {
			Spec struct {
				Options       struct{ Helm struct{ ReleaseName string } }
				StagedOptions struct{ Helm struct{ ReleaseName string } }
			}
		}
	}
	if json.Unmarshal(raw, &downstream) != nil {
		return false, ErrController
	}
	for _, bd := range downstream.Items {
		if bd.Spec.Options.Helm.ReleaseName == pkg.ReleaseName || bd.Spec.StagedOptions.Helm.ReleaseName == pkg.ReleaseName {
			return false, nil
		}
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return false, err
	}
	_, found, err = cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", applicationRepoName(i, cfg))
	return !found && err == nil, err
}
