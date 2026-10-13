package tfmodule

import (
	"fmt"
	"strings"
)

// ValueType is the subset of Terraform value types a module variable or a
// statically known output may declare: string, number, bool, any and
// list/map with an element type. Sets are treated as lists. Objects and
// tuples are not part of the supported subset.
type ValueType struct {
	Kind string // string, number, bool, any, list, map
	Elem *ValueType
}

func (t ValueType) String() string {
	if t.Elem != nil {
		return t.Kind + "(" + t.Elem.String() + ")"
	}
	return t.Kind
}

// ParseType reads a Terraform type expression of the supported subset.
func ParseType(expr string) (ValueType, error) {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "string", "number", "bool", "any":
		return ValueType{Kind: expr}, nil
	}
	for _, collection := range []string{"list", "set", "map"} {
		inner, ok := strings.CutPrefix(expr, collection+"(")
		if !ok || !strings.HasSuffix(inner, ")") {
			continue
		}
		elem, err := ParseType(strings.TrimSuffix(inner, ")"))
		if err != nil {
			return ValueType{}, err
		}
		kind := collection
		if kind == "set" {
			kind = "list"
		}
		return ValueType{Kind: kind, Elem: &elem}, nil
	}
	return ValueType{}, fmt.Errorf("unsupported Terraform type expression")
}

// CanonicalType returns the canonical form of a supported type expression.
func CanonicalType(expr string) (string, error) {
	t, err := ParseType(expr)
	if err != nil {
		return "", err
	}
	return t.String(), nil
}

// contractKind maps a Resource Type contract type ("" means any) onto the
// value kind it requires.
func contractKind(contractType string) string {
	if contractType == "" {
		return "any"
	}
	return contractType
}

// InputCompatible reports whether a Resource Type input of contractType may
// feed a module variable of moduleType. Unknown ("any") on either side
// defers the check to binding time, where the concrete value is checked.
func InputCompatible(contractType, moduleType string) bool {
	module, err := ParseType(moduleType)
	if err != nil {
		return false
	}
	want := contractKind(contractType)
	return want == "any" || module.Kind == "any" || module.Kind == want
}

// OutputCompatible reports whether a module output of moduleType (empty when
// not statically known) may fill a Resource Type output of contractType.
// An unknown module type is checked against the contract after apply.
func OutputCompatible(contractType, moduleType string) bool {
	want := contractKind(contractType)
	if moduleType == "" || want == "any" {
		return true
	}
	module, err := ParseType(moduleType)
	if err != nil {
		return false
	}
	return module.Kind == "any" || module.Kind == want
}
