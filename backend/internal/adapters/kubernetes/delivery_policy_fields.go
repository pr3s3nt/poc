package kubernetes

import (
	"context"
	"encoding/json"
	"strings"

	"orchestrator/internal/ports/execution"
)

// ReclaimManualDeliveryFields is only for an explicitly admitted self-heal
// policy. It relinquishes manual Update ownership of declared fields, without
// changing their values. Fleet performs the actual correction from Git.
func ReclaimManualDeliveryFields(ctx context.Context, cli CLI, target execution.Target, ref execution.ObjectRef, uid string, manager DeliveryManager, desired map[string]any, guard func(context.Context) error, revisions ...string) error {
	if guard == nil || ref.Kind == "Job" || ref.Kind == "Secret" {
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
	if _, err := ownershipPatch(object, target, uid, manager, manager); err != nil {
		return err
	}
	md, _ := object["metadata"].(map[string]any)
	annotations, _ := md["annotations"].(map[string]any)
	allowed := false
	for _, revision := range revisions {
		if revision != "" && annotations["orchestrator.io/delivery-revision"] == revision {
			allowed = true
		}
	}
	if !allowed {
		return ErrDeliveryOwnership
	}
	if ref.Kind == "Deployment" || ref.Kind == "StatefulSet" {
		spec, _ := desired["spec"].(map[string]any)
		if _, declared := spec["replicas"]; declared {
			raw, err := cli.Run(ctx, nil, "get", "horizontalpodautoscalers.autoscaling", "-n", target.Namespace, "-o", "json")
			if err != nil {
				return ErrDeliveryOwnership
			}
			var hpas struct {
				Items []struct {
					Spec struct {
						ScaleTargetRef struct{ APIVersion, Kind, Name string }
					}
				}
			}
			if json.Unmarshal(raw, &hpas) != nil {
				return ErrDeliveryOwnership
			}
			for _, hpa := range hpas.Items {
				r := hpa.Spec.ScaleTargetRef
				if r.APIVersion == ref.APIVersion && r.Kind == ref.Kind && r.Name == ref.Name {
					return ErrDeliveryOwnership
				}
			}
		}
	}
	fields, changed, err := manualDesiredFields(md["managedFields"], desired)
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
	if err := guard(ctx); err != nil {
		return err
	}
	_, err = cli.Run(ctx, nil, "patch", name, ref.Name, "-n", target.Namespace, "--type=json", "--field-manager", "orchestrator-policy-fields", "--patch", string(patch))
	return err
}

func manualDesiredFields(raw any, desired map[string]any) ([]any, bool, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false, ErrDeliveryOwnership
	}
	var fields []any
	if json.Unmarshal(encoded, &fields) != nil {
		return nil, false, ErrDeliveryOwnership
	}
	result := []any{}
	changed := false
	for _, item := range fields {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false, ErrDeliveryOwnership
		}
		name, _ := entry["manager"].(string)
		if entry["operation"] != "Update" || !strings.HasPrefix(name, "kubectl-") {
			result = append(result, item)
			continue
		}
		body, ok := entry["fieldsV1"].(map[string]any)
		if !ok {
			return nil, false, ErrDeliveryOwnership
		}
		for _, root := range []string{"spec", "data", "binaryData", "metadata"} {
			value, present := desired[root]
			if !present {
				continue
			}
			if root == "metadata" {
				metadata, _ := value.(map[string]any)
				allowed := map[string]any{}
				for _, key := range []string{"labels", "annotations"} {
					if v, ok := metadata[key]; ok {
						allowed[key] = v
					}
				}
				value = allowed
			}
			key := "f:" + root
			tree, owned := body[key].(map[string]any)
			if !owned {
				continue
			}
			if pruneDeclaredFields(tree, value) {
				changed = true
			}
			if len(tree) == 0 {
				delete(body, key)
			}
		}
		if len(body) > 0 {
			result = append(result, item)
		}
	}
	return result, changed, nil
}

// Unknown field-set encodings stay owned. List map keys are resolved against
// the admitted list, never positional guesses or all fields of the live object.
func pruneDeclaredFields(fields map[string]any, desired any) bool {
	changed := false
	for key, raw := range fields {
		if key == "." {
			continue
		}
		var value any
		present := false
		if strings.HasPrefix(key, "f:") {
			if object, ok := desired.(map[string]any); ok {
				value, present = object[strings.TrimPrefix(key, "f:")]
			}
		} else if strings.HasPrefix(key, "k:") {
			var selector map[string]any
			if json.Unmarshal([]byte(strings.TrimPrefix(key, "k:")), &selector) == nil {
				if list, ok := desired.([]any); ok {
					for _, item := range list {
						object, ok := item.(map[string]any)
						if !ok {
							continue
						}
						matches := true
						for field, want := range selector {
							got, _ := json.Marshal(object[field])
							expected, _ := json.Marshal(want)
							if string(got) != string(expected) {
								matches = false
							}
						}
						if matches {
							value, present = item, true
							break
						}
					}
				}
			}
		}
		if !present {
			continue
		}
		child, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if len(child) == 0 {
			delete(fields, key)
			changed = true
			continue
		}
		if pruneDeclaredFields(child, value) {
			changed = true
		}
		if len(child) == 0 || (len(child) == 1 && child["."] != nil) {
			delete(fields, key)
			changed = true
		}
	}
	return changed
}
