package fleet

import (
	"context"
	"encoding/json"
	"testing"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
)

func TestCDPackageOwnsVSOAndNonsecretAuxiliaries(t *testing.T) {
	i := packageIntent()
	i.Revision.Phase = domain.Prerequisite
	i.Revision.Selection.SelfHeal = true
	i.Target.Extra = map[string]string{"application": "shop", "environment": "staging", "workload": "web"}
	// A deliberately invalid kubectl path proves rendering does not touch a cluster.
	renderer := &k8s.VSOSynchronizer{KubectlPath: "/must-not-run"}
	bundle := execution.ConfigBundle{StoreKey: "vault", Address: "https://vault.internal", Mount: "kv", AuthMount: "kubernetes", Path: "orchestrator/apps/shop/envs/staging/values/revision", Role: "web", ServiceAccount: "web-vault", SecretName: "orch-revision", Keys: map[string]map[string]string{"main": {"PASSWORD": "main_PASSWORD"}}}
	manifests, err := renderer.RenderBundle(context.Background(), i.Target, bundle)
	if err != nil {
		t.Fatal(err)
	}
	auxiliary := configMap()
	auxiliary.Auxiliary = true
	manifests = append(manifests, auxiliary)
	pkg, err := PackageUnit(i, manifests)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Objects) != 5 {
		t.Fatalf("CD silently dropped auxiliaries or secret declarations: %v", pkg.Objects)
	}
	for _, m := range manifests {
		if m.Secret || m.Kind == "Secret" {
			t.Fatal("value-bearing Secret reached CD artifact")
		}
	}
	raw, _ := json.Marshal(pkg)
	exported, err := (&Provider{}).ExportArtifact(context.Background(), raw)
	if err != nil || len(exported) != len(manifests) {
		t.Fatalf("policy changes/handoff cannot preserve complete CD artifacts: %v", err)
	}
	// Secret material cannot bypass admission by omitting the Secret flag.
	secret := configMap()
	secret.Kind = "Secret"
	secret.Object["kind"] = "Secret"
	secret.Object["stringData"] = map[string]any{"password": "private-sentinel"}
	if _, err := PackageUnit(i, []execution.Manifest{secret}); err == nil {
		t.Fatal("private Secret accepted for Git publication")
	}
	// Different revisions must not claim a shared VaultConnection release object.
	bundle.SecretName = "orch-next-revision"
	next, err := renderer.RenderBundle(context.Background(), i.Target, bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range manifests {
		for _, b := range next {
			if a.Kind == "VaultConnection" && b.Kind == a.Kind && b.Name == a.Name {
				t.Fatal("independent bundles share controller-owned VaultConnection")
			}
		}
	}
}

func TestPodMonitorUsesOrdinaryContinuousOwnedDelivery(t *testing.T) {
	i := packageIntent()
	i.Revision.Phase = domain.Prerequisite
	i.Revision.Selection.SelfHeal = true
	m := execution.Manifest{APIVersion: "monitoring.coreos.com/v1", Kind: "PodMonitor", Name: "owned-monitor", Namespace: "app", Object: map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "PodMonitor", "metadata": map[string]any{"name": "owned-monitor", "namespace": "app", "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator"}}, "spec": map[string]any{"selector": map[string]any{"matchLabels": map[string]string{"app": "owned"}}}}}
	p, e := PackageUnit(i, []execution.Manifest{m})
	if e != nil || len(p.Objects) != 1 {
		t.Fatal("PodMonitor rejected by CD", e)
	}
	m.APIVersion = "foreign/v1"
	m.Object["apiVersion"] = "foreign/v1"
	if _, e = PackageUnit(i, []execution.Manifest{m}); e == nil {
		t.Fatal("wrong monitoring API admitted")
	}
}
