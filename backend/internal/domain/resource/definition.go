package resource

import (
	"fmt"
	"sort"
)

// DriverType selects the executor adapter used to provision a resource.
type DriverType string

// Driver types supported by the MVP.
const (
	DriverTerraform       DriverType = "terraform"
	DriverKubernetes      DriverType = "kubernetes"
	DriverExistingCluster DriverType = "existing-cluster"
	DriverEcho            DriverType = "echo"
)

// MatchContext carries the values a Matching Criterion is evaluated against.
// The five fields are the Humanitec matching contract (UC-03 BR-06).
type MatchContext struct {
	EnvironmentType string
	ApplicationID   string
	EnvironmentID   string
	ResourceID      string
	Class           string
}

// CriteriaWeights is the fixed specificity table of UC-03 BR-06.
var CriteriaWeights = map[string]int{
	"env_type": 1,
	"app_id":   2,
	"env_id":   4,
	"res_id":   8,
	"class":    16,
}

// Criterion is one Matching Criterion of a Resource Definition (UC-03).
type Criterion struct {
	EnvironmentType string `json:"env_type,omitempty"`
	ApplicationID   string `json:"app_id,omitempty"`
	EnvironmentID   string `json:"env_id,omitempty"`
	ResourceID      string `json:"res_id,omitempty"`
	Class           string `json:"class,omitempty"`
}

func (c Criterion) fields() [][2]string {
	return [][2]string{
		{c.EnvironmentType, "env_type"},
		{c.ApplicationID, "app_id"},
		{c.EnvironmentID, "env_id"},
		{c.ResourceID, "res_id"},
		{c.Class, "class"},
	}
}

// Specificity is the sum of the weights of every declared field.
func (c Criterion) Specificity() int {
	score := 0
	for _, f := range c.fields() {
		if f[0] != "" {
			score += CriteriaWeights[f[1]]
		}
	}
	return score
}

// Matches reports whether every declared criterion field equals the context value.
func (c Criterion) Matches(ctx MatchContext) bool {
	pairs := [][2]string{
		{c.EnvironmentType, ctx.EnvironmentType},
		{c.ApplicationID, ctx.ApplicationID},
		{c.EnvironmentID, ctx.EnvironmentID},
		{c.ResourceID, ctx.ResourceID},
		{c.Class, ctx.Class},
	}
	for _, p := range pairs {
		if p[0] != "" && p[0] != p[1] {
			return false
		}
	}
	return true
}

// ProvisionRule is one `provision` entry of a Resource Definition. The map key
// is the descriptor of the co-provisioned resource, written as
// `TYPE[.CLASS][#ID]`, where a missing or `@` segment is inherited from the
// node that owns the rule (UC-06 BR-08).
type ProvisionRule struct {
	IsDependent     bool           `json:"is_dependent,omitempty"`
	MatchDependents bool           `json:"match_dependents,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
}

// Definition is a Resource Definition: how a Resource Type is implemented (UC-03).
type Definition struct {
	Key             string                   `json:"key"`
	ResourceTypeKey string                   `json:"resourceType"`
	DriverType      DriverType               `json:"driverType"`
	ConnectionKey   string                   `json:"connectionKey,omitempty"`
	DriverInputs    map[string]any           `json:"driverInputs,omitempty"`
	Provision       map[string]ProvisionRule `json:"provision,omitempty"`
	Criteria        []Criterion              `json:"criteria"`
	SourceFingerpr  string                   `json:"sourceFingerprint,omitempty"`
}

// DriverValues returns `driverInputs.values`, the Humanitec container for the
// Terraform source and variables.
func (d Definition) DriverValues() map[string]any {
	values, _ := d.DriverInputs["values"].(map[string]any)
	if values == nil {
		return map[string]any{}
	}
	return values
}

// Source returns `driverInputs.values.source`.
func (d Definition) Source() map[string]any {
	source, _ := d.DriverValues()["source"].(map[string]any)
	if source == nil {
		return map[string]any{}
	}
	return source
}

// Variables returns `driverInputs.values.variables`.
func (d Definition) Variables() map[string]any {
	variables, _ := d.DriverValues()["variables"].(map[string]any)
	if variables == nil {
		return map[string]any{}
	}
	return variables
}

// BestCriterion returns the highest scoring criterion that matches the context.
func (d Definition) BestCriterion(ctx MatchContext) (Criterion, int, bool) {
	best, score, found := Criterion{}, -1, false
	for _, c := range d.Criteria {
		if !c.Matches(ctx) {
			continue
		}
		if s := c.Specificity(); s > score {
			best, score, found = c, s, true
		}
	}
	return best, score, found
}

// Validate reports structural problems in a Resource Definition.
func (d Definition) Validate() error {
	if d.Key == "" {
		return fmt.Errorf("resource: definition key is empty")
	}
	if d.ResourceTypeKey == "" {
		return fmt.Errorf("resource: definition %q has no resource type", d.Key)
	}
	switch d.DriverType {
	case DriverTerraform, DriverKubernetes, DriverExistingCluster, DriverEcho:
	default:
		return fmt.Errorf("resource: definition %q has unknown driver %q", d.Key, d.DriverType)
	}
	if len(d.Criteria) == 0 {
		return fmt.Errorf("resource: definition %q has no matching criteria", d.Key)
	}
	for key := range d.Provision {
		if key == "" {
			return fmt.Errorf("resource: definition %q has an empty provision key", d.Key)
		}
	}
	return nil
}

// SortDefinitions orders definitions by key so planning stays deterministic.
func SortDefinitions(defs []Definition) {
	sort.Slice(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key })
}
