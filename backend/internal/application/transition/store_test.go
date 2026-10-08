package transition_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/adapters/configmemory"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/transition"
	configdomain "orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
)

// withSecret selects a store, writes a Secret and deploys the backend with a
// workload reference to it through the normal Preview/Deploy path, so the
// workload really reads store-backed configuration.
func (h *harness) withSecret() *configmemory.Registry {
	h.t.Helper()
	ctx := context.Background()
	for _, key := range []string{"vault-a", "vault-b"} {
		if err := h.app.Store.CreateSecretStore(ctx, secretstore.Store{Key: key, OrganizationKey: h.opts.OrganizationKey, Name: key, Provider: secretstore.ProviderVaultKV2, Status: secretstore.StatusReady,
			BackendAddress: "http://" + key + ".example:8200", WorkloadAddress: "http://" + key + ".vault.svc:8200", Mount: "kv", AuthMount: "kubernetes-" + key}); err != nil {
			h.t.Fatal(err)
		}
	}
	registry := h.app.StoreRegistry.(*configmemory.Registry)
	env := h.env()
	scope, _ := h.app.Store.GetConfigurationScope(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if _, err := h.app.Configurations.SetSecretStore(ctx, configSwitch(h, "vault-a", env.Version, scope.Version)); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.app.Configurations.Put(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey, "API_TOKEN", configdomain.Secret, "super-secret-token-value", 0); err != nil {
		h.t.Fatal(err)
	}
	original := cloneScore(h.scores["backend"])
	backend := cloneScore(h.scores["backend"])
	main := backend["containers"].(map[string]any)["main"].(map[string]any)
	main["variables"].(map[string]any)["API_TOKEN"] = "${resources.env.API_TOKEN}"
	backend["resources"].(map[string]any)["env"] = map[string]any{"type": "environment"}
	scope, _ = h.app.Store.GetConfigurationScope(ctx, h.opts.ApplicationKey, h.opts.EnvironmentKey)
	if _, err := h.app.Deployments.DeployWorkload(ctx, appsvc.DeployCommand{
		OrganizationKey: h.opts.OrganizationKey, ApplicationKey: h.opts.ApplicationKey, EnvironmentKey: h.opts.EnvironmentKey,
		WorkloadID: "backend", ScoreBefore: original, ScoreAfter: backend, Actor: "tester", RunID: "run-secret", ConfigRevisionID: scope.DesiredRevisionID,
	}); err != nil {
		h.t.Fatalf("deploy with a secret: %v", err)
	}
	return registry
}

func TestPreflightChecksTheSelectedStoreAuthBeforeAnyWriterStops(t *testing.T) {
	h := newHarness(t)
	registry := h.withSecret()
	h.seedJobs(2)
	registry.FailWorkloadAuth("vault-a", true)
	calls := len(h.cluster.Calls())
	_, err := h.service.Preview(context.Background(), h.request(environment.ModeMigratePostgres))
	var unsupported *transition.UnsupportedError
	if !errors.As(err, &unsupported) || !strings.Contains(err.Error(), "Kubernetes auth") {
		t.Fatalf("store auth failure must stop the transition in preflight: %v", err)
	}
	for _, line := range h.cluster.Calls()[calls:] {
		if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") {
			t.Fatalf("writers stopped before the auth check passed: %s", line)
		}
	}
}

func TestDestinationProbeFailureStopsBeforeQuiesce(t *testing.T) {
	h := newHarness(t)
	h.seedJobs(2)
	h.cluster.Inject("probe:fake-second-context", errors.New("cluster unreachable with row 'secret'"))
	calls := len(h.cluster.Calls())
	_, err := h.service.Preview(context.Background(), h.request(environment.ModeMigratePostgres))
	if !errors.Is(err, transition.ErrDestinationFailed) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("destination probe: %v", err)
	}
	for _, line := range h.cluster.Calls()[calls:] {
		if strings.HasPrefix(line, "scale") || strings.HasPrefix(line, "backup") {
			t.Fatalf("side effect after failed probe: %s", line)
		}
	}
}

func TestTransitionKeepsOldRefsAndDeliversFromTheSelectedStore(t *testing.T) {
	h := newHarness(t)
	registry := h.withSecret()
	h.seedJobs(3)
	ctx := context.Background()
	appliedBefore, _ := h.app.Store.ListWorkloadInstances(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey)
	var oldRevision string
	for _, instance := range appliedBefore {
		if instance.WorkloadID == "backend" {
			oldRevision = instance.AppliedConfigRevisionID
		}
	}
	if oldRevision == "" {
		t.Fatal("backend must have an applied revision")
	}
	accessBefore := len(registry.Access)
	p := h.preview(environment.ModeMigratePostgres)
	detail, err := h.execute(p, environment.ModeMigratePostgres, true)
	if err != nil || detail.Status != environment.TransitionSucceeded {
		t.Fatalf("%v %+v", err, detail)
	}
	if len(registry.Access) != accessBefore+1 {
		t.Fatalf("destination backend must receive exactly one delivery: %d -> %d", accessBefore, len(registry.Access))
	}
	if access := registry.Access[len(registry.Access)-1]; access.StoreKey != "vault-a" || access.Namespace != h.env().Namespace() {
		t.Fatalf("delivery request: %+v", access)
	}
	after, _ := h.app.Store.ListWorkloadInstances(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey)
	for _, instance := range after {
		if instance.WorkloadID != "backend" {
			continue
		}
		// The destination runs the same applied revision (no pending drift) and
		// pins its secret delivery metadata per store.
		delivery, _ := instance.TargetRef["secretDelivery"].(map[string]any)
		if instance.AppliedConfigRevisionID != oldRevision || delivery["storeKey"] != "vault-a" || delivery["address"] != "http://vault-a.vault.svc:8200" {
			t.Fatalf("destination delivery pin: %+v", instance)
		}
	}
	old, _ := h.app.Store.ListWorkloadInstancesFor(ctx, h.opts.ApplicationKey+"/"+h.opts.EnvironmentKey, 0)
	for _, instance := range old {
		if instance.WorkloadID == "backend" && instance.AppliedConfigRevisionID != oldRevision {
			t.Fatalf("source revision rewritten: %+v", instance)
		}
	}
	// Nothing leaks into the persisted record.
	stored, _ := h.app.Store.GetTransition(ctx, detail.ID)
	if strings.Contains(strings.Join(stored.Compensation, " "), "super-secret") {
		t.Fatal("transition record carries a secret")
	}
	_ = ids.New
	_ = configport.ErrNoStore
}

func cloneScore(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
