package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Kubectl runs one read or delete command against the k8s-4f cluster with the
// operator's host kubeconfig handle (supervisor safety net and verification
// only, never the product).
type Kubectl interface {
	Run(ctx context.Context, args ...string) (stdout []byte, stderr string, exit int)
}

// ExecKubectl is the real kubectl runner.
type ExecKubectl struct {
	Binary, Kubeconfig, Context string
}

// Run implements Kubectl.
func (k ExecKubectl) Run(ctx context.Context, args ...string) ([]byte, string, int) {
	full := append([]string{"--kubeconfig", k.Kubeconfig, "--context", k.Context}, args...)
	cmd := exec.CommandContext(ctx, k.Binary, full...)
	cmd.Env = baseEnv()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, stderr.String(), exitCode(err)
}

// Guard exit codes of GuardNamespace besides 0 and ExitNotOwned: 1 = unknown.

type namespaceDoc struct {
	Metadata struct {
		Name        string            `json:"name"`
		UID         string            `json:"uid"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
}

type listedObject struct {
	Kind             string            `json:"kind"`
	Data             map[string]string `json:"data"`
	Secrets          []any             `json:"secrets"`
	ImagePullSecrets []any             `json:"imagePullSecrets"`
	Automount        *bool             `json:"automountServiceAccountToken"`
	Metadata         struct {
		Name        string            `json:"name"`
		UID         string            `json:"uid"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		Owners      []any             `json:"ownerReferences"`
		Managed     []struct {
			Manager string `json:"manager"`
		} `json:"managedFields"`
	} `json:"metadata"`
}

type listedObjects struct {
	Items []listedObject `json:"items"`
}

var physicalInstancePattern = regexp.MustCompile(`^[0-9a-f]{8,64}$`)

func generatedDeliveryObject(o listedObject, runID string) bool {
	labels := o.Metadata.Labels
	if labels["orchestrator-managed"] == "true" && labels["orchestrator-created-by-run"] == runID &&
		physicalInstancePattern.MatchString(labels["orchestrator-instance"]) && labels["app.kubernetes.io/managed-by"] == "orchestrator" {
		instance := labels["app.kubernetes.io/instance"]
		name := labels["app.kubernetes.io/name"]
		for _, app := range []string{"postgres", "redis"} {
			key := strings.TrimPrefix(instance, app+"-")
			if name != app || !physicalInstancePattern.MatchString(key) || labels["orchestrator-instance"] != key {
				continue
			}
			if o.Kind == "Service" && o.Metadata.Name == instance {
				return true
			}
			if o.Kind == "Secret" && o.Metadata.Name == instance+"-auth" {
				return true
			}
		}
	}
	return false
}

// pristineNamespaceDefault recognizes only the two control-plane defaults
// that the Terraform namespace layer later checks by full UID inventory.
func pristineNamespaceDefault(o listedObject) bool {
	if o.Metadata.UID == "" || len(o.Metadata.Labels) != 0 || len(o.Metadata.Owners) != 0 {
		return false
	}
	managerOK := len(o.Metadata.Managed) == 0 || len(o.Metadata.Managed) == 1 && o.Metadata.Managed[0].Manager == "kube-controller-manager"
	if !managerOK {
		return false
	}
	switch {
	case o.Kind == "ServiceAccount" && o.Metadata.Name == "default":
		if len(o.Metadata.Annotations) != 0 || len(o.Secrets) != 0 || len(o.ImagePullSecrets) != 0 || o.Automount != nil {
			return false
		}
		return true
	case o.Kind == "ConfigMap" && o.Metadata.Name == "kube-root-ca.crt":
		if len(o.Data) != 1 || o.Data["ca.crt"] == "" {
			return false
		}
		for key := range o.Metadata.Annotations {
			if key != "kubernetes.io/description" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// GuardNamespace proves the run namespace is run-owned before any effect:
// exact recorded name, the orch-v2-test-<run> prefix, and the run-id and
// managed-by tags the product stamped on it. A missing namespace is trivially
// clean (0). Foreign tags or no tags mean NOT owned (ExitNotOwned): it is
// never deleted. Any API uncertainty is unknown (1).
func GuardNamespace(ctx context.Context, k Kubectl, m Manifest) int {
	if m.Namespace == "" || m.Target != TargetK8s4f || !strings.HasPrefix(m.Namespace, NamespacePrefix+m.RunID) {
		return 1
	}
	out, stderr, exit := k.Run(ctx, "get", "namespace", m.Namespace, "-o", "json")
	if exit != 0 {
		if notFound.MatchString(stderr) {
			return 0
		}
		return 1
	}
	var ns namespaceDoc
	if json.Unmarshal(out, &ns) != nil || ns.Metadata.Name != m.Namespace {
		return 1
	}
	tag := func(key string) string {
		if v := ns.Metadata.Labels[key]; v != "" {
			return v
		}
		return ns.Metadata.Annotations[key]
	}
	if tag("run-id") == m.RunID && tag("managed-by") == ProductManagedByTag {
		return 0
	}
	return ExitNotOwned
}

// GuardNamespaceForRun also recognizes a pre-deployment run whose Environment
// was never bound. In that case the manifest has no physical namespace or
// Terraform run IDs; the configured deterministic namespace and its PV claims
// must both be verified absent before the supervisor may skip effects.
func GuardNamespaceForRun(ctx context.Context, k Kubectl, m Manifest, c Config) int {
	if m.Namespace != "" {
		return GuardNamespace(ctx, k, m)
	}
	if !m.Armed || m.Target != TargetK8s4f || c.Target != TargetK8s4f || c.Kube == nil || c.RunID != m.RunID ||
		c.Kube.Namespace != NamespacePrefix+m.RunID || len(m.TerraformRuns) != 0 || len(m.OwnedByRun()) != 0 {
		return 1
	}
	probe := m
	probe.Namespace = c.Kube.Namespace
	if err := VerifyLayer(ctx, k, probe, LayerNamespace); err != nil {
		return 1
	}
	return ExitNotOwned
}

// layerResources are the kinds each layer is verified absent for. They are
// only READ: no object is ever deleted by this package. Terraform-owned objects
// (StatefulSets, PVCs, Services, Secrets of the modules, the namespace) are
// destroyed by the original engine (v2cleanup terraform); delivered Workloads
// by the product's own Remove (v2cleanup delivery).
var layerResources = map[string]string{
	// The platform-generated default account/CA ConfigMap are routed to the
	// namespace engine's full UID inventory. Known postgres/redis Service+auth
	// Secret shapes are routed to Terraform state/UID verification. The latter are checked
	// here as routed Terraform inventory and must later disappear in the TF layer.
	LayerDelivery:     "deployments,daemonsets,replicasets,jobs,cronjobs,services,ingresses,secrets,roles,rolebindings,configmaps,serviceaccounts",
	LayerAppResources: "statefulsets,persistentvolumeclaims",
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// VerifyLayer proves a layer absent by reading the cluster, never by trusting
// the delete's exit code.
func VerifyLayer(ctx context.Context, k Kubectl, m Manifest, layer string) error {
	nsOut, stderr, exit := k.Run(ctx, "get", "namespace", m.Namespace, "-o", "name")
	nsGone := false
	switch {
	case exit == 0 && strings.TrimSpace(string(nsOut)) != "":
	case notFound.MatchString(stderr):
		nsGone = true
	default:
		return errors.New("namespace existence could not be determined")
	}
	switch layer {
	case LayerNamespace:
		if !nsGone {
			return errors.New("namespace still present")
		}
		return verifyOwnedPVsGone(ctx, k, m)
	case LayerDelivery, LayerAppResources:
		if nsGone {
			return nil
		}
		format := "name"
		if layer == LayerDelivery {
			format = "json"
		}
		out, stderr, exit := k.Run(ctx, "-n", m.Namespace, "get", layerResources[layer], "-o", format)
		if exit != 0 {
			if notFound.MatchString(stderr) {
				return nil
			}
			return errors.New("remaining objects could not be listed")
		}
		var left []string
		if layer == LayerDelivery {
			var inventory listedObjects
			if err := json.Unmarshal(out, &inventory); err != nil {
				return errors.New("delivery inventory could not be decoded")
			}
			for _, object := range inventory.Items {
				// Only the exact Terraform module Service and auth Secret identity
				// shapes are routed to the later state/UID-verified Terraform layer.
				// A same-name or same-label object alone never proves cleanup.
				if object.Metadata.Name == "" || object.Kind == "" || (!generatedDeliveryObject(object, m.RunID) && !pristineNamespaceDefault(object)) {
					left = append(left, object.Kind+"/"+object.Metadata.Name)
				}
			}
		} else {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					left = append(left, line)
				}
			}
		}
		if len(left) > 0 {
			return fmt.Errorf("%d object(s) remain in the %s layer", len(left), layer)
		}
		if layer == LayerAppResources {
			return verifyOwnedPVsGone(ctx, k, m)
		}
		return nil
	}
	return fmt.Errorf("k8s verification has no %q layer", layer)
}

// verifyOwnedPVsGone proves no PV claimed from the run namespace and no
// recorded run-owned PV remains. A PV outside the run is never considered.
func verifyOwnedPVsGone(ctx context.Context, k Kubectl, m Manifest) error {
	out, stderr, exit := k.Run(ctx, "get", "pv", "-o", "json")
	if exit != 0 {
		return fmt.Errorf("persistent volumes could not be listed: %s", firstLine(stderr))
	}
	claimed, err := PVsClaimedBy(m.Namespace, out)
	if err != nil {
		return err
	}
	owned := map[string]bool{}
	for _, a := range m.OwnedByRun() {
		if a.Kind == "pv" {
			owned[a.ID] = true
		}
	}
	var left []string
	for _, name := range claimed {
		left = append(left, name)
	}
	var all struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	_ = json.Unmarshal(out, &all)
	for _, it := range all.Items {
		if owned[it.Metadata.Name] {
			left = append(left, it.Metadata.Name)
		}
	}
	if len(left) > 0 {
		return fmt.Errorf("%d run-owned persistent volume(s) remain", len(left))
	}
	return nil
}

// VerifySentinel proves the existing outside-run sentinel still has the UID
// observed before the run (the run never created or removed it).
func VerifySentinel(ctx context.Context, k Kubectl, c KubeConfig) error {
	s := c.Sentinel
	args := []string{"get", s.Kind, s.Name, "-o", "jsonpath={.metadata.uid}"}
	if s.Namespace != "" {
		args = append([]string{"-n", s.Namespace}, args...)
	}
	out, _, exit := k.Run(ctx, args...)
	if exit != 0 || strings.TrimSpace(string(out)) != s.UID {
		return errors.New("the preexisting sentinel is missing or its UID changed")
	}
	return nil
}

// K8sCleanupSteps are the supervisor steps of a k8s-4f run: delivery -> app
// resources -> namespace. None of them deletes a Kubernetes object with
// kubectl. cleanup is the absolute path of v2cleanup, handles the private
// cleanup handles; ctl is v2ctl (ownership guard and independent physical
// absence verification through the host kubeconfig handle, read-only).
//
//	delivery       v2cleanup delivery   (product Deployer.Remove of the delivered Workloads)
//	app-resources  v2cleanup terraform  (original engine, RUN_CLEANUP: consumers first, state kept, no unlock/resume)
//	namespace      v2cleanup status     (the namespace instance is DESTROYED in the registry; never a namespace delete)
func K8sCleanupSteps(ctl, cleanup, manifestPath, handlesPath, configPath, vaultSnapshotPath string, c Config) []Step {
	env := map[string]string{"V2_KUBECONFIG": c.Kube.Kubeconfig, "V2_KUBECONTEXT": c.Kube.Context, "V2_KUBECTL": c.Binaries["kubectl"].Path}
	budgets := CleanupStepsFor(c.Cleanup.Duration)
	timeouts := map[string]time.Duration{LayerDelivery: budgets.Delivery, LayerAppResources: budgets.Terraform, LayerNamespace: budgets.Status}
	step := func(layer, action string) Step {
		return Step{
			Name: layer, Layer: layer, Env: env, Timeout: Duration{Duration: timeouts[layer]}, MaxAttempts: 2,
			Guard:  []string{ctl, "k8s-guard", "-manifest", manifestPath, "-config", configPath},
			Argv:   []string{cleanup, action, "-handles", handlesPath, "-manifest", manifestPath},
			Verify: []string{ctl, "k8s-verify", "-manifest", manifestPath, "-layer", layer},
		}
	}
	steps := []Step{step(LayerDelivery, "delivery"), step(LayerAppResources, "terraform"), step(LayerNamespace, "status")}
	if c.SecretStore != nil && !c.SecretStore.RunOwned {
		// The run-scoped objects in the configuration Vault (values below the unique Application key, the policies and
		// roles proven by content). Only after the workload layers: the Pods no longer need them.
		steps = append(steps, Step{
			Name: "vault-config", Layer: LayerAux, Env: env, Timeout: Duration{Duration: budgets.Vault}, MaxAttempts: 3,
			Guard:  []string{ctl, "k8s-guard", "-manifest", manifestPath, "-config", configPath},
			Argv:   []string{ctl, "vault-clean", "-config", configPath, "-snapshot", vaultSnapshotPath},
			Verify: []string{ctl, "vault-clean", "-verify", "-config", configPath, "-snapshot", vaultSnapshotPath},
		})
	}
	return steps
}
