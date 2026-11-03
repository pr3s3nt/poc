package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"sort"
	"strings"
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
	raw, err := cli.Run(ctx, nil, "get", "deployments", "--all-namespaces", "-o", "json")
	if err != nil {
		return out, ErrVerification
	}
	var deployments struct {
		Items []struct {
			Metadata struct{ Name, Namespace string }
			Spec     struct {
				Template struct {
					Spec struct {
						Containers []struct{ Name, Image string }
					}
				}
			}
		}
	}
	if json.Unmarshal(raw, &deployments) != nil {
		return out, ErrVerification
	}
	controller := map[string]string{}
	for _, d := range deployments.Items {
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "fleet-controller" && (strings.HasSuffix(c.Image, "/fleet:"+Version) || c.Image == "rancher/fleet:"+Version) {
				controller[d.Metadata.Namespace+"/"+d.Metadata.Name] = d.Metadata.Namespace
			}
		}
	}
	if len(controller) != 1 {
		return out, ErrVerification
	}
	values := map[string]string{"targetAccountKey": target.Connection}
	for name, namespace := range controller {
		values["controllerName"] = strings.SplitN(name, "/", 2)[1]
		values["controllerNamespace"] = namespace
	}
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
	raw, err = cli.Run(ctx, nil, "get", "clusters.fleet.cattle.io", "--all-namespaces", "-o", "json")
	if err != nil {
		return out, ErrVerification
	}
	var clusters struct {
		Items []struct {
			Metadata struct {
				Name, Namespace, UID string
				Annotations          map[string]string
			}
			Spec struct{ ClientID, KubeConfigSecret string }
		}
	}
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
		delete(copy, "targetAccountKey")
		for _, candidate := range candidates {
			if registrationMatches(c.Metadata.Name, c.Spec.ClientID, c.Spec.KubeConfigSecret, c.Metadata.Annotations["objectset.rio.cattle.io/id"], managerUID, candidate.uid) {
				copy["targetAccountKey"] = candidate.target.Connection
				copy["agentNamespace"] = candidate.agentNamespace
				break
			}
		}
		option := port.ConfigurationOption{Name: c.Metadata.Namespace + "/" + c.Metadata.Name, Value: c.Metadata.Name, Values: copy}
		if copy["targetAccountKey"] == "" {
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
