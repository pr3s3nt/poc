package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/application/connection"
)

// ConnectionVerifier validates a Kubernetes Connection without mutating
// Kubernetes: either an existing host kube context (legacy) or an uploaded,
// normalized selected-context kubeconfig.
type ConnectionVerifier struct{ KubectlPath string }

const verifyTimeout = 30 * time.Second

func (v ConnectionVerifier) Verify(parent context.Context, kubeContext string) (connection.KubernetesVerification, error) {
	ctx, cancel := context.WithTimeout(parent, verifyTimeout)
	defer cancel()
	cli := NewCLI(v.KubectlPath, kubeContext, "")
	contexts, err := cli.Run(ctx, nil, "config", "get-contexts", "-o", "name")
	if err != nil {
		return connection.KubernetesVerification{}, fmt.Errorf("%w: %v", connection.ErrContextMissing, err)
	}
	found := false
	for _, name := range strings.Split(string(contexts), "\n") {
		if strings.TrimSpace(name) == kubeContext {
			found = true
			break
		}
	}
	if !found {
		return connection.KubernetesVerification{}, fmt.Errorf("%w: %q", connection.ErrContextMissing, kubeContext)
	}
	version, err := apiVersion(ctx, cli)
	if err != nil {
		return connection.KubernetesVerification{}, err
	}
	endpoint, err := clusterEndpoint(ctx, cli, kubeContext)
	if err != nil {
		return connection.KubernetesVerification{}, fmt.Errorf("%w: %v", connection.ErrClusterUnreachable, err)
	}
	if err := checkPermissions(ctx, cli); err != nil {
		return connection.KubernetesVerification{}, err
	}
	return connection.KubernetesVerification{Endpoint: endpoint, Version: version}, nil
}

// VerifyKubeconfig checks API reachability, authentication and the VAR-01
// permissions with only the uploaded selected context. The document is
// written to a private temporary file for the duration of the check; the
// backend host's kube configuration is never consulted. Raw kubectl output is
// never part of the returned error.
func (v ConnectionVerifier) VerifyKubeconfig(parent context.Context, selected connection.SelectedKubeconfig) (connection.KubernetesVerification, error) {
	ctx, cancel := context.WithTimeout(parent, verifyTimeout)
	defer cancel()
	cli, cleanup, err := materialize(v.KubectlPath, selected.Context.Name, selected.Document)
	if err != nil {
		return connection.KubernetesVerification{}, connection.ErrClusterUnreachable
	}
	defer cleanup()
	version, err := apiVersion(ctx, cli)
	if err != nil {
		return connection.KubernetesVerification{}, err
	}
	if err := checkPermissions(ctx, cli); err != nil {
		return connection.KubernetesVerification{}, err
	}
	return connection.KubernetesVerification{Endpoint: selected.Context.Endpoint, Version: version}, nil
}

func apiVersion(ctx context.Context, cli CLI) (string, error) {
	out, err := cli.Run(ctx, nil, "version", "-o", "json")
	if err != nil {
		if unauthorized(err) {
			return "", connection.ErrAuthentication
		}
		return "", connection.ErrClusterUnreachable
	}
	var version struct {
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal(out, &version); err != nil {
		return "", connection.ErrClusterUnreachable
	}
	return version.ServerVersion.GitVersion, nil
}

func checkPermissions(ctx context.Context, cli CLI) error {
	for _, permission := range [][3]string{{"create", "namespaces", ""}, {"create", "deployments", "--all-namespaces"}, {"create", "statefulsets", "--all-namespaces"}, {"create", "services", "--all-namespaces"}, {"create", "secrets", "--all-namespaces"}} {
		args := []string{"auth", "can-i", permission[0], permission[1]}
		if permission[2] != "" {
			args = append(args, permission[2])
		}
		allowed, err := cli.Run(ctx, nil, args...)
		if err != nil && strings.TrimSpace(string(allowed)) != "no" {
			if unauthorized(err) {
				return connection.ErrAuthentication
			}
			return connection.ErrClusterUnreachable
		}
		if strings.TrimSpace(string(allowed)) != "yes" {
			return fmt.Errorf("%w: %s %s", connection.ErrPermissionDenied, permission[0], permission[1])
		}
	}
	return nil
}

func unauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden)
}

var (
	_ connection.KubernetesVerifier = ConnectionVerifier{}
	_ connection.KubeconfigVerifier = ConnectionVerifier{}
)
