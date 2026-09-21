package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CLI is the kubectl transport used by the Kubernetes adapters. It keeps the
// orchestrator free of a Kubernetes client dependency and works the same way for
// a kind cluster and for EKS.
type CLI struct {
	Path       string
	Context    string
	Kubeconfig string
}

// NewCLI returns a kubectl transport for one target.
func NewCLI(path, kubeContext, kubeconfig string) CLI {
	if path == "" {
		path = "kubectl"
	}
	return CLI{Path: path, Context: kubeContext, Kubeconfig: kubeconfig}
}

func (c CLI) baseArgs() []string {
	var args []string
	if c.Kubeconfig != "" {
		args = append(args, "--kubeconfig", c.Kubeconfig)
	}
	if c.Context != "" {
		args = append(args, "--context", c.Context)
	}
	return args
}

// Run executes kubectl and returns stdout. Stdin is used for apply payloads.
func (c CLI) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	full := append(c.baseArgs(), args...)
	cmd := exec.CommandContext(ctx, c.Path, full...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("kubectl %s: %w: %s", strings.Join(redactArgs(args), " "), err, strings.TrimSpace(errOut.String()))
	}
	return out.Bytes(), nil
}

// Apply applies a list of objects server side. Manifests are never logged
// because they may carry generated credentials.
func (c CLI) Apply(ctx context.Context, objects []map[string]any) error {
	list := map[string]any{"apiVersion": "v1", "kind": "List", "items": objects}
	payload, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("kubernetes: encode manifests: %w", err)
	}
	_, err = c.Run(ctx, payload, "apply", "-f", "-")
	return err
}

// Get returns one object as a decoded JSON tree. The second result reports
// whether the object exists.
func (c CLI) Get(ctx context.Context, namespace, kind, name string) (map[string]any, bool, error) {
	args := []string{"get", kind, name, "-o", "json"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	out, err := c.Run(ctx, nil, args...)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil, false, nil
		}
		return nil, false, err
	}
	obj := map[string]any{}
	if err := json.Unmarshal(out, &obj); err != nil {
		return nil, false, fmt.Errorf("kubernetes: decode %s/%s: %w", kind, name, err)
	}
	return obj, true, nil
}

// RolloutStatus waits until a Deployment or StatefulSet reports readiness.
func (c CLI) RolloutStatus(ctx context.Context, namespace, kind, name string, timeout time.Duration) error {
	_, err := c.Run(ctx, nil, "rollout", "status", fmt.Sprintf("%s/%s", kind, name),
		"-n", namespace, fmt.Sprintf("--timeout=%ds", int(timeout.Seconds())))
	return err
}

// Delete removes one object and ignores a missing object.
func (c CLI) Delete(ctx context.Context, namespace, kind, name string) error {
	args := []string{"delete", kind, name, "--ignore-not-found"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	_, err := c.Run(ctx, nil, args...)
	return err
}

func redactArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	return out
}
