package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"time"

	"orchestrator/internal/adapters/delivery/artifact"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
	hosting "orchestrator/internal/ports/repositoryhosting"
)

type PublisherResolver interface {
	ResolvePublisher(context.Context, port.BindingVersion) (gitport.Publisher, error)
}
type Provider struct {
	Manager         *Manager
	Publishers      PublisherResolver
	RepositoryHosts hosting.Factory
}

func Capabilities() domain.Capabilities {
	return domain.Capabilities{ContractVersion: "v1", ExactRevision: true, DriftDetection: true, SelfHeal: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true}
}
func (p *Provider) ValidateConfiguration(b port.BindingVersion) error {
	if b.Config["readSecretMode"] == "MANAGED" && b.Config["readSecret"] != managedReaderName(b) {
		return ErrConfig
	}
	if b.Config["applicationID"] != "" || b.Config["environmentKey"] != "" || b.Config["repositoryURL"] != "" || b.Config["branch"] != "" || b.Config["existingGitRepo"] != "" || b.Config["existingGitRepoUID"] != "" || b.Config["gitRepoMode"] == "EXISTING" {
		return ErrConfig
	}
	_, err := ParseConfig(b.Config)
	return err
}
func (p *Provider) Describe(context.Context, port.BindingVersion) (domain.Capabilities, error) {
	return Capabilities(), nil
}
func (p *Provider) Verify(ctx context.Context, b port.BindingVersion) (port.Verification, error) {
	if p.Manager == nil || p.Manager.Verifier == nil {
		return port.Verification{}, ErrVerification
	}
	result, err := p.Manager.Verifier.Verify(ctx, b)
	if err != nil {
		return result, err
	}
	policy, policyErr := p.RepositoryPolicy(b)
	if policyErr != nil {
		return port.Verification{ReasonCode: "VERIFICATION_FAILED"}, policyErr
	}
	if policy.Enabled {
		if p.RepositoryHosts == nil {
			return port.Verification{ReasonCode: "GIT_ACCESS_FAILED"}, ErrVerification
		}
		host, err := p.RepositoryHosts.Open(ctx, b, policy)
		if err != nil {
			return port.Verification{ReasonCode: "GIT_ACCESS_FAILED"}, err
		}
		if err := host.Verify(ctx); err != nil {
			return port.Verification{ReasonCode: "GIT_ACCESS_FAILED"}, err
		}
		result.Capabilities = Capabilities()
		return result, nil
	}
	// Resolve performs a scoped HTTPS clone/read with the private writer
	// credential. Verification never pushes or creates a GitRepo.
	if p.Publishers == nil {
		return port.Verification{ReasonCode: "VERIFICATION_FAILED"}, ErrVerification
	}
	if _, err := p.Publishers.ResolvePublisher(ctx, b); err != nil {
		return port.Verification{ReasonCode: "VERIFICATION_FAILED"}, ErrVerification
	}
	result.Capabilities = Capabilities()
	return result, nil
}
func (p *Provider) Prepare(_ context.Context, i port.Intent, manifests []execution.Manifest) (port.Prepared, error) {
	manifests, err := preserveClaimProvenance(i, manifests)
	if err != nil {
		return port.Prepared{}, err
	}
	pkg, err := PackageUnit(i, manifests)
	if err != nil {
		return port.Prepared{}, err
	}
	raw, err := json.Marshal(pkg)
	if err != nil {
		return port.Prepared{}, ErrConfig
	}
	revision := i.Revision
	revision.PayloadDigest = pkg.Publication.TreeDigest
	return port.Prepared{Revision: revision, Objects: pkg.Objects, Payload: raw}, nil
}
func (p *Provider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (result port.Receipt, resultErr error) {
	stage := "VALIDATE"
	defer func() {
		if resultErr == nil {
			return
		}
		code := "UNCLASSIFIED"
		for _, known := range []struct {
			err  error
			code string
		}{
			{ErrOwnership, "OWNERSHIP"}, {ErrController, "CONTROLLER"}, {ErrVerification, "VERIFICATION"}, {ErrConfig, "CONFIG"},
			{k8s.ErrUnreachable, "CLUSTER_UNREACHABLE"}, {k8s.ErrRequestRejected, "CLUSTER_REJECTED"}, {k8s.ErrForbidden, "CLUSTER_FORBIDDEN"},
		} {
			if errors.Is(resultErr, known.err) {
				code = known.code
				break
			}
		}
		switch resultErr.Error() {
		case "gitpublication: operation failed":
			code = "GIT_FAILED"
		case "gitpublication: generated path changed concurrently":
			code = "GIT_CONFLICT"
		case "gitpublication: invalid generated publication":
			code = "GIT_INVALID"
		}
		log.Printf("fleet submission stopped: stage=%s code=%s type=%T", stage, code, resultErr)
	}()
	if p.Manager == nil || p.Publishers == nil || p.Manager.Guard == nil {
		return port.Receipt{}, ErrController
	}
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil || pkg.Publication.TreeDigest != i.Revision.PayloadDigest || gitport.Digest(pkg.Publication.Files) != i.Revision.PayloadDigest {
		return port.Receipt{}, ErrConfig
	}
	if pkg.Publication.ID != i.ID {
		return port.Receipt{}, ErrOwnership
	}
	cfg, err := ParseConfig(i.Binding.Config)
	if err != nil {
		return port.Receipt{}, err
	}
	if cfg.ApplicationID != "" {
		if err := p.validateApplicationSubmission(ctx, i, pkg, cfg); err != nil {
			return port.Receipt{}, err
		}
	}
	if cfg.GitRepoMode == "EXISTING" {
		cli, close, _, err := p.Manager.open(ctx, i)
		if err != nil {
			return port.Receipt{}, err
		}
		_, err = checkExisting(ctx, cli, cfg)
		close()
		if err != nil {
			return port.Receipt{}, err
		}
	}
	stage = "PREFLIGHT"
	if err := p.preflight(ctx, i, pkg); err != nil {
		return port.Receipt{}, err
	}
	stage = "RESOLVE_PUBLISHER"
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return port.Receipt{}, err
	}
	stage = "CONTENT_PUBLICATION"
	git, err := publisher.Publish(ctx, pkg.Publication, func(ctx context.Context) error { return p.Manager.Guard(ctx, i) })
	if err != nil {
		return port.Receipt{}, err
	}
	if cfg.ApplicationID != "" {
		if pkg.Metadata == nil {
			return port.Receipt{}, ErrConfig
		}
		stage = "METADATA_PUBLICATION"
		metadata, err := publisher.Publish(ctx, *pkg.Metadata, func(ctx context.Context) error { return p.Manager.Guard(ctx, i) })
		if err != nil {

			return port.Receipt{}, err
		}
		git.Commit = metadata.Commit
		stage = "CONTROLLER_ADMISSION"
		return p.submitApplication(ctx, i, pkg, git)
	}
	return p.Manager.Submit(ctx, i, pkg, git)
}
func (p *Provider) Withdraw(ctx context.Context, i port.Intent, r port.Receipt) (port.WithdrawalProof, error) {
	if r.Handle["completionOnly"] == "true" {
		if p.Manager == nil || p.Manager.Guard == nil {
			return port.WithdrawalProof{}, ErrOwnership
		}
		if err := p.Manager.Guard(ctx, i); err != nil {
			return port.WithdrawalProof{}, err
		}
		return port.WithdrawalProof{Receipt: r, ManagementStopped: true}, nil
	}
	if p.Manager == nil {
		return port.WithdrawalProof{}, ErrController
	}
	if r.Handle["layout"] == applicationLayout {
		return p.withdrawApplication(ctx, i, r)
	}
	cfg, err := ParseConfig(i.Binding.Config)
	if err != nil {
		return port.WithdrawalProof{}, err
	}
	if cfg.GitRepoMode == "EXISTING" {
		if err := p.withdrawExistingPublication(ctx, i, r, cfg); err != nil {
			return port.WithdrawalProof{}, err
		}
	}
	return p.Manager.Withdraw(ctx, i, r)
}
func (p *Provider) Reconcile(context.Context, port.Intent, port.Receipt) (port.Receipt, error) {
	return port.Receipt{}, domain.ErrUnsupported
}
func unknown(i port.Intent) domain.Observation {
	return domain.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, Sync: domain.SyncUnknown, Health: domain.HealthUnknown, Drift: domain.DriftUnknown, ObservedAt: time.Now().UTC()}
}
func (p *Provider) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	if r.Handle["completionOnly"] == "true" {
		cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
		if err != nil {
			return unknown(i), err
		}
		defer close()
		return artifact.ObserveCompletion(ctx, cli, i, r)
	}
	out := unknown(i)
	if p.Manager == nil || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest {
		return out, ErrOwnership
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return out, err
	}
	defer close()
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
	if err != nil || !found {
		return out, ErrController
	}
	md, _ := repo["metadata"].(map[string]any)
	if cfg.GitRepoMode == "EXISTING" {
		if r.Handle["gitrepo"] != cfg.ExistingGitRepo || r.Handle["gitrepoUID"] != cfg.ExistingGitRepoUID {
			return out, ErrOwnership
		}
		if _, err := checkExisting(ctx, cli, cfg); err != nil {
			return out, err
		}
	} else if r.Handle["layout"] == applicationLayout {
		pins, err := readUnitPins(repo)
		if err != nil || !applicationOwns(repo, i, cfg) || md["deletionTimestamp"] != nil || md["uid"] != r.Handle["gitrepoUID"] || !sharedReceiptMatches(pins, i, r) {
			return out, ErrOwnership
		}
	} else if !owns(repo, i, r.Handle["gitrepo"]) || md["uid"] != r.Handle["gitrepoUID"] || controllerRevision(repo) != i.Revision.ID {
		return out, ErrOwnership
	}
	status, _ := repo["status"].(map[string]any)
	if r.Handle["layout"] == applicationLayout {
		spec, _ := repo["spec"].(map[string]any)
		if status["commit"] != spec["revision"] {
			out.Sync, out.Health = domain.Pending, domain.Progressing
			return out, nil
		}
	}
	commitMatches := status["commit"] == r.Handle["commit"]
	if cfg.GitRepoMode == "EXISTING" || r.Handle["layout"] == applicationLayout {
		commitMatches = false
		commit, _ := status["commit"].(string)
		if commit != "" {
			publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
			if err != nil {
				return out, err
			}
			verifier, ok := publisher.(gitport.TreeVerifier)
			if !ok {
				return out, domain.ErrUnsupported
			}
			commitMatches, err = verifier.VerifyTree(ctx, cfg.Branch, r.Handle["path"], commit, r.PayloadDigest)
			if err != nil {
				return out, err
			}
		}
	}
	if !commitMatches {
		out.Sync = domain.Pending
		out.Health = domain.Progressing
		return out, nil
	}
	raw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
	if err != nil {
		return out, ErrController
	}
	var bundles struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(raw, &bundles) != nil {
		return out, ErrController
	}
	if cfg.GitRepoMode == "EXISTING" || r.Handle["layout"] == applicationLayout {
		bundles.Items = selectRelease(bundles.Items, r.Handle["release"])
	}
	if len(bundles.Items) != 1 {
		return out, nil
	}
	bundle := bundles.Items[0]
	bundleMD, _ := bundle["metadata"].(map[string]any)
	cluster, found, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !found {
		return out, ErrController
	}
	clusterMD, _ := cluster["metadata"].(map[string]any)
	if clusterMD["uid"] != cfg.TargetClusterUID {
		return out, ErrOwnership
	}
	clusterStatus, _ := cluster["status"].(map[string]any)
	namespace, _ := clusterStatus["namespace"].(string)
	bundleName, _ := bundleMD["name"].(string)
	if namespace == "" || bundleName == "" {
		return out, nil
	}
	bd, found, err := cli.Get(ctx, namespace, "bundledeployments.fleet.cattle.io", bundleName)
	if err != nil || !found {
		return out, ErrController
	}
	bdSpec, _ := bd["spec"].(map[string]any)
	bdStatus, _ := bd["status"].(map[string]any)
	commit, _ := status["commit"].(string)
	if !controllerCommitMatches(commit, bundle, bd) {
		out.Sync, out.Health = domain.Pending, domain.Progressing
		return out, nil
	}
	desired, _ := bdSpec["deploymentID"].(string)
	applied, _ := bdStatus["appliedDeploymentID"].(string)
	if display, _ := bdStatus["display"].(map[string]any); display != nil && display["state"] == "ErrApplied" {
		out.Sync = domain.OutOfSync
		out.Health = domain.Degraded
		return out, nil
	}
	if desired == "" || desired != applied {
		out.Sync = domain.Pending
		out.Health = domain.Progressing
		return out, nil
	}
	// Fleet reports Modified independently from health; do not collapse to Ready.
	if incomplete, _ := bdStatus["incompleteState"].(bool); incomplete {
		return out, nil
	}
	out.Sync = domain.InSync
	out.Drift = domain.NoDrift
	out.Health = domain.Healthy
	if modified, ok := bdStatus["modifiedStatus"].([]any); ok && len(modified) > 0 {
		out.Sync = domain.OutOfSync
		out.Drift = domain.Detected
	}
	if nonready, ok := bdStatus["nonReadyStatus"].([]any); ok && len(nonready) > 0 {
		// Fleet includes ordinary rollout progress here. Only native terminal
		// conditions below prove degradation; a pending rollout is not failed.
		out.Health = domain.Progressing
	}
	out.ObservedRevision = i.Revision.ID
	out.PayloadDigest = i.Revision.PayloadDigest
	out.Complete = true
	// Status can lag apply; absence of the expected revision on live workload is
	// still pending, even if the controller's aggregate reports Ready.
	targetCLI, targetClose, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return unknown(i), ErrVerification
	}
	defer targetClose()
	var expected []execution.ObjectRef
	if json.Unmarshal([]byte(r.Handle["inventory"]), &expected) != nil || len(expected) == 0 {
		return unknown(i), ErrOwnership
	}
	live, err := k8s.InspectDeliveryObjects(ctx, targetCLI, i.Target, expected)
	if err != nil {
		return unknown(i), ErrController
	}
	for _, ref := range expected {
		object, exists := live[ref.Key()]
		if !exists {
			out.Sync = domain.OutOfSync
			out.Drift = domain.Detected
			out.Health = domain.Degraded
			continue
		}
		metadata, _ := object["metadata"].(map[string]any)
		uid, _ := metadata["uid"].(string)
		if uid == "" {
			return unknown(i), ErrOwnership
		}
		out.Objects = append(out.Objects, domain.ObjectObservation{APIVersion: ref.APIVersion, Kind: ref.Kind, Name: ref.Name, Namespace: i.Target.Namespace, UID: uid})
		a, _ := metadata["annotations"].(map[string]any)
		if a["meta.helm.sh/release-name"] != r.Handle["release"] {
			return unknown(i), ErrOwnership
		}
		if a["orchestrator.io/delivery-revision"] != i.Revision.ID {
			out.Complete = false
			out.ObservedRevision = ""
			out.PayloadDigest = ""
			out.Health = objectHealth(ref.Kind, object)
			if out.Drift != domain.Detected {
				out.Sync = domain.Pending
				out.Drift = domain.DriftUnknown
			}
			return out, nil
		}
		health := objectHealth(ref.Kind, object)
		if ref.APIVersion == "secrets.hashicorp.com/v1beta1" && ref.Kind == "VaultStaticSecret" {
			var err error
			health, err = observeSecret(ctx, targetCLI, i.Target, object)
			if err != nil {
				return unknown(i), err
			}
		}
		if health == domain.Degraded || (health == domain.Progressing && out.Health != domain.Degraded) {
			out.Health = health
		}
	}

	return out, nil
}

var _ port.Provider = (*Provider)(nil)

func controllerCommitMatches(commit string, objects ...map[string]any) bool {
	if commit == "" {
		return false
	}
	for _, object := range objects {
		md, _ := object["metadata"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		if labels["fleet.cattle.io/commit"] != commit {
			return false
		}
	}
	return true
}

func (p *Provider) preflight(ctx context.Context, i port.Intent, pkg Package) error {
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return ErrVerification
	}
	defer close()
	cfg, err := ParseConfig(i.Binding.Config)
	if err != nil {
		return err
	}
	expected, err := p.Manager.Verifier.Targets.ResolveWorkloadTarget(ctx, i.Binding, cfg.TargetAccountKey)
	if err != nil {
		return err
	}
	expectedCLI, expectedClose, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, expected)
	if err != nil {
		return ErrVerification
	}
	defer expectedClose()
	expectedNS, found, err := expectedCLI.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !found {
		return ErrVerification
	}
	actualNS, found, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !found {
		return ErrVerification
	}
	expectedMeta, _ := expectedNS["metadata"].(map[string]any)
	actualMeta, _ := actualNS["metadata"].(map[string]any)
	if expectedMeta["uid"] == nil || actualMeta["uid"] != expectedMeta["uid"] || (cfg.TargetKubeSystemUID != "" && actualMeta["uid"] != cfg.TargetKubeSystemUID) {
		return ErrOwnership
	}
	objects, err := k8s.InspectDeliveryObjects(ctx, cli, i.Target, pkg.Objects)
	if err != nil {
		return err
	}
	expectedUIDs := map[string]string{}
	for _, o := range i.SourceInventory {
		expectedUIDs[o.Key()] = o.UID
	}
	for key, object := range objects {
		md, _ := object["metadata"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		annotations, _ := md["annotations"].(map[string]any)
		if uid, known := expectedUIDs[key]; known && (uid == "" || md["uid"] != uid) {
			return ErrOwnership
		}
		if labels["orchestrator.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != i.Target.Extra["application"] || labels["orchestrator.io/environment"] != i.Target.Extra["environment"] || annotations["meta.helm.sh/release-name"] != pkg.ReleaseName {
			return ErrOwnership
		}
	}
	rawPackage, err := json.Marshal(pkg)
	if err != nil {
		return ErrConfig
	}
	manifests, err := p.ExportArtifact(ctx, rawPackage)
	if err != nil {
		return err
	}
	if err := k8s.ValidateRuntimeObjects(ctx, cli, i.Target, manifests, k8s.DeliveryManager{Release: pkg.ReleaseName, Namespace: cfg.ReleaseNamespace}); err != nil {
		return err
	}
	// Older handoff revisions left a competing Update owner of platform
	// metadata. Relinquish only those fields under the current intent/fence,
	// after validating its persisted source UID. This never changes business
	// fields or forces Fleet's server-side apply.
	for _, o := range i.SourceInventory {
		if _, exists := objects[o.Key()]; !exists {
			continue
		}
		if err := p.Manager.Guard(ctx, i); err != nil {
			return err
		}
		if err := k8s.ReleaseDeliveryMetadata(ctx, cli, i.Target, o.ObjectRef, o.UID, k8s.DeliveryManager{Release: pkg.ReleaseName, Namespace: cfg.ReleaseNamespace}, i.ExpectedRevision, i.Revision.ID); err != nil {
			return err
		}
	}
	return p.reclaimPolicyFields(ctx, cli, cfg, i, pkg, i.SourceInventory, i.ExpectedRevision, p.Manager.Guard)
}

func (p *Provider) PublicationDestination(b port.BindingVersion) (string, error) {
	cfg, err := ParseConfig(b.Config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(cfg.RepositoryURL + "\x00" + cfg.Branch))
	return hex.EncodeToString(sum[:]), nil
}

func (p *Provider) RemoveRetainedObjects(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	if p.Manager == nil || p.Manager.Guard == nil {
		return ErrOwnership
	}
	// Withdrawal proof is rechecked against the original provider receipt before
	// deletion. A retained object cannot be removed while its controller is live.
	proof, err := p.Withdraw(ctx, i, r)
	if err != nil || !proof.ManagementStopped {
		if err != nil {
			return err
		}
		return ErrOwnership
	}
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return err
	}
	defer close()
	for _, object := range objects {
		if r.Handle["completionOnly"] == "true" {
			if err := p.Manager.Guard(ctx, i); err != nil {
				return err
			}
			if err := k8s.RemoveDirectDeliveryObject(ctx, cli, i.Target, object.ObjectRef, object.UID); err != nil {
				return err
			}
			continue
		}
		if object.Release != r.Handle["release"] {
			return ErrOwnership
		}
		if err := p.Manager.Guard(ctx, i); err != nil {
			return err
		}
		if err := k8s.RemoveRetainedDeliveryObject(ctx, cli, i.Target, object.ObjectRef, object.UID, object.Release, r.Handle["releaseNamespace"]); err != nil {
			return err
		}
		if object.APIVersion == "secrets.hashicorp.com/v1beta1" && object.Kind == "VaultStaticSecret" {
			if err := waitSecretRemoved(ctx, cli, i.Target, object.Name, object.UID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Provider) PruneReleasedObjects(ctx context.Context, i port.Intent, r port.Receipt, stale []port.AppliedObject) error {
	if p.Manager == nil || p.Manager.Guard == nil {
		return ErrOwnership
	}
	observed, err := p.Observe(ctx, i, r)
	if err != nil {
		return err
	}
	if !observed.Ready(i.Revision, time.Now(), 30*time.Second) {
		return ErrController
	}
	return p.pruneRetainedInventory(ctx, i, r, stale, observed, p.Manager.Guard)
}

func (p *Provider) pruneRetainedInventory(ctx context.Context, i port.Intent, r port.Receipt, stale []port.AppliedObject, observed domain.Observation, guard func(context.Context, port.Intent) error) error {
	inventory := map[string]bool{}
	for _, o := range observed.Objects {
		inventory[o.APIVersion+"/"+o.Kind+"/"+o.Name] = true
	}
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return err
	}
	defer close()
	for _, o := range stale {
		if inventory[o.Key()] || o.Release != r.Handle["release"] {
			return ErrOwnership
		}
		if err := guard(ctx, i); err != nil {
			return err
		}
		if err := k8s.RemoveRetainedDeliveryObject(ctx, cli, i.Target, o.ObjectRef, o.UID, o.Release, r.Handle["releaseNamespace"]); err != nil {
			return err
		}
		if o.APIVersion == "secrets.hashicorp.com/v1beta1" && o.Kind == "VaultStaticSecret" {
			if err := waitSecretRemoved(ctx, cli, i.Target, o.Name, o.UID); err != nil {
				return err
			}
		}
	}
	return nil
}
