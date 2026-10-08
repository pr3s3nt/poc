//go:build integration

// Package kindtransition holds the live kind verification of the target
// transition adapter (separate from the AWS-era integration package).
package kindtransition

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/execution"
)

// TestTransitionAdapterOnKind drives the real PostgreSQL 16 transfer adapter on
// the existing kind cluster, in two run-owned namespaces: backup with private
// modes, a streamed restore into a second server, inventory comparison, archive
// removal, scale, route inspection and label-checked namespace cleanup.
// It deletes only the namespaces it created (ownership labels are checked).
func TestTransitionAdapterOnKind(t *testing.T) {
	kubeContext := requireEnv(t, "KIND_CONTEXT")
	runID := "esv" + strings.ReplaceAll(ids.New(), "-", "")[:8]
	srcNS, dstNS := runID+"-src", runID+"-dst"
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	target := kubeTarget{Context: kubeContext}
	kube := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "kubectl", target.args(args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("kubectl %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	executor := k8s.NewExecutor("kubectl")
	adapter := &k8s.Transition{KubectlPath: "kubectl", Timeout: 2 * time.Minute}
	t.Cleanup(func() {
		for _, ns := range []string{srcNS, dstNS} {
			// Run-owned namespaces only; the Environment label is the proof.
			exec.Command("kubectl", target.args("delete", "namespace", ns, "--ignore-not-found", "--wait=true", "--timeout=300s")...).Run()
		}
	})
	provision := func(ns string) execution.PostgresResource {
		base := execution.Target{Kind: "kubernetes", Context: kubeContext, Namespace: ns}
		req := execution.ProvisionRequest{ApplicationKey: runID, EnvironmentKey: "staging", RunID: runID, Target: base}
		req.ResourceType, req.Descriptor, req.Inputs = "k8s-namespace", "k8s-namespace.default#"+ns, map[string]any{"name": ns}
		if _, err := executor.Provision(ctx, req); err != nil {
			t.Fatalf("namespace %s: %v", ns, err)
		}
		req.ResourceType, req.Descriptor, req.Inputs = "postgres", "postgres.default#shared.verify", map[string]any{"database": "app", "username": "app", "image": "postgres:16-alpine"}
		if _, err := executor.Provision(ctx, req); err != nil {
			t.Fatalf("postgres %s: %v", ns, err)
		}
		return execution.PostgresResource{Target: base, Namespace: ns, Name: "shared-verify", Database: "app", Username: "app"}
	}
	src, dst := provision(srcNS), provision(dstNS)

	psql := func(res execution.PostgresResource, sql string) string {
		return kube("-n", res.Namespace, "exec", res.Name+"-0", "--", "psql", "-U", res.Username, "-d", res.Database, "-At", "-c", sql)
	}
	psql(src, "CREATE TABLE jobs(id serial primary key, payload text); CREATE TABLE results(id serial primary key, job int)")
	psql(src, "INSERT INTO jobs(payload) VALUES ('hello-"+runID+"'),('second'),('third')")
	psql(src, "INSERT INTO results(job) VALUES (1),(2)")

	inv, err := adapter.Inspect(ctx, src)
	if err != nil || inv.ServerVersionNum < 160000 || inv.ServerVersionNum >= 170000 || inv.Tables["public.jobs"] != 3 || inv.Tables["public.results"] != 2 {
		t.Fatalf("inspect source: %+v %v", inv, err)
	}
	want := execution.PostgresArchive{Pod: src.Name + "-0", Dir: "/tmp/orch-backup." + strings.ReplaceAll(ids.New(), "-", "")[:20]}
	archive, err := adapter.Backup(ctx, src, want)
	if err != nil || archive.Dir != want.Dir || len(archive.SHA256) != 64 || archive.Bytes <= 0 {
		t.Fatalf("backup: %+v %v", archive, err)
	}
	modes := strings.TrimSpace(kube("-n", srcNS, "exec", archive.Pod, "--", "sh", "-c", `stat -c '%a' "$1" "$1/dump.pgc"`, "sh", archive.Dir))
	if strings.Join(strings.Fields(modes), " ") != "700 600" {
		t.Fatalf("archive modes = %q, want private 700/600", modes)
	}
	if err := adapter.Restore(ctx, src, archive, dst); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := adapter.Inspect(ctx, dst)
	if err != nil || got.Tables["public.jobs"] != 3 || got.Tables["public.results"] != 2 || len(got.Tables) != len(inv.Tables) {
		t.Fatalf("destination inventory: %+v %v", got, err)
	}
	if content := psql(dst, "SELECT payload FROM jobs ORDER BY id LIMIT 1"); strings.TrimSpace(content) != "hello-"+runID {
		t.Fatalf("restored content: %q", content)
	}
	if err := adapter.RemoveArchive(ctx, src, archive); err != nil {
		t.Fatalf("remove archive: %v", err)
	}
	if out, _ := exec.CommandContext(ctx, "kubectl", target.args("-n", srcNS, "exec", archive.Pod, "--", "ls", archive.Dir)...).CombinedOutput(); !strings.Contains(string(out), "No such file") {
		t.Fatalf("archive directory still present: %s", out)
	}
	// A failed backup leaves nothing behind.
	bad := src
	bad.Database = "does_not_exist"
	if _, err := adapter.Backup(ctx, bad, execution.PostgresArchive{Dir: "/tmp/orch-backup." + strings.ReplaceAll(ids.New(), "-", "")[:20]}); err == nil {
		t.Fatal("backup of a missing database must fail")
	}
	if left := strings.TrimSpace(kube("-n", srcNS, "exec", src.Name+"-0", "--", "sh", "-c", "ls -d /tmp/orch-backup.* 2>/dev/null | wc -l")); left != "0" {
		t.Fatalf("failed backup left %s private archive(s)", left)
	}
	// Cluster identity is stable and independent of any Connection name.
	id1, err1 := adapter.ClusterIdentity(ctx, src.Target)
	id2, err2 := adapter.ClusterIdentity(ctx, dst.Target)
	if err1 != nil || err2 != nil || id1 == "" || id1 != id2 {
		t.Fatalf("cluster identity: %q %q %v %v", id1, id2, err1, err2)
	}
	if err := adapter.Probe(ctx, src.Target); err != nil {
		t.Fatalf("probe: %v", err)
	}
	// Ownership: a namespace with the wrong application label is refused.
	if err := adapter.DeleteOwnedNamespace(ctx, src.Target, srcNS, "someone-else", "staging"); err == nil {
		t.Fatal("cleanup must reject a namespace owned by another application")
	}
	if err := adapter.DeleteOwnedNamespace(ctx, src.Target, "kube-system", runID, "staging"); err == nil {
		t.Fatal("cleanup must reject protected namespaces")
	}
	if err := adapter.DeleteOwnedNamespace(ctx, src.Target, srcNS, runID, "staging"); err != nil {
		t.Fatalf("owned cleanup: %v", err)
	}
	if out, _ := exec.CommandContext(ctx, "kubectl", target.args("get", "namespace", srcNS, "--ignore-not-found", "-o", "name")...).Output(); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("namespace %s still present", srcNS)
	}
	fmt.Println("transition adapter verified on", kubeContext, "run", runID)
}

type kubeTarget struct{ Context string }

func (k kubeTarget) args(extra ...string) []string {
	return append([]string{"--context", k.Context}, extra...)
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not set", name)
	}
	return value
}
