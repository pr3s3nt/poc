package planner

// Document is the generic JSON-like representation used across phases.
type Document = map[string]any

// Result is rendered directly as the planner's stdout JSON object.
type Result = map[string]any

type CaseManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Context              string   `yaml:"context"`
		CurrentDeploymentSet string   `yaml:"currentDeploymentSet"`
		BeforeScore          *string  `yaml:"beforeScore"`
		AfterScore           *string  `yaml:"afterScore"`
		ResourceTypes        []string `yaml:"resourceTypes"`
		ResourceDefinitions  []string `yaml:"resourceDefinitions"`
		ActiveResources      string   `yaml:"activeResources"`
		TerraformSourceMap   []struct {
			URL       string `yaml:"url"`
			Rev       string `yaml:"rev"`
			Directory string `yaml:"directory"`
		} `yaml:"terraformSourceMap"`
	} `yaml:"spec"`
}

// ResourceType is the reduced Entity YAML form used by the fixtures.
type ResourceType struct {
	ID      string
	Inputs  Document // properties map
	Outputs Document // properties map
}

// Definition is the reduced entity.humanitec.io/v1b1 Definition form.
type Definition struct {
	ID           string
	Type         string
	DriverType   string
	Criteria     []Document
	DriverInputs Document // entity.driver_inputs
	Provision    Document // entity.provision
}

type LoadedCase struct {
	Root                 string
	Manifest             CaseManifest
	Context              Document
	CurrentDeploymentSet Document
	BeforeScore          Document
	AfterScore           Document
	ResourceTypes        map[string]*ResourceType
	ResourceDefinitions  []*Definition
	ActiveResources      []Document
}

// WorkloadFragment is the Deployment Set contribution of a single Score file.
type WorkloadFragment struct {
	WorkloadID string
	Module     Document
	Shared     Document
}

type Node struct {
	Type           string
	Class          string
	ResourceID     string
	ResourceInputs Document
	Origins        []string
	Definition     *Definition
	Criterion      Document
	Score          int
	expanded       bool
}

func (n *Node) Descriptor() string { return n.Type + "." + n.Class + "#" + n.ResourceID }

type Edge struct {
	From   string
	To     string
	Reason string
	Path   string
}

type ResourceGraph struct {
	Nodes map[string]*Node
	Edges []Edge
	seen  map[string]bool
}

type TerraformVariable struct {
	Name       string
	Type       string
	Default    any
	HasDefault bool
}

type TerraformModule struct {
	Directory   string // relative to the case root
	Variables   map[string]TerraformVariable
	Outputs     []string
	Fingerprint string
}

type TerraformRecord struct {
	Resource     string
	DefinitionID string
	Source       Document
	Directory    string
	Fingerprint  string
	Inputs       []Document
	Outputs      []string
}

type PlannerError struct {
	Phase string
	Code  string
	Path  string
}

func (e *PlannerError) Error() string { return e.Phase + ": " + e.Code }

func newError(phase, code, path string) *PlannerError {
	return &PlannerError{Phase: phase, Code: code, Path: path}
}
