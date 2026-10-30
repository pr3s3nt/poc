package fleet

import (
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"strings"
	"testing"
)

func packageIntent() port.Intent {
	return port.Intent{ID: "intent", OrganizationKey: "org", Owner: port.Owner{OperationID: "op", Instance: "worker", Fence: 1}, Binding: port.BindingVersion{ID: "binding", Version: 1, OrganizationID: "org", Config: validConfig(), Capabilities: domain.Capabilities{ContractVersion: "v1", ExactRevision: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true, SelfHeal: true, DriftDetection: true}}, Revision: domain.Revision{ID: "revision", Scope: domain.Scope{OrganizationID: "org", EnvironmentID: "environment", UnitKey: "app/rollout"}, Phase: domain.Rollout, PayloadDigest: strings.Repeat("a", 64), ArtifactRef: "private", Selection: domain.Selection{Mode: domain.CD, BindingID: "binding", BindingVersion: 1}}, Target: execution.Target{Organization: "org", Connection: "target", Namespace: "app"}}
}
func configMap() execution.Manifest {
	return execution.Manifest{APIVersion: "v1", Kind: "ConfigMap", Name: "app-config", Namespace: "app", Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "app-config", "namespace": "app", "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator"}}, "data": map[string]any{"template": "{{ untouched }}"}}}
}
func TestPackagePreservesPayloadAndRetainsObjectsOnWithdrawal(t *testing.T) {
	i := packageIntent()
	m := configMap()
	before, _ := json.Marshal(m.Object)
	a, err := PackageUnit(i, []execution.Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	b, err := PackageUnit(i, []execution.Manifest{m})
	if err != nil || a.Publication.TreeDigest != b.Publication.TreeDigest || a.ReleaseName != b.ReleaseName {
		t.Fatal("nondeterministic package", err)
	}
	after, _ := json.Marshal(m.Object)
	if string(before) != string(after) {
		t.Fatal("modified renderer artifact")
	}
	for file, raw := range a.Publication.Files {
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			t.Fatal("invalid generated JSON")
		}
		if file == "fleet.yaml" {
			helm := obj["helm"].(map[string]any)
			if helm["force"] != false || helm["takeOwnership"] != false || obj["deleteNamespace"] != false {
				t.Fatal("unsafe Fleet settings")
			}
			continue
		}
		metadata := obj["metadata"].(map[string]any)
		annotations := metadata["annotations"].(map[string]any)
		if annotations["helm.sh/resource-policy"] != "keep" {
			t.Fatal("native unsafe prune enabled")
		}
		if obj["data"].(map[string]any)["template"] != "{{ untouched }}" {
			t.Fatal("literal payload changed")
		}
	}
}
func TestPackageRefusesSecretAndLifecyclePolicyConflicts(t *testing.T) {
	i := packageIntent()
	m := configMap()
	m.Secret = true
	if _, err := PackageUnit(i, []execution.Manifest{m}); err == nil {
		t.Fatal("Secret entered Git")
	}
	i.Revision.Phase = domain.Job
	if _, err := PackageUnit(i, []execution.Manifest{configMap()}); err == nil {
		t.Fatal("Job phase admitted continuous objects")
	}
	i = packageIntent()
	i.Revision.Selection.SelfHeal = true
	m = configMap()
	m.Kind = "PersistentVolumeClaim"
	m.Object["kind"] = m.Kind
	if _, err := PackageUnit(i, []execution.Manifest{m}); err == nil {
		t.Fatal("PVC authorized for autonomous recreation")
	}
}

func TestHPARequiresUnambiguousReplicaOwnership(t *testing.T) {
	target := execution.Manifest{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", Namespace: "app", Object: map[string]any{"spec": map[string]any{}}}
	hpa := execution.Manifest{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "web", Namespace: "app", Object: map[string]any{"spec": map[string]any{"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "web"}}}}
	if err := validateReplicaOwnership([]execution.Manifest{hpa, target}); err != nil {
		t.Fatal(err)
	}
	target.Object["spec"].(map[string]any)["replicas"] = 1
	if err := validateReplicaOwnership([]execution.Manifest{hpa, target}); err != domain.ErrUnsupported {
		t.Fatal("configured replicas competed with HPA", err)
	}
	if err := validateReplicaOwnership([]execution.Manifest{hpa}); err != domain.ErrUnsupported {
		t.Fatal("unproven external scale target admitted", err)
	}
}
