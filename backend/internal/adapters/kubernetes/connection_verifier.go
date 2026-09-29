package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/application/connection"
)

// ConnectionVerifier validates an existing host kube context without mutating Kubernetes.
type ConnectionVerifier struct{ KubectlPath string }

func (v ConnectionVerifier) Verify(parent context.Context, kubeContext string) (connection.KubernetesVerification, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cli := NewCLI(v.KubectlPath, kubeContext, "")
	contexts, err := cli.Run(ctx, nil, "config", "get-contexts", "-o", "name")
	if err != nil {
		return connection.KubernetesVerification{}, err
	}
	found := false
	for _, name := range strings.Split(string(contexts), "\n") {
		if strings.TrimSpace(name) == kubeContext {
			found = true
			break
		}
	}
	if !found {
		return connection.KubernetesVerification{}, fmt.Errorf("kube context %q is not configured on backend host", kubeContext)
	}
	out, err := cli.Run(ctx, nil, "version", "-o", "json")
	if err != nil {
		return connection.KubernetesVerification{}, err
	}
	var version struct {
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal(out, &version); err != nil {
		return connection.KubernetesVerification{}, err
	}
	endpoint, err := clusterEndpoint(ctx, cli, kubeContext)
	if err != nil {
		return connection.KubernetesVerification{}, err
	}
	for _, permission := range [][3]string{{"create", "namespaces", ""}, {"create", "deployments", "--all-namespaces"}, {"create", "statefulsets", "--all-namespaces"}, {"create", "services", "--all-namespaces"}, {"create", "secrets", "--all-namespaces"}} {
		args := []string{"auth", "can-i", permission[0], permission[1]}
		if permission[2] != "" {
			args = append(args, permission[2])
		}
		allowed, err := cli.Run(ctx, nil, args...)
		if err != nil {
			return connection.KubernetesVerification{}, err
		}
		if strings.TrimSpace(string(allowed)) != "yes" {
			return connection.KubernetesVerification{}, fmt.Errorf("kube context %q lacks %s %s permission", kubeContext, permission[0], permission[1])
		}
	}
	return connection.KubernetesVerification{Endpoint: endpoint, Version: version.ServerVersion.GitVersion}, nil
}

var _ connection.KubernetesVerifier = ConnectionVerifier{}
