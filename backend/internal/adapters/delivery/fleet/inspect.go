package fleet

import (
	"context"
	"encoding/json"
	"net/url"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"sort"
	"strings"
)

func (p *Provider) InspectConfiguration(ctx context.Context, b port.BindingVersion) (port.ConfigurationDiscovery, error) {
	out := port.ConfigurationDiscovery{Options: map[string][]port.ConfigurationOption{"targetCluster": {}, "existingGitRepo": {}}}
	workspace := b.Config["workspace"]
	if !label.MatchString(workspace) || p.Manager == nil || p.Manager.Verifier == nil {
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
	raw, err := cli.Run(ctx, nil, "get", "clusters.fleet.cattle.io", "-n", workspace, "-o", "json")
	if err != nil {
		return out, ErrVerification
	}
	var clusters struct {
		Items []struct{ Metadata struct{ Name, UID string } }
	}
	if json.Unmarshal(raw, &clusters) != nil {
		return out, ErrVerification
	}
	for _, c := range clusters.Items {
		out.Options["targetCluster"] = append(out.Options["targetCluster"], port.ConfigurationOption{Name: c.Metadata.Name, Value: c.Metadata.Name, Values: map[string]string{"targetCluster": c.Metadata.Name, "targetClusterUID": c.Metadata.UID}})
	}
	raw, err = cli.Run(ctx, nil, "get", "gitrepos.fleet.cattle.io", "-n", workspace, "-o", "json")
	if err != nil {
		return out, ErrVerification
	}
	var repos struct {
		Items []struct {
			Metadata struct{ Name, UID string }
			Spec     struct {
				Repo, Branch, Revision, ClientSecretName string
				Paths                                    []string
				Targets                                  []struct{ ClusterName string }
			}
		}
	}
	if json.Unmarshal(raw, &repos) != nil {
		return out, ErrVerification
	}
	for _, r := range repos.Items {
		u, err := url.Parse(r.Spec.Repo)
		if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(r.Spec.Targets) > 1 || r.Spec.Revision != "" || r.Spec.Branch == "" {
			continue
		}
		values := map[string]string{"existingGitRepo": r.Metadata.Name, "existingGitRepoUID": r.Metadata.UID, "repositoryURL": r.Spec.Repo, "branch": r.Spec.Branch, "readSecret": r.Spec.ClientSecretName}
		if len(r.Spec.Targets) == 1 {
			values["targetCluster"] = r.Spec.Targets[0].ClusterName
		}
		if len(r.Spec.Paths) == 1 {
			values["root"] = strings.TrimSuffix(r.Spec.Paths[0], "/") + "/orchestrator"
		}
		out.Options["existingGitRepo"] = append(out.Options["existingGitRepo"], port.ConfigurationOption{Name: r.Metadata.Name, Value: r.Metadata.Name, Values: values})
	}
	for _, options := range out.Options {
		sort.Slice(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	}
	return out, nil
}
