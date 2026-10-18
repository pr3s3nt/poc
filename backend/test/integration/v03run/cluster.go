package v03run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"orchestrator/test/integration/v2run"
)

// Baseline is the pre-run cluster evidence: every namespace with its UID, so
// the post-run comparison can prove that nothing outside the run changed.
type Baseline struct {
	Context    string            `json:"context"`
	Namespaces map[string]string `json:"namespaces"`
	// RunNamespaces are the exact names the run may own. None exists before the run.
	RunNamespaces []string `json:"runNamespaces"`
}

// OwnedNamespace is one namespace proven to belong to the run: its recorded
// UID and the platform ownership label the product's Terraform module applied.
type OwnedNamespace struct {
	Name         string `json:"name"`
	UID          string `json:"uid"`
	RunID        string `json:"runId"`
	Instance     string `json:"instance"`
	TerraformRun string `json:"terraformRun"`
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

var reserved = map[string]bool{"default": true, "kube-system": true, "kube-public": true, "kube-node-lease": true}

// ValidateRunNamespaces refuses names that are not exact, owned and
// non-reserved: a run never owns default, kube-* or a shared namespace.
func ValidateRunNamespaces(names []string) error {
	if len(names) == 0 {
		return errors.New("at least one run namespace is required")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !dnsLabel.MatchString(name) || reserved[name] || strings.HasPrefix(name, "kube-") {
			return fmt.Errorf("namespace %q is not an acceptable run-owned name", name)
		}
		if seen[name] {
			return fmt.Errorf("namespace %q is listed twice", name)
		}
		seen[name] = true
	}
	return nil
}

type nsList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			UID    string            `json:"uid"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	} `json:"items"`
}

func listNamespaces(ctx context.Context, k v2run.Kubectl) (nsList, error) {
	raw, stderr, exit := k.Run(ctx, "get", "namespaces", "-o", "json")
	var list nsList
	if exit != 0 || json.Unmarshal(raw, &list) != nil {
		return nsList{}, fmt.Errorf("namespaces cannot be listed: %s", firstLine(stderr))
	}
	return list, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// Preflight is read-only: the cluster answers, a node is Ready, no run
// namespace exists yet and the operator may list namespaces. It returns the
// namespace UID baseline. Nothing is created.
func Preflight(ctx context.Context, k v2run.Kubectl, contextName string, runNamespaces []string) (Baseline, []v2run.Check, error) {
	if err := ValidateRunNamespaces(runNamespaces); err != nil {
		return Baseline{}, nil, err
	}
	var checks []v2run.Check
	add := func(name string, ok bool, note string) {
		checks = append(checks, v2run.Check{Name: name, OK: ok, Note: note})
	}
	raw, stderr, exit := k.Run(ctx, "get", "nodes", "-o", "json")
	var nodes struct {
		Items []struct {
			Status struct {
				Conditions []struct{ Type, Status string } `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if exit != 0 || json.Unmarshal(raw, &nodes) != nil {
		add("cluster-reachable", false, firstLine(stderr))
		return Baseline{}, checks, errors.New("the cluster is not reachable with the host context")
	}
	add("cluster-reachable", true, contextName)
	ready := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				ready++
			}
		}
	}
	add("ready-node", ready >= 1, fmt.Sprintf("%d ready", ready))
	list, err := listNamespaces(ctx, k)
	if err != nil {
		add("namespaces-listed", false, err.Error())
		return Baseline{}, checks, err
	}
	add("namespaces-listed", true, fmt.Sprintf("%d namespaces", len(list.Items)))
	base := Baseline{Context: contextName, Namespaces: map[string]string{}, RunNamespaces: append([]string(nil), runNamespaces...)}
	for _, item := range list.Items {
		base.Namespaces[item.Metadata.Name] = item.Metadata.UID
	}
	for _, name := range runNamespaces {
		_, exists := base.Namespaces[name]
		add("run-namespace-absent:"+name, !exists, "")
	}
	for _, check := range checks {
		if !check.OK {
			return base, checks, errors.New("preflight failed")
		}
	}
	return base, checks, nil
}

// Report is the safe summary of one verified workload.
type Report struct {
	Namespace  string            `json:"namespace"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	Env        map[string]string `json:"env"`
	Ready      bool              `json:"ready"`
	Replicas   int               `json:"replicas"`
	Service    bool              `json:"service"`
	Managed    bool              `json:"namespaceManaged"`
	Observed   bool              `json:"observedGeneration"`
	Pods       int               `json:"pods"`
	DigestPods int               `json:"podsWithExpectedDigest"`
	// Safe diagnostics (never a value of an environment variable): rollout counters, the image IDs the
	// Pods actually run, and the NAMES of the expected variables that differ.
	Generation, ObservedGeneration            int64
	UpdatedReplicas, AvailableReplicas, Total int
	ReadyReplicas                             int
	PodImageIDs                               []string
	ImageMatch                                bool
	EnvMismatch                               []string
}

// VerifyWorkload reads the Deployment, its Pods, its Service and the namespace
// labels and checks the image reference, the plain env values, a complete
// rollout (the controller observed the current generation; every replica is
// updated, available and ready; no old replica is left) and, when wantDigest is
// given, that the digest of the image RUNNING in every Pod equals it. A mismatch
// is an error naming the field only.
func VerifyWorkload(ctx context.Context, k v2run.Kubectl, namespace, name, wantImage, wantDigest string, wantEnv map[string]string, requireReady bool) (Report, error) {
	report := Report{Namespace: namespace, Name: name, Env: map[string]string{}}
	raw, stderr, exit := k.Run(ctx, "-n", namespace, "get", "deployment", name, "-o", "json")
	if exit != 0 {
		return report, fmt.Errorf("deployment %s/%s is missing: %s", namespace, name, firstLine(stderr))
	}
	var deployment struct {
		Metadata struct {
			Generation int64 `json:"generation"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int `json:"replicas"`
			Selector struct {
				MatchLabels map[string]string `json:"matchLabels"`
			} `json:"selector"`
			Template struct {
				Spec struct {
					Containers []struct {
						Image string `json:"image"`
						Env   []struct {
							Name  string `json:"name"`
							Value string `json:"value"`
						} `json:"env"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
		Status struct {
			ObservedGeneration int64 `json:"observedGeneration"`
			ReadyReplicas      int   `json:"readyReplicas"`
			AvailableReplicas  int   `json:"availableReplicas"`
			UpdatedReplicas    int   `json:"updatedReplicas"`
			Replicas           int   `json:"replicas"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &deployment); err != nil || len(deployment.Spec.Template.Spec.Containers) == 0 {
		return report, errors.New("the deployment cannot be read")
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	report.Image = container.Image
	for _, e := range container.Env {
		report.Env[e.Name] = e.Value
	}
	want := 1
	if deployment.Spec.Replicas != nil {
		want = *deployment.Spec.Replicas
	}
	report.Replicas = want
	st := deployment.Status
	report.Generation, report.ObservedGeneration = deployment.Metadata.Generation, st.ObservedGeneration
	report.UpdatedReplicas, report.AvailableReplicas, report.ReadyReplicas, report.Total = st.UpdatedReplicas, st.AvailableReplicas, st.ReadyReplicas, st.Replicas
	report.Observed = deployment.Metadata.Generation > 0 && st.ObservedGeneration == deployment.Metadata.Generation
	report.Ready = report.Observed && st.ReadyReplicas == want && st.AvailableReplicas == want && st.UpdatedReplicas == want && st.Replicas == want
	if _, _, svcExit := k.Run(ctx, "-n", namespace, "get", "service", name, "-o", "name"); svcExit == 0 {
		report.Service = true
	}
	nsRaw, _, nsExit := k.Run(ctx, "get", "namespace", namespace, "-o", "json")
	var doc struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	if nsExit == 0 && json.Unmarshal(nsRaw, &doc) == nil {
		report.Managed = doc.Metadata.Labels["orchestrator-managed"] == "true"
	}
	report.ImageMatch = wantImage == "" || report.Image == wantImage
	keys := make([]string, 0, len(wantEnv))
	for key := range wantEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if got, ok := report.Env[key]; !ok || got != wantEnv[key] {
			report.EnvMismatch = append(report.EnvMismatch, key)
		}
	}
	if wantDigest != "" || requireReady {
		// Pod image IDs are collected first so a mismatch can be diagnosed from the report.
		podErr := verifyPods(ctx, k, namespace, deployment.Spec.Selector.MatchLabels, want, wantDigest, &report)
		if !report.ImageMatch {
			return report, errors.New("the deployed image differs from the expected image")
		}
		if len(report.EnvMismatch) > 0 {
			return report, fmt.Errorf("environment variable %s differs from the expected value", report.EnvMismatch[0])
		}
		if podErr != nil {
			return report, podErr
		}
	} else {
		if !report.ImageMatch {
			return report, errors.New("the deployed image differs from the expected image")
		}
		if len(report.EnvMismatch) > 0 {
			return report, fmt.Errorf("environment variable %s differs from the expected value", report.EnvMismatch[0])
		}
	}
	if requireReady && !report.Ready {
		return report, errors.New("the deployment is not fully rolled out and ready (observed generation, updated, available and ready replicas)")
	}
	return report, nil
}

// verifyPods lists the Pods of the Deployment and checks that exactly the wanted number run, all
// Running and Ready, and that the digest of each running image equals wantDigest.
func verifyPods(ctx context.Context, k v2run.Kubectl, namespace string, selector map[string]string, want int, wantDigest string, report *Report) error {
	if len(selector) == 0 {
		return errors.New("the deployment has no pod selector")
	}
	pairs := make([]string, 0, len(selector))
	for key, value := range selector {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	raw, stderr, exit := k.Run(ctx, "-n", namespace, "get", "pods", "-l", strings.Join(pairs, ","), "-o", "json")
	if exit != 0 {
		return fmt.Errorf("the pods cannot be listed: %s", firstLine(stderr))
	}
	var pods struct {
		Items []struct {
			Metadata struct {
				DeletionTimestamp *string `json:"deletionTimestamp"`
			} `json:"metadata"`
			Status struct {
				Phase      string                          `json:"phase"`
				Conditions []struct{ Type, Status string } `json:"conditions"`
				Containers []struct {
					ImageID string `json:"imageID"`
					Ready   bool   `json:"ready"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &pods) != nil {
		return errors.New("the pod list cannot be read")
	}
	for _, pod := range pods.Items {
		if len(pod.Status.Containers) > 0 {
			report.PodImageIDs = append(report.PodImageIDs, safeImageID(pod.Status.Containers[0].ImageID))
		}
	}
	for _, pod := range pods.Items {
		if pod.Metadata.DeletionTimestamp != nil {
			return errors.New("an old pod is still terminating; the rollout is not complete")
		}
		report.Pods++
		ready := pod.Status.Phase == "Running"
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status != "True" {
				ready = false
			}
		}
		if !ready || len(pod.Status.Containers) == 0 || !pod.Status.Containers[0].Ready {
			return errors.New("a pod is not Running and Ready")
		}
		if wantDigest != "" {
			id := pod.Status.Containers[0].ImageID
			if id != wantDigest && !strings.HasSuffix(id, "@"+wantDigest) {
				return errors.New("the image running in a pod has a different digest than expected")
			}
			report.DigestPods++
		}
	}
	if report.Pods != want {
		return fmt.Errorf("%d pod(s) run, %d wanted", report.Pods, want)
	}
	return nil
}

// Receipt is the product's own durable record of one namespace it created:
// the Terraform instance (the physical key the product stamped as the
// orchestrator-instance label) and the Terraform run that created it. It is read
// from the product's Console database after creation, never guessed from a label.
type Receipt struct {
	Descriptor   string `json:"descriptor"`
	Instance     string `json:"instance"`
	TerraformRun string `json:"terraformRun"`
}

type nsFull struct {
	UID    string
	Labels map[string]string
}

func namespacesFull(ctx context.Context, k v2run.Kubectl) (map[string]nsFull, error) {
	list, err := listNamespaces(ctx, k)
	if err != nil {
		return nil, err
	}
	out := map[string]nsFull{}
	for _, item := range list.Items {
		out[item.Metadata.Name] = nsFull{UID: item.Metadata.UID, Labels: item.Metadata.Labels}
	}
	return out, nil
}

// ownershipProblem is empty when the namespace carries the exact ownership
// tags of this run AND matches a product receipt: run-id, managed-by,
// orchestrator-managed, and the instance/created-by-run pair of a namespace receipt.
func ownershipProblem(runID string, labels map[string]string, receipts []Receipt) string {
	switch {
	case runID == "" || labels["run-id"] != runID:
		return "the run-id tag differs from this run"
	case labels["managed-by"] != v2run.ProductManagedByTag:
		return "the managed-by tag is not the product's"
	case labels["orchestrator-managed"] != "true":
		return "the product ownership label is missing"
	case labels["orchestrator-instance"] == "" || labels["orchestrator-created-by-run"] == "":
		return "the instance tags are missing"
	}
	for _, r := range receipts {
		if strings.HasPrefix(r.Descriptor, "k8s-namespace.") && r.Instance == labels["orchestrator-instance"] && r.TerraformRun == labels["orchestrator-created-by-run"] {
			return ""
		}
	}
	return "no product receipt matches the instance tags"
}

// RecordOwned records, for every run namespace that exists, its name, UID and
// the exact ownership tags, but only when the namespace (a) is one of the
// baseline's run names (a subset, never a wider set), (b) did not exist at the
// baseline, (c) carries this run's exact tags and (d) matches a product receipt.
// A namespace that does not exist yet is reported absent (partial creation is
// normal); one that exists without proof is an error and is never recorded. The
// proven ones are returned even when another is not (partial ownership).
func RecordOwned(ctx context.Context, k v2run.Kubectl, base Baseline, runID string, receipts []Receipt, names []string) (owned []OwnedNamespace, absent []string, err error) {
	if err := ValidateRunNamespaces(names); err != nil {
		return nil, nil, err
	}
	allowed := map[string]bool{}
	for _, name := range base.RunNamespaces {
		allowed[name] = true
	}
	for _, name := range names {
		if !allowed[name] {
			return nil, nil, fmt.Errorf("namespace %s is not one of the baseline run namespaces", name)
		}
	}
	current, err := namespacesFull(ctx, k)
	if err != nil {
		return nil, nil, err
	}
	var problems []string
	for _, name := range names {
		got, ok := current[name]
		if !ok {
			absent = append(absent, name)
			continue
		}
		if _, existed := base.Namespaces[name]; existed {
			problems = append(problems, "namespace "+name+" existed before the run")
			continue
		}
		if got.UID == "" {
			problems = append(problems, "namespace "+name+" has no UID")
			continue
		}
		if reason := ownershipProblem(runID, got.Labels, receipts); reason != "" {
			problems = append(problems, "namespace "+name+" is not proven run-owned: "+reason)
			continue
		}
		owned = append(owned, OwnedNamespace{Name: name, UID: got.UID, RunID: runID, Instance: got.Labels["orchestrator-instance"], TerraformRun: got.Labels["orchestrator-created-by-run"]})
	}
	if len(problems) > 0 {
		return owned, absent, errors.New(strings.Join(problems, "; "))
	}
	return owned, absent, nil
}

// VerifyAbsent proves, after cleanup, that every run namespace is gone (its
// recorded UID no longer exists) and that every namespace that existed before
// the run is unchanged (same UID).
func VerifyAbsent(ctx context.Context, k v2run.Kubectl, base Baseline, owned []OwnedNamespace) error {
	list, err := listNamespaces(ctx, k)
	if err != nil {
		return err
	}
	now := map[string]string{}
	uids := map[string]bool{}
	for _, item := range list.Items {
		now[item.Metadata.Name] = item.Metadata.UID
		uids[item.Metadata.UID] = true
	}
	var problems []string
	for _, name := range base.RunNamespaces {
		if _, ok := now[name]; ok {
			problems = append(problems, "run namespace "+name+" still exists")
		}
	}
	for _, ns := range owned {
		if uids[ns.UID] {
			problems = append(problems, "the recorded UID of "+ns.Name+" still exists")
		}
	}
	for name, uid := range base.Namespaces {
		if got, ok := now[name]; !ok {
			problems = append(problems, "baseline namespace "+name+" disappeared")
		} else if got != uid {
			problems = append(problems, "baseline namespace "+name+" changed identity")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// ExitNotOwned is the guard exit code for a namespace not proven run-owned.
const ExitNotOwned = 10

// GuardDelete deletes one namespace only when its current UID equals the
// recorded UID and it still carries this run's exact ownership tags, and the
// delete itself is atomic on that UID: the API server refuses it (precondition
// failed) if the namespace was replaced between the read and the delete, so a
// replacement object is never removed. It reports (false, nil) when the
// namespace is already gone and an error when ownership cannot be proven; it
// never deletes by name alone. tmpDir is a private directory for the request body.
func GuardDelete(ctx context.Context, k v2run.Kubectl, tmpDir string, ns OwnedNamespace) (bool, error) {
	if err := ValidateRunNamespaces([]string{ns.Name}); err != nil {
		return false, err
	}
	if ns.UID == "" || ns.RunID == "" || ns.Instance == "" || ns.TerraformRun == "" {
		return false, errors.New("the record lacks the UID or exact ownership tags")
	}
	raw, stderr, exit := k.Run(ctx, "get", "namespace", ns.Name, "-o", "json")
	if exit != 0 {
		if !strings.Contains(stderr, "NotFound") && !strings.Contains(stderr, "not found") {
			return false, errors.New("the namespace cannot be read")
		}
		current, err := namespacesFull(ctx, k)
		if err != nil {
			return false, err
		}
		if _, still := current[ns.Name]; still {
			return false, errors.New("the namespace cannot be read")
		}
		return false, nil
	}
	var doc struct {
		Metadata struct {
			UID    string            `json:"uid"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Metadata.UID == "" || doc.Metadata.UID != ns.UID {
		return false, errors.New("the namespace is not the recorded object (UID differs)")
	}
	l := doc.Metadata.Labels
	if l["run-id"] != ns.RunID || l["managed-by"] != v2run.ProductManagedByTag || l["orchestrator-managed"] != "true" ||
		l["orchestrator-instance"] != ns.Instance || l["orchestrator-created-by-run"] != ns.TerraformRun {
		return false, errors.New("the namespace does not carry the recorded exact ownership tags")
	}
	body := filepath.Join(tmpDir, "delete-"+ns.Name+".json")
	options, _ := json.Marshal(map[string]any{"kind": "DeleteOptions", "apiVersion": "v1", "preconditions": map[string]string{"uid": ns.UID}})
	if err := os.WriteFile(body, options, 0o600); err != nil {
		return false, fmt.Errorf("delete body: %w", err)
	}
	defer os.Remove(body)
	if _, deleteErr, deleteExit := k.Run(ctx, "delete", "--raw", "/api/v1/namespaces/"+ns.Name, "-f", body); deleteExit != 0 {
		if strings.Contains(deleteErr, "NotFound") || strings.Contains(deleteErr, "not found") {
			return false, nil
		}
		if strings.Contains(deleteErr, "Precondition") || strings.Contains(deleteErr, "Conflict") {
			return false, errors.New("the namespace was replaced after the ownership check (UID precondition failed); it was not deleted")
		}
		return false, fmt.Errorf("delete failed: %s", firstLine(deleteErr))
	}
	return true, nil
}

// safeImageID keeps only the digest part of a Pod image ID (an image digest is not a secret); anything else is
// reduced to a fixed marker so no arbitrary text reaches the evidence.
func safeImageID(id string) string {
	if i := strings.LastIndex(id, "sha256:"); i >= 0 {
		digest := id[i:]
		if len(digest) == len("sha256:")+64 {
			return digest
		}
	}
	return "unrecognized"
}
