package deployment

import (
	"context"
	"errors"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

// RedactedValue replaces every secret output in a deployment view (UC-09 BR-03).
const RedactedValue = "***redacted***"

// ResourceView is one Active Resource row of the deployment view.
type ResourceView struct {
	Descriptor     string         `json:"descriptor"`
	ResourceType   string         `json:"resourceType"`
	DefinitionKey  string         `json:"definitionKey"`
	Status         string         `json:"status"`
	BatchIndex     int            `json:"batchIndex"`
	Scope          map[string]any `json:"scope,omitempty"`
	Outputs        map[string]any `json:"outputs"`
	ResolvedInputs map[string]any `json:"resolvedInputs,omitempty"`
}

// WorkloadView is one workload instance row of the deployment view.
type WorkloadView struct {
	WorkloadID       string         `json:"workloadId"`
	Status           string         `json:"status"`
	TargetRef        map[string]any `json:"targetRef"`
	ManifestDigest   string         `json:"manifestDigest"`
	LastDeploymentID string         `json:"lastDeploymentId"`
	ObservedAt       string         `json:"observedAt"`
}

// View is the read-only UC-09 deployment view.
type View struct {
	Deployment     domain.Deployment    `json:"deployment"`
	DeploymentSet  environment.Document `json:"deploymentSet"`
	SetID          string               `json:"deploymentSetId"`
	Delta          any                  `json:"delta"`
	Graph          any                  `json:"graph"`
	Matches        any                  `json:"matches"`
	Batches        any                  `json:"batches"`
	Classification any                  `json:"classification"`
	PlanHash       string               `json:"planHash"`
	Resources      []ResourceView       `json:"resources"`
	Workloads      []WorkloadView       `json:"workloads"`
}

// QueryService implements UC-09: assemble a read-only deployment view.
type QueryService struct {
	store persistence.Store
}

// NewQueryService wires UC-09 with the read ports.
func NewQueryService(store persistence.Store) *QueryService { return &QueryService{store: store} }

// ListDeployments returns deployment records, newest first.
func (q *QueryService) ListDeployments(ctx context.Context, applicationKey, environmentKey string) ([]domain.Deployment, error) {
	return q.store.ListDeployments(ctx, applicationKey, environmentKey)
}

// GetDeployment assembles the deployment view and redacts secret outputs (OC-11).
func (q *QueryService) GetDeployment(ctx context.Context, deploymentID string) (*View, error) {
	record, err := q.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	view := &View{Deployment: record, DeploymentSet: environment.NewDocument()}

	setID := record.CandidateDeploymentSet
	if record.Status != domain.StatusSucceeded && setID == "" {
		setID = record.BaseDeploymentSetID
	}
	if setID != "" {
		set, err := q.store.GetDeploymentSet(ctx, setID)
		if err != nil && !isNotFound(err) {
			return nil, err
		}
		if err == nil {
			view.DeploymentSet = set.Document
			view.SetID = set.ID
		}
	}

	plan, err := q.store.GetPlan(ctx, deploymentID)
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	if plan != nil {
		view.Delta = plan["delta"]
		view.Graph = plan["graph"]
		view.Matches = plan["matches"]
		view.Batches = plan["batches"]
		view.Classification = plan["classification"]
		if h, ok := plan["planHash"].(string); ok {
			view.PlanHash = h
		}
	}

	resources, err := q.store.ListDeploymentResources(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	types, err := q.store.ListResourceTypes(ctx)
	if err != nil {
		return nil, err
	}
	secretsByType := map[string]map[string]bool{}
	for _, t := range types {
		fields := map[string]bool{}
		for _, name := range t.SecretOutputs() {
			fields[name] = true
		}
		secretsByType[t.Key] = fields
	}
	for _, r := range resources {
		view.Resources = append(view.Resources, ResourceView{
			Descriptor:     r.NodeDescriptor,
			ResourceType:   r.ResourceTypeKey,
			DefinitionKey:  r.DefinitionKey,
			Status:         string(r.Status),
			BatchIndex:     r.BatchIndex,
			Outputs:        redact(r.OutputSnapshot, secretsByType[r.ResourceTypeKey]),
			ResolvedInputs: r.ResolvedInputs,
		})
	}

	instances, err := q.store.ListWorkloadInstances(ctx, record.ApplicationKey+"/"+record.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	for _, w := range instances {
		view.Workloads = append(view.Workloads, WorkloadView{
			WorkloadID:       w.WorkloadID,
			Status:           string(w.Status),
			TargetRef:        w.TargetRef,
			ManifestDigest:   w.ManifestDigest,
			LastDeploymentID: w.LastDeploymentID,
			ObservedAt:       w.ObservedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	if view.Resources == nil {
		view.Resources = []ResourceView{}
	}
	if view.Workloads == nil {
		view.Workloads = []WorkloadView{}
	}
	return view, nil
}

// redact removes secret output values and any stored secret reference.
func redact(outputs map[string]any, secretFields map[string]bool) map[string]any {
	out := map[string]any{}
	for name, value := range outputs {
		if secretFields[name] {
			out[name] = RedactedValue
			continue
		}
		if m, ok := value.(map[string]any); ok {
			if _, hasRef := m["secretRef"]; hasRef {
				out[name] = RedactedValue
				continue
			}
		}
		out[name] = value
	}
	return out
}

func isNotFound(err error) bool {
	return errors.Is(err, persistence.ErrNotFound)
}
