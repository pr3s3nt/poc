package fleet

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"orchestrator/internal/adapters/delivery/artifact"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	gitport "orchestrator/internal/ports/gitpublication"
)

// ObserveTerminalJobFailure is read-only. A failed Helm wait is not publication
// absence, and never a successful completion. Bind the failed execution to its
// admitted tree, release manifest and stable live incarnations for later removal.
func (p *Provider) ObserveTerminalJobFailure(ctx context.Context, i port.Intent, prepared port.Prepared, r port.Receipt) (result []port.AppliedObject, settled bool, resultErr error) {
	stage := "guard"
	defer func() {
		if resultErr != nil {
			log.Printf("Fleet terminal Job proof stopped: stage=%s type=%T", stage, resultErr)
		}
	}()
	if i.Revision.Phase != domain.Job {
		return nil, false, nil
	}
	if p.Manager == nil || p.Manager.RecoveryGuard == nil || p.Publishers == nil {
		return nil, false, ErrOwnership
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return nil, false, err
	}
	stage = "artifact"
	var pkg Package
	if json.Unmarshal(prepared.Payload, &pkg) != nil || pkg.Publication.ID != i.ID || pkg.Publication.TreeDigest != i.Revision.PayloadDigest || gitport.Digest(pkg.Publication.Files) != i.Revision.PayloadDigest || r.IntentID != i.ID || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest {
		return nil, false, ErrOwnership
	}
	stage = "target-config"
	cli, done, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return nil, false, err
	}
	defer done()
	// Only the current managed per-Application layout is admitted by this extension.
	stage = "receipt"
	if r.Handle["layout"] != applicationLayout || cfg.ApplicationID == "" || r.Handle["gitrepo"] != applicationRepoName(i, cfg) || r.Handle["release"] != pkg.ReleaseName || r.Handle["releaseNamespace"] != cfg.ReleaseNamespace || r.Handle["namespace"] != i.Target.Namespace || r.Handle["path"] != pkg.Publication.Path {
		return nil, false, ErrOwnership
	}
	stage = "gitrepo"
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
	if err != nil || !found {
		return nil, false, ErrController
	}
	md := obj(repo["metadata"])
	pins, err := readUnitPins(repo)
	if err != nil || md["uid"] != r.Handle["gitrepoUID"] || md["resourceVersion"] == nil || md["deletionTimestamp"] != nil || !applicationOwns(repo, i, cfg) || !sharedReceiptMatches(pins, i, r) {
		return nil, false, ErrOwnership
	}
	commit, _ := obj(repo["spec"])["revision"].(string)
	if commit == "" || obj(repo["status"])["commit"] != commit {
		return nil, false, nil
	}
	stage = "shared-trees"
	if err = p.verifySharedTrees(ctx, i, commit, pins); err != nil {
		return nil, false, err
	}
	stage = "metadata"
	if pkg.Metadata == nil || gitport.Digest(pkg.Metadata.Files) != pkg.Metadata.TreeDigest || pkg.Metadata.DestinationKey != pkg.Publication.DestinationKey || pkg.Metadata.Branch != pkg.Publication.Branch {
		return nil, false, ErrOwnership
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return nil, false, err
	}
	stage = "publication"
	publication, ok, err := publisher.ObservePublication(ctx, pkg.Publication)
	if err != nil || !ok || publication.TreeDigest != i.Revision.PayloadDigest || publication.Commit != commit {
		return nil, false, domain.ErrOutcomeUnknown
	}
	metadataPublication, ok, err := publisher.ObservePublication(ctx, *pkg.Metadata)
	if err != nil || !ok || metadataPublication.Commit != commit || metadataPublication.TreeDigest != pkg.Metadata.TreeDigest {
		return nil, false, domain.ErrOutcomeUnknown
	}
	stage = "bundle"
	raw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
	if err != nil {
		return nil, false, ErrController
	}
	var bundles struct{ Items []map[string]any }
	if json.Unmarshal(raw, &bundles) != nil {
		return nil, false, ErrController
	}
	bs := selectRelease(bundles.Items, pkg.ReleaseName)
	if len(bs) != 1 || !owns(bs[0], i, pkg.ReleaseName) {
		return nil, false, ErrOwnership
	}
	stage = "cluster"
	cluster, found, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !found {
		return nil, false, ErrController
	}
	if obj(cluster["metadata"])["uid"] != cfg.TargetClusterUID {
		return nil, false, ErrOwnership
	}
	ns, _ := obj(cluster["status"])["namespace"].(string)
	bn, _ := obj(bs[0]["metadata"])["name"].(string)
	if ns == "" || bn == "" {
		return nil, false, ErrOwnership
	}
	stage = "bundledeployment"
	bd, found, err := cli.Get(ctx, ns, "bundledeployments.fleet.cattle.io", bn)
	if err != nil || !found {
		return nil, false, ErrController
	}
	if !controllerCommitMatches(commit, bs[0], bd) || obj(bd["spec"])["deploymentID"] == nil {
		return nil, false, nil
	}
	stage = "target"
	target, closeTarget, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return nil, false, err
	}
	defer closeTarget()
	physical, found, err := target.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !found || cfg.TargetKubeSystemUID == "" || obj(physical["metadata"])["uid"] != cfg.TargetKubeSystemUID {
		return nil, false, ErrOwnership
	}
	// Failed releases retain a Helm history Secret, whose exact manifest is proof
	// of what Helm attempted; aggregate Bundle readiness is deliberately not used.
	raw, err = target.Run(ctx, nil, "get", "secrets", "-n", cfg.ReleaseNamespace, "-l", "owner=helm,name="+pkg.ReleaseName, "-o", "json")
	if err != nil {
		return nil, false, err
	}
	var releases struct{ Items []map[string]any }
	if json.Unmarshal(raw, &releases) != nil {
		return nil, false, ErrOwnership
	}
	stage = "helm-release"
	release, err := latestFailedJobRelease(releases.Items, pkg.ReleaseName, cfg.ReleaseNamespace)
	if errors.Is(err, domain.ErrOutcomeUnknown) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	stage = "helm-manifest"
	manifest, err := helmJobManifest(release, pkg.ReleaseName, cfg.ReleaseNamespace)
	if err != nil {
		return nil, false, err
	}
	stage = "artifact-manifest"
	expected, err := p.ExportArtifact(ctx, prepared.Payload)
	if err != nil {
		return nil, false, err
	}
	a, b := []map[string]any{}, []map[string]any{}
	for _, m := range expected {
		if m.Kind != "Job" {
			return nil, false, ErrOwnership
		}
		a = append(a, cloneObject(m.Object))
	}
	for _, m := range manifest {
		b = append(b, terminalJobExecutionObject(m))
	}
	stage = "manifest-equality"
	same, err := artifact.SameExecution(a, b)
	if err != nil || !same {
		return nil, false, ErrOwnership
	}
	refs := []execution.ObjectRef{}
	for _, m := range expected {
		refs = append(refs, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name})
	}
	stage = "native-job"
	live, err := k8s.InspectDeliveryObjects(ctx, target, i.Target, refs)
	if err != nil {
		return nil, false, err
	}
	inventory := []port.AppliedObject{}
	failed := false
	for _, m := range expected {
		key := execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name}
		job := live[key.Key()]
		if job == nil {
			return nil, false, ErrOwnership
		}
		stage = "native-job-identity"
		uid, terminal, isFailed, err := terminalJobIdentity(job, m.Object, i, r)
		if err != nil {
			return nil, false, err
		}
		if !terminal {
			return nil, false, nil
		}
		failed = failed || isFailed
		stage = "job-pods"
		podsRaw, err := target.Run(ctx, nil, "get", "pods", "-n", i.Target.Namespace, "-l", "batch.kubernetes.io/controller-uid="+uid, "-o", "json")
		if err != nil {
			return nil, false, err
		}
		var pods struct{ Items []map[string]any }
		if json.Unmarshal(podsRaw, &pods) != nil {
			return nil, false, ErrOwnership
		}
		if !jobPodsStopped(pods.Items, uid, m.Name) {
			return nil, false, nil
		}
		inventory = append(inventory, port.AppliedObject{ObjectRef: key, UID: uid, Release: pkg.ReleaseName})
	}
	if !failed || len(inventory) == 0 {
		return nil, false, nil
	}
	// Observe again under the same operation fence. A recreated Job, publication
	// change, release replacement or controller update invalidates this snapshot.
	if err = p.Manager.RecoveryGuard(ctx, i); err != nil {
		return nil, false, err
	}
	stage = "stable-snapshot"
	again, err := k8s.InspectDeliveryObjects(ctx, target, i.Target, refs)
	if err != nil {
		return nil, false, err
	}
	for key, job := range live {
		if !sameIncarnation(job, again[key]) {
			return nil, false, ErrOwnership
		}
	}
	for _, read := range []struct {
		cli                   k8s.CLI
		namespace, kind, name string
		previous              map[string]any
	}{{cli, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"], repo}, {cli, cfg.Workspace, "bundles.fleet.cattle.io", bn, bs[0]}, {cli, ns, "bundledeployments.fleet.cattle.io", bn, bd}, {target, cfg.ReleaseNamespace, "secrets", obj(release["metadata"])["name"].(string), release}} {
		current, ok, err := read.cli.Get(ctx, read.namespace, read.kind, read.name)
		if err != nil || !ok || !sameIncarnation(read.previous, current) {
			return nil, false, ErrOwnership
		}
	}
	raw, err = target.Run(ctx, nil, "get", "secrets", "-n", cfg.ReleaseNamespace, "-l", "owner=helm,name="+pkg.ReleaseName, "-o", "json")
	if err != nil || json.Unmarshal(raw, &releases) != nil {
		return nil, false, ErrOwnership
	}
	latest, err := latestFailedJobRelease(releases.Items, pkg.ReleaseName, cfg.ReleaseNamespace)
	if err != nil || !sameIncarnation(release, latest) {
		return nil, false, ErrOwnership
	}
	return inventory, true, nil
}
func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func cloneObject(m map[string]any) map[string]any {
	raw, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
func sameIncarnation(a, b map[string]any) bool {
	x, y := obj(a["metadata"]), obj(b["metadata"])
	return x["uid"] != "" && x["uid"] != nil && x["resourceVersion"] != nil && x["uid"] == y["uid"] && x["resourceVersion"] == y["resourceVersion"] && y["deletionTimestamp"] == nil
}
func latestFailedJobRelease(items []map[string]any, name, namespace string) (map[string]any, error) {
	var selected map[string]any
	version := 0
	for _, item := range items {
		md := obj(item["metadata"])
		labels := obj(md["labels"])
		v, _ := strconv.Atoi(stringValue(labels["version"]))
		if v <= 0 || labels["owner"] != "helm" || labels["name"] != name || md["namespace"] != namespace || md["name"] != "sh.helm.release.v1."+name+".v"+strconv.Itoa(v) || md["uid"] == nil || md["resourceVersion"] == nil || md["deletionTimestamp"] != nil {
			return nil, ErrOwnership
		}
		if v == version {
			return nil, ErrOwnership
		}
		if v > version {
			selected = item
			version = v
		}
	}
	if selected == nil || obj(obj(selected["metadata"])["labels"])["status"] != "failed" {
		return nil, domain.ErrOutcomeUnknown
	}
	return selected, nil
}
func stringValue(v any) string { s, _ := v.(string); return s }
func helmJobManifest(secret map[string]any, name, namespace string) ([]map[string]any, error) {
	raw, err := base64.StdEncoding.DecodeString(stringValue(obj(secret["data"])["release"]))
	if err != nil {
		return nil, ErrOwnership
	}
	raw, err = base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, ErrOwnership
	}
	z, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrOwnership
	}
	defer z.Close()
	raw, err = io.ReadAll(io.LimitReader(z, 10<<20))
	if err != nil || len(raw) >= 10<<20 {
		return nil, ErrOwnership
	}
	var release struct {
		Name, Namespace, Manifest string
		Info                      struct{ Status string }
	}
	if json.Unmarshal(raw, &release) != nil || release.Name != name || release.Namespace != namespace || release.Info.Status != "failed" {
		return nil, ErrOwnership
	}
	decoder := yaml.NewDecoder(strings.NewReader(release.Manifest))
	objects := []map[string]any{}
	for {
		var m map[string]any
		err := decoder.Decode(&m)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrOwnership
		}
		if len(m) > 0 {
			if m["kind"] != "Job" {
				return nil, ErrOwnership
			}
			objects = append(objects, m)
		}
	}
	return objects, nil
}
func terminalJobIdentity(job, expected map[string]any, i port.Intent, r port.Receipt) (string, bool, bool, error) {
	md := obj(job["metadata"])
	labels, ann := obj(md["labels"]), obj(md["annotations"])
	uid := stringValue(md["uid"])
	if uid == "" || md["resourceVersion"] == nil || md["namespace"] != i.Target.Namespace || md["name"] != obj(expected["metadata"])["name"] || md["deletionTimestamp"] != nil || labels["orchestrator.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != i.Target.Extra["application"] || labels["orchestrator.io/environment"] != i.Target.Extra["environment"] || ann["orchestrator.io/delivery-revision"] != i.Revision.ID || ann["meta.helm.sh/release-name"] != r.Handle["release"] || ann["meta.helm.sh/release-namespace"] != r.Handle["releaseNamespace"] || obj(cloneObject(job)["metadata"])["generation"] != float64(1) || !expectedJobFields(obj(cloneObject(expected)["spec"]), obj(cloneObject(job)["spec"])) {
		return "", false, false, ErrOwnership
	}
	status := obj(cloneObject(job)["status"])
	if n, _ := status["active"].(float64); n != 0 {
		return uid, false, false, nil
	}
	if n, _ := status["terminating"].(float64); n != 0 {
		return uid, false, false, nil
	}
	failed, complete := false, false
	conditions, _ := status["conditions"].([]any)
	for _, raw := range conditions {
		c := obj(raw)
		if c["status"] == "True" {
			failed = failed || c["type"] == "Failed"
			complete = complete || c["type"] == "Complete"
		}
	}
	if failed && complete {
		return "", false, false, ErrOwnership
	}
	return uid, failed || complete, failed, nil
}
func expectedJobFields(expected, live any) bool {
	switch e := expected.(type) {
	case map[string]any:
		l, ok := live.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range e {
			if !expectedJobFields(v, l[k]) {
				return false
			}
		}
		return true
	case []any:
		l, ok := live.([]any)
		if !ok || len(e) != len(l) {
			return false
		}
		for n := range e {
			if !expectedJobFields(e[n], l[n]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(expected, live)
	}
}
func jobPodsStopped(pods []map[string]any, uid, name string) bool {
	for _, pod := range pods {
		md := obj(pod["metadata"])
		if md["deletionTimestamp"] != nil {
			return false
		}
		owned := false
		owners, _ := md["ownerReferences"].([]any)
		for _, raw := range owners {
			o := obj(raw)
			owned = owned || (o["uid"] == uid && o["kind"] == "Job" && o["name"] == name)
		}
		phase := obj(pod["status"])["phase"]
		if !owned || (phase != "Succeeded" && phase != "Failed") {
			return false
		}
	}
	return true
}

// Fleet adds these two ObjectSet tracking keys during post-render. They are
// controller provenance; exact release/controller and native ownership are
// verified separately. No business metadata or Pod field is ignored here.
func terminalJobExecutionObject(source map[string]any) map[string]any {
	out := cloneObject(source)
	md := obj(out["metadata"])
	delete(obj(md["annotations"]), "objectset.rio.cattle.io/id")
	delete(obj(md["labels"]), "objectset.rio.cattle.io/hash")
	return out
}
