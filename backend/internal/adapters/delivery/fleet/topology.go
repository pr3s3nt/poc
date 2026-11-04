package fleet

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	k8s "orchestrator/internal/adapters/kubernetes"
)

// Compatibility is deliberately bounded. A version alone never grants support:
// the served schemas must also expose the exact contracts used by this adapter.
var fleetVersion = regexp.MustCompile(`^v?0\.(13|14|15)\.(0|[1-9][0-9]*)(?:-rancher\.?[0-9]+)?(?:\+[A-Za-z0-9.-]+)?$`)

func fleetRelease(raw string) (string, bool) {
	m := fleetVersion.FindStringSubmatch(raw)
	if len(m) != 3 {
		return "", false
	}
	return "0." + m[1] + "." + m[2], true
}

type fleetWorkload struct {
	Kind     string
	Metadata struct {
		Name, Namespace, UID string
		Generation           int64
		Labels               map[string]string
	}
	Spec struct {
		Replicas *int64
		Template struct {
			Metadata struct{ Labels map[string]string }
			Spec     struct {
				ServiceAccountName string
				Containers         []struct {
					Name, Image   string
					Command, Args []string
				}
			}
		}
	}
	Status struct{ AvailableReplicas, ReadyReplicas, UpdatedReplicas, ObservedGeneration int64 }
}

func readFleetWorkloads(ctx context.Context, cli k8s.CLI) ([]fleetWorkload, error) {
	raw, err := cli.Run(ctx, nil, "get", "deployments,statefulsets", "--all-namespaces", "-o", "json")
	var list struct{ Items []fleetWorkload }
	if err != nil || json.Unmarshal(raw, &list) != nil {
		return nil, ErrVerification
	}
	return list.Items, nil
}

func workloadVersion(w fleetWorkload, role string) (string, bool) {
	if w.Kind != "Deployment" && w.Kind != "StatefulSet" {
		return "", false
	}
	version, identified := "", false
	for _, c := range w.Spec.Template.Spec.Containers {
		if c.Name != role {
			continue
		}
		identified = true
		image := strings.SplitN(c.Image, "@", 2)[0]
		name := image[strings.LastIndex(image, "/")+1:]
		if i := strings.LastIndex(name, ":"); i >= 0 {
			tag := name[i+1:]
			var ok bool
			version, ok = fleetRelease(tag)
			if !ok {
				return "", false
			}
			name = name[:i]
		}
		if name != "fleet" && !(role == "fleet-agent" && name == "fleet-agent") {
			return "", false
		}
	}
	if !identified {
		return "", false
	}
	for _, labels := range []map[string]string{w.Metadata.Labels, w.Spec.Template.Metadata.Labels} {
		if v := labels["app.kubernetes.io/version"]; v != "" {
			v, ok := fleetRelease(v)
			if !ok {
				return "", false
			}
			if version != "" && version != v {
				return "", false
			}
			version = v
		}
	}
	return version, version != ""
}

func workloadHealthy(w fleetWorkload) bool {
	if w.Metadata.Generation > 0 && w.Status.ObservedGeneration < w.Metadata.Generation {
		return false
	}
	desired := int64(1)
	if w.Spec.Replicas != nil {
		desired = *w.Spec.Replicas
	}
	if desired < 1 {
		return false
	}
	if w.Metadata.Generation > 0 && w.Status.UpdatedReplicas < desired {
		return false
	}
	ready := w.Status.AvailableReplicas
	if w.Kind == "StatefulSet" {
		ready = w.Status.ReadyReplicas
	}
	return ready >= desired
}

func controllerFrom(workloads []fleetWorkload) (fleetWorkload, string, error) {
	var found fleetWorkload
	version, count := "", 0
	for _, w := range workloads {
		// A shard is not a separate manager; the unsharded controller owns agent management.
		if w.Spec.Template.Metadata.Labels["fleet.cattle.io/shard-id"] != "" {
			continue
		}
		for _, c := range w.Spec.Template.Spec.Containers {
			if c.Name == "fleet-controller" {
				if _, ok := workloadVersion(w, "fleet-controller"); !ok {
					return found, "", ErrVerification
				}
			}
		}
		if v, ok := workloadVersion(w, "fleet-controller"); ok {
			if !workloadHealthy(w) {
				return found, "", ErrVerification
			}
			found, version, count = w, v, count+1
		}
	}
	if count != 1 {
		return found, "", ErrVerification
	}
	return found, version, nil
}

func sameFleetMinor(left, right string) bool {
	l, r := strings.Split(left, "."), strings.Split(right, ".")
	return len(l) >= 2 && len(r) >= 2 && l[0] == r[0] && l[1] == r[1]
}

var fleetSchemaPaths = map[string][]string{
	"gitrepos.fleet.cattle.io":          {"spec.repo", "spec.revision", "spec.paths", "spec.targets", "spec.clientSecretName", "spec.forceSyncGeneration", "spec.keepResources", "spec.serviceAccount"},
	"bundles.fleet.cattle.io":           {"spec.resources", "spec.helm", "spec.serviceAccount", "spec.keepResources", "spec.correctDrift"},
	"bundledeployments.fleet.cattle.io": {"spec.deploymentID", "status.appliedDeploymentID", "status.modifiedStatus"},
	"clusters.fleet.cattle.io":          {"spec.clientID", "status.namespace"},
}

func supportsFleetSchema(crd map[string]any, paths []string) bool {
	spec, _ := crd["spec"].(map[string]any)
	versions, _ := spec["versions"].([]any)
	for _, raw := range versions {
		v, _ := raw.(map[string]any)
		if v["name"] != "v1alpha1" || v["served"] != true {
			continue
		}
		schema, _ := v["schema"].(map[string]any)
		root, _ := schema["openAPIV3Schema"].(map[string]any)
		for _, path := range paths {
			node := root
			for _, key := range strings.Split(path, ".") {
				props, _ := node["properties"].(map[string]any)
				node, _ = props[key].(map[string]any)
				if node == nil {
					return false
				}
			}
			kind := "string"
			switch path {
			case "spec.paths", "spec.targets", "spec.resources", "status.modifiedStatus":
				kind = "array"
			case "spec.keepResources":
				kind = "boolean"
			case "spec.forceSyncGeneration":
				kind = "integer"
			case "spec.helm", "spec.correctDrift":
				kind = "object"
			}
			if node["type"] != kind {
				return false
			}
		}
		return true
	}
	return false
}

func checkFleetSchemas(ctx context.Context, cli k8s.CLI) error {
	for name, paths := range fleetSchemaPaths {
		crd, found, err := cli.Get(ctx, "", "customresourcedefinitions.apiextensions.k8s.io", name)
		if err != nil || !found || !supportsFleetSchema(crd, paths) {
			return ErrVerification
		}
	}
	return nil
}
