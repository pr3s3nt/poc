package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
)

func endpointOwner(ctx planning.Context, workload string) endpoint.Owner {
	return endpoint.Owner{Organization: ctx.OrganizationKey, Application: ctx.App.Key, Environment: ctx.Env.Key, Generation: ctx.Env.TargetGeneration, Workload: workload}
}
func (s *Service) retainEndpoints(ctx context.Context, owner endpoint.Owner, keep []string) error {
	repo, ok := s.store.(persistence.EndpointRepository)
	if !ok {
		return nil
	} // no endpoint could have been admitted without it
	return repo.RetainEndpoints(ctx, owner, keep)
}

// Endpoint manifests join the normal workload inventory and CD publication.
// Resolve the actual rendered Service, not an assumed template-specific name.
func endpointManifest(desired endpoint.Desired, owner endpoint.Owner, namespace, deploymentID string, manifests []execution.Manifest) (execution.Manifest, error) {
	var service string
	var port int
	var serviceLabels map[string]any
	for _, m := range manifests {
		if m.Kind == "HTTPRoute" || m.Kind == "Ingress" {
			return execution.Manifest{}, fmt.Errorf("endpoint: template must not also generate a public route")
		}
		if m.Kind != "Service" {
			continue
		}
		// JSON normalization accepts both template-decoded and native typed values.
		raw, err := json.Marshal(m.Object)
		if err != nil {
			return execution.Manifest{}, err
		}
		var object struct {
			Metadata struct{ Name, Namespace string }
			Spec     struct {
				Ports []struct {
					Name     string
					Port     int
					Protocol string
				}
				Type         string
				ExternalName string
			}
		}
		if err := json.Unmarshal(raw, &object); err != nil {
			return execution.Manifest{}, err
		}
		if object.Metadata.Namespace != "" && object.Metadata.Namespace != namespace {
			continue
		}
		if object.Spec.Type == "ExternalName" || object.Spec.ExternalName != "" {
			return execution.Manifest{}, fmt.Errorf("endpoint: external Services are not supported")
		}
		for _, p := range object.Spec.Ports {
			if p.Name != desired.Port {
				continue
			}
			if p.Protocol != "" && p.Protocol != "TCP" {
				return execution.Manifest{}, fmt.Errorf("endpoint: HTTP endpoints require TCP")
			}
			if service != "" {
				return execution.Manifest{}, fmt.Errorf("endpoint: named port matches multiple rendered Services")
			}
			service = object.Metadata.Name
			port = p.Port
			metadata, _ := m.Object["metadata"].(map[string]any)
			serviceLabels, _ = metadata["labels"].(map[string]any)
		}
	}
	if service == "" || port < 1 || port > 65535 {
		return execution.Manifest{}, fmt.Errorf("endpoint: named port is absent from rendered Service")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s/%d/%s", owner.Organization, owner.Application, owner.Environment, owner.Generation, owner.Workload)))
	name := "endpoint-" + hex.EncodeToString(sum[:])[:20]
	parent := map[string]any{"group": "gateway.networking.k8s.io", "kind": "Gateway", "name": desired.Policy.GatewayName, "namespace": desired.Policy.GatewayNamespace}
	if desired.Policy.GatewaySectionName != "" {
		parent["sectionName"] = desired.Policy.GatewaySectionName
	}
	object := map[string]any{"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute", "metadata": map[string]any{"name": name, "namespace": namespace, "labels": endpointLabels(serviceLabels, owner, deploymentID)}, "spec": map[string]any{
		"parentRefs": []any{parent}, "hostnames": []any{desired.Hostname}, "rules": []any{map[string]any{
			"matches":     []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": desired.Path}}},
			"backendRefs": []any{map[string]any{"group": "", "kind": "Service", "name": service, "port": port}},
		}},
	}}
	return execution.Manifest{APIVersion: "gateway.networking.k8s.io/v1", Kind: "HTTPRoute", Name: name, Namespace: namespace, Object: object}, nil
}

// endpointLabels copies the rendered Service's ownership identity so the route
// joins the same workload inventory (ADR-024): it is pruned with the endpoint
// and removed with the workload, and foreign objects are never overwritten.
func endpointLabels(service map[string]any, owner endpoint.Owner, deploymentID string) map[string]any {
	labels := map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": owner.Application, "orchestrator.io/environment": owner.Environment, "orchestrator.io/workload": owner.Workload}
	for _, key := range []string{"orchestrator.io/application", "orchestrator.io/environment", "orchestrator.io/workload"} {
		if value, ok := service[key].(string); ok && value != "" {
			labels[key] = value
		}
	}
	labels["orchestrator.io/deployment-id"] = deploymentID
	return labels
}
