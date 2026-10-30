package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"orchestrator/internal/ports/execution"
	"strings"
)

const deliveryHandoffManager = "orchestrator-delivery-handoff"

// ReleaseDeliveryMetadata relinquishes only handoff-owned platform metadata
// fields. It preserves all business/spec/status field ownership and values.
// This is part of an explicitly fenced ownership handoff/recovery, never GET
// observation and never a force-apply or wholesale managedFields reset.
func ReleaseDeliveryMetadata(ctx context.Context, cli CLI, target execution.Target, ref execution.ObjectRef, uid string, manager DeliveryManager, allowedRevisions ...string) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := releaseDeliveryMetadata(ctx, cli, target, ref, uid, manager, allowedRevisions...)
		if !errors.Is(err, errMetadataVersionChanged) {
			return err
		}
	}
	return errMetadataVersionChanged
}

var errMetadataVersionChanged = errors.New("kubernetes: ownership metadata resource version changed")

func releaseDeliveryMetadata(ctx context.Context, cli CLI, target execution.Target, ref execution.ObjectRef, uid string, manager DeliveryManager, allowedRevisions ...string) error {
	discovery := newDiscovery(cli)
	object, found, err := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
	if err != nil {
		return err
	}
	if !found {
		return ErrDeliveryOwnership
	}
	if _, err := ownershipPatch(object, target, uid, manager, manager); err != nil {
		return err
	}
	md, _ := object["metadata"].(map[string]any)
	annotations, _ := md["annotations"].(map[string]any)
	allowed := false
	for _, revision := range allowedRevisions {
		if revision != "" && annotations["orchestrator.io/delivery-revision"] == revision {
			allowed = true
		}
	}
	if !allowed {
		return ErrDeliveryOwnership
	}
	fields, changed, err := releaseMetadataFields(md["managedFields"])
	if err != nil || !changed {
		return err
	}
	patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": md["resourceVersion"]}, {"op": "add", "path": "/metadata/managedFields", "value": fields}})
	resource, err := discovery.resolve(ctx, ref.APIVersion, ref.Kind)
	if err != nil {
		return err
	}
	name := resource.plural
	if group, _, ok := strings.Cut(ref.APIVersion, "/"); ok {
		name += "." + group
	}
	_, err = cli.Run(ctx, nil, "patch", name, ref.Name, "--namespace", target.Namespace, "--type=json", "--field-manager", "orchestrator-delivery-metadata", "-p", string(patch))

	if errors.Is(err, ErrRequestRejected) {
		current, exists, readErr := discovery.get(ctx, target.Namespace, ref.APIVersion, ref.Kind, ref.Name)
		if readErr == nil && exists {
			latest, _ := current["metadata"].(map[string]any)
			if latest["uid"] == uid && latest["resourceVersion"] != md["resourceVersion"] {
				return errMetadataVersionChanged
			}
		}
	}
	return err
}

func releaseMetadataFields(raw any) ([]any, bool, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false, ErrDeliveryOwnership
	}
	var fields []any
	if json.Unmarshal(encoded, &fields) != nil {
		return nil, false, ErrDeliveryOwnership
	}
	changed := false
	result := []any{}
	for _, item := range fields {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false, ErrDeliveryOwnership
		}
		name, _ := entry["manager"].(string)
		if entry["operation"] != "Update" || (name != deliveryHandoffManager && name != "kubectl-patch") {
			result = append(result, item)
			continue
		}
		body, ok := entry["fieldsV1"].(map[string]any)
		if !ok {
			return nil, false, ErrDeliveryOwnership
		}
		metadata, _ := body["f:metadata"].(map[string]any)
		for group, keys := range map[string][]string{"f:labels": {"app.kubernetes.io/managed-by", "orchestrator.io/managed-by"}, "f:annotations": {"orchestrator.io/delivery-revision", "meta.helm.sh/release-name", "meta.helm.sh/release-namespace", "helm.sh/resource-policy"}} {
			values, _ := metadata[group].(map[string]any)
			for _, key := range keys {
				field := "f:" + key
				if _, exists := values[field]; exists {
					delete(values, field)
					changed = true
				}
			}
			if len(values) == 0 || (len(values) == 1 && values["."] != nil) {
				delete(metadata, group)
			}
		}
		if len(metadata) == 0 {
			delete(body, "f:metadata")
		}
		if len(body) > 0 {
			result = append(result, item)
		}
	}
	return result, changed, nil
}
