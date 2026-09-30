// Package preview implements the UC-05 standalone Score Preview (OC-06): a
// read-only planning run over one consistent snapshot, shared with UC-06.
package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	appsvc "orchestrator/internal/application/deployment"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	"orchestrator/internal/ports/persistence"
)

// Public failure kinds. Every error of these kinds is a *PublicError whose
// message is safe to return: it is derived only from the caller's own request
// or is a fixed sentence. Catalog, Definition, Terraform module and store
// detail never reaches the message.
var (
	// ErrInvalidRequest reports a missing field or action/input combination.
	ErrInvalidRequest = errors.New("preview: invalid request")
	// ErrInvalidScore reports a Score or before-state the planner rejects.
	ErrInvalidScore = errors.New("preview: invalid Score")
	// ErrPlanningRejected reports a catalog-side planning failure.
	ErrPlanningRejected = errors.New("preview: planning rejected")
	// ErrNotReady reports that the Application connection is not READY.
	ErrNotReady = errors.New("preview: connection not ready")
)

// PublicError carries a safe message for one public failure kind.
type PublicError struct {
	Kind    error
	Message string
}

func (e *PublicError) Error() string        { return e.Message }
func (e *PublicError) Is(target error) bool { return target == e.Kind }

func public(kind error, format string, args ...any) error {
	return &PublicError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Planner messages are never echoed (planning.PublicMessage). These stages
// are caused by the caller's Score or before-state.
var (
	callerStages = map[planning.Stage]bool{planning.StageScore: true, planning.StageBefore: true, planning.StageCandidate: true, planning.StageRoutes: true, planning.StageServiceRefs: true}
)

// publicPlanningError maps a staged planner failure to a fixed sentence.
// Errors without a stage are internal and stay unchanged for a generic 500.
func publicPlanningError(err error) error {
	var stage *planning.StageError
	if !errors.As(err, &stage) {
		return err
	}
	message, _ := planning.PublicMessage(err)
	kind := ErrPlanningRejected
	if callerStages[stage.Stage] {
		kind = ErrInvalidScore
	}
	return public(kind, "%s", message)
}

// Actions accepted by the standalone Preview contract.
const (
	ActionDeploy = "deploy"
	ActionUpdate = "update"
	ActionRemove = "remove"
)

// PreviewDeploymentQuery is the explicit OC-06 input. Organization comes from
// the authenticated session; the caller never supplies base state.
type PreviewDeploymentQuery struct {
	OrganizationKey string
	ApplicationKey  string
	EnvironmentKey  string
	WorkloadID      string
	Action          string
	RunID           string
	ScoreBefore     map[string]any
	ScoreAfter      map[string]any
}

// DeploymentPreview is the unredacted, immutable result of one Preview. Plan
// holds the internal planning artifacts; Public renders the sanitized view.
type DeploymentPreview struct {
	ApplicationKey string
	EnvironmentKey string
	BaseSetID      string
	BaseVersion    int64
	RunID          string
	WorkloadID     string
	Action         string
	Plan           *planning.Plan
}

// Service is read-only: it holds no executor, provisioner or deployer.
type Service struct {
	store     persistence.Store
	planner   *planning.Service
	terraform planning.ModuleInspector
}

// NewService wires UC-05 Preview.
func NewService(store persistence.Store, planner *planning.Service, inspector planning.ModuleInspector) *Service {
	return &Service{store: store, planner: planner, terraform: inspector}
}

// PreviewDeployment validates the query, loads one consistent planning
// snapshot and runs the shared planner without writing any state.
func (s *Service) PreviewDeployment(ctx context.Context, q PreviewDeploymentQuery) (*DeploymentPreview, error) {
	before, after, action, err := validateQuery(q)
	if err != nil {
		return nil, err
	}
	snapshot, err := appsvc.LoadPlanningSnapshot(ctx, s.store, q.OrganizationKey, q.ApplicationKey, q.EnvironmentKey)
	if errors.Is(err, appsvc.ErrConnectionNotReady) {
		return nil, public(ErrNotReady, "the Application connection is not READY; ask a platform engineer to verify it")
	}
	if err != nil {
		return nil, err
	}
	plan, err := s.planner.Plan(planning.Request{
		OrganizationKey: q.OrganizationKey,
		App:             snapshot.App,
		Env:             snapshot.Env,
		Connection:      snapshot.Connection,
		BaseSet:         snapshot.BaseSet,
		Before:          before,
		After:           after,
		WorkloadID:      q.WorkloadID,
		RunID:           q.RunID,
		Action:          action,
		Catalog:         snapshot.Catalog,
		Active:          snapshot.Active,
		Terraform:       s.terraform,
	})
	if err != nil {
		return nil, publicPlanningError(err)
	}
	return &DeploymentPreview{
		ApplicationKey: snapshot.App.Key,
		EnvironmentKey: snapshot.Env.Key,
		BaseSetID:      snapshot.BaseSetID,
		BaseVersion:    snapshot.Env.Version,
		RunID:          q.RunID,
		WorkloadID:     q.WorkloadID,
		Action:         q.Action,
		Plan:           plan,
	}, nil
}

func validateQuery(q PreviewDeploymentQuery) (*score.Document, *score.Document, domain.Action, error) {
	if strings.TrimSpace(q.WorkloadID) == "" {
		return nil, nil, "", public(ErrInvalidRequest, "workloadId is required")
	}
	if strings.TrimSpace(q.RunID) == "" {
		return nil, nil, "", public(ErrInvalidRequest, "runId is required")
	}
	var action domain.Action
	switch q.Action {
	case ActionDeploy:
		action = domain.ActionDeploy
		if q.ScoreAfter == nil || q.ScoreBefore != nil {
			return nil, nil, "", public(ErrInvalidRequest, "deploy needs scoreAfter and no scoreBefore")
		}
	case ActionUpdate:
		action = domain.ActionUpdate
		if q.ScoreAfter == nil || q.ScoreBefore == nil {
			return nil, nil, "", public(ErrInvalidRequest, "update needs scoreBefore and scoreAfter")
		}
	case ActionRemove:
		action = domain.ActionRemove
		if q.ScoreBefore == nil || q.ScoreAfter != nil {
			return nil, nil, "", public(ErrInvalidRequest, "remove needs scoreBefore and no scoreAfter")
		}
	default:
		return nil, nil, "", public(ErrInvalidRequest, "action must be deploy, update or remove")
	}
	before, err := parse("scoreBefore", q.ScoreBefore, q.WorkloadID)
	if err != nil {
		return nil, nil, "", err
	}
	after, err := parse("scoreAfter", q.ScoreAfter, q.WorkloadID)
	if err != nil {
		return nil, nil, "", err
	}
	return before, after, action, nil
}

func parse(field string, raw map[string]any, workloadID string) (*score.Document, error) {
	if raw == nil {
		return nil, nil
	}
	doc, err := score.FromMap(raw)
	if err != nil {
		return nil, public(ErrInvalidScore, "%s: %s", field, err.Error())
	}
	if doc.Metadata.Name != workloadID {
		return nil, public(ErrInvalidRequest, "%s metadata.name %q does not match workloadId %q", field, doc.Metadata.Name, workloadID)
	}
	return doc, nil
}

// RedactedValue marks a literal variable value hidden from the public view.
const RedactedValue = appsvc.RedactedValue

// View is the explicit, sanitized HTTP projection of a DeploymentPreview.
type View struct {
	ApplicationKey string                  `json:"applicationKey"`
	EnvironmentKey string                  `json:"environmentKey"`
	BaseSetID      string                  `json:"baseSetId"`
	BaseVersion    int64                   `json:"baseVersion"`
	RunID          string                  `json:"runId"`
	WorkloadID     string                  `json:"workloadId"`
	Action         string                  `json:"action"`
	PlanHash       string                  `json:"planHash"`
	Delta          domain.DeltaDocument    `json:"delta"`
	CandidateSet   environment.Document    `json:"candidateSet"`
	Graph          GraphView               `json:"graph"`
	Matches        []MatchView             `json:"matches"`
	Batches        [][]string              `json:"batches"`
	Classification planning.Classification `json:"classification"`
}

// GraphView omits node parameter values, which may come from Definition
// provision rules or driver defaults; only parameter names remain.
type GraphView struct {
	Nodes []NodeView      `json:"nodes"`
	Edges []planning.Edge `json:"edges"`
}

// NodeView is one sanitized graph node.
type NodeView struct {
	Descriptor   string            `json:"descriptor"`
	Kind         planning.NodeKind `json:"kind"`
	ResourceType string            `json:"resourceType"`
	Class        string            `json:"class"`
	Origins      []planning.Origin `json:"origins"`
	WorkloadID   string            `json:"workloadId,omitempty"`
	ParamKeys    []string          `json:"paramKeys"`
	Bindings     map[string]string `json:"bindings"`
}

// MatchView is the selected Definition for one resource node.
type MatchView struct {
	Descriptor    string `json:"descriptor"`
	DefinitionKey string `json:"definitionKey"`
	DriverType    string `json:"driverType"`
	Specificity   int    `json:"specificity"`
}

// Public renders the sanitized view. It deep-copies the Candidate Set before
// redaction, so the internal Candidate, Delta and plan hash never change.
func (p *DeploymentPreview) Public() (View, error) {
	candidate, err := redactOtherWorkloads(p.Plan.CandidateSet, p.WorkloadID)
	if err != nil {
		return View{}, err
	}
	view := View{
		ApplicationKey: p.ApplicationKey, EnvironmentKey: p.EnvironmentKey,
		BaseSetID: p.BaseSetID, BaseVersion: p.BaseVersion, RunID: p.RunID,
		WorkloadID: p.WorkloadID, Action: p.Action, PlanHash: p.Plan.PlanHash,
		Delta: p.Plan.Delta, CandidateSet: candidate,
		Graph:   GraphView{Nodes: []NodeView{}, Edges: []planning.Edge{}},
		Matches: []MatchView{}, Batches: [][]string{},
		Classification: planning.Classification{
			Existing: nonNil(p.Plan.Classification.Existing), New: nonNil(p.Plan.Classification.New),
			Unreferenced: nonNil(p.Plan.Classification.Unreferenced),
		},
	}
	for _, n := range p.Plan.Graph.Nodes {
		keys := make([]string, 0, len(n.Params))
		for key := range n.Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		bindings := map[string]string{}
		for k, v := range n.Bindings {
			bindings[k] = v
		}
		origins := append([]planning.Origin{}, n.Origins...)
		view.Graph.Nodes = append(view.Graph.Nodes, NodeView{Descriptor: n.Descriptor, Kind: n.Kind, ResourceType: n.ResourceType, Class: n.Class, Origins: origins, WorkloadID: n.WorkloadID, ParamKeys: keys, Bindings: bindings})
	}
	view.Graph.Edges = append(view.Graph.Edges, p.Plan.Graph.Edges...)
	descriptors := make([]string, 0, len(p.Plan.Matches))
	for d := range p.Plan.Matches {
		descriptors = append(descriptors, d)
	}
	sort.Strings(descriptors)
	for _, d := range descriptors {
		m := p.Plan.Matches[d]
		view.Matches = append(view.Matches, MatchView{Descriptor: m.Descriptor, DefinitionKey: m.DefinitionKey, DriverType: string(m.DriverType), Specificity: m.Specificity})
	}
	for _, batch := range p.Plan.Batches {
		view.Batches = append(view.Batches, append([]string{}, batch...))
	}
	return view, nil
}

// symbolic matches a value made only of placeholder expressions.
var symbolic = regexp.MustCompile(`^(\$\{[^{}]+\})+$`)

// redactOtherWorkloads copies set and hides literal container variable values
// of every workload except the submitted one. Placeholders stay symbolic.
func redactOtherWorkloads(set environment.Document, workloadID string) (environment.Document, error) {
	copied, err := copyDocument(set)
	if err != nil {
		return environment.Document{}, err
	}
	for id, module := range copied.Modules {
		if id == workloadID {
			continue
		}
		for name, container := range module.Spec.Containers {
			for key, value := range container.Variables {
				if !symbolic.MatchString(value) {
					container.Variables[key] = RedactedValue
				}
			}
			module.Spec.Containers[name] = container
		}
		copied.Modules[id] = module
	}
	return copied, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string{}, values...)
}

func copyDocument(set environment.Document) (environment.Document, error) {
	raw, err := json.Marshal(set)
	if err != nil {
		return environment.Document{}, err
	}
	var out environment.Document
	if err := json.Unmarshal(raw, &out); err != nil {
		return environment.Document{}, err
	}
	if out.Modules == nil {
		out.Modules = map[string]environment.Module{}
	}
	if out.Shared == nil {
		out.Shared = map[string]environment.ResourceEntry{}
	}
	return out, nil
}
