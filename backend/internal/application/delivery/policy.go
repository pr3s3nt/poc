package delivery

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
	"sort"
)

// UpdatePolicy republishes the exact admitted business artifacts. It performs
// no Terraform invocation, source fetch, template rendering or ownership handoff.
func (c *Coordinator) UpdatePolicy(ctx context.Context, orgKey string, env environment.Environment, stage func(string, map[string]any) error) error {
	applied, desired := env.AppliedDelivery.Effective(), env.DesiredDelivery.Effective()
	a, b := applied, desired
	a.SelfHeal, b.SelfHeal = false, false
	if a != b || desired.Mode != domain.CD || applied.SelfHeal == desired.SelfHeal {
		return domain.ErrInvalid
	}
	owner, ok := persistence.OwnerFrom(ctx)
	if !ok {
		return persistence.ErrOperationLost
	}
	org, err := c.Store.GetOrganization(ctx, orgKey)
	if err != nil {
		return err
	}
	records, err := c.Store.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return err
	}
	scoped := []persistence.DeliveryExecution{}
	workloads := map[string]bool{}
	for _, e := range records {
		if e.Revision.Scope.TargetGeneration == env.TargetGeneration && e.Revision.Scope.Epoch == env.DeliveryEpoch {
			scoped = append(scoped, e)
			workloads[e.WorkloadID] = true
		}
	}
	grouped := map[string]DeployRequest{}
	// Validate all source identities and artifacts before the first publication.
	for workload := range workloads {
		active, err := activeExecutions(scoped, workload)
		if err != nil {
			return err
		}
		for _, source := range active {
			binding, provider, err := c.sourceProvider(ctx, source)
			if err != nil {
				return err
			}
			validator, ok := provider.(port.InventoryValidator)
			if !ok {
				return domain.ErrUnsupported
			}
			exporter, ok := provider.(port.ArtifactExporter)
			if !ok {
				return domain.ErrUnsupported
			}
			if len(source.Inventory) == 0 {
				return ErrBaselineRequired
			}
			intent := port.Intent{ID: source.Mutation.ID, OrganizationKey: orgKey, Owner: port.Owner{OperationID: owner.OperationID, Instance: owner.Owner, Fence: owner.Fence}, Binding: binding, Revision: source.Revision, Target: source.Target}
			if err := validator.ValidateInventory(ctx, intent, source.Receipt, source.Inventory); err != nil {
				return err
			}
			raw, err := c.providerArtifact(ctx, source)
			if err != nil {
				return err
			}
			manifests, err := exporter.ExportArtifact(ctx, raw)
			if err != nil {
				return err
			}
			request, exists := grouped[workload]
			if !exists {
				request = DeployRequest{PolicyOnly: true, OrganizationKey: orgKey, ApplicationKey: env.ApplicationKey, Environment: env, WorkloadID: workload, DeploymentID: owner.OperationID + "-policy", Target: source.Target}
			}
			request.Manifests = append(request.Manifests, manifests...)
			grouped[workload] = request
		}
	}
	if len(grouped) == 0 {
		return ErrBaselineRequired
	}
	keys := []string{}
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := stage("DELIVERY_POLICY", map[string]any{"workload": key, "deliveryPolicyOnly": true}); err != nil {
			return err
		}
		if err := c.Deploy(ctx, grouped[key]); err != nil {
			return err
		}
	}
	return c.Activate(ctx, env.ApplicationKey, env)
}
