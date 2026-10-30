package persistencetest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

type DeliveryStore interface {
	persistence.Store
	persistence.DeliveryBindingRepository
}

func DeliveryBindings(t *testing.T, st DeliveryStore) delivery.BindingVersion {
	t.Helper()
	ctx := context.Background()
	org := application.Organization{ID: ids.New(), Key: "delivery-org", Name: "Delivery"}
	if err := st.SaveOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	org, err := st.GetOrganization(ctx, org.Key)
	if err != nil {
		t.Fatal(err)
	}
	c := application.Connection{ID: ids.New(), Key: "delivery-manager", Name: "Manager", OrganizationKey: org.Key, Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig, Config: map[string]any{}, Verification: map[string]any{}, SecretRef: "manager-ref", Status: application.ConnectionReady, Version: 1}
	if err := st.SaveConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	b := delivery.BindingVersion{ID: ids.New(), Version: 1, OrganizationID: org.ID, Name: "Fleet staging", ProviderKind: "fleet", ContractVersion: "v1", ManagementAccountID: c.ID, ManagementAccountVersion: 1, Config: map[string]string{"namespace": "fleet-local"}, WriterCredentialRef: "opaque-writer-ref", Status: delivery.Unverified, Revision: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	if err := st.CreateDeliveryBinding(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateDeliveryBinding(ctx, b); !errors.Is(err, persistence.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := st.GetDeliveryBinding(ctx, ids.New(), b.ID, 1); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	foreign := b
	foreign.ID = ids.New()
	foreign.OrganizationID = ids.New()
	if err := st.CreateDeliveryBinding(ctx, foreign); err == nil {
		t.Fatal("foreign account admitted")
	}
	copy, err := st.GetDeliveryBinding(ctx, b.OrganizationID, b.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	copy.Config["namespace"] = "foreign"
	caps := delivery.Capabilities{ContractVersion: "v1", ExactRevision: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true}
	at := b.CreatedAt.Add(time.Second)
	verified, err := st.VerifyDeliveryBinding(ctx, b.OrganizationID, b.ID, 1, 1, delivery.BindingReady, caps, at, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.VerifyDeliveryBinding(ctx, b.OrganizationID, b.ID, 1, 1, delivery.Rejected, caps, at, ""); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale verify: %v", err)
	}
	if verified.Config["namespace"] != "fleet-local" || verified.WriterCredentialRef != b.WriterCredentialRef {
		t.Fatal("configuration lost/mutated")
	}
	raw, _ := json.Marshal(verified)
	if strings.Contains(string(raw), b.WriterCredentialRef) {
		t.Fatal("credential reference exposed")
	}
	next := b.Clone()
	next.Version = 2
	next.Config["namespace"] = "workspace"
	if err := st.CreateDeliveryBinding(ctx, next); err != nil {
		t.Fatal(err)
	}
	original, err := st.GetDeliveryBinding(ctx, b.OrganizationID, b.ID, 1)
	if err != nil || !original.SameConfiguration(b) {
		t.Fatal("original binding rewritten", err)
	}
	return verified
}
func AssertDeliveryBinding(t *testing.T, st persistence.DeliveryBindingRepository, b delivery.BindingVersion) {
	t.Helper()
	got, err := st.GetDeliveryBinding(context.Background(), b.OrganizationID, b.ID, b.Version)
	if err != nil || got.WriterCredentialRef != b.WriterCredentialRef || got.Status != delivery.BindingReady || got.Revision != 2 || !got.SameConfiguration(b) {
		t.Fatalf("binding reopen: %v", err)
	}
	list, err := st.ListDeliveryBindings(context.Background(), b.OrganizationID)
	if err != nil || len(list) != 2 {
		t.Fatalf("version list: %d %v", len(list), err)
	}
}

func DeliverySelections(t *testing.T, st interface {
	DeliveryStore
	persistence.DeliverySelectionRepository
}, b delivery.BindingVersion) {
	t.Helper()
	ctx := context.Background()
	a := application.Application{ID: ids.New(), Key: "delivery-app", OrganizationKey: "delivery-org", Name: "Delivery App", Subdomain: "delivery-app", Version: 1}
	if err := st.SaveApplication(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"staging", "production"} {
		if err := st.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), ApplicationKey: a.Key, Key: key, Name: key, Type: key, NamespaceIdentity: "delivery-" + key}); err != nil {
			t.Fatal(err)
		}
	}
	env, err := st.GetEnvironment(ctx, a.Key, "staging")
	if err != nil {
		t.Fatal(err)
	}
	cmd := persistence.DeliverySelectionCommand{OrganizationID: b.OrganizationID, ApplicationKey: a.Key, EnvironmentKey: "staging", ExpectedEnvironmentVersion: env.Version, ExpectedConfigurationRevision: 0, Selection: delivery.Selection{Mode: delivery.CD, BindingID: b.ID, BindingVersion: b.Version}}
	selected, err := st.SelectDelivery(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if selected.AppliedDelivery.Effective().Mode != delivery.Direct || selected.DesiredDelivery.Mode != delivery.CD || selected.DeliveryConfigRevision != 1 || selected.Version != env.Version+1 {
		t.Fatal("selection changed runtime or omitted version guards")
	}
	if _, err := st.SelectDelivery(ctx, cmd); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale selection: %v", err)
	}
	cmd.ExpectedEnvironmentVersion = selected.Version
	cmd.ExpectedConfigurationRevision = 1
	same, err := st.SelectDelivery(ctx, cmd)
	if err != nil || same.Version != selected.Version {
		t.Fatal("no-op changed revision", err)
	}
	cmd.OrganizationID = ids.New()
	if _, err := st.SelectDelivery(ctx, cmd); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("foreign selection: %v", err)
	}
	prod, err := st.GetEnvironment(ctx, a.Key, "production")
	if err != nil || prod.DesiredDelivery.Effective().Mode != delivery.Direct {
		t.Fatal("staging selection leaked into production", err)
	}
	// Generic Environment writes cannot bypass dedicated selection authority.
	selected.DesiredDelivery = delivery.Selection{Mode: delivery.Direct}
	selected.DeliveryConfigRevision = 99
	if err := st.SaveEnvironment(ctx, selected); err != nil {
		t.Fatal(err)
	}
	saved, err := st.GetEnvironment(ctx, a.Key, "staging")
	if err != nil || saved.DesiredDelivery.Mode != delivery.CD || saved.DeliveryConfigRevision != 1 {
		t.Fatal("generic write changed delivery", err)
	}
}
