package tfexec

import (
	"fmt"
	"sort"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/terraformexec"
)

// runtimeType reads a Terraform runtime output type expression: "string",
// "number", "bool", ["list", T], ["set", T], ["map", T], ["object", {...}]
// and ["tuple", [...]]. Object and tuple types have no subset counterpart and
// are reported with their own kind so they never match a scalar contract.
func runtimeType(t any) tfmodule.ValueType {
	switch v := t.(type) {
	case string:
		return tfmodule.ValueType{Kind: v}
	case []any:
		if len(v) == 2 {
			kind, _ := v[0].(string)
			switch kind {
			case "list", "set", "map":
				elem := runtimeType(v[1])
				if kind == "set" {
					kind = "list"
				}
				return tfmodule.ValueType{Kind: kind, Elem: &elem}
			case "object", "tuple":
				return tfmodule.ValueType{Kind: kind}
			}
		}
	}
	return tfmodule.ValueType{Kind: "unknown"}
}

func typeMatches(declared, runtime tfmodule.ValueType) bool {
	if declared.Kind == "any" {
		return true
	}
	if runtime.Kind == "dynamic" {
		return true
	}
	if declared.Kind != runtime.Kind {
		return false
	}
	if declared.Elem == nil {
		return true
	}
	if runtime.Elem == nil {
		return false
	}
	return typeMatches(*declared.Elem, *runtime.Elem)
}

// checkedOutputs is the validated runtime output set of one instance, by
// module output name.
type checkedOutputs struct {
	// Module holds module outputs (and executor-added outputs) by name.
	Module map[string]any
	// Sensitive marks module outputs that are sensitive at runtime or in the
	// module contract; classification only ever grows.
	Sensitive map[string]bool
}

// checkOutputs verifies the runtime outputs of one apply against the module
// contract and the Resource Type contract before anything is published:
//
//   - every output the mapping uses must exist at runtime;
//   - its runtime type must match the statically declared type (a deferred
//     type with no static declaration is checked against the Type contract);
//   - sensitivity at runtime or in the contract is kept (never downgraded);
//   - after mapping, the Type contract (required, unknown, value kinds) holds.
//
// Errors name outputs only, never values.
func checkOutputs(b tfmodule.Bundle, mapping map[string]string, typ resource.Type, actual map[string]terraformexec.Output, executor map[string]any) (checkedOutputs, error) {
	fail := func(format string, args ...any) (checkedOutputs, error) {
		return checkedOutputs{}, fmt.Errorf("%w: %s", ErrOutputContract, fmt.Sprintf(format, args...))
	}
	out := checkedOutputs{Module: map[string]any{}, Sensitive: map[string]bool{}}
	names := make([]string, 0, len(mapping))
	for name := range mapping {
		names = append(names, name)
	}
	sort.Strings(names)
	execOutputs := map[string]bool{}
	for _, name := range b.Contract.ExecutorOutputs {
		execOutputs[name] = true
	}
	for _, name := range names {
		if execOutputs[name] {
			value, ok := executor[name]
			if !ok {
				continue
			}
			out.Module[name] = value
			continue
		}
		declared, isDeclared := b.Contract.Outputs[name]
		runtime, ok := actual[name]
		switch {
		case !isDeclared:
			return fail("output %s is not an output of the module contract", safeName(name))
		case !ok:
			return fail("module output %s is missing at runtime", safeName(name))
		}
		if declared.Type != "" {
			want, err := tfmodule.ParseType(declared.Type)
			if err != nil || !typeMatches(want, runtimeType(runtime.Type)) {
				return fail("module output %s has a runtime type different from its contract", safeName(name))
			}
		}
		out.Module[name] = runtime.Value
		if runtime.Sensitive || declared.Sensitive {
			out.Sensitive[name] = true
		}
	}
	mapped := map[string]any{}
	for name, target := range mapping {
		if value, ok := out.Module[name]; ok {
			mapped[target] = value
		}
	}
	if err := typ.ValidateOutputs(mapped); err != nil {
		return fail("the Resource Type output contract is not met")
	}
	for name, target := range mapping {
		if !out.Sensitive[name] {
			if field, ok := typ.Output(target); ok && field.Secret {
				out.Sensitive[name] = true
			}
		}
	}
	return out, nil
}
