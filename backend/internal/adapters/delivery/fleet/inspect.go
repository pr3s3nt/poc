package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"sort"
)

// InspectConfiguration performs read-only discovery through the selected
// Kubernetes Connection: it locates the Fleet controller and lists every
// registered cluster grouped by workspace as selectable options. The UI
// auto-fills when exactly one option is eligible; every option carries the
// derived technical identities pinned into the immutable binding version.
func (p *Provider) InspectConfiguration(ctx context.Context, b port.BindingVersion) (port.ConfigurationDiscovery, error) {
	out := port.ConfigurationDiscovery{Options: map[string][]port.ConfigurationOption{"workspace": {}, "targetCluster": {}}}
	if p.Manager == nil || p.Manager.Verifier == nil || p.Manager.Verifier.Targets == nil || p.Manager.Verifier.Credentials == nil {
		return out, ErrConfig
	}
	v := p.Manager.Verifier
	target, err := v.Targets.ResolveManagementTarget(ctx, b)
	if err != nil {
		return out, ErrVerification
	}
	cli, close, err := k8s.OpenCLI(ctx, v.KubectlPath, v.Credentials, target)
	if err != nil {
		return out, ErrVerification
	}
	defer close()
	workloads, err := readFleetWorkloads(ctx, cli)
	if err != nil {
		return out, err
	}
	controller, version, err := controllerFrom(workloads)
	if err != nil {
		return out, err
	}
	if err = checkFleetSchemas(ctx, cli); err != nil {
		return out, err
	}
	values := map[string]string{"controllerName": controller.Metadata.Name, "controllerNamespace": controller.Metadata.Namespace, "controllerKind": controller.Kind, "controllerVersion": version}
	managerNS, exists, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !exists {
		return out, ErrVerification
	}
	managerMetadata, _ := managerNS["metadata"].(map[string]any)
	managerUID, _ := managerMetadata["uid"].(string)
	candidates, err := v.workloadCandidates(ctx, b, target)
	if err != nil {
		return out, ErrVerification
	}
	raw, err := cli.Run(ctx, nil, "get", "clusters.fleet.cattle.io", "--all-namespaces", "-o", "json")
	if err != nil {
		return out, ErrVerification
	}
	var clusters struct{ Items []fleetRegistration }
	if json.Unmarshal(raw, &clusters) != nil {
		return out, ErrVerification
	}
	workspaces := map[string]bool{}
	for _, c := range clusters.Items {
		if c.Metadata.Name == "" || c.Metadata.Namespace == "" || c.Metadata.UID == "" {
			continue
		}
		workspaces[c.Metadata.Namespace] = true
		copy := map[string]string{}
		for k, v := range values {
			copy[k] = v
		}
		copy["workspace"] = c.Metadata.Namespace
		copy["targetCluster"] = c.Metadata.Name
		copy["targetClusterUID"] = c.Metadata.UID
		candidate, matched := matchingTarget(c, managerUID, version, candidates)
		display := c.Metadata.Namespace + "/" + c.Metadata.Name
		if name := c.Metadata.Labels["management.cattle.io/cluster-display-name"]; name != "" {
			display = name + " · " + display
		}
		option := port.ConfigurationOption{Name: display, Value: c.Metadata.Name, Values: copy}
		if matched {
			copy["targetAccountKey"] = candidate.target.Connection
			copy["targetKubeSystemUID"] = candidate.uid
			copy["agentNamespace"] = candidate.agentNamespace
			copy["agentName"] = candidate.agentName
			copy["agentKind"] = candidate.agentKind
			copy["serviceAccount"] = candidate.serviceAccount
			copy["serviceAccountMode"] = ""
			if candidate.serviceAccount == "" {
				copy["serviceAccountMode"] = "NATIVE"
			}
			copy["releaseNamespace"] = candidate.releaseNamespace
		} else {
			option.Disabled = true
			option.ReasonCode = "TARGET_MAPPING_UNSUPPORTED"
		}
		out.Options["targetCluster"] = append(out.Options["targetCluster"], option)
	}
	list := make([]string, 0, len(workspaces))
	for w := range workspaces {
		list = append(list, w)
	}
	sort.Strings(list)
	for _, w := range list {
		out.Options["workspace"] = append(out.Options["workspace"], port.ConfigurationOption{Name: w, Value: w, Values: map[string]string{"workspace": w}})
	}
	sort.Slice(out.Options["targetCluster"], func(i, j int) bool {
		return out.Options["targetCluster"][i].Name < out.Options["targetCluster"][j].Name
	})
	if len(out.Options["targetCluster"]) == 0 {
		return out, ErrVerification
	}
	return out, nil
}
