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

// ConnectionVerifier validates an uploaded, normalized selected-context
// kubeconfig without mutating Kubernetes.
type ConnectionVerifier struct{ KubectlPath string }

const verifyTimeout = 30 * time.Second

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
	_ connection.KubeconfigVerifier = ConnectionVerifier{}
)
