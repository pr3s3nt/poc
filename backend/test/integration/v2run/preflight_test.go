package v2run_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/test/integration/v2run"
)

type scripted struct{ answers map[string]string }

func (s scripted) Run(_ context.Context, args ...string) ([]byte, string, int) {
	line := strings.Join(args, " ")
	for prefix, answer := range s.answers {
		if strings.HasPrefix(line, prefix) {
			if strings.HasPrefix(answer, "ERR:") {
				return nil, strings.TrimPrefix(answer, "ERR:"), 1
			}
			return []byte(answer), "", 0
		}
	}
	return nil, "unexpected " + line, 99
}

const readyNode = `{"items":[{"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"architecture":"amd64"}}}]}`

func goodAnswers() map[string]string {
	return map[string]string{
		"get nodes": readyNode, "get namespaces": "namespace/default\nnamespace/kube-system\n",
		"-n kube-public get configmap cluster-info": "uid-1", "get storageclass": "storageclass.storage.k8s.io/standard\n", "auth can-i": "yes\n",
	}
}

func TestPreflightK8sAcceptsTheExpectedCluster(t *testing.T) {
	checks := v2run.PreflightK8s(context.Background(), scripted{goodAnswers()}, goodConfig(t))
	if !v2run.AllOK(checks) {
		t.Fatalf("%+v", checks)
	}
}

func TestPreflightK8sFailsClosed(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"unreachable": func(a map[string]string) { a["get nodes"] = "ERR:Unable to connect to the server" },
		"no ready node": func(a map[string]string) {
			a["get nodes"] = `{"items":[{"status":{"conditions":[{"type":"Ready","status":"False"}],"nodeInfo":{"architecture":"amd64"}}}]}`
		},
		"wrong architecture":   func(a map[string]string) { a["get nodes"] = strings.Replace(readyNode, "amd64", "arm64", 1) },
		"run namespace exists": func(a map[string]string) { a["get namespaces"] = "namespace/orch-v2-test-a01-0001\n" },
		"prefixed namespace":   func(a map[string]string) { a["get namespaces"] = "namespace/orch-v2-test-a01-0001-g1-abcd1234\n" },
		"sentinel changed":     func(a map[string]string) { a["-n kube-public get configmap cluster-info"] = "other-uid" },
		"no storage class":     func(a map[string]string) { a["get storageclass"] = "" },
		"cannot create":        func(a map[string]string) { a["auth can-i"] = "no\n" },
	} {
		answers := goodAnswers()
		mutate(answers)
		if v2run.AllOK(v2run.PreflightK8s(context.Background(), scripted{answers}, goodConfig(t))) {
			t.Errorf("%s passed the preflight", name)
		}
	}
}
