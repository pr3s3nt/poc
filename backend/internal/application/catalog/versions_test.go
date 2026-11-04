package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	catalogapp "orchestrator/internal/application/catalogrelease"
	"orchestrator/internal/application/definitionversions"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

func TestDefinitionRevisionLifecycleKeepsEnvironmentsPinned(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	// Existing-cluster needs no provider calls and has a real scoped READY fixture account.
	d := resource.Definition{Key: "versioned-cluster", Name: "Cluster one", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, DriverAccount: opts.ConnectionKey, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}, Criteria: []resource.Criterion{{Class: "versioned"}}}
	saved, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, d)
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.GetDefinitionHistory(ctx, opts.OrganizationKey, d.Key)
	if err != nil || len(first.Versions) != 1 || first.ActiveID != first.Versions[0].ID {
		t.Fatal(first, err)
	}
	saved.Name = "Cluster two"
	if _, err := svc.UpdateResourceDefinition(ctx, opts.OrganizationKey, d.Key, saved.Version, saved); err != nil {
		t.Fatal(err)
	}
	h, _ := st.GetDefinitionHistory(ctx, opts.OrganizationKey, d.Key)
	if len(h.Versions) != 2 || h.ActiveID != first.ActiveID || h.Versions[0].Definition.Name != "Cluster one" {
		t.Fatal("history was overwritten or proposed auto-promoted", h)
	}
	// Promotion does not change already-frozen Environment policies.
	if err := svc.ChangeRevisionStatus(ctx, opts.OrganizationKey, d.Key, h.Versions[1].ID, h.Generation, false); err != nil {
		t.Fatal(err)
	}
	effective, pin, err := catalogapp.ResolveDefinitions(ctx, st, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range effective {
		if d.Key == saved.Key {
			found = true
			if d.Name != "Cluster one" {
				t.Fatal("promotion silently upgraded existing Environment")
			}
		}
	}
	if !found {
		t.Fatal("missing pinned family")
	}
	selection, _ := st.GetDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	env, _ := st.GetEnvironment(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err := definitionversions.Select(ctx, st, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey, d.Key, h.Versions[1].ID, env.Version, selection.Revision, effective, pin, false); err != nil {
		t.Fatal(err)
	}
	newer, _, err := catalogapp.ResolveDefinitions(ctx, st, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range newer {
		if d.Key == saved.Key && d.Name != "Cluster two" {
			t.Fatal("explicit selection ignored")
		}
	}
	selection, _ = st.GetDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if !selection.Pending() || selection.AppliedVerified {
		t.Fatal("selection claimed deployment")
	}
	if err := st.ApplyDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey, selection.Revision, newer, pin); !errors.Is(err, persistence.ErrOperationLost) {
		t.Fatal("unfenced activation", err)
	}
	lease, err := envops.NewManager(st).Begin(ctx, envops.Claim{ApplicationKey: opts.ApplicationKey, EnvironmentKey: opts.EnvironmentKey, Kind: environment.OpDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyDefinitionSelection(lease.Ctx, opts.ApplicationKey, opts.EnvironmentKey, selection.Revision, newer, pin); err != nil {
		t.Fatal(err)
	}
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	applied, _ := st.GetDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if applied.Pending() || !applied.AppliedVerified {
		t.Fatal("success did not activate applied policy")
	}
	// CAS and organization isolation remain strict.
	h, _ = st.GetDefinitionHistory(ctx, opts.OrganizationKey, d.Key)
	if err := svc.ChangeRevisionStatus(ctx, opts.OrganizationKey, d.Key, h.Versions[0].ID, h.Generation-1, true); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale policy mutation", err)
	}
	if err := svc.ChangeRevisionStatus(ctx, "other-org", d.Key, h.Versions[0].ID, 0, true); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("cross-org revision access", err)
	}
	if err := svc.ChangeRevisionStatus(ctx, opts.OrganizationKey, d.Key, h.Versions[0].ID, h.Generation, true); err != nil {
		t.Fatal(err)
	}
	h, _ = st.GetDefinitionHistory(ctx, opts.OrganizationKey, d.Key)
	if err := svc.ChangeRevisionStatus(ctx, opts.OrganizationKey, d.Key, h.Versions[0].ID, h.Generation, false); !errors.Is(err, definitionversions.ErrInvalid) {
		t.Fatal("archived promoted", err)
	}
	// Existing archived snapshots are immutable and remain retained.
	raw, _ := json.Marshal(h)
	var bad persistence.DefinitionHistory
	json.Unmarshal(raw, &bad)
	bad.Versions[0].Definition.Name = "tampered"
	bad.Generation++
	if err := st.SaveDefinitionHistory(ctx, opts.OrganizationKey, h.Generation, bad); !errors.Is(err, persistence.ErrImmutable) {
		t.Fatal("immutable revision overwritten", err)
	}
}

func TestNewDefinitionDoesNotChangeLegacyRunningEnvironment(t *testing.T) {
	ctx := context.Background()
	st, svc, opts := seededCatalog(t, withRegistry)
	if err := st.UpsertWorkloadInstance(ctx, deployment.WorkloadInstance{EnvironmentKey: opts.ApplicationKey + "/" + opts.EnvironmentKey, WorkloadID: "legacy", LastDeploymentID: "missing-historical-plan"}); err != nil {
		t.Fatal(err)
	}
	d := resource.Definition{Key: "new-default", Name: "New default", ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster, DriverAccount: opts.ConnectionKey, DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{}}}, Criteria: []resource.Criterion{{Class: "new-default"}}}
	if _, err := svc.RegisterResourceDefinition(ctx, opts.OrganizationKey, d); err != nil {
		t.Fatal(err)
	}
	selected, err := st.GetDefinitionSelection(ctx, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil || !selected.Frozen || selected.AppliedVerified {
		t.Fatal("legacy policy not frozen", err)
	}
	for _, old := range selected.Desired {
		if old.Key == d.Key {
			t.Fatal("new default entered legacy policy")
		}
	}
	defs, _, err := catalogapp.ResolveDefinitions(ctx, st, opts.OrganizationKey, opts.ApplicationKey, opts.EnvironmentKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range defs {
		if old.DriverType == resource.DriverTemplate && (old.RenderPin == nil || old.RenderPin.ID != "unverified") {
			t.Fatal("missing historical evidence silently used the current renderer")
		}
	}
}
