package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/ports/execution"
)

// ExistingGatewayAdapter reads preexisting infrastructure only. It has no
// apply/delete operation and never adds an ownership label to the Gateway.
type ExistingGatewayAdapter struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
}

func NewExistingGatewayAdapter(path string) *ExistingGatewayAdapter {
	return &ExistingGatewayAdapter{KubectlPath: path}
}

func (a *ExistingGatewayAdapter) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	fail := func() (execution.ProvisionResult, error) {
		return execution.ProvisionResult{}, fmt.Errorf("kubernetes: existing Gateway or HTTP listener is unavailable")
	}
	policy, err := endpoint.GatewayPolicy(req.Inputs)
	if err != nil {
		return fail()
	}
	cluster, err := execution.ParseClusterOutput(req.Inputs["cluster"])
	if err != nil || req.Connection.Key != req.Target.Connection || cluster.AccountVersion != req.Connection.Version || cluster.Target.Organization != req.OrganizationKey || cluster.Target.Connection != req.Target.Connection || cluster.Target.ClusterName != req.Target.ClusterName || cluster.Target.Provider != req.Target.Provider || cluster.Target.Region != req.Target.Region {
		return fail()
	}
	cli, cleanup, err := OpenCLI(ctx, a.KubectlPath, a.Credentials, req.Target)
	if err != nil {
		return fail()
	}
	defer cleanup()
	raw, err := cli.Run(ctx, nil, "get", "gateways.gateway.networking.k8s.io", policy.GatewayName, "-n", policy.GatewayNamespace, "-o", "json")
	if err != nil {
		return fail()
	}
	var object gatewayObservation
	if json.Unmarshal(raw, &object) != nil || object.Metadata.UID == "" || !object.ready(policy) {
		return fail()
	}
	return execution.ProvisionResult{Outputs: map[string]any{"gateway": req.Inputs}, State: map[string]any{"driver": "existing-gateway", "uid": object.Metadata.UID}}, nil
}

// CheckGateway preserves the earlier Go adapter surface for legacy callers.
func (a *ExistingGatewayAdapter) CheckGateway(ctx context.Context, target execution.Target, desired endpoint.Desired) error {
	return (&GatewayVerifier{KubectlPath: a.KubectlPath, Credentials: a.Credentials}).CheckGateway(ctx, target, desired)
}
