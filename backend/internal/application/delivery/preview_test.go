package delivery

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
	"testing"
)

func TestPreviewPinUsesSnapshotAndRejectsChangedVerification(t *testing.T) {
	c, _, ctx, req, _ := coordinatorFixture(t)
	var pin PreviewPin
	err := c.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		var err error
		pin, err = PinForPreview(ctx, view, req.OrganizationKey, req.Environment)
		return err
	})
	if err != nil || pin.Revision < 1 || len(pin.CapabilitiesDigest) != 64 {
		t.Fatal("binding not pinned in snapshot", err)
	}
	selection := req.Environment.DesiredDelivery
	org, err := c.Store.GetOrganization(ctx, req.OrganizationKey)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Store.GetDeliveryBinding(ctx, org.ID, selection.BindingID, selection.BindingVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Store.VerifyDeliveryBinding(ctx, org.ID, b.ID, b.Version, b.Revision, domain.Rejected, b.Capabilities, b.CreatedAt, "VERIFICATION_FAILED"); err != nil {
		t.Fatal(err)
	}
	if _, err := PinForPreview(ctx, c.Store, req.OrganizationKey, req.Environment); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal("rejected verification admitted", err)
	}
}

func TestTransferPreviewRequiresEveryCurrentWorkloadAndShowsOnlySafeSourceFacts(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &transferFake{neutralProvider: original, live: map[string]string{}}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
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
		m.Object = map[string]any{"apiVersion": m.APIVersion, "kind": m.Kind, "metadata": map[string]any{"name": m.Name, "namespace": m.Namespace}}
	}
	if _, err := TransferPreview(ctx, c.Store, req.OrganizationKey, req.Environment, []string{"app"}); !errors.Is(err, ErrBaselineRequired) {
		t.Fatal("legacy baseline was admitted", err)
	}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	units, err := TransferPreview(ctx, c.Store, req.OrganizationKey, req.Environment, []string{"app"})
	if err != nil || len(units) != 3 {
		t.Fatal("phase inventory missing", err)
	}
	for _, unit := range units {
		if unit.WorkloadID != "app" || unit.Objects < 1 || unit.Revision == "" {
			t.Fatal("invalid source summary")
		}
	}
	if _, err := TransferPreview(ctx, c.Store, req.OrganizationKey, req.Environment, []string{"app", "unrecorded"}); !errors.Is(err, ErrBaselineRequired) {
		t.Fatal("partial handoff baseline admitted", err)
	}
}
