package deployment

import (
	"context"
	"errors"
	"fmt"

	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

// RedactedValue replaces every secret output in a deployment view (UC-09 BR-03).
const RedactedValue = "***redacted***"

// ResourceView is one Active Resource row of the deployment view.
type ResourceView struct {
	Descriptor    string         `json:"descriptor"`
	ResourceType  string         `json:"resourceType"`
	DefinitionKey string         `json:"definitionKey"`
	Status        string         `json:"status"`
	BatchIndex    int            `json:"batchIndex"`
	Scope         map[string]any `json:"scope,omitempty"`
	Outputs       map[string]any `json:"outputs"`
}

// WorkloadView is one workload instance row of the deployment view.
type WorkloadView struct {
	WorkloadID              string         `json:"workloadId"`
	Status                  string         `json:"status"`
	TargetRef               map[string]any `json:"targetRef"`
	ManifestDigest          string         `json:"manifestDigest"`
	LastDeploymentID        string         `json:"lastDeploymentId"`
	AppliedConfigRevisionID string         `json:"appliedConfigRevisionId,omitempty"`
	ObservedAt              string         `json:"observedAt"`
}

// View is the read-only UC-09 deployment view.
type View struct {
	Deployment     domain.Deployment     `json:"deployment"`
	DeploymentSet  environment.Document  `json:"deploymentSet"`
	SetID          string                `json:"deploymentSetId"`
	Delta          *domain.DeltaDocument `json:"delta"`
	DeltaHash      string                `json:"deltaDocumentHash,omitempty"`
	Graph          any                   `json:"graph"`
	Matches        any                   `json:"matches"`
	Batches        any                   `json:"batches"`
	Classification any                   `json:"classification"`
	PlanHash       string                `json:"planHash"`
	Resources      []ResourceView        `json:"resources"`
	Workloads      []WorkloadView        `json:"workloads"`
}

// QueryService implements UC-09: assemble a read-only deployment view.
type QueryService struct {
	store persistence.Store
}

// NewQueryService wires UC-09 with the read ports.
func NewQueryService(store persistence.Store) *QueryService { return &QueryService{store: store} }

// ErrInvalidStatus marks an unsupported history filter.
var ErrInvalidStatus = errors.New("deployment: invalid status")

type ListDeploymentsQuery struct {
	OrganizationKey string
	ApplicationKey  string
	EnvironmentKey  string
	Status          domain.Status
}

type GetDeploymentQuery struct {
	OrganizationKey string
	ApplicationKey  string
	EnvironmentKey  string
	DeploymentID    string
}

// ListDeployments validates the complete tenant scope and returns newest first.
func (q *QueryService) ListDeployments(ctx context.Context, query ListDeploymentsQuery) ([]domain.Deployment, error) {
	if query.Status != "" && !validStatus(query.Status) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, query.Status)
	}
	var out []domain.Deployment
	err := q.store.ReadSnapshot(ctx, func(ctx context.Context, st persistence.Store) error {
		if err := authorizeScope(ctx, st, query.OrganizationKey, query.ApplicationKey, query.EnvironmentKey); err != nil {
			return err
		}
		list, err := st.ListDeployments(ctx, query.ApplicationKey, query.EnvironmentKey)
		if err != nil {
			return err
		}
		out = make([]domain.Deployment, 0, len(list))
		for _, record := range list {
			if record.OrganizationKey != query.OrganizationKey || record.ApplicationKey != query.ApplicationKey || record.EnvironmentKey != query.EnvironmentKey {
				continue
			}
			if query.Status != "" && record.Status != query.Status {
				continue
			}
			record.FailureReason = SafeFailureReason(record.FailureReason)
			out = append(out, record)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetDeployment assembles the deployment view and redacts secret outputs
// (OC-11). Every read happens inside one read-only snapshot so the view never
// mixes state from different commits.
func (q *QueryService) GetDeployment(ctx context.Context, query GetDeploymentQuery) (*View, error) {
	var view *View
	err := q.store.ReadSnapshot(ctx, func(ctx context.Context, st persistence.Store) error {
		var err error
		view, err = assembleView(ctx, st, query)
		return err
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func assembleView(ctx context.Context, st persistence.Store, query GetDeploymentQuery) (*View, error) {
	if err := authorizeScope(ctx, st, query.OrganizationKey, query.ApplicationKey, query.EnvironmentKey); err != nil {
		return nil, err
	}
	record, err := st.GetDeployment(ctx, query.DeploymentID)
	if err != nil {
		return nil, err
	}
	if record.OrganizationKey != query.OrganizationKey || record.ApplicationKey != query.ApplicationKey || record.EnvironmentKey != query.EnvironmentKey {
		return nil, persistence.ErrNotFound
	}
	record.FailureReason = SafeFailureReason(record.FailureReason)
	view := &View{Deployment: record, DeploymentSet: environment.NewDocument()}

	setID := record.CandidateDeploymentSet
	if record.Status != domain.StatusSucceeded && setID == "" {
		setID = record.BaseDeploymentSetID
	}
	if setID != "" {
		set, err := st.GetDeploymentSet(ctx, setID)
		if err != nil && !isNotFound(err) {
			return nil, err
		}
		if err == nil {
			view.DeploymentSet = set.Document
			view.SetID = set.ID
		}
	}

	if record.DeltaSnapshotID != "" {
		snapshot, err := st.GetDeltaSnapshot(ctx, record.DeltaSnapshotID)
		if err != nil {
			return nil, err
		}
		view.Delta = &snapshot.Document
		view.DeltaHash = snapshot.DocumentHash
	}

	plan, err := st.GetPlan(ctx, record.ID)
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	if plan != nil {
		view.Graph = plan["graph"]
		view.Matches = plan["matches"]
		view.Batches = plan["batches"]
		view.Classification = plan["classification"]
		if h, ok := plan["planHash"].(string); ok {
			view.PlanHash = h
		}
	}

	resources, err := st.ListDeploymentResources(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	types, err := st.ListResourceTypes(ctx, record.OrganizationKey)
	if err != nil {
		return nil, err
	}
	// Only outputs explicitly declared non-secret by the Resource Type are
	// visible (BR-03). A missing Type or an undeclared output is redacted.
	visibleByType := map[string]map[string]bool{}
	for _, t := range types {
		fields := map[string]bool{}
		for _, field := range t.Outputs {
			if !field.Secret {
				fields[field.Name] = true
			}
		}
		visibleByType[t.Key] = fields
	}
	for _, r := range resources {
		view.Resources = append(view.Resources, ResourceView{
			Descriptor:    r.NodeDescriptor,
			ResourceType:  r.ResourceTypeKey,
			DefinitionKey: r.DefinitionKey,
			Status:        string(r.Status),
			BatchIndex:    r.BatchIndex,
			Outputs:       redact(r.OutputSnapshot, visibleByType[r.ResourceTypeKey]),
		})
	}

	instances, err := st.ListDeploymentWorkloads(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	for _, w := range instances {
		view.Workloads = append(view.Workloads, WorkloadView{
			WorkloadID:              w.WorkloadID,
			Status:                  string(w.Status),
			TargetRef:               w.TargetRef,
			ManifestDigest:          w.ManifestDigest,
			LastDeploymentID:        w.DeploymentID,
			AppliedConfigRevisionID: w.AppliedConfigRevisionID,
			ObservedAt:              w.ObservedAt.UTC().Format("2006-01-02T15:04:05Z"),
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

// authorizeScope reports a missing or foreign Application/Environment as
// ErrNotFound, without revealing tenant existence. Any other read failure is
// returned unchanged so callers can retry an outage.
func authorizeScope(ctx context.Context, st persistence.Store, organizationKey, applicationKey, environmentKey string) error {
	if organizationKey == "" || applicationKey == "" || environmentKey == "" {
		return persistence.ErrNotFound
	}
	app, err := st.GetApplication(ctx, applicationKey)
	if isNotFound(err) || (err == nil && app.OrganizationKey != organizationKey) {
		return persistence.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := st.GetEnvironment(ctx, applicationKey, environmentKey); err != nil {
		if isNotFound(err) {
			return persistence.ErrNotFound
		}
		return err
	}
	return nil
}

func validStatus(status domain.Status) bool {
	switch status {
	case domain.StatusPlanning, domain.StatusProvisioning, domain.StatusDeploying, domain.StatusSucceeded, domain.StatusFailed:
		return true
	default:
		return false
	}
}

// redact fails closed: an output is returned only when visible marks it
// explicitly non-secret and no nested value carries a secret reference.
func redact(outputs map[string]any, visible map[string]bool) map[string]any {
	out := map[string]any{}
	for name, value := range outputs {
		if !visible[name] || containsSecretRef(value) {
			out[name] = RedactedValue
			continue
		}
		out[name] = value
	}
	return out
}

func containsSecretRef(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "secretRef" || containsSecretRef(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsSecretRef(item) {
				return true
			}
		}
	}
	return false
}

func isNotFound(err error) bool {
	return errors.Is(err, persistence.ErrNotFound)
}
