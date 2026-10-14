package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"orchestrator/internal/ports/terraformexec"
)

var _ terraformexec.NamespaceInspector = (*Checker)(nil)
var _ terraformexec.NamespaceAbsenceInspector = (*Checker)(nil)

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
		if errors.Is(err, terraformexec.ErrNamespaceAbsent) {
			return terraformexec.NamespaceInspection{Absent: true}, terraformexec.ErrNamespaceAbsent
		}
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

var absenceResource = map[string]string{
	"kubernetes_stateful_set": "statefulsets", "kubernetes_stateful_set_v1": "statefulsets",
	"kubernetes_deployment": "deployments", "kubernetes_deployment_v1": "deployments",
	"kubernetes_service": "services", "kubernetes_service_v1": "services",
	"kubernetes_secret": "secrets", "kubernetes_secret_v1": "secrets",
	"kubernetes_config_map": "configmaps", "kubernetes_config_map_v1": "configmaps",
	"kubernetes_persistent_volume_claim": "persistentvolumeclaims", "kubernetes_persistent_volume_claim_v1": "persistentvolumeclaims",
	"kubernetes_service_account": "serviceaccounts", "kubernetes_service_account_v1": "serviceaccounts",
	"kubernetes_resource_quota": "resourcequotas", "kubernetes_resource_quota_v1": "resourcequotas",
	"kubernetes_limit_range": "limitranges", "kubernetes_limit_range_v1": "limitranges",
	"kubernetes_network_policy": "networkpolicies", "kubernetes_network_policy_v1": "networkpolicies",
	"kubernetes_role": "roles", "kubernetes_role_v1": "roles",
	"kubernetes_role_binding": "rolebindings", "kubernetes_role_binding_v1": "rolebindings",
}

var namespaceTypes = map[string]bool{"kubernetes_namespace": true, "kubernetes_namespace_v1": true}

// VerifyAbsent is used only after Inspect receives an exact namespace NotFound.
// It rechecks that namespace and queries every historical namespaced state ID;
// any malformed ID, API error, recreated namespace or surviving object fails.
func (c *Checker) VerifyAbsent(ctx context.Context, env []string, namespace string, resources []terraformexec.StateResource) error {
	if !validName(namespace) {
		return ErrUnavailable
	}
	dir, err := os.MkdirTemp(c.Dir, "inspect-absent-")
	if err != nil {
		return ErrUnavailable
	}
	defer os.RemoveAll(dir)
	kubeconfig, err := writeKubeconfig(dir, env)
	if err != nil {
		return ErrUnavailable
	}
	base := []string{"--kubeconfig", kubeconfig}
	if name := value(env, "KUBE_CTX"); name != "" {
		base = append(base, "--context", name)
	}
	cmdEnv := []string{"PATH=" + os.TempDir() + "/no-path", "HOME=" + dir, "KUBECONFIG=" + kubeconfig}
	checkNamespaceAbsent := func() error {
		_, err := c.run()(ctx, c.Kubectl, append(append([]string(nil), base...), "get", "namespace", namespace, "-o", "name"), cmdEnv)
		if !errors.Is(err, terraformexec.ErrNamespaceAbsent) {
			return ErrUnavailable
		}
		return nil
	}
	if err := checkNamespaceAbsent(); err != nil {
		return err
	}
	for _, r := range resources {
		if namespaceTypes[r.Type] {
			if r.ID != namespace || r.UID == "" {
				return ErrUnavailable
			}
			continue
		}
		plural, ok := absenceResource[r.Type]
		if !ok {
			if strings.HasPrefix(r.Type, "kubernetes_") {
				return ErrUnavailable
			}
			continue
		}
		idNamespace, name, found := strings.Cut(r.ID, "/")
		if !found || idNamespace != namespace || name == "" || strings.Contains(name, "/") || r.UID == "" {
			return ErrUnavailable
		}
		out, err := c.run()(ctx, c.Kubectl, append(append([]string(nil), base...), "get", plural, name, "-n", namespace, "-o", "name"), cmdEnv)
		if err == nil {
			if strings.TrimSpace(string(out)) != "" {
				return ErrUnavailable
			}
			return ErrUnavailable
		}
		if !errors.Is(err, terraformexec.ErrNamespaceAbsent) {
			return ErrUnavailable
		}
	}
	return checkNamespaceAbsent()
}
