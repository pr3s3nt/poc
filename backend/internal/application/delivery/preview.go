package delivery

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/ports/persistence"
	"sort"
)

// PreviewPin contains only safe verification metadata. Account and provider
// configuration stays private; immutable binding version/config is referenced by
// the selection. Read through the caller's snapshot, never an outer store.
type PreviewPin struct {
	Revision           int64  `json:"revision"`
	CapabilitiesDigest string `json:"capabilitiesDigest"`
	BaselineDigest     string `json:"baselineDigest,omitempty"`
}

func PinForPreview(ctx context.Context, st persistence.Store, orgKey string, env environment.Environment) (PreviewPin, error) {
	selection := env.DesiredDelivery.Effective()
	var pin PreviewPin
	if selection != env.AppliedDelivery.Effective() {
		repo, ok := st.(persistence.DeliveryExecutionRepository)
		if !ok {
			return pin, ErrUnavailable
		}
		org, err := st.GetOrganization(ctx, orgKey)
		if err != nil {
			return pin, err
		}
		records, err := repo.ListDeliveryExecutions(ctx, org.ID, env.ID)
		if err != nil {
			return pin, err
		}
		current := []persistence.DeliveryExecution{}
		for _, record := range records {
			if record.Revision.Scope.TargetGeneration == env.TargetGeneration && record.Revision.Scope.Epoch == env.DeliveryEpoch {
				current = append(current, record)
			}
		}
		sort.Slice(current, func(i, j int) bool { return current[i].Mutation.ID < current[j].Mutation.ID })
		pin.BaselineDigest, err = canon.Hash(current)
		if err != nil {
			return pin, err
		}
	}
	if selection.Mode == domain.Direct {
		return pin, nil
	}
	repo, ok := st.(persistence.DeliveryBindingRepository)
	if !ok {
		return PreviewPin{}, ErrUnavailable
	}
	org, err := st.GetOrganization(ctx, orgKey)
	if err != nil {
		return PreviewPin{}, err
	}
	b, err := repo.GetDeliveryBinding(ctx, org.ID, selection.BindingID, selection.BindingVersion)
	if err != nil {
		return PreviewPin{}, err
	}
	if b.Status != domain.BindingReady {
		return PreviewPin{}, domain.ErrUnsupported
	}
	if err := b.Capabilities.ValidateSelection(selection); err != nil {
		return PreviewPin{}, err
	}
	digest, err := canon.Hash(b.Capabilities)
	pin.Revision, pin.CapabilitiesDigest = b.Revision, digest
	return pin, err
}

// TransferUnit is the safe source inventory a user reviews before handing off
// management. It excludes private artifact/credential/provider handles.
type TransferUnit struct {
	WorkloadID string       `json:"workloadId"`
	Phase      domain.Phase `json:"phase"`
	Objects    int          `json:"objects"`
	Revision   string       `json:"revision"`
}

func TransferPreview(ctx context.Context, st persistence.Store, orgKey string, env environment.Environment, workloads []string) ([]TransferUnit, error) {
	repo, ok := st.(persistence.DeliveryExecutionRepository)
	if !ok {
		return nil, ErrUnavailable
	}
	org, err := st.GetOrganization(ctx, orgKey)
	if err != nil {
		return nil, err
	}
	records, err := repo.ListDeliveryExecutions(ctx, org.ID, env.ID)
	if err != nil {
		return nil, err
	}
	scoped := []persistence.DeliveryExecution{}
	ids := map[string]bool{}
	for _, e := range records {
		if e.Revision.Scope.TargetGeneration == env.TargetGeneration && e.Revision.Scope.Epoch == env.DeliveryEpoch {
			scoped = append(scoped, e)
			ids[e.WorkloadID] = true
		}
	}
	covered := map[string]bool{}
	units := []TransferUnit{}
	for id := range ids {
		active, err := activeExecutions(scoped, id)
		if err != nil {
			return nil, err
		}
		for _, e := range active {
			if len(e.Inventory) == 0 || e.Receipt.RevisionID != e.Revision.ID {
				return nil, ErrBaselineRequired
			}
			covered[id] = true
			units = append(units, TransferUnit{id, e.Revision.Phase, len(e.Inventory), e.Revision.ID})
		}
	}
	for _, id := range workloads {
		if !covered[id] {
			return nil, ErrBaselineRequired
		}
	}
	if len(units) == 0 {
		return nil, ErrBaselineRequired
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].WorkloadID != units[j].WorkloadID {
			return units[i].WorkloadID < units[j].WorkloadID
		}
		return units[i].Phase < units[j].Phase
	})
	return units, nil
}
