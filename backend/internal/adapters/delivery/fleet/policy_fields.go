package fleet

import (
	"context"
	"encoding/json"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

func (p *Provider) reclaimPolicyFields(ctx context.Context, cli k8s.CLI, cfg Config, i port.Intent, pkg Package, inventory []port.AppliedObject, sourceRevision string, guard func(context.Context, port.Intent) error) error {
	if !i.PolicyOnly || !i.Revision.Selection.SelfHeal || i.Revision.Phase == domain.Job || len(inventory) == 0 {
		return nil
	}
	raw, err := json.Marshal(pkg)
	if err != nil {
		return ErrConfig
	}
	manifests, err := p.ExportArtifact(ctx, raw)
	if err != nil {
		return err
	}
	objects := map[string]execution.Manifest{}
	for _, m := range manifests {
		objects[(execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name}).Key()] = m
	}
	for _, object := range inventory {
		m, exists := objects[object.Key()]
		if !exists {
			continue
		}
		if err := k8s.ReclaimManualDeliveryFields(ctx, cli, i.Target, object.ObjectRef, object.UID, k8s.DeliveryManager{Release: pkg.ReleaseName, Namespace: cfg.ReleaseNamespace}, m.Object, func(ctx context.Context) error { return guard(ctx, i) }, sourceRevision, i.Revision.ID); err != nil {
			return err
		}
	}
	return nil
}
