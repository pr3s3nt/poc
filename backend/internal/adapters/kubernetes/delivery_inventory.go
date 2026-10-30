package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/ports/execution"
	"time"
)

var ErrDeliveryOwnership = errors.New("kubernetes: delivery ownership or UID mismatch")

// InspectDeliveryObjects resolves every GVK through API discovery and reads it
// by its namespaced REST path. It does not adopt, apply, or delete any object.
// CD adapters use this shared transport to preserve arbitrary-kind boundaries.
func InspectDeliveryObjects(ctx context.Context, cli CLI, target execution.Target, refs []execution.ObjectRef) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	discovery := newDiscovery(cli)
	for _, ref := range refs {
		object, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
		if err != nil {
			return nil, err
		}
		if found {
			out[ref.Key()] = object
		}
	}
	return out, nil
}

// RemoveRetainedDeliveryObject deletes exactly the observed incarnation after
// ownership has been released by its CD provider. It does not infer permission
// from a namespace or name prefix.
func RemoveRetainedDeliveryObject(ctx context.Context, cli CLI, target execution.Target, ref execution.ObjectRef, uid, release, releaseNamespace string) error {
	if uid == "" || release == "" {
		return ErrDeliveryOwnership
	}
	discovery := newDiscovery(cli)
	object, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	md, _ := object["metadata"].(map[string]any)
	labels, _ := md["labels"].(map[string]any)
	annotations, _ := md["annotations"].(map[string]any)
	if md["uid"] != uid || labels["orchestrator.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != target.Extra["application"] || labels["orchestrator.io/environment"] != target.Extra["environment"] || annotations["meta.helm.sh/release-name"] != release || annotations["meta.helm.sh/release-namespace"] != releaseNamespace {
		return ErrDeliveryOwnership
	}
	return deleteDeliveryObject(ctx, cli, discovery, target, ref, uid, md["resourceVersion"])
}

// RemoveDirectDeliveryObject uses the persisted incarnation proof of a direct
// admission. Foreign Helm ownership is rejected even if platform labels match.
func RemoveDirectDeliveryObject(ctx context.Context, cli CLI, target execution.Target, ref execution.ObjectRef, uid string) error {
	if uid == "" {
		return ErrDeliveryOwnership
	}
	discovery := newDiscovery(cli)
	object, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	md, _ := object["metadata"].(map[string]any)
	labels, _ := md["labels"].(map[string]any)
	a, _ := md["annotations"].(map[string]any)
	if md["uid"] != uid || labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != target.Extra["application"] || labels["orchestrator.io/environment"] != target.Extra["environment"] || a["meta.helm.sh/release-name"] != nil || a["meta.helm.sh/release-namespace"] != nil {
		return ErrDeliveryOwnership
	}
	return deleteDeliveryObject(ctx, cli, discovery, target, ref, uid, md["resourceVersion"])
}

func deleteDeliveryObject(ctx context.Context, cli CLI, discovery *discovery, target execution.Target, ref execution.ObjectRef, uid string, resourceVersion any) error {
	resource, err := discovery.resolve(ctx, ref.APIVersion, ref.Kind)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]any{"uid": uid, "resourceVersion": resourceVersion}})
	if err != nil {
		return err
	}
	_, err = cli.Run(ctx, payload, "delete", "--raw", objectPath(ref.APIVersion, target.Namespace, resource, ref.Name), "-f", "-")
	if errors.Is(err, ErrObjectNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		object, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		md, _ := object["metadata"].(map[string]any)
		if md["uid"] != uid {
			return ErrDeliveryOwnership
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
