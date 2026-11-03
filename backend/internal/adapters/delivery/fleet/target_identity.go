package fleet

import (
	"context"
	"encoding/json"
	"sort"

	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

type workloadCandidate struct {
	target              execution.Target
	uid, agentNamespace string
}

// Discovery uses already managed credentials only. No host kubeconfig or Fleet
// Secret is read to invent an otherwise unregistered target connection.
func (v *Verifier) workloadCandidates(ctx context.Context, b port.BindingVersion, manager execution.Target) ([]workloadCandidate, error) {
	targets := []execution.Target{manager}
	if reader, ok := v.Targets.(interface {
		ListWorkloadTargets(context.Context, port.BindingVersion) ([]execution.Target, error)
	}); ok {
		var err error
		targets, err = reader.ListWorkloadTargets(ctx, b)
		if err != nil {
			return nil, err
		}
	}
	if len(targets) > 100 {
		return nil, ErrVerification
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if (targets[i].Connection == manager.Connection) != (targets[j].Connection == manager.Connection) {
			return targets[i].Connection == manager.Connection
		}
		return targets[i].Connection < targets[j].Connection
	})
	out := []workloadCandidate{}
	for _, target := range targets {
		if target.Organization != manager.Organization || !target.CredentialBacked() {
			continue
		}
		func() {
			cli, close, err := k8s.OpenCLI(ctx, v.KubectlPath, v.Credentials, target)
			if err != nil {
				return
			}
			defer close()
			namespace, exists, err := cli.Get(ctx, "", "namespace", "kube-system")
			if err != nil || !exists {
				return
			}
			metadata, _ := namespace["metadata"].(map[string]any)
			uid, _ := metadata["uid"].(string)
			if uid == "" {
				return
			}
			raw, err := cli.Run(ctx, nil, "get", "deployments", "--all-namespaces", "-o", "json")
			if err != nil {
				return
			}
			var deployments struct {
				Items []struct {
					Metadata struct{ Name, Namespace string }
				}
			}
			if json.Unmarshal(raw, &deployments) != nil {
				return
			}
			agents := map[string]bool{}
			for _, d := range deployments.Items {
				if d.Metadata.Name == "fleet-agent" && d.Metadata.Namespace != "" {
					agents[d.Metadata.Namespace] = true
				}
			}
			if len(agents) != 1 {
				return
			}
			for agent := range agents {
				out = append(out, workloadCandidate{target: target, uid: uid, agentNamespace: agent})
			}
		}()
	}
	return out, nil
}

// registrationMatches proves the Fleet target using authenticated physical
// cluster identity, never a display name or an endpoint resemblance.
func registrationMatches(name, clientID, kubeconfigSecret, bootstrapID, managerUID, targetUID string) bool {
	if targetUID == "" {
		return false
	}
	if clientID != "" && clientID == targetUID {
		return true
	}
	return name == "local" && kubeconfigSecret == "local-cluster" && bootstrapID == "fleet-bootstrap" && managerUID != "" && managerUID == targetUID
}
