// Package planning is the deterministic, side-effect free core that turns a
// Score document into a Deployment Plan (UC-05, UC-06, UC-07).
package planning

import (
	"sort"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
)

// NodeKind separates workload nodes from resource nodes. UC-08 executes
// resource nodes only; the workload is applied by UC-06 after outputs exist.
type NodeKind string

// Node kinds.
const (
	NodeWorkload NodeKind = "workload"
	NodeResource NodeKind = "resource"
)

// Origin records why a node is part of the graph.
type Origin string

// Node origins.
const (
	OriginWorkload  Origin = "implicit-workload"
	OriginPrivate   Origin = "private-dependency"
	OriginShared    Origin = "shared-dependency"
	OriginProfile   Origin = "execution-profile"
	OriginReference Origin = "resource-reference"
	OriginProvision Origin = "co-provision"
)

// Edge reasons, mirroring the planner reference so a plan can be audited.
const (
	ReasonPrivate        = "deployment-set-private"
	ReasonWorkloadPlace  = "workload-placeholder"
	ReasonInputPlace     = "resource-input-placeholder"
	ReasonProfile        = "execution-profile"
	ReasonReference      = "resource-reference"
	ReasonDependent      = "co-provision-is-dependent"
	ReasonMatchDependent = "co-provision-match-dependents"
)

// Node is one vertex of the Resource Graph.
type Node struct {
	Descriptor   string            `json:"descriptor"`
	Kind         NodeKind          `json:"kind"`
	ResourceType string            `json:"resourceType"`
	Class        string            `json:"class"`
	Scope        resource.Scope    `json:"scope"`
	Origins      []Origin          `json:"origins"`
	WorkloadID   string            `json:"workloadId,omitempty"`
	Params       map[string]any    `json:"params,omitempty"`
	Bindings     map[string]string `json:"bindings,omitempty"`
}

// Edge points from a consumer to the provider it depends on (UC-08 BR-01).
type Edge struct {
	Consumer string `json:"consumer"`
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
	Path     string `json:"path,omitempty"`
}

// Graph is the complete resource graph of a Candidate Deployment Set.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node returns the node with the given descriptor.
func (g Graph) Node(descriptor string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.Descriptor == descriptor {
			return n, true
		}
	}
	return Node{}, false
}

// Providers lists the providers of a consumer in deterministic order.
func (g Graph) Providers(consumer string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range g.Edges {
		if e.Consumer == consumer && !seen[e.Provider] {
			seen[e.Provider] = true
			out = append(out, e.Provider)
		}
	}
	sort.Strings(out)
	return out
}

// Match records the Resource Definition selected for a resource node (UC-06 MS-07).
type Match struct {
	Descriptor    string              `json:"descriptor"`
	DefinitionKey string              `json:"definitionKey"`
	DriverType    resource.DriverType `json:"driverType"`
	ConnectionKey string              `json:"connectionKey,omitempty"`
	Specificity   int                 `json:"specificity"`
	Criterion     resource.Criterion  `json:"criterion"`
}

// TerraformContract is the planning-time inspection of a Terraform module
// (UC-06 BR-09).
type TerraformContract struct {
	Descriptor    string          `json:"descriptor"`
	DefinitionKey string          `json:"definitionKey"`
	Module        string          `json:"module"`
	Fingerprint   string          `json:"fingerprint"`
	Outputs       []string        `json:"outputs"`
	Inputs        []ContractInput `json:"inputs"`
}

// ContractInput records where the value of one Terraform variable comes from.
// Resource references stay raw: they only resolve once the provider has run.
type ContractInput struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Source string `json:"source"`
	Value  any    `json:"value,omitempty"`
}

// Classification is the planning evidence about Active Resource reuse (UC-08 MS-02).
type Classification struct {
	Existing     []string `json:"existing"`
	New          []string `json:"new"`
	Unreferenced []string `json:"unreferenced"`
}

// Plan is the immutable planning artifact persisted with a Deployment.
type Plan struct {
	Rendering   map[string]resource.RenderingSelection `json:"rendering,omitempty"`
	WorkloadID  string                                 `json:"workloadId"`
	Action      deployment.Action                      `json:"action"`
	ScoreBefore map[string]any                         `json:"scoreBefore,omitempty"`
	ScoreAfter  map[string]any                         `json:"scoreAfter,omitempty"`
	// Delta is the transient Humanitec-shaped Delta document. It is not part
	// of the persisted plan: UC-06/07 persist it as a DeploymentDeltaSnapshot.
	Delta          deployment.DeltaDocument `json:"-"`
	BaseSet        environment.Document     `json:"baseSet"`
	CandidateSet   environment.Document     `json:"candidateSet"`
	Graph          Graph                    `json:"graph"`
	Matches        map[string]Match         `json:"matches"`
	Terraform      []TerraformContract      `json:"terraform"`
	Batches        [][]string               `json:"batches"`
	Classification Classification           `json:"classification"`
	// Target pins the nonsecret Environment execution binding the plan used.
	Target environment.Binding `json:"target"`
	// UnreferencedResources are the Active Resources the deployment marks
	// UNREFERENCED in its final transaction (UC-07 MS-07). Not persisted with
	// the plan and not part of the plan hash.
	UnreferencedResources []resource.ActiveResource `json:"-"`
	PlanHash              string                    `json:"planHash"`
}

// Catalog is the read-only registry planning matches against.
type Catalog struct {
	Types       map[string]resource.Type
	Definitions []resource.Definition
}

// ModuleInspector reads the Terraform module a Definition points at.
type ModuleInspector interface {
	Inspect(module string) (ModuleContract, error)
	// ExecutorVariables lists variables the executor supplies itself, for
	// example the region, the resource tags and generated credentials.
	ExecutorVariables() []string
	// ExecutorOutputs lists outputs the executor adds after apply, such as the
	// kubeconfig it writes for a new cluster.
	ExecutorOutputs(module string) []string
}

// ModuleContract is the parsed contract of one Terraform module.
type ModuleContract struct {
	Module      string
	Fingerprint string
	Outputs     []string
	Variables   map[string]ModuleVariable
}

// ModuleVariable is one declared Terraform variable.
type ModuleVariable struct {
	Name       string
	Type       string
	HasDefault bool
	Default    any
}

// Request is the input of one planning run.
type Request struct {
	// AllowIntermediateEnvironmentState skips the complete-Environment rules
	// (public routes, Service references; UC-07 BR-08) for one step of a
	// multi-workload pending batch whose FINAL state the caller has already
	// validated. Single-operation Preview/Deploy must leave it false.
	AllowIntermediateEnvironmentState bool
	OrganizationKey                   string
	App                               application.Application
	Env                               environment.Environment
	Connection                        application.Connection
	BaseSet                           environment.Document
	Before                            *score.Document
	After                             *score.Document
	WorkloadID                        string
	RunID                             string
	Action                            deployment.Action
	Catalog                           Catalog
	Active                            []resource.ActiveResource
	Terraform                         ModuleInspector
}
