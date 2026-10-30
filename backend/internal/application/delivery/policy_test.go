package delivery

import (
	"context"
	"encoding/json"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
	"testing"
	"time"
)

func TestPolicyUpdateReusesAdmittedBusinessArtifactsAndCompletedJob(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	provider := &transferFake{neutralProvider: original, live: map[string]string{}}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", provider); err != nil {
		t.Fatal(err)
	}
	for n := range req.Manifests {
		m := &req.Manifests[n]
		m.APIVersion, m.Name, m.Namespace = "v1", "app", "app"
		if m.Kind == "Job" {
			m.APIVersion = "batch/v1"
		}
		if m.Kind == "Deployment" {
			m.APIVersion = "apps/v1"
		}
		m.Object = map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": map[string]any{"name": m.Name, "namespace": m.Namespace}, "spec": map[string]any{"business": "unchanged"}}
	}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := c.Activate(ctx, req.ApplicationKey, req.Environment); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(provider.live)
	owner, _ := persistence.OwnerFrom(ctx)
	if err := c.Store.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	env, err := c.Store.GetEnvironment(context.Background(), req.ApplicationKey, req.Environment.Key)
	if err != nil {
		t.Fatal(err)
	}
	org, err := c.Store.GetOrganization(context.Background(), req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := c.Store.GetDeliveryBinding(context.Background(), org.ID, env.DesiredDelivery.BindingID, env.DesiredDelivery.BindingVersion)
	if err != nil {
		t.Fatal(err)
	}
	caps := binding.Capabilities
	caps.SelfHeal = true
	caps.DriftDetection = true
	if _, err := c.Store.VerifyDeliveryBinding(context.Background(), org.ID, binding.ID, binding.Version, binding.Revision, domain.BindingReady, caps, time.Now().UTC(), ""); err != nil {
		t.Fatal(err)
	}
	selection := env.DesiredDelivery
	selection.SelfHeal = true
	env, err = c.Store.(persistence.DeliverySelectionRepository).SelectDelivery(context.Background(), persistence.DeliverySelectionCommand{OrganizationID: org.ID, ApplicationKey: env.ApplicationKey, EnvironmentKey: env.Key, ExpectedEnvironmentVersion: env.Version, ExpectedConfigurationRevision: env.DeliveryConfigRevision, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	op, err := c.Store.ClaimEnvironment(context.Background(), persistence.OperationClaim{ID: ids.New(), ApplicationKey: env.ApplicationKey, EnvironmentKey: env.Key, Owner: "policy", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	policyCtx := persistence.WithOwner(context.Background(), persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence})
	if err := c.UpdatePolicy(policyCtx, req.OrganizationKey, env, func(string, map[string]any) error { return nil }); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(provider.live)
	if string(before) != string(after) || provider.jobs != 1 || provider.released != 0 || provider.adopted != 0 {
		t.Fatal("policy changed identities, replayed Job or transferred ownership")
	}
	applied, err := c.Store.GetEnvironment(policyCtx, env.ApplicationKey, env.Key)
	if err != nil || !applied.AppliedDelivery.SelfHeal || applied.DeliveryEpoch != env.DeliveryEpoch {
		t.Fatal("policy activation changed epoch or was not acknowledged", err)
	}
	records, err := c.Store.ListDeliveryExecutions(policyCtx, org.ID, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Mutation.OperationID != op.ID {
			continue
		}
		raw, err := c.providerArtifact(policyCtx, record)
		if err != nil {
			t.Fatal(err)
		}
		objects, err := provider.ExportArtifact(policyCtx, raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range objects {
			if m.Object["spec"].(map[string]any)["business"] != "unchanged" {
				t.Fatal("business artifact rerendered")
			}
		}
	}
}
