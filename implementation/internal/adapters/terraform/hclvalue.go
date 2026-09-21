package terraform

import (
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// variableDefault evaluates the default of a Terraform variable block so the
// planner can report the value a module falls back to.
func variableDefault(block *hclsyntax.Block) (any, bool) {
	attr, ok := block.Body.Attributes["default"]
	if !ok {
		return nil, false
	}
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return nil, true
	}
	return CtyToGo(value), true
}

// CtyToGo converts an HCL value into the JSON-compatible Go value the plan
// snapshot stores.
func CtyToGo(value cty.Value) any {
	if value.IsNull() || !value.IsKnown() {
		return nil
	}
	switch {
	case value.Type() == cty.String:
		return value.AsString()
	case value.Type() == cty.Bool:
		return value.True()
	case value.Type() == cty.Number:
		float, _ := value.AsBigFloat().Float64()
		return float
	case value.Type().IsTupleType() || value.Type().IsListType() || value.Type().IsSetType():
		out := []any{}
		for it := value.ElementIterator(); it.Next(); {
			_, element := it.Element()
			out = append(out, CtyToGo(element))
		}
		return out
	case value.Type().IsObjectType() || value.Type().IsMapType():
		out := map[string]any{}
		for it := value.ElementIterator(); it.Next(); {
			key, element := it.Element()
			out[key.AsString()] = CtyToGo(element)
		}
		return out
	}
	return nil
}
