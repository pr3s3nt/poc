package fleet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

func TestRetainedReceiptFindsExactFailedJobWithoutAppliedUID(t *testing.T) {
	for _, revision := range []string{"failed-revision", "replacement-revision"} {
		t.Run(revision, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubectl")
			script := `#!/bin/sh
case "$*" in
 *"get --raw /apis/batch/v1") printf '%s' '{"resources":[{"name":"jobs","kind":"Job","namespaced":true}]}';;
 *"get --raw /apis/batch/v1/namespaces/test/jobs/migrate") printf '%s' '{"metadata":{"uid":"job-uid","annotations":{"orchestrator.io/delivery-revision":"` + revision + `"}}}';;
 *) exit 42;;
esac
`
			if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			kubeconfig := filepath.Join(filepath.Dir(path), "test-kubeconfig")
			if err := os.WriteFile(kubeconfig, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			target := execution.Target{Namespace: "test"}
			receipt := port.Receipt{RevisionID: "failed-revision", Handle: map[string]string{
				"release": "migration", "inventory": `[{"apiVersion":"batch/v1","kind":"Job","name":"migrate"}]`,
			}}
			observed, err := retainedReceiptObjects(context.Background(), k8s.CLI{Path: path, Kubeconfig: kubeconfig}, port.Intent{Target: target}, receipt, nil)
			if revision != receipt.RevisionID {
				if !errors.Is(err, ErrOwnership) || len(observed) != 0 {
					t.Fatalf("replacement revision accepted: %v %v", observed, err)
				}
			} else if err != nil || len(observed) != 1 || observed[0].UID != "job-uid" || observed[0].Release != "migration" {
				t.Fatalf("failed Job missing from receipt inventory: %v %v", observed, err)
			}
		})
	}
}
