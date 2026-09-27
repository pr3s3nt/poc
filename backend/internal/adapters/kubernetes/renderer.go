// Package kubernetes renders Kubernetes manifests and talks to a cluster through
// the kubectl transport. It implements the execution ports for both kind and EKS.
package kubernetes

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// Platform default container requests for a field whose Score declares neither
// a request nor a limit (UC-06 BR-11). Limits have no default.
const (
	DefaultCPURequest    = "10m"
	DefaultMemoryRequest = "32Mi"
)

// Renderer turns a workload module into Deployment, Service and Secret manifests.
type Renderer struct{}

var shellIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var vaultRefPattern = regexp.MustCompile(`^kv2://([A-Za-z0-9_-]+)/([A-Za-z0-9_/-]+)$`)

// NewRenderer returns the Kubernetes workload renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// Render implements execution.WorkloadRenderer (UC-06 MS-11).
func (r *Renderer) Render(_ context.Context, req execution.RenderRequest) ([]execution.Manifest, error) {
	if req.Namespace == "" {
		return nil, fmt.Errorf("kubernetes: render %q without a namespace", req.WorkloadID)
	}
	labels := map[string]any{"app.kubernetes.io/name": req.WorkloadID, "app.kubernetes.io/managed-by": "orchestrator"}
	for k, v := range req.Labels {
		labels[k] = v
	}
	var podAnnotations map[string]any
	if req.Vault != nil {
		if req.Vault.Role == "" || req.Vault.Address == "" || req.Vault.ServiceAccount == "" {
			return nil, fmt.Errorf("kubernetes: incomplete Vault injection")
		}
		template, firstPath, err := vaultTemplate(req.Vault.Bindings)
		if err != nil {
			return nil, err
		}
		podAnnotations = map[string]any{
			"vault.hashicorp.com/agent-inject":                  "true",
			"vault.hashicorp.com/agent-pre-populate-only":       "true",
			"vault.hashicorp.com/role":                          req.Vault.Role,
			"vault.hashicorp.com/service":                       req.Vault.Address,
			"vault.hashicorp.com/agent-inject-secret-app-env":   firstPath,
			"vault.hashicorp.com/agent-inject-template-app-env": template,
			"vault.hashicorp.com/agent-inject-perms-app-env":    "0400",
		}
	}

	var manifests []execution.Manifest
	secretName := req.WorkloadID + "-env"
	secretData := map[string]any{}
	for _, containerName := range sortedKeys(req.SecretEnv) {
		for _, key := range sortedStringKeys(req.SecretEnv[containerName]) {
			secretData[secretKey(containerName, key)] = req.SecretEnv[containerName][key]
		}
	}
	if len(secretData) > 0 {
		manifests = append(manifests, execution.Manifest{
			APIVersion: "v1", Kind: "Secret", Name: secretName, Namespace: req.Namespace, Secret: true,
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]any{"name": secretName, "namespace": req.Namespace, "labels": labels},
				"type":       "Opaque",
				"stringData": secretData,
			},
		})
	}

	containerNames := make([]string, 0, len(req.Module.Spec.Containers))
	for name := range req.Module.Spec.Containers {
		containerNames = append(containerNames, name)
	}
	sort.Strings(containerNames)

	containers := make([]any, 0, len(containerNames))
	for _, name := range containerNames {
		c := req.Module.Spec.Containers[name]
		env := make([]any, 0)
		for _, key := range sortedStringKeys(req.PlainEnv[name]) {
			env = append(env, map[string]any{"name": key, "value": req.PlainEnv[name][key]})
		}
		for _, key := range sortedStringKeys(req.SecretEnv[name]) {
			env = append(env, map[string]any{
				"name": key,
				"valueFrom": map[string]any{
					"secretKeyRef": map[string]any{"name": secretName, "key": secretKey(name, key)},
				},
			})
		}
		if req.Vault != nil {
			env = append(env, map[string]any{"name": "ORCHESTRATOR_CONFIG_FILE", "value": "/vault/secrets/app-env"})
		}
		container := map[string]any{
			"name":            name,
			"image":           c.Image,
			"imagePullPolicy": "IfNotPresent",
		}
		if len(env) > 0 {
			container["env"] = env
		}
		if len(c.Command) > 0 {
			container["command"] = toAnySlice(c.Command)
		}
		if len(c.Args) > 0 {
			container["args"] = toAnySlice(c.Args)
		}
		var ports []any
		if req.Module.Spec.Service != nil {
			for _, portName := range sortedPortKeys(req.Module.Spec.Service.Ports) {
				p := req.Module.Spec.Service.Ports[portName]
				target := p.TargetPort
				if target == 0 {
					target = p.Port
				}
				ports = append(ports, map[string]any{"name": portName, "containerPort": target})
			}
		}
		if len(ports) > 0 && name == containerNames[0] {
			container["ports"] = ports
		}
		if c.ReadinessProbe != nil {
			container["readinessProbe"] = httpProbe(*c.ReadinessProbe)
		}
		if c.LivenessProbe != nil {
			container["livenessProbe"] = httpProbe(*c.LivenessProbe)
		}
		container["resources"] = containerResources(c.Resources)
		containers = append(containers, container)
	}

	replicas := req.Module.Spec.ReplicaCount()
	selector := map[string]any{"app.kubernetes.io/name": req.WorkloadID}
	deploymentObject := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": req.WorkloadID, "namespace": req.Namespace, "labels": labels},
		"spec": map[string]any{
			"replicas": replicas,
			"selector": map[string]any{"matchLabels": selector},
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec":     map[string]any{"containers": containers},
			},
		},
	}
	if req.Vault != nil {
		podTemplate := deploymentObject["spec"].(map[string]any)["template"].(map[string]any)
		podTemplate["metadata"].(map[string]any)["annotations"] = podAnnotations
		podTemplate["spec"].(map[string]any)["serviceAccountName"] = req.Vault.ServiceAccount
		manifests = append(manifests, execution.Manifest{APIVersion: "v1", Kind: "ServiceAccount", Name: req.Vault.ServiceAccount, Namespace: req.Namespace, Object: map[string]any{
			"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": req.Vault.ServiceAccount, "namespace": req.Namespace, "labels": labels},
		}})
	}
	manifests = append(manifests, execution.Manifest{
		APIVersion: "apps/v1", Kind: "Deployment", Name: req.WorkloadID, Namespace: req.Namespace, Object: deploymentObject,
	})

	if req.Module.Spec.Service != nil && len(req.Module.Spec.Service.Ports) > 0 {
		var servicePorts []any
		for _, portName := range sortedPortKeys(req.Module.Spec.Service.Ports) {
			p := req.Module.Spec.Service.Ports[portName]
			target := p.TargetPort
			if target == 0 {
				target = p.Port
			}
			protocol := p.Protocol
			if protocol == "" {
				protocol = "TCP"
			}
			servicePorts = append(servicePorts, map[string]any{
				"name": portName, "port": p.Port, "targetPort": target, "protocol": protocol,
			})
		}
		manifests = append(manifests, execution.Manifest{
			APIVersion: "v1", Kind: "Service", Name: req.WorkloadID, Namespace: req.Namespace,
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]any{"name": req.WorkloadID, "namespace": req.Namespace, "labels": labels},
				"spec":       map[string]any{"type": "ClusterIP", "selector": selector, "ports": servicePorts},
			},
		})
	}
	return manifests, nil
}

func vaultTemplate(bindings map[string]map[string]string) (string, string, error) {
	entries := map[string]string{}
	for _, container := range sortedKeys(bindings) {
		for _, name := range sortedStringKeys(bindings[container]) {
			if !shellIdentifier.MatchString(name) {
				return "", "", fmt.Errorf("kubernetes: invalid injected environment name %q", name)
			}
			ref := bindings[container][name]
			if prior, exists := entries[name]; exists && prior != ref {
				return "", "", fmt.Errorf("kubernetes: conflicting injected environment name %q", name)
			}
			entries[name] = ref
		}
	}
	if len(entries) == 0 {
		return "", "", fmt.Errorf("kubernetes: no Vault bindings")
	}
	var builder strings.Builder
	builder.WriteString("# Sourced by the workload startup script. Values are encoded before entering shell syntax.\n")
	first := ""
	for _, name := range sortedStringKeys(entries) {
		parts := vaultRefPattern.FindStringSubmatch(entries[name])
		if parts == nil || strings.Contains(parts[2], "//") {
			return "", "", fmt.Errorf("kubernetes: invalid Vault value reference")
		}
		path := parts[1] + "/data/" + parts[2]
		if first == "" {
			first = path
		}
		builder.WriteString("{{- with secret \"" + path + "\" }}\n")
		builder.WriteString("__orch_value=$(printf '%s' '{{ .Data.data.value | base64Encode }}' | base64 -d; printf '.')\n")
		builder.WriteString("export " + name + "=\"${__orch_value%.}\"\n")
		builder.WriteString("unset __orch_value\n{{- end }}\n")
	}
	return builder.String(), first, nil
}

// containerResources maps declared Score requirements onto Kubernetes container
// resources without changing their values. Each request field is the declared
// request, else the declared limit of the same field, else the platform
// default; the limit fallback keeps a platform default from exceeding a
// declared limit. A declared request/limit pair is kept as declared even when
// the request exceeds the limit; the Kubernetes API validates it at apply time.
// Limits carry only declared fields. The module input is never modified.
func containerResources(declared *environment.ContainerResourceRequirements) map[string]any {
	var requests, limits environment.ComputeResources
	if declared != nil && declared.Requests != nil {
		requests = *declared.Requests
	}
	if declared != nil && declared.Limits != nil {
		limits = *declared.Limits
	}
	requests.CPU = firstDeclared(requests.CPU, limits.CPU, DefaultCPURequest)
	requests.Memory = firstDeclared(requests.Memory, limits.Memory, DefaultMemoryRequest)
	out := map[string]any{"requests": computeResources(requests)}
	if rendered := computeResources(limits); len(rendered) > 0 {
		out["limits"] = rendered
	}
	return out
}

func firstDeclared(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func computeResources(r environment.ComputeResources) map[string]any {
	out := map[string]any{}
	if r.CPU != "" {
		out["cpu"] = r.CPU
	}
	if r.Memory != "" {
		out["memory"] = r.Memory
	}
	return out
}

func httpProbe(p environment.Probe) map[string]any {
	return map[string]any{
		"httpGet":             map[string]any{"path": p.Path, "port": p.Port},
		"initialDelaySeconds": 2,
		"periodSeconds":       3,
		"failureThreshold":    30,
	}
}

func secretKey(container, key string) string { return container + "_" + key }

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedStringKeys(m map[string]string) []string { return sortedKeys(m) }

func sortedPortKeys(m map[string]environment.Port) []string { return sortedKeys(m) }

var _ execution.WorkloadRenderer = (*Renderer)(nil)
