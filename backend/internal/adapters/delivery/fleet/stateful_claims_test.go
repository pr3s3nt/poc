package fleet

import (
	"context"
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"testing"
)

func TestStatefulClaimProvenancePreservesIdentityWithoutChangingStorageIntent(t *testing.T) {
	i := packageIntent()
	i.Revision.Phase = domain.Rollout
	manifest := func(id, size string) execution.Manifest {
		return execution.Manifest{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "web", Namespace: i.Target.Namespace, Object: map[string]any{"apiVersion": "apps/v1", "kind": "StatefulSet", "metadata": map[string]any{"name": "web", "namespace": i.Target.Namespace, "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator"}}, "spec": map[string]any{"volumeClaimTemplates": []any{map[string]any{"metadata": map[string]any{"name": "data", "labels": map[string]any{"orchestrator.io/deployment-id": id, "business": "keep"}}, "spec": map[string]any{"resources": map[string]any{"requests": map[string]any{"storage": size}}}}}}}}
	}
	p := &Provider{}
	old, err := p.Prepare(context.Background(), i, []execution.Manifest{manifest("initial", "1Gi")})
	if err != nil {
		t.Fatal(err)
	}
	i.ExpectedRevision = old.Revision.ID
	i.PriorSubmission = &port.PriorSubmission{Prepared: old, Receipt: port.Receipt{RevisionID: old.Revision.ID, PayloadDigest: old.Revision.PayloadDigest}}
	i.SourceInventory = []port.AppliedObject{{ObjectRef: execution.ObjectRef{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "web"}, UID: "original"}}
	next := manifest("next", "2Gi")
	prepared, err := p.Prepare(context.Background(), i, []execution.Manifest{next})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.ExportArtifact(context.Background(), prepared.Payload)
	if err != nil {
		t.Fatal(err)
	}
	claims := obj(got[0].Object["spec"])["volumeClaimTemplates"].([]any)
	claim := obj(claims[0])
	if obj(obj(claim["metadata"])["labels"])["orchestrator.io/deployment-id"] != "initial" || obj(obj(claim["spec"])["resources"])["requests"].(map[string]any)["storage"] != "2Gi" {
		t.Fatal("Changed immutable identity or suppressed storage intent")
	}
	raw, _ := json.Marshal(next.Object)
	var input map[string]any
	_ = json.Unmarshal(raw, &input)
	if obj(obj(obj(input["spec"])["volumeClaimTemplates"].([]any)[0])["metadata"])["labels"].(map[string]any)["orchestrator.io/deployment-id"] != "next" {
		t.Fatal("Preparation mutated renderer output")
	}
	i.SourceInventory[0].UID = ""
	if _, err := p.Prepare(context.Background(), i, []execution.Manifest{next}); err == nil {
		t.Fatal("Missing source UID admitted")
	}
}
