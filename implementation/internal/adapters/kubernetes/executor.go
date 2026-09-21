package kubernetes

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// Executor provisions Kubernetes-backed resources: the Environment namespace and
// the internal PostgreSQL StatefulSet with its Service.
type Executor struct {
	KubectlPath string
	Timeout     time.Duration
}

// NewExecutor returns the Kubernetes resource executor.
func NewExecutor(kubectlPath string) *Executor {
	return &Executor{KubectlPath: kubectlPath, Timeout: 5 * time.Minute}
}

// Provision implements execution.ResourceExecutor for the kubernetes driver.
func (e *Executor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	cli := NewCLI(e.KubectlPath, req.Target.Context, req.Target.Kubeconfig)
	switch req.ResourceType {
	case "k8s-namespace":
		return e.provisionNamespace(ctx, cli, req)
	case "postgres":
		return e.provisionPostgres(ctx, cli, req)
	default:
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: no executor for resource type %q", req.ResourceType)
	}
}

func labelsFor(req execution.ProvisionRequest) map[string]any {
	labels := map[string]any{
		"app.kubernetes.io/managed-by": "orchestrator",
		"orchestrator.io/application":  req.ApplicationKey,
		"orchestrator.io/environment":  req.EnvironmentKey,
	}
	if req.RunID != "" {
		labels["orchestrator.io/run-id"] = req.RunID
	}
	return labels
}

func (e *Executor) provisionNamespace(ctx context.Context, cli CLI, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	name, _ := req.Inputs["name"].(string)
	if name == "" {
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: namespace resource %s has no name input", req.Descriptor)
	}
	object := map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": name, "labels": labelsFor(req)},
	}
	if err := cli.Apply(ctx, []map[string]any{object}); err != nil {
		return execution.ProvisionResult{}, err
	}
	return execution.ProvisionResult{
		Outputs: map[string]any{"name": name},
		State:   map[string]any{"driver": "kubernetes", "kind": "Namespace", "name": name},
	}, nil
}

// provisionPostgres applies the StatefulSet and Service that implement the
// postgres Resource Type inside the Environment namespace. The generated
// password is created once and then reused, because PostgreSQL only applies
// POSTGRES_PASSWORD when the data directory is initialised.
func (e *Executor) provisionPostgres(ctx context.Context, cli CLI, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	namespace := req.Target.Namespace
	if namespace == "" {
		// The Definition may also bind the namespace through a Resource Reference.
		namespace = stringInput(req.Inputs, "namespace", "")
	}
	if namespace == "" {
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: postgres resource %s has no namespace", req.Descriptor)
	}
	name := objectName(req.Descriptor)
	database := stringInput(req.Inputs, "database", "app")
	username := stringInput(req.Inputs, "username", "app")
	image := stringInput(req.Inputs, "image", "postgres:16-alpine")
	storage := stringInput(req.Inputs, "storage", "1Gi")
	secretName := name + "-credentials"

	password, err := reuseOrCreatePassword(ctx, cli, namespace, secretName)
	if err != nil {
		return execution.ProvisionResult{}, err
	}

	labels := labelsFor(req)
	selector := map[string]any{"app.kubernetes.io/name": name}
	podLabels := map[string]any{"app.kubernetes.io/name": name}
	for k, v := range labels {
		podLabels[k] = v
	}

	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": secretName, "namespace": namespace, "labels": labels},
		"type":       "Opaque",
		"stringData": map[string]any{"password": password},
	}
	service := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": name, "namespace": namespace, "labels": labels},
		"spec": map[string]any{
			"selector": selector,
			"ports":    []any{map[string]any{"name": "postgres", "port": 5432, "targetPort": 5432}},
		},
	}
	statefulSet := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata":   map[string]any{"name": name, "namespace": namespace, "labels": labels},
		"spec": map[string]any{
			"serviceName": name,
			"replicas":    1,
			"selector":    map[string]any{"matchLabels": selector},
			"template": map[string]any{
				"metadata": map[string]any{"labels": podLabels},
				"spec": map[string]any{
					"containers": []any{map[string]any{
						"name":  "postgres",
						"image": image,
						"env": []any{
							map[string]any{"name": "POSTGRES_DB", "value": database},
							map[string]any{"name": "POSTGRES_USER", "value": username},
							map[string]any{"name": "PGDATA", "value": "/var/lib/postgresql/data/pgdata"},
							map[string]any{"name": "POSTGRES_PASSWORD", "valueFrom": map[string]any{
								"secretKeyRef": map[string]any{"name": secretName, "key": "password"},
							}},
						},
						"ports":        []any{map[string]any{"name": "postgres", "containerPort": 5432}},
						"volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/var/lib/postgresql/data"}},
						"readinessProbe": map[string]any{
							"exec":                map[string]any{"command": []any{"pg_isready", "-U", username, "-d", database}},
							"initialDelaySeconds": 5,
							"periodSeconds":       5,
							"failureThreshold":    30,
						},
						"resources": map[string]any{"requests": map[string]any{"cpu": "50m", "memory": "128Mi"}},
					}},
				},
			},
			"volumeClaimTemplates": []any{map[string]any{
				"metadata": map[string]any{"name": "data", "labels": labels},
				"spec": map[string]any{
					"accessModes": []any{"ReadWriteOnce"},
					"resources":   map[string]any{"requests": map[string]any{"storage": storage}},
				},
			}},
		},
	}

	if err := cli.Apply(ctx, []map[string]any{secret, service, statefulSet}); err != nil {
		return execution.ProvisionResult{}, err
	}
	if err := cli.RolloutStatus(ctx, namespace, "statefulset", name, e.Timeout); err != nil {
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: postgres %s is not ready: %w", name, err)
	}

	return execution.ProvisionResult{
		Outputs: map[string]any{
			"host":     fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace),
			"port":     float64(5432),
			"database": database,
			"username": username,
			"password": password,
		},
		State: map[string]any{
			"driver":      "kubernetes",
			"kind":        "StatefulSet",
			"name":        name,
			"namespace":   namespace,
			"secretName":  secretName,
			"serviceName": name,
		},
	}, nil
}

func reuseOrCreatePassword(ctx context.Context, cli CLI, namespace, secretName string) (string, error) {
	obj, found, err := cli.Get(ctx, namespace, "secret", secretName)
	if err != nil {
		return "", err
	}
	if found {
		data, _ := obj["data"].(map[string]any)
		if encoded, ok := data["password"].(string); ok {
			raw, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil && len(raw) > 0 {
				return string(raw), nil
			}
		}
	}
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("kubernetes: generate password: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// objectName turns a descriptor into a DNS-1123 object name.
func objectName(descriptor string) string {
	name := descriptor
	if idx := strings.Index(name, "#"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")
	if len(name) > 52 {
		name = name[:52]
	}
	return strings.Trim(strings.ToLower(name), "-")
}

func stringInput(inputs map[string]any, key, fallback string) string {
	if v, ok := inputs[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

var _ execution.ResourceExecutor = (*Executor)(nil)
