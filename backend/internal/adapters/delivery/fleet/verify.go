package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

var ErrVerification = errors.New("fleet: controller or registered target could not be verified")

// ManagementTargetResolver maps persisted account IDs to a scoped credential
// target. Implementations must check Organization/account identity; Fleet never
// resolves host kubeconfig or substitutes the workload cluster as its manager.
type ManagementTargetResolver interface {
	ResolveManagementTarget(context.Context, port.BindingVersion) (execution.Target, error)
	ResolveWorkloadTarget(context.Context, port.BindingVersion, string) (execution.Target, error)
}
type Verifier struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
	Targets     ManagementTargetResolver
	Now         func() time.Time
}

func (v *Verifier) Verify(ctx context.Context, b port.BindingVersion) (port.Verification, error) {
	fail := port.Verification{ReasonCode: "VERIFICATION_FAILED"}
	if v.Targets == nil || v.Credentials == nil {
		return fail, ErrVerification
	}
	cfg, err := ParseConfig(b.Config)
	if err != nil {
		return fail, err
	}
	target, err := v.Targets.ResolveManagementTarget(ctx, b)
	if err != nil || !target.CredentialBacked() {
		return fail, ErrVerification
	}
	cli, close, err := k8s.OpenCLI(ctx, v.KubectlPath, v.Credentials, target)
	if err != nil {
		return fail, ErrVerification
	}
	defer close()
	deployment, exists, err := cli.Get(ctx, cfg.ControllerNamespace, cfg.ControllerKind, cfg.ControllerName)
	if err != nil || !exists {
		return port.Verification{ReasonCode: "CONTROLLER_UNAVAILABLE"}, ErrVerification
	}
	raw, _ := json.Marshal(deployment)
	var controller fleetWorkload
	if json.Unmarshal(raw, &controller) != nil {
		return fail, ErrVerification
	}
	version, supported := workloadVersion(controller, "fleet-controller")
	if !supported || (cfg.ControllerVersion != "" && !sameFleetMinor(version, cfg.ControllerVersion)) {
		return port.Verification{ReasonCode: "VERSION_UNSUPPORTED"}, ErrVerification
	}
	if !workloadHealthy(controller) {
		return port.Verification{ReasonCode: "CONTROLLER_UNAVAILABLE"}, ErrVerification
	}
	if err := checkFleetSchemas(ctx, cli); err != nil {
		return port.Verification{ReasonCode: "VERSION_UNSUPPORTED"}, err
	}
	cluster, exists, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !exists {
		return fail, ErrVerification
	}
	metadata, _ := cluster["metadata"].(map[string]any)
	if metadata["uid"] != cfg.TargetClusterUID {
		return fail, ErrVerification
	}
	// Explicit target credentials independently prove this registered cluster is
	// the same physical destination, rather than merely sharing a context/name.
	workload, err := v.Targets.ResolveWorkloadTarget(ctx, b, cfg.TargetAccountKey)
	if err != nil || !workload.CredentialBacked() {
		return fail, ErrVerification
	}
	workloadCLI, workloadClose, err := k8s.OpenCLI(ctx, v.KubectlPath, v.Credentials, workload)
	if err != nil {
		return fail, ErrVerification
	}
	defer workloadClose()
	// Default downstream client IDs identify the kube-system namespace UID.
	// Bootstrap local additionally proves equality with the manager cluster.
	local, exists, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !exists {
		return fail, ErrVerification
	}
	actual, exists, err := workloadCLI.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !exists {
		return fail, ErrVerification
	}
	left, _ := local["metadata"].(map[string]any)
	right, _ := actual["metadata"].(map[string]any)
	managerUID, _ := left["uid"].(string)
	targetUID, _ := right["uid"].(string)
	if cfg.TargetKubeSystemUID != "" && cfg.TargetKubeSystemUID != targetUID {
		return port.Verification{ReasonCode: "TARGET_MAPPING_UNSUPPORTED"}, ErrOwnership
	}
	raw, _ = json.Marshal(cluster)
	var registration fleetRegistration
	if json.Unmarshal(raw, &registration) != nil {
		return fail, ErrVerification
	}
	legacyProof := registrationMatches(registration.Metadata.Name, registration.Spec.ClientID, registration.Spec.KubeConfigSecret, registration.Metadata.Annotations["objectset.rio.cattle.io/id"], managerUID, targetUID)
	if !legacyProof && cfg.TargetKubeSystemUID == "" {
		return port.Verification{ReasonCode: "TARGET_MAPPING_UNSUPPORTED"}, ErrOwnership
	}
	candidates, err := v.workloadCandidates(ctx, b, target)
	if err != nil {
		return fail, err
	}
	matched, ok := matchingTarget(registration, managerUID, version, candidates)
	if !ok || matched.uid != targetUID || matched.agentNamespace != cfg.AgentNamespace || (cfg.AgentName != "" && matched.agentName != cfg.AgentName) || (cfg.AgentKind != "" && matched.agentKind != cfg.AgentKind) {
		return port.Verification{ReasonCode: "TARGET_MAPPING_UNSUPPORTED"}, domain.ErrUnsupported
	}
	if cfg.GitRepoMode == "MANAGED" {
		for _, verb := range []string{"get", "create", "update", "delete"} {
			if _, err := cli.Run(ctx, nil, "auth", "can-i", verb, "gitrepos.fleet.cattle.io", "-n", cfg.Workspace); err != nil {
				return port.Verification{ReasonCode: "PERMISSION_DENIED"}, ErrVerification
			}
		}
	}
	if cfg.ManagedReadSecret {
		if cfg.ReadSecret != managedReaderName(b) {
			return fail, ErrConfig
		}
		for _, verb := range []string{"get", "create", "update"} {
			if _, err := cli.Run(ctx, nil, "auth", "can-i", verb, "secrets", "-n", cfg.Workspace); err != nil {
				return port.Verification{ReasonCode: "PERMISSION_DENIED"}, ErrVerification
			}
		}
	} else if cfg.ReadSecret != "" {
		uid, err := cli.Run(ctx, nil, "get", "secret", cfg.ReadSecret, "-n", cfg.Workspace, "-o", "jsonpath={.metadata.uid}")
		if err != nil || len(uid) == 0 {
			return port.Verification{ReasonCode: "READER_SECRET_MISSING"}, ErrVerification
		}
	}
	if _, found, err := workloadCLI.Get(ctx, "", "namespace", cfg.ReleaseNamespace); err != nil || !found {
		return port.Verification{ReasonCode: "RELEASE_NAMESPACE_MISSING"}, ErrVerification
	}
	if !cfg.NativeServiceAccount {
		if _, found, err := workloadCLI.Get(ctx, cfg.AgentNamespace, "serviceaccount", cfg.ServiceAccount); err != nil || !found {
			return port.Verification{ReasonCode: "SERVICE_ACCOUNT_MISSING"}, ErrVerification
		}
	}
	if cfg.GitRepoMode == "EXISTING" {
		if _, err := checkExisting(ctx, cli, cfg); err != nil {
			return fail, err
		}
	}
	// Verification alone cannot grant unproven lifecycle capabilities. Describe
	// advertises semantic support only after the adapter's conformance gates.
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now()
	}
	return port.Verification{Ready: true, Capabilities: domain.Capabilities{ContractVersion: b.ContractVersion}, CheckedAt: now}, nil
}
