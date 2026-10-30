package kubernetes

import (
	"context"
	"encoding/json"
	"strings"

	"orchestrator/internal/ports/execution"
)

// DeliveryManager names only ownership metadata, never a cluster destination.
// Empty Release/Namespace denotes the native direct manager.
type DeliveryManager struct{ Release, Namespace string }

func (m DeliveryManager) valid() bool {
	return (m.Release == "" && m.Namespace == "") || (validDeliverySegment(m.Release) && dnsLabel.MatchString(m.Namespace))
}
func validDeliverySegment(s string) bool {
	return len(s) > 0 && len(s) <= 253 && kubernetesSubdomain.MatchString(s)
}

// TransferDeliveryOwnership changes metadata only, guarded by the admitted UID
// and the resourceVersion read in this operation. It is safe to observe/retry
// after a lost patch acknowledgement: only the exact source or destination
// manager is accepted, and an object replacement is always refused.
func TransferDeliveryOwnership(ctx context.Context, cli CLI, target execution.Target, ref execution.ObjectRef, uid string, source, destination DeliveryManager, revision ...string) error {
	if !target.Explicit() || target.Namespace == "" || target.Extra["application"] == "" || target.Extra["environment"] == "" || uid == "" || !source.valid() || !destination.valid() {
		return ErrDeliveryOwnership
	}
	discovery := newDiscovery(cli)
	object, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
	if err != nil {
		return err
	}
	if !found {
		return ErrDeliveryOwnership
	}
	patch, err := ownershipPatch(object, target, uid, source, destination, revision...)
	if err != nil {
		return err
	}
	if len(patch) == 0 {
		return ReleaseDeliveryMetadata(ctx, cli, target, ref, uid, destination, revision...)
	}
	resource, err := discovery.resolve(ctx, ref.APIVersion, ref.Kind)
	if err != nil {
		return err
	}
	resourceName := resource.plural
	if group, _, grouped := strings.Cut(ref.APIVersion, "/"); grouped {
		resourceName += "." + group
	}
	_, err = cli.Run(ctx, nil, "patch", resourceName, ref.Name, "--namespace", target.Namespace, "--type=json", "--field-manager", deliveryHandoffManager, "-p", string(patch))
	if err != nil {
		return err
	}
	updated, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
	if err != nil {
		return err
	}
	if !found {
		return ErrDeliveryOwnership
	}
	verify, err := ownershipPatch(updated, target, uid, destination, destination, revision...)
	if err != nil || len(verify) != 0 {
		return ErrDeliveryOwnership
	}
	return ReleaseDeliveryMetadata(ctx, cli, target, ref, uid, destination, revision...)
}
func ownershipPatch(object map[string]any, target execution.Target, uid string, source, destination DeliveryManager, revision ...string) ([]byte, error) {
	md, _ := object["metadata"].(map[string]any)
	labels, _ := md["labels"].(map[string]any)
	annotations, _ := md["annotations"].(map[string]any)
	rv, _ := md["resourceVersion"].(string)
	if md["uid"] != uid || rv == "" || labels["orchestrator.io/application"] != target.Extra["application"] || labels["orchestrator.io/environment"] != target.Extra["environment"] {
		return nil, ErrDeliveryOwnership
	}
	owns := func(manager DeliveryManager) bool {
		release, _ := annotations["meta.helm.sh/release-name"].(string)
		namespace, _ := annotations["meta.helm.sh/release-namespace"].(string)
		if manager.Release == "" {
			return release == "" && namespace == "" && labels["app.kubernetes.io/managed-by"] == "orchestrator"
		}
		return release == manager.Release && namespace == manager.Namespace && labels["app.kubernetes.io/managed-by"] == "Helm" && labels["orchestrator.io/managed-by"] == "orchestrator"
	}
	newRevision := ""
	if len(revision) > 0 {
		newRevision = revision[0]
	}
	if owns(destination) && (newRevision == "" || annotations["orchestrator.io/delivery-revision"] == newRevision) {
		return nil, nil
	}
	if !owns(source) && !owns(destination) {
		return nil, ErrDeliveryOwnership
	}
	copiedLabels := map[string]any{}
	copiedAnnotations := map[string]any{}
	for k, v := range labels {
		copiedLabels[k] = v
	}
	for k, v := range annotations {
		copiedAnnotations[k] = v
	}
	if destination.Release == "" {
		copiedLabels["app.kubernetes.io/managed-by"] = "orchestrator"
		delete(copiedLabels, "orchestrator.io/managed-by")
		for _, key := range []string{"meta.helm.sh/release-name", "meta.helm.sh/release-namespace", "helm.sh/resource-policy"} {
			delete(copiedAnnotations, key)
		}
	} else {
		copiedLabels["app.kubernetes.io/managed-by"] = "Helm"
		copiedLabels["orchestrator.io/managed-by"] = "orchestrator"
		copiedAnnotations["meta.helm.sh/release-name"] = destination.Release
		copiedAnnotations["meta.helm.sh/release-namespace"] = destination.Namespace
		copiedAnnotations["helm.sh/resource-policy"] = "keep"
	}
	if newRevision != "" {
		copiedAnnotations["orchestrator.io/delivery-revision"] = newRevision
	}
	// Replace whole metadata maps only after a resourceVersion test. No escaped
	// user key becomes a JSON pointer and business metadata remains intact.
	operations := []map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": rv}, {"op": "add", "path": "/metadata/labels", "value": copiedLabels}, {"op": "add", "path": "/metadata/annotations", "value": copiedAnnotations}}
	for _, op := range operations {
		path, _ := op["path"].(string)
		if !strings.HasPrefix(path, "/metadata/") {
			return nil, ErrDeliveryOwnership
		}
	}
	return json.Marshal(operations)
}
