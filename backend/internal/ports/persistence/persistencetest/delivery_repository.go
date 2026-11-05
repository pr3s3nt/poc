package persistencetest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

type ManagedDeliveryStore interface {
	DeliveryStore
	persistence.DeliveryPlatformRepository
}

// ManagedDelivery exercises the same concurrency, tenant and immutable identity
// contract against JSON and the real PostgreSQL runtime role.
func ManagedDelivery(t *testing.T, st ManagedDeliveryStore, b delivery.BindingVersion) delivery.ApplicationRepository {
	t.Helper()
	ctx := context.Background()
	d := delivery.PlatformDefault{OrganizationID: b.OrganizationID, BindingID: b.ID, BindingVersion: b.Version}
	if _, err := st.SetPlatformDeliveryDefault(ctx, d, 1); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatalf("stale default: %v", err)
	}
	saved, err := st.SetPlatformDeliveryDefault(ctx, d, 0)
	if err != nil || saved.Revision != 1 {
		t.Fatal("default", err)
	}
	unverified := d
	unverified.BindingVersion = 2
	if _, err := st.SetPlatformDeliveryDefault(ctx, unverified, 1); err == nil {
		t.Fatal("unverified default")
	}
	if _, err := st.GetPlatformDeliveryDefault(ctx, ids.New()); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("tenant default", err)
	}
	a := application.Application{ID: ids.New(), Key: ids.New(), OrganizationKey: "delivery-org", Name: "Managed", Version: 1}
	if err := st.SaveApplication(ctx, a); err != nil {
		t.Fatal(err)
	}
	pending := delivery.ApplicationRepository{ID: ids.New(), OrganizationID: b.OrganizationID, ApplicationID: a.ID, BindingID: b.ID, BindingVersion: b.Version, Name: "orch-app-" + strings.ReplaceAll(a.ID, "-", ""), Marker: ids.New(), Status: "PENDING", Revision: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	var wg sync.WaitGroup
	results := make(chan delivery.ApplicationRepository, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate := pending
			candidate.ID = ids.New()
			candidate.Marker = ids.New()
			r, e := st.ReserveApplicationDeliveryRepository(ctx, candidate)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var allocation delivery.ApplicationRepository
	for r := range results {
		if allocation.ID != "" && !r.SameAllocation(allocation) {
			t.Fatal("concurrent environments allocated different repositories")
		}
		allocation = r
	}
	foreign := pending
	foreign.OrganizationID = ids.New()
	if _, err := st.ReserveApplicationDeliveryRepository(ctx, foreign); err == nil {
		t.Fatal("foreign allocation")
	}
	if _, err := st.GetApplicationDeliveryRepository(ctx, foreign.OrganizationID, a.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("foreign read", err)
	}
	ack := allocation
	ack.Status = "READY"
	ack.RemoteID = "12345"
	ack.CloneURL = "https://github.com/platform/" + ack.Name + ".git"
	ack.Branch = "trunk"
	if _, err := st.AcknowledgeApplicationDeliveryRepository(ctx, ack, 99); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale acknowledgement", err)
	}
	ready, err := st.AcknowledgeApplicationDeliveryRepository(ctx, ack, 1)
	if err != nil || ready.Revision != 2 {
		t.Fatal("acknowledgement", err)
	}
	again, err := st.AcknowledgeApplicationDeliveryRepository(ctx, ack, 1)
	if err != nil || again != ready {
		t.Fatal("lost acknowledgement retry", err)
	}
	replaced := ready
	replaced.RemoteID = "54321"
	if _, err := st.AcknowledgeApplicationDeliveryRepository(ctx, replaced, 2); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatal("replacement adopted", err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ID: ids.New(), ApplicationKey: a.Key, Key: "staging", Name: "staging", Type: "staging", NamespaceIdentity: "managed-staging"}); err != nil {
		t.Fatal(err)
	}
	op, err := st.ClaimEnvironment(ctx, persistence.OperationClaim{ID: ids.New(), ApplicationKey: a.Key, EnvironmentKey: "staging", Owner: "repository-test", Kind: environment.OpDeploy, Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	if err := st.ReleaseOperation(ctx, owner, environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	stale := persistence.WithOwner(ctx, owner)
	if _, err := st.ReserveApplicationDeliveryRepository(stale, pending); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("stale owner reserved", err)
	}
	if _, err := st.AcknowledgeApplicationDeliveryRepository(stale, ready, 2); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("stale owner acknowledged", err)
	}

	return ready
}

func AssertManagedDelivery(t *testing.T, st persistence.DeliveryPlatformRepository, r delivery.ApplicationRepository) {
	t.Helper()
	ctx := context.Background()
	got, err := st.GetApplicationDeliveryRepository(ctx, r.OrganizationID, r.ApplicationID)
	if err != nil || got != r {
		t.Fatal("allocation restart", err)
	}
	d, err := st.GetPlatformDeliveryDefault(ctx, r.OrganizationID)
	if err != nil || d.BindingID != r.BindingID || d.BindingVersion != r.BindingVersion || d.Revision != 1 {
		t.Fatal("default restart", err)
	}
}
