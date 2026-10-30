package drift

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/envops"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/persistence/persistencetest"
	"orchestrator/internal/ports/terraformstate"
)

type scanFixture struct {
	instance tfresource.Instance
	baseline terraformstate.ExecutionBaseline
	reads    int
	report   domain.Report
	err      error
}

func (f *scanFixture) ListInstances(context.Context, string) ([]tfresource.Instance, error) {
	return []tfresource.Instance{f.instance}, nil
}
func (f *scanFixture) GetInstance(context.Context, string) (tfresource.Instance, error) {
	return f.instance, nil
}
func (f *scanFixture) LatestExecutionBaseline(context.Context, string, string) (terraformstate.ExecutionBaseline, error) {
	if f.baseline.ID == "" {
		return f.baseline, persistence.ErrNotFound
	}
	return f.baseline, nil
}
func (f *scanFixture) InspectInstance(ctx context.Context, _, _ string) (domain.Report, error) {
	if _, ok := persistence.OwnerFrom(ctx); !ok {
		return domain.Report{}, persistence.ErrOperationLost
	}
	f.reads++
	return f.report, f.err
}
func TestScanWorkerStoresSafeResultsWithoutChangingDeploymentAndSurvivesReopen(t *testing.T) {
	for _, scenario := range []string{"clean", "external-update", "missing-baseline", "provider-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "snapshot.json")
			st, err := store.NewWithSnapshot(path)
			if err != nil {
				t.Fatal(err)
			}
			binding := persistencetest.DeliveryBindings(t, st)
			persistencetest.DeliverySelections(t, st, binding)
			env, err := st.GetEnvironment(ctx, "delivery-app", "staging")
			if err != nil {
				t.Fatal(err)
			}
			fixture := &scanFixture{instance: tfresource.Instance{ID: ids.New(), Identity: tfresource.Identity{OrganizationID: binding.OrganizationID, EnvironmentID: env.ID, TargetGeneration: env.TargetGeneration, Descriptor: "redis.in-cluster#cache"}, Version: 2, Status: tfresource.InstanceReady, CreatedAt: time.Now().Add(-time.Hour)}, baseline: terraformstate.ExecutionBaseline{ID: ids.New()}, report: domain.Report{Complete: true, PrivateChangeDigest: strings.Repeat("a", 64)}}
			expected := domain.InSync
			switch scenario {
			case "external-update":
				fixture.report.External = []domain.Finding{{Address: "resource.cache", Type: "cache", Actions: []string{"update"}, Paths: []string{"spec.replicas"}}}
				expected = domain.Drifted
			case "missing-baseline":
				fixture.baseline.ID = ""
				expected = domain.BaselineUnavailable
			case "provider-unavailable":
				fixture.err = errors.New("private provider diagnostic must never escape")
				expected = domain.Unknown
			}
			service := &Service{Store: st, Instances: fixture, Inspector: fixture, Operations: envops.NewManager(st)}
			command := QueueCommand{OrganizationKey: "delivery-org", ApplicationKey: "delivery-app", EnvironmentKey: "staging", RequestID: ids.New(), ExpectedEnvironmentVersion: env.Version}
			queued, err := service.Queue(ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.reads != 0 {
				t.Fatal("queue performed provider read")
			}
			service.ProcessQueued(ctx)
			row, err := service.Get(ctx, command.OrganizationKey, command.ApplicationKey, command.EnvironmentKey, queued.ID)
			if err != nil || row.Result != expected || !row.Status.Terminal() {
				t.Fatal("unsafe scan classification", row.Result, row.Status, err)
			}
			if scenario == "missing-baseline" && fixture.reads != 0 {
				t.Fatal("missing baseline read today's provider inputs")
			}
			public, _ := json.Marshal(row)
			if strings.Contains(string(public), fixture.report.PrivateChangeDigest) || strings.Contains(string(public), "private provider diagnostic") {
				t.Fatal("private material escaped scan result")
			}
			before := fixture.reads
			if _, err := service.List(ctx, command.OrganizationKey, command.ApplicationKey, command.EnvironmentKey, "", 50); err != nil {
				t.Fatal(err)
			}
			if fixture.reads != before {
				t.Fatal("stored GET invoked provider")
			}
			after, err := st.GetEnvironment(ctx, command.ApplicationKey, command.EnvironmentKey)
			if err != nil || after.CurrentDeploymentSetID != env.CurrentDeploymentSetID || after.DraftVersion != env.DraftVersion || after.Busy() {
				t.Fatal("scan changed deployment/draft or leaked hold", err)
			}
			reopened, err := store.NewWithSnapshot(path)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := reopened.GetDriftScan(ctx, binding.OrganizationID, env.ID, queued.ID)
			if err != nil || loaded.Result != expected || loaded.RequestDigest != queued.RequestDigest {
				t.Fatal("scan lost across restart", err)
			}
			duplicate, err := service.Queue(ctx, command)
			if err != nil || duplicate.ID != row.ID || fixture.reads != before {
				t.Fatal("request retried provider execution", err)
			}
		})
	}
}
