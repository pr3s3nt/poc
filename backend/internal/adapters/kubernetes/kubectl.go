package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// CLI is the kubectl transport used by the Kubernetes adapters. It keeps the
// orchestrator free of a Kubernetes client dependency and works the same way for
// a kind cluster and for EKS.
type CLI struct {
	Path       string
	Context    string
	Kubeconfig string
	// private is a per-operation credential file path that must not appear
	// in returned errors.
	private string
}

// ErrCredentialTarget is returned when a credential-backed target cannot be
// opened; the cause is never a host-credential fallback.
var ErrCredentialTarget = errors.New("kubernetes: the target connection credential could not be resolved")

// kubectl failure categories. They are derived from kubectl output so callers
// can branch with errors.Is without parsing remote text.
var (
	ErrObjectNotFound  = errors.New("the Kubernetes object was not found")
	ErrUnauthorized    = errors.New("the cluster rejected the connection credentials")
	ErrForbidden       = errors.New("the connection credentials are not permitted to perform the operation")
	ErrUnreachable     = errors.New("the cluster API is not reachable")
	ErrRequestRejected = errors.New("the cluster rejected the request")
	ErrKubectlCommand  = errors.New("the kubectl command failed")
)

// CommandError is one failed kubectl invocation. For credential-backed targets
// it never carries kubectl output: an API server or proxy may echo submitted
// credential bytes, so only the operation, a fixed category and the process
// status are reported. Legacy host-context targets keep kubectl diagnostics.
type CommandError struct {
	// Operation is the orchestrator-built kubectl arguments, without
	// --kubeconfig or --context.
	Operation []string
	// Category is one of the Err* categories above.
	Category error
	// Process is the process status (exit status, signal or context error).
	Process error
	// diagnostics is raw kubectl stderr, kept for legacy targets only.
	diagnostics string
}

func (e *CommandError) Error() string {
	operation := "kubectl " + strings.Join(e.Operation, " ")
	if e.diagnostics != "" {
		return fmt.Sprintf("%s: %v: %s", operation, e.Process, e.diagnostics)
	}
	return fmt.Sprintf("%s: %v (%s)", operation, e.Category, processStatus(e.Process))
}

func (e *CommandError) Unwrap() []error { return []error{e.Category, e.Process} }

// processStatus reports the safe process status of a failed command.
func processStatus(err error) string {
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &exitErr):
		return exitErr.ProcessState.String()
	case errors.Is(err, exec.ErrNotFound):
		return "kubectl executable not found"
	default:
		return "kubectl did not run"
	}
}

// classify maps kubectl stderr to a failure category. Only the category leaves
// this function for credential-backed targets.
func classify(stderr string) error {
	msg := strings.ToLower(stderr)
	switch {
	case strings.Contains(stderr, "NotFound") || strings.Contains(msg, "not found"):
		return ErrObjectNotFound
	case strings.Contains(msg, "unauthorized") || strings.Contains(msg, "must be logged in"):
		return ErrUnauthorized
	case strings.Contains(msg, "forbidden"):
		return ErrForbidden
	case strings.Contains(msg, "unable to connect to the server") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") || strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "dial tcp") || strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:") ||
		strings.Contains(msg, "deadline exceeded"):
		return ErrUnreachable
	case strings.Contains(msg, "error from server"):
		return ErrRequestRejected
	default:
		return ErrKubectlCommand
	}
}

// credentialTargetError keeps the resolver failure identity for errors.Is
// while its text stays fixed: resolver messages are never reported.
type credentialTargetError struct{ cause error }

func (e credentialTargetError) Error() string   { return ErrCredentialTarget.Error() }
func (e credentialTargetError) Unwrap() []error { return []error{ErrCredentialTarget, e.cause} }

// OpenCLI returns the kubectl transport of one target and a cleanup function
// that must run after the operation. Legacy targets use their host context or
// kubeconfig path. Credential-backed targets resolve the Connection's
// normalized kubeconfig into a private mode-0600 file in a private directory
// that cleanup removes; the path is never stored in a Target or output.
func OpenCLI(ctx context.Context, path string, source execution.KubeconfigSource, target execution.Target) (CLI, func(), error) {
	if !target.CredentialBacked() {
		return NewCLI(path, target.Context, target.Kubeconfig), func() {}, nil
	}
	if source == nil {
		return CLI{}, func() {}, ErrCredentialTarget
	}
	document, err := source.ResolveKubeconfig(ctx, target)
	if err != nil {
		return CLI{}, func() {}, credentialTargetError{cause: err}
	}
	return materialize(path, target.Context, document)
}

// materialize writes a kubeconfig document to a private file for one operation.
func materialize(path, kubeContext string, document []byte) (CLI, func(), error) {
	dir, err := os.MkdirTemp("", "orch-kube-")
	if err != nil {
		return CLI{}, func() {}, fmt.Errorf("%w: private directory unavailable", ErrCredentialTarget)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := os.Chmod(dir, 0o700); err != nil {
		cleanup()
		return CLI{}, func() {}, fmt.Errorf("%w: private directory unavailable", ErrCredentialTarget)
	}
	file := filepath.Join(dir, "config")
	if err := os.WriteFile(file, document, 0o600); err != nil {
		cleanup()
		return CLI{}, func() {}, fmt.Errorf("%w: private file unavailable", ErrCredentialTarget)
	}
	cli := NewCLI(path, kubeContext, file)
	cli.private = file
	return cli, cleanup, nil
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
		stderr := strings.TrimSpace(errOut.String())
		failure := &CommandError{Operation: redactArgs(args), Category: classify(stderr), Process: err}
		if c.private == "" {
			failure.diagnostics = stderr
		}
		return out.Bytes(), failure
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
		if errors.Is(err, ErrObjectNotFound) {
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
