package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
)

// Execution probes create only run-owned objects: a cryptographically unique
// namespace carrying the run, Organization and account ownership labels, and
// inside it a platform-owned pod (and claim). Nothing is applied into an
// Application namespace and no existing object is replaced.

func (s *kubeSession) labels(runID string) map[string]any {
	return map[string]any{
		"app.kubernetes.io/managed-by":   managedBy,
		"orchestrator.io/diagnostic-run": runID,
		"orchestrator.io/organization":   s.org,
		"orchestrator.io/account":        s.account,
	}
}

func (s *kubeSession) create(ctx context.Context, object map[string]any) (string, error) {
	payload, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	out, err := s.cli.Run(ctx, payload, "create", "-f", "-", "-o", "json")
	if err != nil {
		return "", err
	}
	var created map[string]any
	if json.Unmarshal(out, &created) != nil {
		return "", errors.New("decode")
	}
	return str(created, "metadata", "uid"), nil
}

func (s *kubeSession) probeImage(ctx context.Context) string {
	if s.d.ProbeImage != "" {
		return s.d.ProbeImage
	}
	// Prefer an image a node already holds, so no registry is contacted.
	if l, err := s.list(ctx, "get", "nodes"); err == nil {
		for _, n := range l.Items {
			images, _ := mapAt(n, "status")["images"].([]any)
			for _, img := range images {
				names, _ := asMap(img)["names"].([]any)
				for _, name := range names {
					if text, _ := name.(string); strings.Contains(text, "/pause:") {
						return text
					}
				}
			}
		}
	}
	return "registry.k8s.io/pause:3.10"
}

func createFailure(err error) (domain.Finding, bool) {
	switch {
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrUnauthorized):
		return finding(domain.VerdictFail, "PERMISSION_DENIED", "grant-rbac", nil), true
	case errors.Is(err, ErrRequestRejected):
		return finding(domain.VerdictFail, "REQUEST_REJECTED", "check-admission", nil), true
	}
	return finding(domain.VerdictUnknown, "CREATE_UNCONFIRMED", "run-cleanup-check", nil), false
}

func (s *kubeSession) execNamespace(ctx context.Context, rec probes.Recorder) domain.Finding {
	name := "orch-diag-" + randomHex(6)
	res := domain.ProbeResource{Kind: "namespace", Name: name, State: domain.ResourceIntended}
	if err := rec.Record(ctx, res); err != nil {
		return finding(domain.VerdictUnknown, "RECORD_FAILED", "rerun", nil)
	}
	uid, err := s.create(ctx, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": name, "labels": s.labels(rec.RunID())}})
	if err != nil {
		f, definitive := createFailure(err)
		if definitive {
			res.State = domain.ResourceAbsent
			_ = rec.Record(ctx, res)
		}
		return f
	}
	res.UID, res.State = uid, domain.ResourceCreated
	if err := rec.Record(ctx, res); err != nil {
		return finding(domain.VerdictUnknown, "RECORD_FAILED", "rerun", nil)
	}
	s.nsName, s.nsUID = name, uid
	obj, found, err := s.cli.Get(ctx, "", "namespace", name)
	if err != nil || !found || str(obj, "status", "phase") != "Active" {
		return finding(domain.VerdictFail, "NAMESPACE_NOT_ACTIVE", "check-cluster", fact("namespace", name))
	}
	return finding(domain.VerdictPass, "NAMESPACE_CREATED", "", fact("namespace", name, "uid", uid))
}

func (s *kubeSession) podManifest(name, runID, image string, claims []string) map[string]any {
	spec := map[string]any{
		"restartPolicy": "Never", "terminationGracePeriodSeconds": 0, "automountServiceAccountToken": false,
		"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "seccompProfile": map[string]any{"type": "RuntimeDefault"}},
		"containers": []any{map[string]any{
			"name": "probe", "image": image, "imagePullPolicy": "IfNotPresent",
			"resources":       map[string]any{"requests": map[string]any{"cpu": "5m", "memory": "8Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "32Mi"}},
			"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []any{"ALL"}}},
		}},
	}
	if len(claims) > 0 {
		var volumes, mounts []any
		for i, claim := range claims {
			name := "data-" + domain.Itoa(i)
			volumes = append(volumes, map[string]any{"name": name, "persistentVolumeClaim": map[string]any{"claimName": claim}})
			mounts = append(mounts, map[string]any{"name": name, "mountPath": "/data/" + name})
		}
		spec["volumes"] = volumes
		spec["containers"].([]any)[0].(map[string]any)["volumeMounts"] = mounts
	}
	return map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": s.nsName, "labels": s.labels(runID)}, "spec": spec}
}

// createOwned records the intent, creates the object and records its UID.
func (s *kubeSession) createOwned(ctx context.Context, rec probes.Recorder, kind, name string, object map[string]any) (domain.Finding, bool) {
	res := domain.ProbeResource{Kind: kind, Namespace: s.nsName, Name: name, State: domain.ResourceIntended}
	if err := rec.Record(ctx, res); err != nil {
		return finding(domain.VerdictUnknown, "RECORD_FAILED", "rerun", nil), false
	}
	uid, err := s.create(ctx, object)
	if err != nil {
		f, definitive := createFailure(err)
		if definitive {
			res.State = domain.ResourceAbsent
			_ = rec.Record(ctx, res)
		}
		return f, false
	}
	res.UID, res.State = uid, domain.ResourceCreated
	if rec.Record(ctx, res) != nil {
		return finding(domain.VerdictUnknown, "RECORD_FAILED", "rerun", nil), false
	}
	return domain.Finding{}, true
}

// waitPod polls the pod until Ready or a fixed, observed condition. A failed
// read (authorization, transient API or network error) is never a verdict
// about the pod: it ends the wait as READ_FORBIDDEN / READ_FAILED so the caller
// reports UNKNOWN and cleanup still runs.
func (s *kubeSession) waitPod(ctx context.Context, name string, limit time.Duration) (string, map[string]string) {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var unschedulableSince time.Time
	phase, failures := "Pending", 0
	for {
		obj, found, err := s.cli.Get(ctx, s.nsName, "pod", name)
		switch {
		case err != nil && errors.Is(err, ErrForbidden):
			return "READ_FORBIDDEN", nil
		case err != nil && ctx.Err() == nil:
			if failures++; failures >= 3 {
				return "READ_FAILED", nil
			}
		case err == nil && !found:
			return "POD_DISAPPEARED", fact("podPhase", phase)
		case err == nil:
			failures = 0
			if p := str(obj, "status", "phase"); p != "" {
				phase = p
			}
			conds, _ := mapAt(obj, "status")["conditions"].([]any)
			for _, c := range conds {
				cm := asMap(c)
				if cm["type"] == "Ready" && cm["status"] == "True" {
					return "READY", fact("podPhase", phase)
				}
				if cm["type"] == "PodScheduled" && cm["status"] == "False" && cm["reason"] == "Unschedulable" {
					if unschedulableSince.IsZero() {
						unschedulableSince = time.Now()
					} else if time.Since(unschedulableSince) > 10*time.Second {
						return "POD_UNSCHEDULABLE", fact("podPhase", phase)
					}
				}
			}
			statuses, _ := mapAt(obj, "status")["containerStatuses"].([]any)
			for _, cs := range statuses {
				switch str(asMap(cs), "state", "waiting", "reason") {
				case "ImagePullBackOff", "InvalidImageName", "ErrImageNeverPull":
					return "IMAGE_PULL_FAILED", fact("podPhase", phase)
				}
			}
		}
		select {
		case <-ctx.Done():
			return "CANCELLED", nil
		case <-deadline.C:
			return "POD_NOT_READY", fact("podPhase", phase)
		case <-tick.C:
		}
	}
}

// waitVerdict maps a waitPod outcome to a finding: observed conditions are
// FAIL, unreadable state or cancellation is UNKNOWN.
func waitVerdict(reason string, facts map[string]string, remediation string) domain.Finding {
	switch reason {
	case "READ_FORBIDDEN":
		return finding(domain.VerdictUnknown, "READ_FORBIDDEN", "grant-read", nil)
	case "READ_FAILED":
		return finding(domain.VerdictUnknown, "READ_FAILED", "rerun", nil)
	case "CANCELLED":
		return finding(domain.VerdictUnknown, "CHECK_TIMEOUT", "rerun", nil)
	}
	return finding(domain.VerdictFail, reason, remediation, facts)
}

func (s *kubeSession) execWorkload(ctx context.Context, rec probes.Recorder) domain.Finding {
	if s.nsName == "" {
		return finding(domain.VerdictBlocked, "PREREQUISITE_FAILED", "", nil)
	}
	name := "probe-" + randomHex(4)
	if f, ok := s.createOwned(ctx, rec, "pod", name, s.podManifest(name, rec.RunID(), s.probeImage(ctx), nil)); !ok {
		return f
	}
	started := time.Now()
	reason, facts := s.waitPod(ctx, name, 90*time.Second)
	if reason == "READY" {
		facts["latencyMs"] = domain.Itoa(int(time.Since(started).Milliseconds()))
		return finding(domain.VerdictPass, "WORKLOAD_READY", "", facts)
	}
	return waitVerdict(reason, facts, "check-scheduling")
}

// volumeNeed is what the selected capability resources placed on this account
// request from the cluster's storage.
type volumeNeed struct {
	classes                         []string // explicit StorageClasses
	needDefault                     bool
	unresolved, undeclared, hasNeed bool
}

// volumeNeeds derives the storage requirement of the selected capabilities
// from the resolved requirements. reason is non-empty when nothing can be said.
func volumeNeeds(req *probes.Requirements, scope domain.Scope, account string) (volumeNeed, string) {
	if req == nil || !req.Resolved {
		return volumeNeed{}, "STORAGE_REQUIREMENT_UNRESOLVED"
	}
	selected := map[string]bool{}
	for _, u := range scope.UseCases {
		selected[u] = true
	}
	var storage []probes.ResourceNeed
	var need volumeNeed
	for _, r := range req.Resources {
		if !selected[domain.CapabilityOf(r.Type)] || !placedOn(r, account) {
			continue
		}
		switch {
		case r.Storage.Required:
			storage = append(storage, r)
		case !r.Storage.Resolved:
			need.undeclared = true
		}
	}
	need.hasNeed = len(storage) > 0
	need.classes, need.needDefault, need.unresolved = req.StorageClassesNeeded(storage)
	return need, ""
}

// volumeTargets returns the storage classes an execution probe must exercise,
// refusing (with a reason) whenever the requirement is not exact.
func volumeTargets(req *probes.Requirements, scope domain.Scope, account string) (classes []string, needDefault bool, reason string) {
	need, reason := volumeNeeds(req, scope, account)
	switch {
	case reason != "":
		return nil, false, reason
	case !need.hasNeed && need.undeclared:
		return nil, false, "STORAGE_REQUIREMENT_UNDECLARED"
	case !need.hasNeed:
		return nil, false, "STORAGE_NOT_REQUIRED"
	case need.unresolved:
		return nil, false, "STORAGE_CLASS_UNRESOLVED"
	case len(need.classes)+btoi(need.needDefault) > 3:
		return nil, false, "TOO_MANY_STORAGE_CLASSES"
	}
	return need.classes, need.needDefault, ""
}

// storageRequirements compares the exact requirement of the selected work with
// the FULL StorageClass inventory (never the truncated public list).
func (s *kubeSession) storageRequirements(ctx context.Context, req *probes.Requirements) domain.Finding {
	need, reason := volumeNeeds(req, s.scope, s.account)
	switch {
	case reason != "":
		return finding(domain.VerdictUnknown, reason, "select-context", nil)
	case !need.hasNeed && need.undeclared:
		return finding(domain.VerdictUnknown, "STORAGE_REQUIREMENT_UNDECLARED", "", nil)
	case !need.hasNeed:
		return finding(domain.VerdictNA, "STORAGE_NOT_REQUIRED", "", nil)
	case need.unresolved:
		return finding(domain.VerdictUnknown, "STORAGE_CLASS_UNRESOLVED", "use-literal-storage-class", nil)
	}
	inventory, err := s.storageClassList(ctx)
	if err != nil {
		return readFailure(err)
	}
	have, defaults := map[string]bool{}, 0
	for _, sc := range inventory {
		have[str(sc, "metadata", "name")] = true
		if isDefaultClass(sc) {
			defaults++
		}
	}
	var missing []string
	for _, c := range need.classes {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	facts := fact("storageNeeds", domain.Itoa(len(need.classes)+btoi(need.needDefault)), "defaultClasses", domain.Itoa(defaults))
	if len(need.classes) > 0 {
		facts["requiredClasses"] = domain.JoinList(sortedList(need.classes))
	}
	switch {
	case len(missing) > 0:
		facts["requiredClasses"] = domain.JoinList(sortedList(missing))
		return finding(domain.VerdictFail, "STORAGE_CLASS_MISSING", "create-storage-class", facts)
	case need.needDefault && defaults == 0:
		return finding(domain.VerdictFail, "NO_DEFAULT_STORAGE_CLASS", "set-default-storage-class", facts)
	case need.needDefault && defaults > 1:
		return finding(domain.VerdictFail, "MULTIPLE_DEFAULT_STORAGE_CLASSES", "set-default-storage-class", facts)
	}
	return finding(domain.VerdictPass, "STORAGE_REQUIREMENTS_MET", "", facts)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func placedOn(r probes.ResourceNeed, account string) bool {
	for _, p := range r.Providers {
		if p.Kind == "kubernetes" && p.Connection == account {
			return true
		}
	}
	return r.Cluster == account
}

// execVolume exercises the exact StorageClass(es) the selected capability
// resolved to. It creates nothing when the requirement is unresolved,
// undeclared or the class's reclaim policy would leave backing storage behind.
func (s *kubeSession) execVolume(ctx context.Context, rec probes.Recorder, req *probes.Requirements) domain.Finding {
	classes, needDefault, reason := volumeTargets(req, s.scope, s.account)
	switch reason {
	case "STORAGE_NOT_REQUIRED":
		return finding(domain.VerdictNA, reason, "", nil)
	case "":
	default:
		return finding(domain.VerdictUnknown, reason, "use-literal-storage-class", nil)
	}
	if s.nsName == "" {
		return finding(domain.VerdictBlocked, "PREREQUISITE_FAILED", "", nil)
	}
	inventory, err := s.storageClassList(ctx)
	if err != nil {
		return readFailure(err)
	}
	byName, defaultName, defaults := map[string]map[string]any{}, "", 0
	for _, sc := range inventory {
		byName[str(sc, "metadata", "name")] = sc
		if isDefaultClass(sc) {
			defaultName, defaults = str(sc, "metadata", "name"), defaults+1
		}
	}
	type target struct{ requested, name string }
	var targets []target
	for _, c := range classes {
		targets = append(targets, target{requested: c, name: c})
	}
	if needDefault {
		if defaults != 1 {
			return finding(domain.VerdictFail, map[bool]string{true: "NO_DEFAULT_STORAGE_CLASS", false: "MULTIPLE_DEFAULT_STORAGE_CLASSES"}[defaults == 0], "set-default-storage-class", fact("defaultClasses", domain.Itoa(defaults)))
		}
		targets = append(targets, target{requested: "", name: defaultName})
	}
	var tested []string
	for _, t := range targets {
		sc, ok := byName[t.name]
		if !ok {
			return finding(domain.VerdictFail, "STORAGE_CLASS_MISSING", "create-storage-class", fact("requiredClasses", t.name))
		}
		if policy := str(sc, "reclaimPolicy"); policy != "Delete" {
			// Retain would leave the backing volume behind after the probe.
			return finding(domain.VerdictUnknown, "EXECUTION_UNSAFE_RECLAIM_POLICY", "", fact("reclaimPolicy", strings.ToUpper(policy), "requiredClasses", t.name))
		}
		tested = append(tested, t.name)
	}
	var claims []string
	for _, t := range targets {
		claim := "probe-data-" + randomHex(4)
		spec := map[string]any{"accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": "1Gi"}}}
		if t.requested != "" {
			spec["storageClassName"] = t.requested
		}
		pvc := map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": claim, "namespace": s.nsName, "labels": s.labels(rec.RunID())}, "spec": spec}
		if f, ok := s.createOwned(ctx, rec, "persistentvolumeclaim", claim, pvc); !ok {
			return f
		}
		claims = append(claims, claim)
	}
	pod := "probe-volume-" + randomHex(4)
	if f, ok := s.createOwned(ctx, rec, "pod", pod, s.podManifest(pod, rec.RunID(), s.probeImage(ctx), claims)); !ok {
		return f
	}
	reason2, facts := s.waitPod(ctx, pod, 120*time.Second)
	if reason2 == "READ_FORBIDDEN" || reason2 == "READ_FAILED" || reason2 == "CANCELLED" {
		return waitVerdict(reason2, nil, "")
	}
	facts = fact("classesTested", domain.JoinList(tested))
	for _, claim := range claims {
		obj, found, err := s.cli.Get(ctx, s.nsName, "persistentvolumeclaim", claim)
		if err != nil {
			return readFailure(err)
		}
		phase := ""
		if found {
			phase = str(obj, "status", "phase")
		}
		if phase != "Bound" {
			facts["claimPhase"] = "Pending"
			if reason2 == "READY" || reason2 == "POD_NOT_READY" || reason2 == "POD_UNSCHEDULABLE" {
				reason2 = "PVC_NOT_BOUND"
			}
			return finding(domain.VerdictFail, reason2, "repair-provisioner", facts)
		}
		// Record the bound volume identity; if that fails the run stops here and
		// cleanup finds the volume through the claim's UID.
		if pv := str(obj, "spec", "volumeName"); pv != "" {
			pvObj, ok, err := s.cli.Get(ctx, "", "persistentvolume", pv)
			if err != nil || !ok {
				return finding(domain.VerdictUnknown, "VOLUME_UNOBSERVED", "run-cleanup-check", facts)
			}
			if rec.Record(ctx, domain.ProbeResource{Kind: "persistentvolume", Name: pv, UID: str(pvObj, "metadata", "uid"), State: domain.ResourceCreated}) != nil {
				return finding(domain.VerdictUnknown, "RECORD_FAILED", "run-cleanup-check", facts)
			}
		}
	}
	if reason2 != "READY" {
		return waitVerdict(reason2, facts, "repair-provisioner")
	}
	facts["claimPhase"] = "Bound"
	return finding(domain.VerdictPass, "VOLUME_PROVISIONED", "", facts)
}

// deleteRawUID deletes one object only while its API UID still equals uid; the
// API server enforces the precondition atomically.
func (s *kubeSession) deleteRawUID(ctx context.Context, path, uid string) error {
	payload, err := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]any{"uid": uid}})
	if err != nil {
		return err
	}
	_, err = s.cli.Run(ctx, payload, "delete", "--raw", path, "-f", "-")
	if errors.Is(err, ErrObjectNotFound) {
		return nil
	}
	return err
}

// ownedBy reports whether an observed object carries this run's ownership.
func (s *kubeSession) ownedBy(obj map[string]any, runID string) bool {
	labels := mapAt(mapAt(obj, "metadata"), "labels")
	return labels["app.kubernetes.io/managed-by"] == managedBy && labels["orchestrator.io/diagnostic-run"] == runID &&
		labels["orchestrator.io/organization"] == s.org && labels["orchestrator.io/account"] == s.account
}

// waitAbsent polls until the object is gone, or reports a replacement.
func (s *kubeSession) waitAbsent(ctx context.Context, namespace, kind, name, uid string) string {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		obj, found, err := s.cli.Get(ctx, namespace, kind, name)
		switch {
		case err == nil && !found:
			return domain.ResourceDeleted
		case err == nil && str(obj, "metadata", "uid") != uid:
			return domain.ResourcePreserved
		}
		select {
		case <-ctx.Done():
			return domain.ResourceUnknown
		case <-tick.C:
		}
	}
}

// Cleanup implements probes.Session. It deletes the run's namespace only after
// proving ownership and the recorded UID, verifies native absence, and leaves a
// same-name replacement or foreign object untouched. Backing volumes are found
// through the claim UIDs (never trusted from a record alone) and are verified
// absent; a volume the provisioner did not remove is reported UNKNOWN and is
// never deleted by hand, so physical backing storage is never claimed cleaned.
func (s *kubeSession) Cleanup(ctx context.Context, resources []domain.ProbeResource, runID string) []domain.ProbeResource {
	out := append([]domain.ProbeResource(nil), resources...)
	if s.reason != "" {
		for i := range out {
			if !out[i].Settled() {
				out[i].State = domain.ResourceUnknown
			}
		}
		return out
	}
	nsState, nsName := "", ""
	claimsKnown := true
	for i := range out {
		if out[i].Kind != "namespace" {
			continue
		}
		if !out[i].Settled() {
			// A claim whose creation or record acknowledgement was lost may still
			// exist (and may have bound a volume): find the owned claims in the
			// namespace before it is deleted.
			name := out[i].Name
			out[i] = s.cleanNamespace(ctx, out[i], runID, func() {
				var ok bool
				out, ok = s.adoptClaims(ctx, out, name, runID)
				claimsKnown = claimsKnown && ok
			})
		}
		nsState, nsName = out[i].State, out[i].Name
	}
	claims := map[string]string{} // claim UID or namespace/name -> namespace
	hasClaim := false
	for _, r := range out {
		if r.Kind != "persistentvolumeclaim" {
			continue
		}
		hasClaim = true
		if r.UID != "" {
			claims["uid:"+r.UID] = r.Namespace
		}
		claims["name:"+r.Namespace+"/"+r.Name] = r.Namespace
	}
	volumesKnown := !hasClaim
	// Volumes are looked up whenever a claim was ever intended, not only when
	// its UID was persisted: an empty UID map proves nothing.
	if (nsState == domain.ResourceDeleted || nsState == domain.ResourceAbsent) && hasClaim {
		out, volumesKnown = s.scanVolumes(ctx, out, claims, nsName)
	}
	for i := range out {
		r := &out[i]
		if r.Settled() || r.Kind == "namespace" || r.Kind == "persistentvolume" {
			continue
		}
		switch nsState {
		case domain.ResourceDeleted, domain.ResourceAbsent:
			switch {
			case r.Kind == "persistentvolumeclaim" && (!volumesKnown || !claimsKnown):
				r.State = domain.ResourceUnknown
			case r.UID != "":
				r.State = domain.ResourceDeleted
			default:
				r.State = domain.ResourceAbsent
			}
		case domain.ResourcePreserved:
			r.State = domain.ResourcePreserved
		default:
			r.State = domain.ResourceUnknown
		}
	}
	return out
}

// namespaceWait bounds how long cleanup waits for a deleted namespace to
// disappear.
var namespaceWait = 120 * time.Second

// volumeWait bounds how long cleanup waits for a provisioner to remove a bound
// volume (a variable so tests can shorten it).
var volumeWait = 90 * time.Second

// adoptClaims records the run-owned PersistentVolumeClaims actually present in
// the namespace (with their UIDs), including ones whose creation or record
// acknowledgement was lost. The bool is false when the claims could not be listed.
func (s *kubeSession) adoptClaims(ctx context.Context, out []domain.ProbeResource, namespace, runID string) ([]domain.ProbeResource, bool) {
	list, err := s.list(ctx, "get", "persistentvolumeclaims", "-n", namespace)
	if err != nil {
		return out, false
	}
	for _, item := range list.Items {
		if !s.ownedBy(item, runID) {
			continue
		}
		name, uid := str(item, "metadata", "name"), str(item, "metadata", "uid")
		if name == "" || uid == "" {
			continue
		}
		found := false
		for i := range out {
			if out[i].Kind == "persistentvolumeclaim" && out[i].Namespace == namespace && out[i].Name == name {
				found = true
				if out[i].UID == "" && !out[i].Settled() {
					out[i].UID, out[i].State = uid, domain.ResourceCreated
				}
			}
		}
		if !found && len(out) < domain.MaxProbeItems {
			out = append(out, domain.ProbeResource{Kind: "persistentvolumeclaim", Namespace: namespace, Name: name, UID: uid, State: domain.ResourceCreated})
		}
	}
	return out, true
}

// scanVolumes finds every PersistentVolume bound to one of the run's claims,
// records it, and waits for the provisioner to remove it.
func (s *kubeSession) scanVolumes(ctx context.Context, out []domain.ProbeResource, claims map[string]string, namespace string) ([]domain.ProbeResource, bool) {
	all, err := s.list(ctx, "get", "persistentvolumes")
	if err != nil {
		for i := range out {
			if out[i].Kind == "persistentvolume" && !out[i].Settled() {
				out[i].State = domain.ResourceUnknown
			}
		}
		return out, false
	}
	owned := map[string]map[string]any{}
	for _, pv := range all.Items {
		claim := mapAt(mapAt(pv, "spec"), "claimRef")
		if _, byUID := claims["uid:"+str(claim, "uid")]; (byUID || claims["name:"+str(claim, "namespace")+"/"+str(claim, "name")] != "") && str(claim, "namespace") == namespace {
			owned[str(pv, "metadata", "name")] = pv
		}
	}
	// A recorded volume that is no longer listed under its claim is checked by name.
	byName := map[string]int{}
	for i := range out {
		if out[i].Kind == "persistentvolume" {
			byName[out[i].Name] = i
		}
	}
	for name, pv := range owned {
		if _, recorded := byName[name]; !recorded && len(out) < domain.MaxProbeItems {
			out = append(out, domain.ProbeResource{Kind: "persistentvolume", Name: name, UID: str(pv, "metadata", "uid"), State: domain.ResourceCreated})
			byName[name] = len(out) - 1
		}
	}
	for i := range out {
		if out[i].Kind != "persistentvolume" || out[i].Settled() {
			continue
		}
		wait, cancel := context.WithTimeout(ctx, volumeWait)
		state := s.waitAbsent(wait, "", "persistentvolume", out[i].Name, out[i].UID)
		cancel()
		switch state {
		case domain.ResourceDeleted:
			out[i].State = domain.ResourceDeleted
		case domain.ResourcePreserved:
			out[i].State = domain.ResourcePreserved
		default:
			// Still present (for example a Retain volume) or unreadable: the
			// backing storage is not shown to be gone.
			out[i].State = domain.ResourceUnknown
		}
	}
	return out, true
}

// cleanNamespace deletes one owned namespace. beforeDelete runs once ownership
// and UID are proven and the namespace still exists.
func (s *kubeSession) cleanNamespace(ctx context.Context, r domain.ProbeResource, runID string, beforeDelete func()) domain.ProbeResource {
	obj, found, err := s.cli.Get(ctx, "", "namespace", r.Name)
	switch {
	case err != nil:
		r.State = domain.ResourceUnknown
		return r
	case !found:
		r.State = domain.ResourceAbsent
		if r.UID != "" {
			r.State = domain.ResourceDeleted
		}
		return r
	}
	uid := str(obj, "metadata", "uid")
	if !s.ownedBy(obj, runID) || (r.UID != "" && uid != r.UID) || uid == "" {
		r.State = domain.ResourcePreserved
		return r
	}
	if r.UID == "" {
		r.UID = uid
	}
	beforeDelete()
	if err := s.deleteRawUID(ctx, "/api/v1/namespaces/"+r.Name, uid); err != nil {
		r.State = domain.ResourceUnknown
		return r
	}
	wait, cancel := context.WithTimeout(ctx, namespaceWait)
	defer cancel()
	r.State = s.waitAbsent(wait, "", "namespace", r.Name, uid)
	return r
}
