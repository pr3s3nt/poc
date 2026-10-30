package tfexec_test

import (
	"errors"
	"testing"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/domain/environment"
)

func TestConsumerScanUsesImmutableHistoricalDependencyOutputs(t *testing.T) {
	h := newHarness(t)
	providerDescriptor, consumerDescriptor := "postgres.default#shared.provider", "postgres.default#shared.consumer"
	provider := h.createInstance("provider", providerDescriptor, scopeOf("provider"), nil)
	consumer := h.createInstance("consumer", consumerDescriptor, scopeOf("consumer"), []string{provider.InstanceID})
	update := func(run, note string) {
		t.Helper()
		ctx, op := h.claim(run)
		req := h.request(run, h.mod, inputs(note, "k"))
		req.Descriptor, req.Scope, req.Identity = providerDescriptor, scopeOf("provider"), h.identity(providerDescriptor, scopeOf("provider"))
		mustExecute(t, h, ctx, req)
		h.release(op, environment.OpSucceeded)
	}
	update("provider-no-op-ack", "db-provider")
	ctx, op := h.claim("scan-stable-output-consumer")
	report, err := h.svc.InspectInstance(ctx, h.orgID, consumer.InstanceID)
	if err != nil || !report.Complete || len(report.Proposed) > 0 {
		t.Fatal("stable output acknowledgement invalidated consumer baseline", err)
	}
	h.release(op, environment.OpSucceeded)
	update("provider-output-change", "changed-database")
	ctx, op = h.claim("scan-changed-output-consumer")
	before := h.runner.opens
	if _, err := h.svc.InspectInstance(ctx, h.orgID, consumer.InstanceID); !errors.Is(err, tfexec.ErrBaselineUnavailable) {
		t.Fatal("changed provider outputs replaced the consumer's original inputs", err)
	}
	if h.runner.opens != before {
		t.Fatal("provider-output mismatch invoked Terraform before refusing the stale baseline")
	}
	h.release(op, environment.OpFailed)
}
