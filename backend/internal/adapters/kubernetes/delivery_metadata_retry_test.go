package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/ports/execution"
)

func TestMetadataReleaseRetriesOnlyChangedVersionOfSameIncarnation(t *testing.T) {
	for _, mode := range []string{"changed", "unchanged", "replaced", "forbidden", "persistent"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DELIVERY_METADATA_FIXTURE", dir)
			t.Setenv("DELIVERY_METADATA_MODE", mode)
			body := `{"metadata":{"uid":"UID","resourceVersion":"RV","labels":{"app.kubernetes.io/managed-by":"orchestrator","orchestrator.io/application":"app","orchestrator.io/environment":"staging"},"annotations":{"orchestrator.io/delivery-revision":"revision"},"managedFields":[{"manager":"orchestrator-delivery-handoff","operation":"Update","fieldsV1":{"f:metadata":{"f:annotations":{"f:orchestrator.io/delivery-revision":{}}},"f:spec":{"f:replicas":{}}}}]}}`
			for name, value := range map[string]string{"body": body, "uid": "admitted", "rv": "1", "count": "0", "kubeconfig": "fixture"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			script := `#!/bin/sh
r="$DELIVERY_METADATA_FIXTURE"
case "$*" in
 *"patch "*)
  n=$(cat "$r/count"); n=$((n+1)); echo "$n" > "$r/count"
  if [ "$DELIVERY_METADATA_MODE" = changed ] && [ "$n" -gt 1 ]; then exit 0; fi
  if [ "$DELIVERY_METADATA_MODE" = forbidden ]; then echo 'Error from server (Forbidden)' >&2;exit 1;fi
  if [ "$DELIVERY_METADATA_MODE" != unchanged ]; then echo "$((n+1))" > "$r/rv";fi
  if [ "$DELIVERY_METADATA_MODE" = replaced ]; then echo replacement > "$r/uid";fi
  echo 'Error from server (Conflict): resource version changed' >&2;exit 1;;
 *"/horizontalpodautoscalers/frontend"*)
  sed -e "s/UID/$(cat "$r/uid" | tr -d '\n')/" -e "s/RV/$(cat "$r/rv" | tr -d '\n')/" "$r/body";;
 *) echo '{"resources":[{"name":"horizontalpodautoscalers","kind":"HorizontalPodAutoscaler","namespaced":true}]}' ;;
esac
`
			path := filepath.Join(dir, "kubectl")
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			err := ReleaseDeliveryMetadata(context.Background(), CLI{Path: path, Kubeconfig: filepath.Join(dir, "kubeconfig")}, execution.Target{Organization: "org", Connection: "cluster", Namespace: "app", Extra: map[string]string{"application": "app", "environment": "staging"}}, execution.ObjectRef{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "frontend"}, "admitted", DeliveryManager{}, "revision")
			if (err == nil) != (mode == "changed") {
				t.Fatalf("unexpected admission result: %v", err)
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "count"))
			want := "1\n"
			if mode == "changed" {
				want = "2\n"
			}
			if mode == "persistent" {
				want = "3\n"
			}
			if string(raw) != want {
				t.Fatalf("patch attempts %q, want %q", raw, want)
			}
		})
	}
}
