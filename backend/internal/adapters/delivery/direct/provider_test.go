package direct

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

type recorder struct {
	calls     int
	manifests []execution.Manifest
}

func (r *recorder) Apply(_ context.Context, _ execution.Target, m []execution.Manifest) error {
	r.calls++
	r.manifests = m
	return nil
}
func (r *recorder) WaitReady(context.Context, execution.Target, []execution.WorkloadRef) error {
	return nil
}
func (r *recorder) Remove(context.Context, execution.Target, string) error { return nil }
func directFixture() (port.Intent, execution.Manifest) {
	i := port.Intent{ID: "intent", OrganizationKey: "org", Owner: port.Owner{OperationID: "operation", Instance: "worker", Fence: 1}, Revision: domain.Revision{ID: "revision", Scope: domain.Scope{OrganizationID: "org", EnvironmentID: "env", UnitKey: "app/prerequisite"}, Phase: domain.Prerequisite, Selection: domain.Selection{Mode: domain.Direct}, PayloadDigest: strings.Repeat("a", 64), ArtifactRef: "private"}, Target: execution.Target{Organization: "org", Connection: "account", Namespace: "app", Extra: map[string]string{"application": "app", "environment": "staging"}}}
	m := execution.Manifest{APIVersion: "v1", Kind: "ConfigMap", Name: "config", Namespace: "app", Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "config", "namespace": "app", "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator", "orchestrator.io/application": "app", "orchestrator.io/environment": "staging"}}, "data": map[string]any{"value": "retained"}}}
	return i, m
}
func TestPreparePersistsExactObjectsWithoutMutatingRender(t *testing.T) {
	i, m := directFixture()
	p := &Provider{}
	before, _ := json.Marshal(m.Object)
	prepared, err := p.Prepare(context.Background(), i, []execution.Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(m.Object)
	if string(before) != string(after) {
		t.Fatal("renderer mutated")
	}
	decoded, err := decode(prepared.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Objects[0].Body["data"].(map[string]any)["value"] != "retained" {
		t.Fatal("business artifact lost")
	}
	annotations := decoded.Objects[0].Body["metadata"].(map[string]any)["annotations"].(map[string]any)
	if annotations["orchestrator.io/delivery-revision"] != i.Revision.ID {
		t.Fatal("revision proof missing")
	}
	m.Secret = true
	if _, err := p.Prepare(context.Background(), i, []execution.Manifest{m}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("Secret entered private replayable workload artifact", err)
	}
}
func TestSubmitChecksFenceBeforeAnyNativeApply(t *testing.T) {
	i, m := directFixture()
	native := &recorder{}
	lost := errors.New("lost fence")
	p := &Provider{Deployer: native, Guard: func(context.Context, port.Intent) error { return lost }}
	prepared, err := p.Prepare(context.Background(), i, []execution.Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	i.Revision = prepared.Revision
	if _, err := p.Submit(context.Background(), i, prepared); !errors.Is(err, lost) || native.calls != 0 {
		t.Fatal("write after lost fence", err)
	}
	p.Guard = func(context.Context, port.Intent) error { return nil }
	receipt, err := p.Submit(context.Background(), i, prepared)
	if err != nil || native.calls != 1 || receipt.RevisionID != i.Revision.ID {
		t.Fatal("direct admission failed", err)
	}
	if len(native.manifests) != 1 || native.manifests[0].Name != m.Name {
		t.Fatal("artifact was not replayed")
	}
	p.RecoveryGuard = func(context.Context, port.Intent) error { return nil }
	if _, err := p.RecoverSubmission(context.Background(), i, prepared, receipt, port.TransferSource{}); err != nil || native.calls != 1 {
		t.Fatal("receipted recovery reapplied objects", err)
	}
	p.RecoveryGuard = func(context.Context, port.Intent) error { return lost }
	if _, err := p.RecoverSubmission(context.Background(), i, prepared, receipt, port.TransferSource{}); !errors.Is(err, lost) || native.calls != 1 {
		t.Fatal("recovery ignored fence", err)
	}
}
