package kubernetes

import (
	"context"
	"encoding/json"
	platformids "orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type metadataFixtureCredentials struct{ document []byte }

func (c metadataFixtureCredentials) ResolveKubeconfig(_ context.Context, t execution.Target) ([]byte, error) {
	if t.Organization != "fixture" || t.Connection != "task-cluster" {
		return nil, ErrDeliveryOwnership
	}
	return c.document, nil
}
func TestOwnedClusterMetadataHandoffAllowsNextFleetSSA(t *testing.T) {
	run := os.Getenv("ORCH_RECON_METADATA_LIVE_DIR")
	if run == "" {
		t.Skip("explicit owned metadata conformance environment required")
	}
	read := func(name string) string {
		body, err := os.ReadFile(filepath.Join(run, name))
		if err != nil {
			t.Fatal("private cluster proof unavailable")
		}
		return strings.TrimSpace(string(body))
	}
	cluster := read("cluster-name")
	if !strings.HasPrefix(cluster, "orch-reconciliation-") {
		t.Fatal("not task cluster")
	}
	actual, err := exec.Command("docker", "inspect", "--format", "{{.Id}}", cluster+"-control-plane").Output()
	if err != nil || strings.TrimSpace(string(actual)) != read("container-id") {
		t.Fatal("cluster incarnation changed")
	}
	document, err := os.ReadFile(filepath.Join(run, "kubeconfig"))
	if err != nil {
		t.Fatal("private kubeconfig unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	target := execution.Target{Organization: "fixture", Connection: "task-cluster", Namespace: "orch-recon-releases", Extra: map[string]string{"application": "metadata-fixture", "environment": "staging", "deployment": "fixture"}}
	cli, close, err := OpenCLI(ctx, "", metadataFixtureCredentials{document}, target)
	if err != nil {
		t.Fatal("fixture CLI unavailable")
	}
	defer close()
	system, found, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !found || system["metadata"].(map[string]any)["uid"] != read("kube-system-uid") {
		t.Fatal("cluster UID changed")
	}
	proof := filepath.Join(run, "metadata-conformance-object.json")
	if _, err := os.Stat(proof); err == nil {
		t.Fatal("prior metadata fixture must be inspected/cleaned before retry")
	}
	name := "orch-metadata-" + platformids.New()[:8]
	object := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": name, "namespace": target.Namespace, "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": "metadata-fixture", "orchestrator.io/environment": "staging", "orchestrator.io/deployment-id": "fixture", "orchestrator.test/run": "reconciliation-20261029"}, "annotations": map[string]any{"orchestrator.io/delivery-revision": "source", "business": "keep"}}, "data": map[string]any{"business": "unchanged"}}
	if err := cli.Apply(ctx, []map[string]any{object}); err != nil {
		t.Fatal("fixture creation failed")
	}
	created, found, err := cli.Get(ctx, target.Namespace, "configmap", name)
	if err != nil || !found {
		t.Fatal("fixture creation unconfirmed")
	}
	uid, _ := created["metadata"].(map[string]any)["uid"].(string)
	receipt, _ := json.Marshal(map[string]string{"name": name, "uid": uid, "namespace": target.Namespace})
	if os.WriteFile(proof, receipt, 0o600) != nil {
		t.Fatal("fixture proof unavailable")
	}
	ref := execution.ObjectRef{APIVersion: "v1", Kind: "ConfigMap", Name: name}
	destination := DeliveryManager{Release: "orch-metadata-fixture", Namespace: target.Namespace}
	if err := TransferDeliveryOwnership(ctx, cli, target, ref, uid, DeliveryManager{}, destination, "adopted"); err != nil {
		t.Fatalf("metadata handoff failed (%T)", err)
	}
	live, found, err := cli.Get(ctx, target.Namespace, "configmap", name)
	if err != nil || !found {
		t.Fatal("adopted fixture missing")
	}
	md := live["metadata"].(map[string]any)
	if md["uid"] != uid {
		t.Fatal("UID changed")
	}
	clean := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": name, "namespace": target.Namespace, "labels": md["labels"], "annotations": md["annotations"]}, "data": live["data"]}
	clean["metadata"].(map[string]any)["annotations"].(map[string]any)["orchestrator.io/delivery-revision"] = "next-admitted"
	raw, _ := json.Marshal(clean)
	if _, err := cli.Run(ctx, raw, "apply", "--server-side", "--field-manager", "fleetagent", "-f", "-"); err != nil {
		t.Fatalf("next Fleet SSA conflicted (%T)", err)
	}
	after, found, err := cli.Get(ctx, target.Namespace, "configmap", name)
	if err != nil || !found || after["metadata"].(map[string]any)["uid"] != uid || after["data"].(map[string]any)["business"] != "unchanged" {
		t.Fatal("SSA changed fixture identity/business data")
	}
	if _, err := cli.Run(ctx, nil, "patch", "configmap", name, "-n", target.Namespace, "--type=merge", "--field-manager", "kubectl-patch", "--patch", `{"data":{"business":"manual-drift"}}`); err != nil {
		t.Fatal("controlled manual edit failed")
	}
	if err := ReclaimManualDeliveryFields(ctx, cli, target, ref, uid, destination, clean, func(context.Context) error { return nil }, "next-admitted"); err != nil {
		t.Fatal("confirmed policy ownership reclaim failed", err)
	}
	drifted, found, err := cli.Get(ctx, target.Namespace, "configmap", name)
	if err != nil || !found || drifted["data"].(map[string]any)["business"] != "manual-drift" {
		t.Fatal("ownership reclaim changed business values")
	}
	if _, err := cli.Run(ctx, raw, "apply", "--server-side", "--field-manager", "fleetagent", "-f", "-"); err != nil {
		t.Fatal("Fleet SSA could not restore the admitted value", err)
	}
	after, found, err = cli.Get(ctx, target.Namespace, "configmap", name)
	if err != nil || !found || after["metadata"].(map[string]any)["uid"] != uid || after["data"].(map[string]any)["business"] != "unchanged" {
		t.Fatal("policy correction changed identity or lost admitted data")
	}
	if err := deleteDeliveryObject(ctx, cli, newDiscovery(cli), target, ref, uid, after["metadata"].(map[string]any)["resourceVersion"]); err != nil {
		t.Fatal("fixture cleanup unverified")
	}
	if err := os.Remove(proof); err != nil {
		t.Fatal("fixture proof cleanup failed")
	}
}
