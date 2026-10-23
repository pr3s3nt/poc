package deploymentreview

import (
	"context"
	"encoding/json"
	"errors"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
)

// CapturePrevious runs inside the planning read snapshot. It walks exact
// successful per-node associations newest first; a later failed deployment
// cannot replace a successful baseline. If the newest successful association
// lacks safe review, it stays unavailable instead of falling back to an older
// (and now incorrect) artifact.
func CapturePrevious(ctx context.Context, st persistence.Store, organizationKey, applicationKey, environmentKey string, generation int64, active []resource.ActiveResource) (map[string][]Artifact, error) {
	out := map[string][]Artifact{}
	activeByID := map[string]resource.ActiveResource{}
	for _, a := range active {
		if a.OrganizationKey == organizationKey && a.Status != resource.StatusDestroyed {
			activeByID[a.ID] = a
		}
	}
	list, err := st.ListDeployments(ctx, applicationKey, environmentKey)
	if err != nil {
		return nil, err
	}
	resourceSeen, workloadSeen := map[string]bool{}, map[string]bool{}
	for _, d := range list {
		if d.OrganizationKey != organizationKey || d.ApplicationKey != applicationKey || d.EnvironmentKey != environmentKey || d.TargetGeneration != generation {
			continue
		}
		var review *Review
		plan, err := st.GetPlan(ctx, d.ID)
		if err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			if raw, ok := plan["review"]; ok {
				data, marshalErr := json.Marshal(raw)
				if marshalErr != nil {
					return nil, marshalErr
				}
				var parsed Review
				if json.Unmarshal(data, &parsed) == nil && parsed.Version == 1 {
					review = &parsed
				}
			}
		}
		resources, err := st.ListDeploymentResources(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			current, active := activeByID[r.ActiveResourceID]
			if resourceSeen[r.NodeDescriptor] || r.Status != domain.ResourceReady || !active || current.Descriptor.String() != r.NodeDescriptor {
				continue
			}
			if review != nil {
				for _, node := range review.Nodes {
					if node.ID == r.NodeDescriptor && node.Scope != "" && node.Scope != current.Scope.Key() {
						active = false
					}
				}
				if !active {
					continue
				}
			}
			resourceSeen[r.NodeDescriptor] = true
			if review != nil {
				for _, a := range review.Artifacts {
					if a.NodeID == r.NodeDescriptor {
						a.After = baselineSide(a.After, d.ID)
						out[a.NodeID] = append(out[a.NodeID], a)
					}
				}
			}
		}
		workloads, err := st.ListDeploymentWorkloads(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		for _, w := range workloads {
			if workloadSeen[w.WorkloadID] || w.Status != domain.InstanceReady {
				continue
			}
			workloadSeen[w.WorkloadID] = true
			if review == nil || w.ManifestReview == nil {
				continue
			}
			data, marshalErr := json.Marshal(w.ManifestReview)
			if marshalErr != nil {
				return nil, marshalErr
			}
			var captured struct {
				Version   int        `json:"version"`
				PlanHash  string     `json:"planHash"`
				Artifacts []Artifact `json:"artifacts"`
			}
			if json.Unmarshal(data, &captured) != nil || captured.Version != 1 || captured.PlanHash != review.PlanHash || review.PlanHash == "" {
				continue
			}
			for _, n := range review.Nodes {
				if n.Kind == "workload" && n.Name == w.WorkloadID {
					for _, a := range captured.Artifacts {
						if a.NodeID != n.ID || a.Kind != "kubernetes-manifest" {
							continue
						}
						a.After = baselineSide(a.After, d.ID)
						out[n.ID] = append(out[n.ID], a)
					}
				}
			}
		}
	}
	return out, nil
}

func baselineSide(side Side, deploymentID string) Side {
	copy := map[string]string{}
	for key, value := range side.Provenance {
		copy[key] = value
	}
	copy["baselineDeploymentId"] = deploymentID
	side.Provenance = copy
	return side
}
