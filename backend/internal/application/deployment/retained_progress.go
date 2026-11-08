package deployment

import (
	"context"
	"reflect"
	"strings"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/ports/persistence"
)

// History comes from the run's base-set ancestry, never today's instance state.
func retainedWorkloadProgress(ctx context.Context, st persistence.Store, record domain.Deployment, review any, current []domain.WorkloadSnapshot) ([]domain.WorkloadSnapshot, error) {
	out := append([]domain.WorkloadSnapshot(nil), current...)
	present := map[string]bool{}
	for _, w := range current {
		present[w.WorkloadID] = true
	}
	retained := map[string]bool{}
	if m, ok := review.(map[string]any); ok {
		nodes, _ := m["nodes"].([]any)
		for _, raw := range nodes {
			n, _ := raw.(map[string]any)
			intent, _ := n["intent"].(string)
			id, _ := n["id"].(string)
			if strings.HasPrefix(id, "workload.") && (intent == "RETAIN" || intent == "BIND") {
				_, wid, ok := strings.Cut(id, "#modules.")
				if ok {
					retained[wid] = true
				}
			}
		}
	}
	if record.BaseDeploymentSetID == "" {
		return out, nil
	}
	base, err := st.GetDeploymentSet(ctx, record.BaseDeploymentSetID)
	if isNotFound(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for wid := range base.Document.Modules {
		if !present[wid] && retained[wid] {
			wanted[wid] = true
		}
	}
	if len(wanted) == 0 || base.EnvironmentID != record.EnvironmentID {
		return out, nil
	}
	history, err := st.ListDeployments(ctx, record.ApplicationKey, record.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	setID := record.BaseDeploymentSetID
	seen := map[string]bool{}
	for setID != "" && !seen[setID] && len(wanted) > 0 {
		seen[setID] = true
		var prior *domain.Deployment
		for i := range history {
			d := &history[i]
			if d.ID != record.ID && d.Status == domain.StatusSucceeded && d.CandidateDeploymentSet == setID && d.OrganizationKey == record.OrganizationKey && d.ApplicationKey == record.ApplicationKey && d.EnvironmentKey == record.EnvironmentKey && d.EnvironmentID == record.EnvironmentID && d.TargetGeneration == record.TargetGeneration && !d.StartedAt.After(record.StartedAt) {
				if prior != nil {
					return out, nil
				}
				prior = d
			}
		}
		if prior == nil {
			break
		}
		set, err := st.GetDeploymentSet(ctx, setID)
		if isNotFound(err) {
			break
		}
		if err != nil {
			return nil, err
		}
		if set.EnvironmentID != record.EnvironmentID {
			break
		}
		for wid := range wanted {
			module, ok := set.Document.Modules[wid]
			if !ok || !reflect.DeepEqual(module, base.Document.Modules[wid]) {
				delete(wanted, wid)
			}
		}
		if wanted[prior.WorkloadID] {
			snapshots, err := st.ListDeploymentWorkloads(ctx, prior.ID)
			if err != nil {
				return nil, err
			}
			var proof *domain.WorkloadSnapshot
			for i := range snapshots {
				w := &snapshots[i]
				if w.WorkloadID == prior.WorkloadID {
					if proof != nil {
						proof = nil
						break
					}
					proof = w
				}
			}
			if proof != nil && proof.DeploymentID == prior.ID && proof.Status == domain.InstanceReady && proof.ManifestDigest != "" {
				out = append(out, *proof)
			}
			// Never skip a missing/failed fact to find an older success for this workload.
			delete(wanted, prior.WorkloadID)
		}
		setID = prior.BaseDeploymentSetID
	}
	return out, nil
}
