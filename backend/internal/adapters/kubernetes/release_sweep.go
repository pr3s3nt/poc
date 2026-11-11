package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"orchestrator/internal/ports/execution"
)

// sweptResources are the namespaced kinds an ADR-042 Environment unit renders.
var sweptResources = []string{
	"vaultstaticsecrets.secrets.hashicorp.com", "vaultauths.secrets.hashicorp.com", "vaultconnections.secrets.hashicorp.com",
	"serviceaccounts", "services", "configmaps", "secrets", "deployments.apps", "statefulsets.apps", "cronjobs.batch", "jobs.batch",
	"poddisruptionbudgets.policy", "httproutes.gateway.networking.k8s.io",
}

// ReleasedObject is a live object of one Helm release with its UID.
type ReleasedObject struct {
	Ref execution.ObjectRef
	UID string
}

// ListReleasedObjects lists live platform-managed objects of the Application
// Environment in the target namespace that belong to the given Helm release.
func ListReleasedObjects(ctx context.Context, cli CLI, target execution.Target, release, releaseNamespace string) ([]ReleasedObject, error) {
	if release == "" || target.Namespace == "" {
		return nil, ErrDeliveryOwnership
	}
	selector := "orchestrator.io/managed-by=orchestrator,orchestrator.io/application=" + target.Extra["application"] + ",orchestrator.io/environment=" + target.Extra["environment"]
	out := []ReleasedObject{}
	for _, resource := range sweptResources {
		raw, err := cli.Run(ctx, nil, "get", resource, "-n", target.Namespace, "-l", selector, "-o", "json")
		if err != nil {
			if errors.Is(err, ErrResourceTypeNotFound) || errors.Is(err, ErrObjectNotFound) {
				continue
			}
			return nil, err
		}
		var list struct {
			Items []map[string]any `json:"items"`
		}
		if json.Unmarshal(raw, &list) != nil {
			return nil, ErrDeliveryOwnership
		}
		for _, item := range list.Items {
			md, _ := item["metadata"].(map[string]any)
			annotations, _ := md["annotations"].(map[string]any)
			if annotations["meta.helm.sh/release-name"] != release || annotations["meta.helm.sh/release-namespace"] != releaseNamespace || md["deletionTimestamp"] != nil {
				continue
			}
			apiVersion, _ := item["apiVersion"].(string)
			kind, _ := item["kind"].(string)
			name, _ := md["name"].(string)
			uid, _ := md["uid"].(string)
			if apiVersion == "" || kind == "" || name == "" || uid == "" || strings.Contains(name, "/") {
				continue
			}
			out = append(out, ReleasedObject{Ref: execution.ObjectRef{APIVersion: apiVersion, Kind: kind, Name: name}, UID: uid})
		}
	}
	return out, nil
}
