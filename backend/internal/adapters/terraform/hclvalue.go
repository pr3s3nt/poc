package terraform

import (
	"github.com/zclconf/go-cty/cty"
)

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
