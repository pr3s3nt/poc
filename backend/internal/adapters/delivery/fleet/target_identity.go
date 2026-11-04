package fleet

import (
	"context"
	"sort"

	"gopkg.in/yaml.v3"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

type workloadCandidate struct {
	target                                                            execution.Target
	uid, agentNamespace, agentName, agentKind, agentVersion, clientID string
	serviceAccount, releaseNamespace                                  string
}

type fleetRegistration struct {
	Metadata struct {
		Name, Namespace, UID string
		Annotations, Labels  map[string]string
	}
	Spec struct{ ClientID, KubeConfigSecret, AgentNamespace string }
}

// Candidates use registered, scoped credentials and nonsecret agent ConfigMaps.
// Fleet credential Secrets and host kubeconfigs never supply target identity.
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
			candidates, err := readTargetCandidates(ctx, cli, target)
			if err == nil {
				out = append(out, candidates...)
			}
		}()
	}
	return out, nil
}

func readTargetCandidates(ctx context.Context, cli k8s.CLI, target execution.Target) ([]workloadCandidate, error) {
	namespace, found, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !found {
		return nil, ErrVerification
	}
	metadata, _ := namespace["metadata"].(map[string]any)
	uid, _ := metadata["uid"].(string)
	if uid == "" {
		return nil, ErrVerification
	}
	workloads, err := readFleetWorkloads(ctx, cli)
	if err != nil {
		return nil, err
	}
	out := []workloadCandidate{}
	for _, w := range workloads {
		version, identified := workloadVersion(w, "fleet-agent")
		if !identified || !workloadHealthy(w) {
			continue
		}
		c := workloadCandidate{target: target, uid: uid, agentNamespace: w.Metadata.Namespace, agentName: w.Metadata.Name, agentKind: w.Kind, agentVersion: version}
		cm, exists, err := cli.Get(ctx, c.agentNamespace, "configmap", "fleet-agent")
		if err != nil {
			return nil, ErrVerification
		}
		if exists {
			data, _ := cm["data"].(map[string]any)
			raw, _ := data["config"].(string)
			var config struct {
				ClientID string `yaml:"clientID"`
			}
			if len(raw) > 65536 || yaml.Unmarshal([]byte(raw), &config) != nil {
				return nil, ErrVerification
			}
			c.clientID = config.ClientID
		}
		for _, name := range []string{"orchestrator-delivery", "fleet-default"} {
			_, exists, err := cli.Get(ctx, c.agentNamespace, "serviceaccount", name)
			if err != nil {
				return nil, ErrVerification
			}
			if exists {
				c.serviceAccount = name
				break
			}
		}
		c.releaseNamespace = c.agentNamespace
		if _, exists, err := cli.Get(ctx, "", "namespace", "fleet-releases"); err != nil {
			return nil, ErrVerification
		} else if exists {
			c.releaseNamespace = "fleet-releases"
		}
		out = append(out, c)
	}
	return out, nil
}

func matchingTarget(r fleetRegistration, managerUID, controllerVersion string, candidates []workloadCandidate) (workloadCandidate, bool) {
	var selected workloadCandidate
	found := false
	for _, c := range candidates {
		if c.uid == "" {
			continue
		}
		if !sameFleetMinor(controllerVersion, c.agentVersion) || (r.Spec.AgentNamespace != "" && r.Spec.AgentNamespace != c.agentNamespace) {
			continue
		}
		if c.clientID != "" && r.Spec.ClientID != "" && c.clientID != r.Spec.ClientID {
			continue
		}
		direct := registrationMatches(r.Metadata.Name, r.Spec.ClientID, r.Spec.KubeConfigSecret, r.Metadata.Annotations["objectset.rio.cattle.io/id"], managerUID, c.uid)
		explicit := c.clientID != "" && c.clientID == r.Spec.ClientID
		if !direct && !explicit {
			continue
		}
		if found && (selected.uid != c.uid || selected.agentNamespace != c.agentNamespace || selected.agentKind != c.agentKind || selected.agentName != c.agentName) {
			return workloadCandidate{}, false
		}
		if !found {
			selected = c
			found = true
		}
	}
	return selected, found
}

func registrationMatches(name, clientID, kubeconfigSecret, bootstrapID, managerUID, targetUID string) bool {
	if targetUID == "" {
		return false
	}
	if clientID != "" && clientID == targetUID {
		return true
	}
	return name == "local" && kubeconfigSecret == "local-cluster" && bootstrapID == "fleet-bootstrap" && managerUID != "" && managerUID == targetUID
}
