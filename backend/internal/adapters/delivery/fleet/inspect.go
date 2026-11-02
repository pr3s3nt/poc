package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"strings"
)

func (p *Provider) InspectConfiguration(ctx context.Context, b port.BindingVersion) (port.ConfigurationDiscovery, error) {
	out := port.ConfigurationDiscovery{Options: map[string][]port.ConfigurationOption{"targetCluster": {}}}
	if p.Manager == nil || p.Manager.Verifier == nil {
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
					Spec struct{ Containers []struct{ Image string } }
				}
			}
		}
	}
	if json.Unmarshal(raw, &deployments) != nil {
		return out, ErrVerification
	}
	values := map[string]string{"targetAccountKey": target.Connection}
	controllers, agents := 0, 0
	for _, d := range deployments.Items {
		controller := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if strings.HasSuffix(c.Image, "/fleet:"+Version) || c.Image == "rancher/fleet:"+Version {
				controller = true
			}
		}
		if controller {
			controllers++
			values["controllerNamespace"] = d.Metadata.Namespace
			values["controllerName"] = d.Metadata.Name
		}
		if d.Metadata.Name == "fleet-agent" {
			agents++
			values["agentNamespace"] = d.Metadata.Namespace
		}
	}
	if controllers != 1 || agents != 1 {
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
			Spec struct{ KubeConfigSecret string }
		}
	}
	if json.Unmarshal(raw, &clusters) != nil {
		return out, ErrVerification
	}
	for _, c := range clusters.Items {
		if c.Metadata.Name != "local" || c.Spec.KubeConfigSecret != "local-cluster" || c.Metadata.Annotations["objectset.rio.cattle.io/id"] != "fleet-bootstrap" {
			continue
		}
		copy := map[string]string{}
		for k, v := range values {
			copy[k] = v
		}
		copy["workspace"] = c.Metadata.Namespace
		copy["targetCluster"] = c.Metadata.Name
		copy["targetClusterUID"] = c.Metadata.UID
		out.Options["targetCluster"] = append(out.Options["targetCluster"], port.ConfigurationOption{Name: c.Metadata.Name, Value: c.Metadata.Name, Values: copy})
	}
	if len(out.Options["targetCluster"]) != 1 {
		return out, ErrVerification
	}
	return out, nil
}
