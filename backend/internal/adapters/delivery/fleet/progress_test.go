package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/progress"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProgressReadsOnlyCurrentOwnedPodsAndDropsRawMessages(t *testing.T) {
	for _, revision := range []string{"rev1", "foreign"} {
		t.Run(revision, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "kubectl")
			log := filepath.Join(dir, "calls")
			if err := os.WriteFile(filepath.Join(dir, "config"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			body := `#!/bin/sh
printf '%s\n' "$*" >> '` + log + `'
case " $* " in
 *" get StatefulSet backend "*) cat <<'JSON'
{"metadata":{"annotations":{"orchestrator.io/delivery-revision":"` + revision + `","meta.helm.sh/release-name":"release"},"labels":{"orchestrator.io/workload":"backend"}},"spec":{"selector":{"matchLabels":{"app":"backend"}}},"status":{"updateRevision":"new"}}
JSON
 ;;
 *" get pods "*) cat <<'JSON'
{"items":[{"metadata":{"name":"backend-0","uid":"pod-uid"},"status":{"phase":"Pending","message":"password=PRIVATE","containerStatuses":[{"image":"ghcr.io/acme/backend:missing","restartCount":2,"state":{"waiting":{"reason":"ImagePullBackOff","message":"token=PRIVATE"}}}]}}]}
JSON
 ;;
 *" get events "*) cat <<'JSON'
{"items":[{"reason":"Failed","message":"secret=PRIVATE","lastTimestamp":"2026-11-11T00:00:00Z","count":3}]}
JSON
 ;;
 *) exit 1;;
esac
`
			if err := os.WriteFile(script, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			inv, _ := json.Marshal([]execution.ObjectRef{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "backend"}})
			pods, events := collectReadinessProgress(context.Background(), k8s.NewCLI(script, "", filepath.Join(dir, "config")), port.Intent{Revision: domain.Revision{ID: "rev1"}, Target: execution.Target{Namespace: "owned"}}, port.Receipt{Handle: map[string]string{"inventory": string(inv), "release": "release"}})
			calls, _ := os.ReadFile(log)
			if revision == "foreign" {
				if len(pods) != 0 || len(events) != 0 || strings.Contains(string(calls), "get pods") {
					t.Fatal("foreign controller supplied evidence")
				}
				return
			}
			safe := progress.Step{ID: "wait", Kind: "READINESS", Status: "RUNNING", Pods: pods, Events: events}.Safe()
			raw, _ := json.Marshal(safe)
			if strings.Contains(string(raw), "PRIVATE") || len(safe.Pods) != 1 || safe.Pods[0].Reason != "ImagePullBackOff" || safe.Pods[0].Restarts != 2 || len(safe.Events) != 1 {
				t.Fatalf("unsafe or missing facts: %s", raw)
			}
			if !strings.Contains(string(calls), "app=backend,controller-revision-hash=new") || !strings.Contains(string(calls), "involvedObject.uid=pod-uid") {
				t.Fatalf("unscoped diagnostic reads: %s", calls)
			}
		})
	}
}

func TestFleetApplyReasonNeverPublishesControllerText(t *testing.T) {
	for _, tc := range []struct{ message, want string }{{"services backend is forbidden; token=PRIVATE", "FLEET_FORBIDDEN"}, {"Unauthorized password=PRIVATE", "FLEET_AUTH"}, {"provider error /private/path", "FLEET_APPLY_FAILED"}} {
		if got := applyReasonCode(tc.message); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}
