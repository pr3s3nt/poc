package kubernetes

import (
	"context"
	"reflect"
	"testing"

	"orchestrator/internal/ports/execution"
)

func TestRenderVersionedBundleKeepsFixedIdentityAndPinsVersion(t *testing.T) {
	render := func(workload string, version int) []execution.Manifest {
		t.Helper()
		target := execution.Target{Namespace: "app-shop-staging", Kubeconfig: "/k", Extra: map[string]string{"application": "shop", "environment": "staging", "workload": workload}}
		out, err := (&VSOSynchronizer{}).RenderBundle(context.Background(), target, execution.ConfigBundle{StoreKey: "vault-a", Address: "http://vault.vault:8200", Mount: "kv", AuthMount: "kubernetes", Path: "orchestrator/apps/shop/envs/staging/values/workload-" + workload, Role: "orch-wl-1", ServiceAccount: "orch-sa-" + workload, SecretName: workload + "-config-v" + string(rune('0'+version)), Keys: map[string]map[string]string{"main": {"A": "main_A"}}, Version: version})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	byName := func(ms []execution.Manifest) map[string]execution.Manifest {
		out := map[string]execution.Manifest{}
		for _, m := range ms {
			out[m.Kind+"/"+m.Name] = m
		}
		return out
	}
	v1, v2 := byName(render("backend", 1)), byName(render("backend", 2))
	vss := v2["VaultStaticSecret/backend-config-v2"]
	spec := vss.Object["spec"].(map[string]any)
	if spec["version"] != 2 || spec["destination"].(map[string]any)["name"] != "backend-config-v2" || spec["vaultAuthRef"] != "orch-sa-backend-auth" {
		t.Fatalf("VSS = %+v", vss.Object)
	}
	for _, key := range []string{"ServiceAccount/orch-sa-backend", "VaultAuth/orch-sa-backend-auth", "VaultConnection/" + VaultConnectionName("vault-a")} {
		if !reflect.DeepEqual(v1[key].Object, v2[key].Object) {
			t.Fatalf("%s changed between versions", key)
		}
	}
	other := byName(render("worker", 1))
	key := "VaultConnection/" + VaultConnectionName("vault-a")
	if !reflect.DeepEqual(other[key].Object, v1[key].Object) {
		t.Fatal("shared VaultConnection differs between workloads")
	}
	if _, labelled := v1[key].Object["metadata"].(map[string]any)["labels"].(map[string]any)["orchestrator.io/workload"]; labelled {
		t.Fatal("shared object carries a workload label")
	}
	if _, err := (&VSOSynchronizer{}).RenderBundle(context.Background(), execution.Target{Namespace: "ns", Kubeconfig: "/k"}, execution.ConfigBundle{StoreKey: "s", Address: "a", Mount: "kv", AuthMount: "k", Path: "p", Role: "r", ServiceAccount: "orch-x", SecretName: "x-config-v3", Keys: map[string]map[string]string{"m": {"A": "m_A"}}, Version: 2}); err == nil {
		t.Fatal("mismatched version name accepted")
	}
}
