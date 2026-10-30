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
		return fail, ErrVerification
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
		return fail, ErrVerification
	}
	status, _ := deployment["status"].(map[string]any)
	ready, _ := status["availableReplicas"].(float64)
	if ready < 1 {
		return fail, ErrVerification
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
	if cfg.TargetCluster != "local" || registrationSpec["kubeConfigSecret"] != "local-cluster" || annotations["objectset.rio.cattle.io/id"] != "fleet-bootstrap" {
		return port.Verification{ReasonCode: "TARGET_MAPPING_UNSUPPORTED"}, domain.ErrUnsupported
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
	// Fleet's local registration must map to the manager cluster. Separate-manager
	// registrations require a verified cluster-identity mapping before support.
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
	if left["uid"] == nil || left["uid"] != right["uid"] {
		return port.Verification{ReasonCode: "TARGET_MAPPING_UNSUPPORTED"}, domain.ErrUnsupported
	}
	if cfg.ReadSecret != "" {
		uid, err := cli.Run(ctx, nil, "get", "secret", cfg.ReadSecret, "-n", cfg.Workspace, "-o", "jsonpath={.metadata.uid}")
		if err != nil || len(uid) == 0 {
			return fail, ErrVerification
		}
	}
	if _, found, err := workloadCLI.Get(ctx, "", "namespace", cfg.ReleaseNamespace); err != nil || !found {
		return fail, ErrVerification
	}
	if _, found, err := workloadCLI.Get(ctx, cfg.AgentNamespace, "serviceaccount", cfg.ServiceAccount); err != nil || !found {
		return fail, ErrVerification
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
