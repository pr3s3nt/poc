package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	if err != nil || cluster.Target.Organization != req.OrganizationKey || cluster.Target.Connection != req.Target.Connection || cluster.Target.ClusterName != req.Target.ClusterName || cluster.Target.Provider != req.Target.Provider || cluster.Target.Region != req.Target.Region {
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

type gatewayCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	ObservedGeneration int64  `json:"observedGeneration"`
}

type gatewayObservation struct {
	Metadata struct {
		UID        string `json:"uid"`
		Generation int64  `json:"generation"`
	} `json:"metadata"`
	Spec struct {
		Listeners []gatewayListener `json:"listeners"`
	} `json:"spec"`
	Status struct {
		Conditions []gatewayCondition `json:"conditions"`
		Listeners  []struct {
			Name           string             `json:"name"`
			Conditions     []gatewayCondition `json:"conditions"`
			SupportedKinds []struct {
				Group string `json:"group"`
				Kind  string `json:"kind"`
			} `json:"supportedKinds"`
		} `json:"listeners"`
	} `json:"status"`
}

func currentGatewayCondition(conditions []gatewayCondition, name string, generation int64) bool {
	for _, condition := range conditions {
		if condition.Type == name && condition.Status == "True" && (condition.ObservedGeneration == 0 || condition.ObservedGeneration == generation) {
			return true
		}
	}
	return false
}

func (g gatewayObservation) ready(policy endpoint.Policy) bool {
	if !currentGatewayCondition(g.Status.Conditions, "Accepted", g.Metadata.Generation) || !currentGatewayCondition(g.Status.Conditions, "Programmed", g.Metadata.Generation) {
		return false
	}
	matches := 0
	for _, listener := range g.Spec.Listeners {
		if policy.GatewaySectionName != "" && policy.GatewaySectionName != listener.Name {
			continue
		}
		if policy.Scheme == "http" && listener.Protocol != "HTTP" || policy.Scheme == "https" && listener.Protocol != "HTTPS" {
			continue
		}
		for _, status := range g.Status.Listeners {
			if status.Name != listener.Name || !currentGatewayCondition(status.Conditions, "Accepted", g.Metadata.Generation) || !currentGatewayCondition(status.Conditions, "ResolvedRefs", g.Metadata.Generation) {
				continue
			}
			for _, kind := range status.SupportedKinds {
				if kind.Kind == "HTTPRoute" && kind.Group == "gateway.networking.k8s.io" {
					matches++
				}
			}
		}
	}
	return matches == 1
}

type gatewayListener struct {
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	Hostname      string `json:"hostname"`
	AllowedRoutes struct {
		Namespaces struct {
			From     string `json:"from"`
			Selector struct {
				MatchLabels      map[string]string `json:"matchLabels"`
				MatchExpressions []struct {
					Key      string   `json:"key"`
					Operator string   `json:"operator"`
					Values   []string `json:"values"`
				} `json:"matchExpressions"`
			} `json:"selector"`
		} `json:"namespaces"`
		Kinds []struct {
			Group string `json:"group"`
			Kind  string `json:"kind"`
		} `json:"kinds"`
	} `json:"allowedRoutes"`
}

func (a *ExistingGatewayAdapter) CheckGateway(ctx context.Context, target execution.Target, desired endpoint.Desired) error {
	refuse := fmt.Errorf("endpoint: existing Gateway does not admit the workload hostname, listener or namespace")
	if target.Namespace == "" {
		return refuse
	}
	cli, cleanup, err := OpenCLI(ctx, a.KubectlPath, a.Credentials, target)
	if err != nil {
		return refuse
	}
	defer cleanup()
	raw, err := cli.Run(ctx, nil, "get", "gateways.gateway.networking.k8s.io", desired.Policy.GatewayName, "-n", desired.Policy.GatewayNamespace, "-o", "json")
	var gateway gatewayObservation
	if err != nil || json.Unmarshal(raw, &gateway) != nil || !gateway.ready(desired.Policy) {
		return refuse
	}
	allowed := 0
	for _, listener := range gateway.Spec.Listeners {
		if desired.Policy.GatewaySectionName != "" && listener.Name != desired.Policy.GatewaySectionName {
			continue
		}
		if desired.Policy.Scheme == "http" && listener.Protocol != "HTTP" || desired.Policy.Scheme == "https" && listener.Protocol != "HTTPS" {
			continue
		}
		if listener.Hostname != "" && listener.Hostname != desired.Hostname && !(strings.HasPrefix(listener.Hostname, "*.") && strings.HasSuffix(desired.Hostname, listener.Hostname[1:])) {
			continue
		}
		if len(listener.AllowedRoutes.Kinds) > 0 {
			found := false
			for _, kind := range listener.AllowedRoutes.Kinds {
				if kind.Kind == "HTTPRoute" && (kind.Group == "" || kind.Group == "gateway.networking.k8s.io") {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		switch listener.AllowedRoutes.Namespaces.From {
		case "All":
			allowed++
		case "", "Same":
			if target.Namespace == desired.Policy.GatewayNamespace {
				allowed++
			}
		case "Selector":
			raw, err := cli.Run(ctx, nil, "get", "namespace", target.Namespace, "-o", "json")
			if err != nil {
				return refuse
			}
			var namespace struct {
				Metadata struct {
					Labels map[string]string `json:"labels"`
				} `json:"metadata"`
			}
			if json.Unmarshal(raw, &namespace) != nil {
				return refuse
			}
			labels := namespace.Metadata.Labels
			matches := true
			selector := listener.AllowedRoutes.Namespaces.Selector
			for key, value := range selector.MatchLabels {
				actual, found := labels[key]
				if !found || actual != value {
					matches = false
				}
			}
			for _, expression := range selector.MatchExpressions {
				actual, exists := labels[expression.Key]
				in := false
				for _, value := range expression.Values {
					if actual == value {
						in = true
					}
				}
				switch expression.Operator {
				case "In":
					if !exists || !in {
						matches = false
					}
				case "NotIn":
					if exists && in {
						matches = false
					}
				case "Exists":
					if !exists {
						matches = false
					}
				case "DoesNotExist":
					if exists {
						matches = false
					}
				default:
					matches = false
				}
			}
			if matches {
				allowed++
			}
		}
	}
	if allowed != 1 {
		return refuse
	}
	return nil
}
