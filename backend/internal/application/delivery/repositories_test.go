package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/adapters/delivery/fleet"
	"orchestrator/internal/adapters/store"
	appservice "orchestrator/internal/application/application"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	hosting "orchestrator/internal/ports/repositoryhosting"
)

type repositoryHost struct {
	calls int
	after func()
	fail  bool
}

func (h *repositoryHost) Verify(context.Context) error { return nil }
func (h *repositoryHost) Ensure(_ context.Context, r domain.ApplicationRepository) (domain.ApplicationRepository, error) {
	h.calls++
	if h.after != nil {
		h.after()
	}
	if h.fail {
		return r, hosting.ErrUnavailable
	}
	r.Status = "READY"
	r.RemoteID = "123"
	r.CloneURL = "https://github.com/platform/" + r.Name + ".git"
	r.Branch = "trunk"
	return r, nil
}

type repositoryFactory struct{ host *repositoryHost }

func (f repositoryFactory) Open(context.Context, domain.BindingVersion, domain.RepositoryPolicy) (hosting.Host, error) {
	return f.host, nil
}

func TestManagedRepositoryProvisioningRequiresOwnerAndConvergesAcrossEnvironments(t *testing.T) {
	c, _, ctx, req, _ := coordinatorFixture(t)
	st := c.Store.(*store.Store)
	org, _ := st.GetOrganization(ctx, "delivery-org")
	b, err := st.GetDeliveryBinding(ctx, org.ID, req.Environment.DesiredDelivery.BindingID, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Config = map[string]string{"repositoryOwner": "platform", "repositoryAPI": "https://api.github.com", "repositoryHost": "https://github.com", "repositoryPrefix": "orch-app-", "root": "environments"}
	provider := &fleet.Provider{}
	host := &repositoryHost{fail: true}
	svc := &Repositories{Store: st, Hosts: repositoryFactory{host}}
	if _, err := svc.Resolve(context.Background(), b, provider, req.Environment, true); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("unowned creation", err)
	}
	if host.calls != 0 {
		t.Fatal("unowned HTTP")
	}
	if _, err := svc.Resolve(ctx, b, provider, req.Environment, false); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("preview created repo", err)
	}
	if host.calls != 0 {
		t.Fatal("read called host")
	}
	if _, err := svc.Resolve(ctx, b, provider, req.Environment, true); !errors.Is(err, hosting.ErrUnavailable) {
		t.Fatal(err)
	}
	app, _ := st.GetApplication(ctx, "delivery-app")
	pending, err := st.GetApplicationDeliveryRepository(ctx, org.ID, app.ID)
	if err != nil || pending.Status != "PENDING" {
		t.Fatal("reservation not durable", err)
	}
	host.fail = false
	staging, err := svc.Resolve(ctx, b, provider, req.Environment, true)
	if err != nil {
		t.Fatal(err)
	}
	if staging.Config["branch"] != "trunk" || staging.Config["root"] != "environments/staging" {
		t.Fatal("destination not bound")
	}
	prod, _ := st.GetEnvironment(ctx, "delivery-app", "production")
	production, err := svc.Resolve(context.Background(), b, provider, prod, false)
	if err != nil || production.Config["repositoryURL"] != staging.Config["repositoryURL"] || production.Config["root"] != "environments/production" {
		t.Fatal("environment isolation", err)
	}
	if staging.Config["applicationID"] != app.ID || production.Config["applicationID"] != app.ID || staging.Config["environmentKey"] != "staging" || production.Config["environmentKey"] != "production" {
		t.Fatal("Application controller identity is not shared across Environments")
	}
	if host.calls != 2 {
		t.Fatal("read/observe provisioned repo")
	}
	// A response from an expired owner cannot acknowledge the durable allocation.
}

func TestApplicationPinsPlatformDefaultWithoutProvisioning(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	b := persistencetest.DeliveryBindings(t, st)
	defaults := NewBindings(st, nil)
	svc := appservice.NewService(st)
	svc.SetDeliveryDefaults(defaults)
	if _, err := svc.Create(ctx, appservice.CreateCommand{OrganizationKey: "delivery-org", Name: "Missing"}); !errors.Is(err, ErrPlatformDefault) {
		t.Fatal(err)
	}
	apps, _ := st.ListApplications(ctx)
	if len(apps) != 0 {
		t.Fatal("partial application")
	}
	if _, err := st.SetPlatformDeliveryDefault(ctx, domain.PlatformDefault{OrganizationID: b.OrganizationID, BindingID: b.ID, BindingVersion: 1}, 0); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Create(ctx, appservice.CreateCommand{OrganizationKey: "delivery-org", Name: "Managed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range result.Environments {
		if env.DesiredDelivery.BindingID != b.ID || env.AppliedDelivery != env.DesiredDelivery {
			t.Fatal("default not pinned")
		}
	}
	if _, err := st.GetApplicationDeliveryRepository(ctx, b.OrganizationID, result.Application.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("creation provisioned repo", err)
	}
	next := b.Clone()
	next.ID = ids.New()
	next.Version = 1
	next.Status = domain.Unverified
	next.Revision = 1
	next.VerifiedAt = nil
	if err := st.CreateDeliveryBinding(ctx, next); err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyDeliveryBinding(ctx, next.OrganizationID, next.ID, 1, 1, domain.BindingReady, b.Capabilities, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetPlatformDeliveryDefault(ctx, domain.PlatformDefault{OrganizationID: b.OrganizationID, BindingID: next.ID, BindingVersion: 1}, 1); err != nil {
		t.Fatal(err)
	}
	existing, _ := st.GetEnvironment(ctx, result.Application.Key, "staging")
	if existing.DesiredDelivery.BindingID != b.ID {
		t.Fatal("default rewrote existing application")
	}
	newer, err := svc.Create(ctx, appservice.CreateCommand{OrganizationKey: "delivery-org", Name: "Newer"})
	if err != nil || newer.Environments[0].DesiredDelivery.BindingID != next.ID {
		t.Fatal("new default ignored", err)
	}
}

func TestRepositoryLostOwnerCannotAcknowledge(t *testing.T) {
	c, _, ctx, req, _ := coordinatorFixture(t)
	st := c.Store.(*store.Store)
	org, _ := st.GetOrganization(ctx, "delivery-org")
	b, _ := st.GetDeliveryBinding(ctx, org.ID, req.Environment.DesiredDelivery.BindingID, 1)
	b.Config = map[string]string{"repositoryOwner": "platform", "repositoryAPI": "https://api.github.com", "repositoryHost": "https://github.com", "repositoryPrefix": "orch-app-", "root": "environments"}
	host := &repositoryHost{after: func() {
		owner, _ := persistence.OwnerFrom(ctx)
		if err := st.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
			t.Fatal(err)
		}
	}}
	svc := &Repositories{Store: st, Hosts: repositoryFactory{host}}
	if _, err := svc.Resolve(ctx, b, &fleet.Provider{}, req.Environment, true); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("lost owner acknowledged", err)
	}
}
