package tfexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformexec"
)

// ErrNamespaceContents means a namespace holds objects that are unknown,
// foreign or unselected, or could not be inspected. Deleting a namespace
// cascades to everything in it, so the destroy was refused before any
// provider call and every object was kept. A namespace name, prefix or the
// namespace's own ownership is never authority to delete what is inside it.
var ErrNamespaceContents = errors.New("tfexec: the namespace holds objects that are not owned by the selected resources; destroy refused")

// namespaceTypes are the Terraform types of a namespace.
var namespaceTypes = map[string]bool{"kubernetes_namespace": true, "kubernetes_namespace_v1": true}

// ownKinds maps a Terraform Kubernetes resource type of the SAME state to the
// Kubernetes kind it manages: objects the selected instance itself created.
var ownKinds = map[string]string{
	"kubernetes_stateful_set": "StatefulSet", "kubernetes_stateful_set_v1": "StatefulSet",
	"kubernetes_deployment": "Deployment", "kubernetes_deployment_v1": "Deployment",
	"kubernetes_service": "Service", "kubernetes_service_v1": "Service",
	"kubernetes_secret": "Secret", "kubernetes_secret_v1": "Secret",
	"kubernetes_config_map": "ConfigMap", "kubernetes_config_map_v1": "ConfigMap",
	"kubernetes_persistent_volume_claim": "PersistentVolumeClaim", "kubernetes_persistent_volume_claim_v1": "PersistentVolumeClaim",
	"kubernetes_service_account": "ServiceAccount", "kubernetes_service_account_v1": "ServiceAccount",
	"kubernetes_resource_quota": "ResourceQuota", "kubernetes_resource_quota_v1": "ResourceQuota",
	"kubernetes_limit_range": "LimitRange", "kubernetes_limit_range_v1": "LimitRange",
	"kubernetes_network_policy": "NetworkPolicy", "kubernetes_network_policy_v1": "NetworkPolicy",
	"kubernetes_role": "Role", "kubernetes_role_v1": "Role",
	"kubernetes_role_binding": "RoleBinding", "kubernetes_role_binding_v1": "RoleBinding",
}

// controllerManager is the field manager recorded for objects the Kubernetes
// control plane generates itself.
const controllerManager = "kube-controller-manager"

type namespaceCreationBaseline struct {
	NamespaceUID             string `json:"namespaceUid"`
	DefaultServiceAccountUID string `json:"defaultServiceAccountUid"`
}

func namespaceBaselineHandle(instanceID string) string { return "baseline-" + instanceID }

func namespaceUIDBaselineHandle(instanceID, namespaceUID string) string {
	return namespaceBaselineHandle(instanceID) + "-" + namespaceUID
}

// captureNamespaceBaseline records the default ServiceAccount UID immediately
// after this run creates and readies its namespace, before dependent workloads
// are provisioned. The encrypted artifact is immutable for this namespace UID,
// so recreating a DESTROYED instance cannot overwrite or reuse its old proof.
func (s *Service) captureNamespaceBaseline(ctx context.Context, st *execState, resources []terraformexec.StateResource) error {
	if st.action != tfresource.ActionCreate || st.bundle.Ref.ModuleID != "k8s-namespace" {
		return nil
	}
	var namespace terraformexec.StateResource
	for _, r := range resources {
		if namespaceTypes[r.Type] {
			if namespace.ID != "" || r.ID == "" || r.UID == "" || strings.Contains(r.ID, "/") {
				return fmt.Errorf("%w: namespace creation provenance is ambiguous", ErrExecution)
			}
			namespace = r
		}
	}
	if namespace.ID == "" || s.deps.Namespaces == nil {
		return fmt.Errorf("%w: namespace creation baseline is unavailable", ErrExecution)
	}
	var serviceAccount *terraformexec.NamespaceObject
	for {
		inspected, err := s.deps.Namespaces.Inspect(ctx, st.env.Env, namespace.ID)
		if err != nil || inspected.UID != namespace.UID {
			return fmt.Errorf("%w: namespace creation baseline could not be verified", ErrExecution)
		}
		serviceAccount = nil
		for i := range inspected.Objects {
			o := &inspected.Objects[i]
			if o.Kind == "ServiceAccount" && o.Name == "default" {
				if serviceAccount != nil {
					return fmt.Errorf("%w: default ServiceAccount identity is ambiguous", ErrExecution)
				}
				serviceAccount = o
			}
		}
		if serviceAccount != nil {
			if !pristineDefaultServiceAccount(*serviceAccount, true) {
				return fmt.Errorf("%w: default ServiceAccount creation provenance is unavailable", ErrExecution)
			}
			break
		}
		// Kubernetes creates the namespace's default ServiceAccount
		// asynchronously. Retry only this exact not-yet-present condition,
		// under the bounded readiness context. API errors and a present but
		// non-pristine object remain immediate failures.
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: default ServiceAccount did not appear before readiness expired", ErrExecution)
		case <-timer.C:
		}
	}
	baseline, err := json.Marshal(namespaceCreationBaseline{NamespaceUID: namespace.UID, DefaultServiceAccountUID: serviceAccount.UID})
	if err != nil {
		return fmt.Errorf("%w: namespace creation baseline could not be encoded", ErrExecution)
	}
	handle := namespaceUIDBaselineHandle(st.inst.ID, namespace.UID)
	// Keep the old instance-only artifact authoritative for its original UID.
	// In particular, a replaced default account cannot establish a new proof.
	if _, found, err := s.deps.Verification.LoadVerification(ctx, handle, st.inst.ID); err != nil {
		return fmt.Errorf("%w: namespace creation baseline is unreadable", ErrExecution)
	} else if !found {
		legacy, legacyFound, legacyErr := s.deps.Verification.LoadVerification(ctx, namespaceBaselineHandle(st.inst.ID), st.inst.ID)
		if legacyErr != nil {
			return fmt.Errorf("%w: namespace creation baseline is unreadable", ErrExecution)
		}
		if legacyFound {
			var previous namespaceCreationBaseline
			if json.Unmarshal(legacy, &previous) != nil || previous.NamespaceUID == "" || previous.DefaultServiceAccountUID == "" {
				return fmt.Errorf("%w: namespace creation baseline is malformed", ErrExecution)
			}
			if previous.NamespaceUID == namespace.UID && string(legacy) != string(baseline) {
				return fmt.Errorf("%w: namespace creation baseline changed", ErrExecution)
			}
		}
	}
	existing, found, err := s.deps.Verification.LoadVerification(ctx, handle, st.inst.ID)
	if err != nil || found && string(existing) != string(baseline) {
		return fmt.Errorf("%w: namespace creation baseline is unreadable or changed", ErrExecution)
	}
	if !found {
		if err := s.deps.Verification.SaveVerification(ctx, handle, st.inst.ID, baseline); err != nil {
			return fmt.Errorf("%w: namespace creation baseline could not be preserved", ErrExecution)
		}
		stored, storedFound, err := s.deps.Verification.LoadVerification(ctx, handle, st.inst.ID)
		if err != nil || !storedFound || string(stored) != string(baseline) {
			return fmt.Errorf("%w: namespace creation baseline was not durably preserved", ErrExecution)
		}
	}
	return nil
}

func pristineDefaultServiceAccount(o terraformexec.NamespaceObject, allowNoManager bool) bool {
	if o.Kind != "ServiceAccount" || o.Name != "default" || o.UID == "" || len(o.Owners) != 0 || len(o.LabelKeys) != 0 ||
		o.SecretRefs != 0 || o.ImagePullSecrets != 0 || o.AutomountSet || len(o.AnnotationKeys) != 0 {
		return false
	}
	if len(o.Managers) == 0 && allowNoManager {
		return true
	}
	return onlyManager(o)
}

// namespaceOf returns the namespaces (name and state UID) a destroy would delete.
type stateNamespace struct{ Name, UID string }

func namespacesOf(resources []terraformexec.StateResource) []stateNamespace {
	var out []stateNamespace
	for _, r := range resources {
		if namespaceTypes[r.Type] && r.ID != "" {
			out = append(out, stateNamespace{Name: r.ID, UID: r.UID})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func onlyManager(o terraformexec.NamespaceObject) bool {
	return len(o.Managers) == 1 && o.Managers[0] == controllerManager
}

func within(keys []string, allowed ...string) bool {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for _, k := range keys {
		if !ok[k] {
			return false
		}
	}
	return true
}

// generatedDefault reports whether an object is a platform-generated default
// with proven provenance: no owner, created and maintained only by the
// control plane (recorded field manager), and carrying no custom metadata or
// settings. A same-name object made or edited by anyone else - another field
// manager, an imagePullSecret, an extra key, label or annotation - is foreign.
func generatedDefault(o terraformexec.NamespaceObject) bool {
	if len(o.Owners) != 0 || o.UID == "" || !onlyManager(o) || len(o.LabelKeys) != 0 {
		return false
	}
	switch {
	case o.Kind == "ConfigMap" && o.Name == "kube-root-ca.crt":
		return len(o.DataKeys) == 1 && o.DataKeys[0] == "ca.crt" && within(o.AnnotationKeys, "kubernetes.io/description")
	case o.Kind == "ServiceAccount" && o.Name == "default":
		return o.SecretRefs == 0 && o.ImagePullSecrets == 0 && !o.AutomountSet && len(o.AnnotationKeys) == 0
	}
	return false
}

// judgeNamespace returns the safe "Kind/name" IDs of every object in the
// namespace that is not accounted for, plus whether the namespace itself is
// the one the state recorded. Accounting is by UID, never by name or shape:
//
//   - an object the selected instance's own state manages (same namespace,
//     kind, name AND the UID recorded in state; a replaced object is foreign);
//   - a platform-generated default with proven provenance (generatedDefault);
//   - an Endpoints object generated by the control plane for an accounted
//     Service of the same name (a standalone or custom Endpoints is foreign);
//   - an object whose every ownerReference names an accounted object by kind,
//     name AND UID (a recreated or spoofed same-name owner is foreign).
func judgeNamespace(ns stateNamespace, inspected terraformexec.NamespaceInspection, own []terraformexec.StateResource) (foreign []string, namespaceIsOurs bool) {
	return judgeNamespaceWithBaseline(ns, inspected, own, nil)
}

func judgeNamespaceWithBaseline(ns stateNamespace, inspected terraformexec.NamespaceInspection, own []terraformexec.StateResource, baseline *namespaceCreationBaseline) (foreign []string, namespaceIsOurs bool) {
	namespaceIsOurs = ns.UID != "" && inspected.UID == ns.UID
	ownUID := map[string]string{} // kind/name -> UID recorded in state
	for _, r := range own {
		kind, ok := ownKinds[r.Type]
		if !ok || r.UID == "" {
			continue
		}
		if namespace, name, found := strings.Cut(r.ID, "/"); found && namespace == ns.Name {
			ownUID[kind+"/"+name] = r.UID
		}
	}
	accounted := map[string]bool{} // UID
	byKey := map[string]terraformexec.NamespaceObject{}
	for _, o := range inspected.Objects {
		byKey[o.Kind+"/"+o.Name] = o
		if o.UID == "" {
			continue
		}
		if uid, ok := ownUID[o.Kind+"/"+o.Name]; ok && uid == o.UID {
			accounted[o.UID] = true
		}
		if generatedDefault(o) {
			accounted[o.UID] = true
		}
		if baseline != nil && baseline.NamespaceUID == ns.UID && inspected.UID == ns.UID &&
			o.Kind == "ServiceAccount" && o.Name == "default" && o.UID == baseline.DefaultServiceAccountUID && pristineDefaultServiceAccount(o, true) {
			accounted[o.UID] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, o := range inspected.Objects {
			if o.UID == "" || accounted[o.UID] {
				continue
			}
			switch {
			case o.Kind == "Endpoints" && len(o.Owners) == 0 && onlyManager(o):
				if svc, ok := byKey["Service/"+o.Name]; ok && accounted[svc.UID] {
					accounted[o.UID], changed = true, true
				}
			case len(o.Owners) > 0:
				all := true
				for _, owner := range o.Owners {
					target, ok := byKey[owner.Kind+"/"+owner.Name]
					if !ok || owner.UID == "" || target.UID != owner.UID || !accounted[owner.UID] {
						all = false
						break
					}
				}
				if all {
					accounted[o.UID], changed = true, true
				}
			}
		}
	}
	for _, o := range inspected.Objects {
		if o.UID == "" || !accounted[o.UID] {
			foreign = append(foreign, o.Kind+"/"+o.Name)
		}
	}
	sort.Strings(foreign)
	return foreign, namespaceIsOurs
}

// inspectNamespaces runs the real content inspection of every namespace the
// destroy would delete, immediately before it. Without an inspector, with an
// unreadable or incomplete listing, or with any object not accounted for, the
// destroy is refused.
func (s *Service) inspectNamespaces(ctx context.Context, st *execState, pre []terraformexec.StateResource) error {
	for _, ns := range namespacesOf(pre) {
		if s.deps.Namespaces == nil {
			return fmt.Errorf("%w: no inspector is configured", ErrNamespaceContents)
		}
		inspected, err := s.deps.Namespaces.Inspect(ctx, st.env.Env, ns.Name)
		if errors.Is(err, terraformexec.ErrNamespaceAbsent) && inspected.Absent {
			verifier, ok := s.deps.Namespaces.(terraformexec.NamespaceAbsenceInspector)
			if !ok || verifier.VerifyAbsent(ctx, st.env.Env, ns.Name, pre) != nil {
				return fmt.Errorf("%w: the absent namespace's historical resources could not be verified", ErrNamespaceContents)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: the contents of %s could not be read", ErrNamespaceContents, ns.Name)
		}
		var baseline *namespaceCreationBaseline
		if st.inst.ID != "" {
			raw, found, loadErr := s.deps.Verification.LoadVerification(ctx, namespaceUIDBaselineHandle(st.inst.ID, ns.UID), st.inst.ID)
			if loadErr == nil && !found {
				raw, found, loadErr = s.deps.Verification.LoadVerification(ctx, namespaceBaselineHandle(st.inst.ID), st.inst.ID)
			}
			if loadErr != nil {
				return fmt.Errorf("%w: the namespace creation baseline could not be read", ErrNamespaceContents)
			} else if found {
				var value namespaceCreationBaseline
				if json.Unmarshal(raw, &value) != nil || value.NamespaceUID == "" || value.DefaultServiceAccountUID == "" {
					return fmt.Errorf("%w: the namespace creation baseline is malformed", ErrNamespaceContents)
				}
				baseline = &value
			}
		}
		foreign, ours := judgeNamespaceWithBaseline(ns, inspected, pre, baseline)
		if !ours {
			return fmt.Errorf("%w: %s is not the namespace this state created", ErrNamespaceContents, ns.Name)
		}
		if len(foreign) > 0 {
			return fmt.Errorf("%w: %s holds %s", ErrNamespaceContents, ns.Name, strings.Join(foreign, ", "))
		}
	}
	return nil
}
