package readiness

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"orchestrator/internal/ports/terraformexec"
)

var _ terraformexec.NamespaceInspector = (*Checker)(nil)

// ignoredNamespacedResources are namespaced API resources the content
// inspection does not list: events are transient records of what happened and
// own nothing. Everything else is listed, including Endpoints.
var ignoredNamespacedResources = map[string]bool{"events": true, "events.events.k8s.io": true}

// coreResources must appear in resource discovery; their absence means the
// discovery is incomplete and the contents are unknown.
var coreResources = []string{"configmaps", "endpoints", "persistentvolumeclaims", "pods", "secrets", "serviceaccounts", "services"}

// Inspect lists the namespace UID and every object of every listable
// namespaced API resource through the scoped provider environment (a private
// kubeconfig, never a host or default one), with owners (including owner
// UIDs) and managedFields managers for provenance. Any failure, a missing
// core resource in discovery, or an object without a UID returns an error:
// unknown contents are never reported as empty.
func (c *Checker) Inspect(ctx context.Context, env []string, namespace string) (terraformexec.NamespaceInspection, error) {
	none := terraformexec.NamespaceInspection{}
	if !validName(namespace) {
		return none, ErrUnavailable
	}
	dir, err := os.MkdirTemp(c.Dir, "inspect-")
	if err != nil {
		return none, ErrUnavailable
	}
	defer os.RemoveAll(dir)
	kubeconfig, err := writeKubeconfig(dir, env)
	if err != nil {
		return none, ErrUnavailable
	}
	base := []string{"--kubeconfig", kubeconfig}
	if name := value(env, "KUBE_CTX"); name != "" {
		base = append(base, "--context", name)
	}
	cmdEnv := []string{"PATH=" + os.TempDir() + "/no-path", "HOME=" + dir, "KUBECONFIG=" + kubeconfig}

	nsOut, err := c.run()(ctx, c.Kubectl, append(append([]string(nil), base...), "get", "namespace", namespace, "-o", "json"), cmdEnv)
	if err != nil {
		return none, ErrUnavailable
	}
	var nsDoc struct {
		Metadata struct {
			UID string `json:"uid"`
		} `json:"metadata"`
	}
	if json.Unmarshal(nsOut, &nsDoc) != nil || nsDoc.Metadata.UID == "" {
		return none, ErrUnavailable
	}

	listed, err := c.run()(ctx, c.Kubectl, append(append([]string(nil), base...), "api-resources", "--namespaced=true", "--verbs=list", "-o", "name"), cmdEnv)
	if err != nil {
		return none, ErrUnavailable
	}
	discovered := map[string]bool{}
	var resources []string
	for _, line := range strings.Fields(string(listed)) {
		if !validName(line) {
			return none, ErrUnavailable
		}
		discovered[line] = true
		if !ignoredNamespacedResources[line] {
			resources = append(resources, line)
		}
	}
	for _, core := range coreResources {
		if !discovered[core] {
			return none, ErrUnavailable
		}
	}
	sort.Strings(resources)
	out, err := c.run()(ctx, c.Kubectl, append(append([]string(nil), base...), "get", strings.Join(resources, ","), "-n", namespace, "-o", "json", "--show-managed-fields"), cmdEnv)
	if err != nil {
		return none, ErrUnavailable
	}
	var doc struct {
		Items []struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name            string            `json:"name"`
				UID             string            `json:"uid"`
				Labels          map[string]string `json:"labels"`
				Annotations     map[string]string `json:"annotations"`
				OwnerReferences []struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
					UID  string `json:"uid"`
				} `json:"ownerReferences"`
				ManagedFields []struct {
					Manager string `json:"manager"`
				} `json:"managedFields"`
			} `json:"metadata"`
			Data                         map[string]json.RawMessage `json:"data"`
			BinaryData                   map[string]json.RawMessage `json:"binaryData"`
			Secrets                      []json.RawMessage          `json:"secrets"`
			ImagePullSecrets             []json.RawMessage          `json:"imagePullSecrets"`
			AutomountServiceAccountToken *bool                      `json:"automountServiceAccountToken"`
		} `json:"items"`
	}
	if json.Unmarshal(out, &doc) != nil {
		return none, ErrUnavailable
	}
	inspection := terraformexec.NamespaceInspection{UID: nsDoc.Metadata.UID, Objects: make([]terraformexec.NamespaceObject, 0, len(doc.Items))}
	for _, item := range doc.Items {
		if item.Kind == "" || item.Metadata.Name == "" || item.Metadata.UID == "" {
			return none, ErrUnavailable
		}
		object := terraformexec.NamespaceObject{
			Kind: item.Kind, Name: item.Metadata.Name, UID: item.Metadata.UID,
			SecretRefs: len(item.Secrets), ImagePullSecrets: len(item.ImagePullSecrets), AutomountSet: item.AutomountServiceAccountToken != nil,
		}
		for _, o := range item.Metadata.OwnerReferences {
			object.Owners = append(object.Owners, terraformexec.NamespaceOwner{Kind: o.Kind, Name: o.Name, UID: o.UID})
		}
		for key := range item.Data {
			object.DataKeys = append(object.DataKeys, key)
		}
		for key := range item.BinaryData {
			object.DataKeys = append(object.DataKeys, key)
		}
		managers := map[string]bool{}
		for _, m := range item.Metadata.ManagedFields {
			managers[m.Manager] = true
		}
		for manager := range managers {
			object.Managers = append(object.Managers, manager)
		}
		for key := range item.Metadata.Labels {
			object.LabelKeys = append(object.LabelKeys, key)
		}
		for key := range item.Metadata.Annotations {
			object.AnnotationKeys = append(object.AnnotationKeys, key)
		}
		sort.Strings(object.DataKeys)
		sort.Strings(object.Managers)
		sort.Strings(object.LabelKeys)
		sort.Strings(object.AnnotationKeys)
		inspection.Objects = append(inspection.Objects, object)
	}
	return inspection, nil
}
