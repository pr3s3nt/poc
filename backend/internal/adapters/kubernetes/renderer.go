// Package kubernetes renders Kubernetes manifests and talks to a cluster through
// the kubectl transport. It implements the execution ports for both kind and EKS.
package kubernetes

import (
	"context"
	"fmt"
	"sort"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/execution"
)

// Renderer turns a workload module into Deployment, Service and Secret manifests.
type Renderer struct{}

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
		container["resources"] = map[string]any{
			"requests": map[string]any{"cpu": "10m", "memory": "32Mi"},
		}
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
