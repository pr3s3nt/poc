package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"orchestrator/internal/ports/execution"
)

// ExistingClusterAdapter resolves a registered Kubernetes cluster connection into
// resource outputs. It provisions nothing; it verifies reachability and reports
// the target other resources and workloads are applied to (UC-08 VAR-02).
type ExistingClusterAdapter struct {
	KubectlPath string
}

// NewExistingClusterAdapter returns the registered-cluster adapter.
func NewExistingClusterAdapter(kubectlPath string) *ExistingClusterAdapter {
	return &ExistingClusterAdapter{KubectlPath: kubectlPath}
}

// Provision implements execution.ResourceExecutor for the existing-cluster driver.
func (a *ExistingClusterAdapter) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	name := stringInput(req.Inputs, "name", req.Connection.ConfigString("cluster"))
	kubeContext := stringInput(req.Inputs, "kubeContext", req.Connection.ConfigString("kubeContext"))
	if kubeContext == "" {
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: connection %q has no kubeContext", req.Connection.Key)
	}
	cli := NewCLI(a.KubectlPath, kubeContext, "")

	out, err := cli.Run(ctx, nil, "version", "-o", "json")
	if err != nil {
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: cluster %q is not reachable: %w", name, err)
	}
	endpoint, err := clusterEndpoint(ctx, cli, kubeContext)
	if err != nil {
		return execution.ProvisionResult{}, err
	}
	serverVersion := ""
	var version map[string]any
	if json.Unmarshal(out, &version) == nil {
		if server, ok := version["serverVersion"].(map[string]any); ok {
			serverVersion, _ = server["gitVersion"].(string)
		}
	}

	return execution.ProvisionResult{
		Outputs: map[string]any{
			"name":        name,
			"endpoint":    endpoint,
			"kubeContext": kubeContext,
		},
		State: map[string]any{
			"driver":        "existing-cluster",
			"kubeContext":   kubeContext,
			"serverVersion": serverVersion,
		},
		Target: &execution.Target{Kind: "kubernetes", Context: kubeContext, ClusterName: name},
	}, nil
}

func clusterEndpoint(ctx context.Context, cli CLI, kubeContext string) (string, error) {
	out, err := cli.Run(ctx, nil, "config", "view", "-o", "json")
	if err != nil {
		return "", err
	}
	var config struct {
		Contexts []struct {
			Name    string `json:"name"`
			Context struct {
				Cluster string `json:"cluster"`
			} `json:"context"`
		} `json:"contexts"`
		Clusters []struct {
			Name    string `json:"name"`
			Cluster struct {
				Server string `json:"server"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(out, &config); err != nil {
		return "", fmt.Errorf("kubernetes: decode kubeconfig view: %w", err)
	}
	clusterName := ""
	for _, c := range config.Contexts {
		if c.Name == kubeContext {
			clusterName = c.Context.Cluster
		}
	}
	for _, c := range config.Clusters {
		if c.Name == clusterName {
			return c.Cluster.Server, nil
		}
	}
	return "", fmt.Errorf("kubernetes: context %q has no cluster endpoint", kubeContext)
}

var _ execution.ResourceExecutor = (*ExistingClusterAdapter)(nil)

// TrimName keeps object names inside the DNS-1123 limit.
func TrimName(name string) string { return strings.Trim(objectName(name), "-") }
