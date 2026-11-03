package fleet

import (
	"context"
	"errors"
	"strings"
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
	deployment, exists, err := cli.Get(ctx, cfg.ControllerNamespace, "deployment", cfg.ControllerName)
	if err != nil || !exists {
		return port.Verification{ReasonCode: "CONTROLLER_UNAVAILABLE"}, ErrVerification
	}
	spec, _ := deployment["spec"].(map[string]any)
	template, _ := spec["template"].(map[string]any)
	pod, _ := template["spec"].(map[string]any)
	containers, _ := pod["containers"].([]any)
	pinned := false
	for _, raw := range containers {
		c, _ := raw.(map[string]any)
		image, _ := c["image"].(string)
		if strings.HasSuffix(image, "/fleet:"+Version) || image == "rancher/fleet:"+Version {
			pinned = true
		}
	}
	if !pinned {
		return port.Verification{ReasonCode: "VERSION_UNSUPPORTED"}, ErrVerification
	}
	status, _ := deployment["status"].(map[string]any)
	ready, _ := status["availableReplicas"].(float64)
	if ready < 1 {
		return port.Verification{ReasonCode: "CONTROLLER_UNAVAILABLE"}, ErrVerification
	}
	cluster, exists, err := cli.Get(ctx, cfg.Workspace, "clusters.fleet.cattle.io", cfg.TargetCluster)
	if err != nil || !exists {
		return fail, ErrVerification
	}
	metadata, _ := cluster["metadata"].(map[string]any)
	if metadata["uid"] != cfg.TargetClusterUID {
		return fail, ErrVerification
	}
	registrationSpec, _ := cluster["spec"].(map[string]any)
	annotations, _ := metadata["annotations"].(map[string]any)
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
	clientID, _ := registrationSpec["clientID"].(string)
	secret, _ := registrationSpec["kubeConfigSecret"].(string)
	bootstrap, _ := annotations["objectset.rio.cattle.io/id"].(string)
	if !registrationMatches(cfg.TargetCluster, clientID, secret, bootstrap, managerUID, targetUID) {
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
	if _, found, err := workloadCLI.Get(ctx, cfg.AgentNamespace, "serviceaccount", cfg.ServiceAccount); err != nil || !found {
		return port.Verification{ReasonCode: "SERVICE_ACCOUNT_MISSING"}, ErrVerification
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
