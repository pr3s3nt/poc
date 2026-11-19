package provisioning

import (
	"context"
	"fmt"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/persistence"
	"strings"
)

func (s *Service) provisionEndpoint(ctx context.Context, req Request, node planning.Node, def resource.Definition, params map[string]any) (execution.ProvisionResult, error) {
	out := execution.ProvisionResult{}
	if node.Scope.Type != resource.ScopeWorkload {
		return out, fmt.Errorf("endpoint: shared endpoint resources are not supported")
	}
	parsed, err := resource.ParseDescriptor(node.Descriptor)
	if err != nil {
		return out, err
	}
	parts := strings.Split(parsed.ID, ".")
	if len(parts) < 4 || parts[0] != "modules" || parts[2] != "externals" {
		return out, fmt.Errorf("endpoint: endpoint must belong to one workload")
	}
	desired, planned := req.Plan.Endpoints[parts[1]]
	if !planned {
		return out, fmt.Errorf("endpoint: resolved policy is absent from the pinned plan")
	}
	owner := endpoint.Owner{Organization: req.Context.OrganizationKey, Application: req.Context.App.Key, Environment: req.Context.Env.Key, Generation: req.Context.Env.TargetGeneration, Workload: parts[1]}
	repo, ok := s.store.(persistence.EndpointRepository)
	if !ok {
		return out, fmt.Errorf("endpoint: reservation repository unavailable")
	}
	if err := repo.ReserveEndpoint(ctx, desired.Claim(), owner); err != nil {
		return out, err
	}
	out.Outputs = map[string]any{"hostname": desired.Hostname, "url": desired.URL}
	out.State = map[string]any{"hostname": desired.Hostname}
	return out, nil
}
