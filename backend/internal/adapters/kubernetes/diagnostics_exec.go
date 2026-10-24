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

func (s *kubeSession) podManifest(name, runID, image, claim string) map[string]any {
	spec := map[string]any{
		"restartPolicy": "Never", "terminationGracePeriodSeconds": 0, "automountServiceAccountToken": false,
		"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "seccompProfile": map[string]any{"type": "RuntimeDefault"}},
		"containers": []any{map[string]any{
			"name": "probe", "image": image, "imagePullPolicy": "IfNotPresent",
			"resources":       map[string]any{"requests": map[string]any{"cpu": "5m", "memory": "8Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "32Mi"}},
			"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []any{"ALL"}}},
		}},
	}
	if claim != "" {
		spec["volumes"] = []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": claim}}}
		spec["containers"].([]any)[0].(map[string]any)["volumeMounts"] = []any{map[string]any{"name": "data", "mountPath": "/data"}}
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

// waitPod polls the pod until Ready or a fixed, observed failure.
func (s *kubeSession) waitPod(ctx context.Context, name string, limit time.Duration) (string, map[string]string) {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var unschedulableSince time.Time
	phase := "Pending"
	for {
		obj, found, err := s.cli.Get(ctx, s.nsName, "pod", name)
		if err == nil && found {
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

func (s *kubeSession) execWorkload(ctx context.Context, rec probes.Recorder) domain.Finding {
	if s.nsName == "" {
		return finding(domain.VerdictBlocked, "PREREQUISITE_FAILED", "", nil)
	}
	name := "probe-" + randomHex(4)
	if f, ok := s.createOwned(ctx, rec, "pod", name, s.podManifest(name, rec.RunID(), s.probeImage(ctx), "")); !ok {
		return f
	}
	started := time.Now()
	reason, facts := s.waitPod(ctx, name, 90*time.Second)
	if reason == "READY" {
		facts["latencyMs"] = domain.Itoa(int(time.Since(started).Milliseconds()))
		return finding(domain.VerdictPass, "WORKLOAD_READY", "", facts)
	}
	if reason == "CANCELLED" {
		return finding(domain.VerdictUnknown, "CHECK_TIMEOUT", "rerun", nil)
	}
	return finding(domain.VerdictFail, reason, "check-scheduling", facts)
}

func (s *kubeSession) execVolume(ctx context.Context, rec probes.Recorder) domain.Finding {
	if s.nsName == "" {
		return finding(domain.VerdictBlocked, "PREREQUISITE_FAILED", "", nil)
	}
	claim, pod := "probe-data-"+randomHex(4), "probe-volume-"+randomHex(4)
	pvc := map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": claim, "namespace": s.nsName, "labels": s.labels(rec.RunID())},
		"spec": map[string]any{"accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": "1Gi"}}}}
	if f, ok := s.createOwned(ctx, rec, "persistentvolumeclaim", claim, pvc); !ok {
		return f
	}
	if f, ok := s.createOwned(ctx, rec, "pod", pod, s.podManifest(pod, rec.RunID(), s.probeImage(ctx), claim)); !ok {
		return f
	}
	reason, facts := s.waitPod(ctx, pod, 120*time.Second)
	if reason == "CANCELLED" {
		return finding(domain.VerdictUnknown, "CHECK_TIMEOUT", "rerun", nil)
	}
	obj, found, err := s.cli.Get(ctx, s.nsName, "persistentvolumeclaim", claim)
	phase := "Unknown"
	if err == nil && found {
		phase = str(obj, "status", "phase")
	}
	facts = fact("claimPhase", phase)
	if reason != "READY" || phase != "Bound" {
		if reason == "READY" || reason == "POD_NOT_READY" || reason == "POD_UNSCHEDULABLE" {
			reason = "PVC_NOT_BOUND"
		}
		return finding(domain.VerdictFail, reason, "repair-provisioner", facts)
	}
	if pv := str(obj, "spec", "volumeName"); pv != "" {
		if pvObj, ok, err := s.cli.Get(ctx, "", "persistentvolume", pv); err == nil && ok {
			_ = rec.Record(ctx, domain.ProbeResource{Kind: "persistentvolume", Name: pv, UID: str(pvObj, "metadata", "uid"), State: domain.ResourceCreated})
		}
	}
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
// same-name replacement or foreign object untouched.
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
	nsState := ""
	var nsName string
	for i := range out {
		if out[i].Kind != "namespace" || out[i].Settled() {
			if out[i].Kind == "namespace" {
				nsState, nsName = out[i].State, out[i].Name
			}
			continue
		}
		out[i] = s.cleanNamespace(ctx, out[i], runID)
		nsState, nsName = out[i].State, out[i].Name
	}
	var pvcUID string
	for _, r := range out {
		if r.Kind == "persistentvolumeclaim" {
			pvcUID = r.UID
		}
	}
	for i := range out {
		r := &out[i]
		if r.Settled() || r.Kind == "namespace" {
			continue
		}
		switch r.Kind {
		case "pod", "persistentvolumeclaim":
			switch nsState {
			case domain.ResourceDeleted, domain.ResourceAbsent:
				r.State = domain.ResourceAbsent
				if r.UID != "" {
					r.State = domain.ResourceDeleted
				}
			case domain.ResourcePreserved:
				r.State = domain.ResourcePreserved
			default:
				r.State = domain.ResourceUnknown
			}
		case "persistentvolume":
			*r = s.cleanVolume(ctx, *r, nsName, nsState, pvcUID)
		}
	}
	return out
}

func (s *kubeSession) cleanNamespace(ctx context.Context, r domain.ProbeResource, runID string) domain.ProbeResource {
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
	if err := s.deleteRawUID(ctx, "/api/v1/namespaces/"+r.Name, uid); err != nil {
		r.State = domain.ResourceUnknown
		return r
	}
	r.State = s.waitAbsent(ctx, "", "namespace", r.Name, uid)
	return r
}

func (s *kubeSession) cleanVolume(ctx context.Context, r domain.ProbeResource, namespace, nsState, pvcUID string) domain.ProbeResource {
	if nsState == domain.ResourcePreserved {
		r.State = domain.ResourcePreserved
		return r
	}
	if nsState != domain.ResourceDeleted && nsState != domain.ResourceAbsent {
		r.State = domain.ResourceUnknown
		return r
	}
	wait, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if state := s.waitAbsent(wait, "", "persistentvolume", r.Name, r.UID); state == domain.ResourceDeleted {
		r.State = state
		return r
	}
	obj, found, err := s.cli.Get(ctx, "", "persistentvolume", r.Name)
	if err != nil {
		r.State = domain.ResourceUnknown
		return r
	}
	if !found {
		r.State = domain.ResourceDeleted
		return r
	}
	uid := str(obj, "metadata", "uid")
	claim := mapAt(mapAt(obj, "spec"), "claimRef")
	if uid != r.UID || str(claim, "namespace") != namespace || (pvcUID != "" && str(claim, "uid") != pvcUID) {
		r.State = domain.ResourcePreserved
		return r
	}
	if err := s.deleteRawUID(ctx, "/api/v1/persistentvolumes/"+r.Name, uid); err != nil {
		r.State = domain.ResourceUnknown
		return r
	}
	r.State = s.waitAbsent(ctx, "", "persistentvolume", r.Name, uid)
	return r
}
