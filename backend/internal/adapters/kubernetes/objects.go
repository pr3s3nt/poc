package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// apiResource is the discovery entry of one apiVersion/kind.
type apiResource struct {
	plural     string
	namespaced bool
}

// discovery resolves kinds through the API server's discovery documents, one
// request per group version and operation (ADR-024).
type discovery struct {
	cli   CLI
	cache map[string]map[string]apiResource
}

var groupVersion = regexp.MustCompile(`^([a-z0-9]([-a-z0-9.]*[a-z0-9])?/)?v[0-9]+((alpha|beta)[0-9]+)?$`)

// ErrUnsupportedObject is an object the target cluster does not serve or that
// is cluster-scoped; it fails the apply before any write.
var ErrUnsupportedObject = errors.New("kubernetes: the object kind is not a namespaced kind the cluster serves")

func newDiscovery(cli CLI) *discovery {
	return &discovery{cli: cli, cache: map[string]map[string]apiResource{}}
}

func (d *discovery) resolve(ctx context.Context, apiVersion, kind string) (apiResource, error) {
	if !groupVersion.MatchString(apiVersion) || kind == "" {
		return apiResource{}, fmt.Errorf("%w: %s %s", ErrUnsupportedObject, apiVersion, kind)
	}
	kinds, ok := d.cache[apiVersion]
	if !ok {
		path := "/apis/" + apiVersion
		if !strings.Contains(apiVersion, "/") {
			path = "/api/" + apiVersion
		}
		out, err := d.cli.Run(ctx, nil, "get", "--raw", path)
		if errors.Is(err, ErrObjectNotFound) {
			out, err = []byte(`{"resources":[]}`), nil
		}
		if err != nil {
			return apiResource{}, err
		}
		var list struct {
			Resources []struct {
				Name       string `json:"name"`
				Kind       string `json:"kind"`
				Namespaced bool   `json:"namespaced"`
			} `json:"resources"`
		}
		if err := json.Unmarshal(out, &list); err != nil {
			return apiResource{}, fmt.Errorf("kubernetes: decode discovery of %s: %w", apiVersion, err)
		}
		kinds = map[string]apiResource{}
		for _, r := range list.Resources {
			if strings.Contains(r.Name, "/") {
				continue // subresource
			}
			kinds[r.Kind] = apiResource{plural: r.Name, namespaced: r.Namespaced}
		}
		d.cache[apiVersion] = kinds
	}
	resource, ok := kinds[kind]
	if !ok || !resource.namespaced {
		return apiResource{}, fmt.Errorf("%w: %s %s", ErrUnsupportedObject, apiVersion, kind)
	}
	return resource, nil
}

// objectPath is the REST path of one namespaced object.
func objectPath(apiVersion, namespace string, resource apiResource, name string) string {
	prefix := "/apis/" + apiVersion
	if !strings.Contains(apiVersion, "/") {
		prefix = "/api/" + apiVersion
	}
	return prefix + "/namespaces/" + namespace + "/" + resource.plural + "/" + name
}

func (d *discovery) get(ctx context.Context, namespace, apiVersion, kind, name string) (map[string]any, bool, error) {
	resource, err := d.resolve(ctx, apiVersion, kind)
	if err != nil {
		return nil, false, err
	}
	if !dnsLabel.MatchString(namespace) || len(name) > 253 || !kubernetesSubdomain.MatchString(name) {
		return nil, false, errors.New("kubernetes: invalid namespace or object name")
	}
	out, err := d.cli.Run(ctx, nil, "get", "--raw", objectPath(apiVersion, namespace, resource, name))
	if errors.Is(err, ErrObjectNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	object := map[string]any{}
	if err := json.Unmarshal(out, &object); err != nil {
		return nil, false, fmt.Errorf("kubernetes: decode %s/%s: %w", kind, name, err)
	}
	return object, true, nil
}

// deleteUID deletes one object only while its UID is the inspected one and
// waits until it is gone, like CLI.DeleteUID for any discovered kind.
func (d *discovery) deleteUID(ctx context.Context, namespace, apiVersion, kind, name, uid string, timeout time.Duration) error {
	resource, err := d.resolve(ctx, apiVersion, kind)
	if err != nil {
		return err
	}
	if !kubernetesUID.MatchString(uid) {
		return errors.New("kubernetes: delete without an inspected UID")
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]any{"uid": uid}})
	_, err = d.cli.Run(waitCtx, payload, "delete", "--raw", objectPath(apiVersion, namespace, resource, name), "-f", "-")
	if errors.Is(err, ErrObjectNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		object, found, err := d.get(waitCtx, namespace, apiVersion, kind, name)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if objectUID(object) != uid {
			return errors.New("kubernetes: a replacement object appeared before UID-preconditioned deletion completed")
		}
		select {
		case <-waitCtx.Done():
			return errors.New("kubernetes: the original object did not disappear before the deletion deadline")
		case <-ticker.C:
		}
	}
}

func objectUID(object map[string]any) string {
	metadata, _ := object["metadata"].(map[string]any)
	uid, _ := metadata["uid"].(string)
	return uid
}

func objectLabels(object map[string]any) map[string]any {
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	return labels
}

// sameWorkloadOwner reports whether an existing object belongs to the same
// Application, Environment and workload. Objects applied before ADR-024 carry
// the workload as app.kubernetes.io/name only.
func sameWorkloadOwner(labels map[string]any, application, environment, workloadID string) bool {
	if labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != application || labels["orchestrator.io/environment"] != environment {
		return false
	}
	if workload, ok := labels["orchestrator.io/workload"]; ok {
		return workload == workloadID
	}
	return labels["app.kubernetes.io/name"] == workloadID
}

// checkApply refuses, before any write, an object kind the cluster does not
// serve as namespaced and an existing object owned by anyone else (RB-C09).
// Platform auxiliaries keep their own delivery rules.
func checkApply(ctx context.Context, d *discovery, target execution.Target, manifests []execution.Manifest) error {
	application, environment := target.Extra["application"], target.Extra["environment"]
	for _, m := range manifests {
		if m.Auxiliary || m.Secret {
			continue
		}
		labels := objectLabels(m.Object)
		workload, _ := labels["orchestrator.io/workload"].(string)
		if workload == "" {
			continue // native/legacy render: unchanged behavior
		}
		if application == "" || environment == "" {
			return errors.New("kubernetes: apply of owned objects without the Application and Environment identity")
		}
		existing, found, err := d.get(ctx, target.Namespace, m.APIVersion, m.Kind, m.Name)
		if err != nil {
			return err
		}
		if found && !sameWorkloadOwner(objectLabels(existing), application, environment, workload) {
			return fmt.Errorf("kubernetes: refusing to overwrite %s/%s owned by another workload or not created by the platform", m.Kind, m.Name)
		}
	}
	return nil
}

// RemoveObjects deletes the inventory objects of one workload: all are
// inspected and verified first, then deleted by UID precondition (RB-C09).
func (d *Deployer) RemoveObjects(ctx context.Context, target execution.Target, workloadID string, objects []execution.ObjectRef) error {
	if len(objects) == 0 {
		return nil
	}
	application, environment := target.Extra["application"], target.Extra["environment"]
	if target.Namespace == "" || !target.Explicit() || application == "" || environment == "" || workloadID == "" {
		return fmt.Errorf("kubernetes: remove objects without a scoped target and owner identity")
	}
	cli, cleanup, err := OpenCLI(ctx, d.KubectlPath, d.Credentials, target)
	if err != nil {
		return err
	}
	defer cleanup()
	disc := newDiscovery(cli)
	type found struct {
		ref execution.ObjectRef
		uid string
	}
	var remove []found
	for _, ref := range objects {
		object, exists, err := disc.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		labels := objectLabels(object)
		owner, _ := labels["orchestrator.io/deployment-id"].(string)
		owned := false
		for _, candidate := range ref.Owners {
			owned = owned || (candidate != "" && candidate == owner)
		}
		if !owned || !sameWorkloadOwner(labels, application, environment, workloadID) {
			return fmt.Errorf("kubernetes: refusing to remove %s/%s with unverified workload ownership", ref.Kind, ref.Name)
		}
		uid := objectUID(object)
		if uid == "" {
			return fmt.Errorf("kubernetes: refusing to remove %s/%s without a verified UID", ref.Kind, ref.Name)
		}
		if ref.Kind == "CronJob" && ref.APIVersion == "batch/v1" {
			spec, _ := object["spec"].(map[string]any)
			if suspended, _ := spec["suspend"].(bool); !suspended {
				return fmt.Errorf("kubernetes: suspend the scheduled workload before removing it")
			}
			out, err := cli.Run(ctx, nil, "get", "jobs", "-n", target.Namespace, "-o", "json")
			if err != nil {
				return err
			}
			if err = ensureCronJobIdle(out, uid); err != nil {
				return err
			}
		}
		remove = append(remove, found{ref: ref, uid: uid})
	}
	for _, item := range remove {
		if err := disc.deleteUID(ctx, target.Namespace, item.ref.APIVersion, item.ref.Kind, item.ref.Name, item.uid, d.Timeout); err != nil {
			return err
		}
	}
	return nil
}

// waitJob polls one Job until Complete; Failed or the deadline fails it.
func waitJob(ctx context.Context, d *discovery, namespace, name string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		object, found, err := d.get(waitCtx, namespace, "batch/v1", "Job", name)
		if err != nil {
			return err
		}
		if found {
			status, _ := object["status"].(map[string]any)
			conditions, _ := status["conditions"].([]any)
			for _, raw := range conditions {
				condition, _ := raw.(map[string]any)
				if condition["status"] != "True" {
					continue
				}
				switch condition["type"] {
				case "Complete":
					return nil
				case "Failed":
					return fmt.Errorf("kubernetes: job/%s failed", name)
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("kubernetes: job/%s did not complete before the deadline", name)
		case <-ticker.C:
		}
	}
}

// An owned Job without a terminal condition is active, including Pending Jobs.
// Ownership is established by controller UID, never by names or labels alone.
func ensureCronJobIdle(raw []byte, uid string) error {
	var jobs struct {
		Items []struct {
			Metadata struct {
				OwnerReferences []struct {
					UID        string `json:"uid"`
					Controller bool   `json:"controller"`
				} `json:"ownerReferences"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return fmt.Errorf("kubernetes: cannot verify scheduled workload Jobs")
	}
	for _, job := range jobs.Items {
		owned := false
		for _, owner := range job.Metadata.OwnerReferences {
			owned = owned || owner.UID == uid && owner.Controller
		}
		if !owned {
			continue
		}
		terminal := false
		for _, condition := range job.Status.Conditions {
			terminal = terminal || condition.Status == "True" && (condition.Type == "Complete" || condition.Type == "Failed")
		}
		if !terminal {
			return fmt.Errorf("kubernetes: scheduled workload has an active Job")
		}
	}
	return nil
}
