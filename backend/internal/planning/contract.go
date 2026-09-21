package planning

import (
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
)

// validateReferenceOutputs checks every Resource Reference against the output
// contract of the provider: the Terraform module outputs for a Terraform
// Definition, otherwise the Resource Type outputs (UC-06 BR-06).
func (b *graphBuilder) validateReferenceOutputs(
	refs []pendingRef,
	matches map[string]Match,
	matcher *definitionMatcher,
	inspector ModuleInspector,
) error {
	ordered := append([]pendingRef(nil), refs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].From != ordered[j].From {
			return ordered[i].From < ordered[j].From
		}
		return ordered[i].Path < ordered[j].Path
	})

	for _, ref := range ordered {
		provider, ok := b.nodes[ref.To]
		if !ok {
			return fmt.Errorf("planning: %s references missing node %s", ref.Path, ref.To)
		}
		match, ok := matches[ref.To]
		if !ok {
			continue
		}
		def, ok := matcher.definition(match.DefinitionKey)
		if !ok {
			continue
		}
		if def.DriverType == resource.DriverTerraform && inspector != nil {
			contract, err := inspector.Inspect(moduleName(def))
			if err != nil {
				return fmt.Errorf("planning: definition %q: %w", def.Key, err)
			}
			if !contains(append(contract.Outputs, inspector.ExecutorOutputs(contract.Module)...), ref.Output) {
				return fmt.Errorf("planning: %s binds output %q which the Terraform module %q does not declare",
					ref.Path, ref.Output, contract.Module)
			}
			continue
		}
		typ, ok := b.catalog.Types[provider.ResourceType]
		if !ok {
			return fmt.Errorf("planning: resource type %q is not registered", provider.ResourceType)
		}
		if _, ok := typ.Output(ref.Output); !ok {
			return fmt.Errorf("planning: %s binds output %q which is not in the %q output contract",
				ref.Path, ref.Output, typ.Key)
		}
	}
	return nil
}

// inspectTerraform checks resource inputs and driver variables against the
// Terraform module contract and records the source fingerprint (UC-06 BR-09).
func (b *graphBuilder) inspectTerraform(
	matches map[string]Match,
	matcher *definitionMatcher,
	inspector ModuleInspector,
) ([]TerraformContract, error) {
	if inspector == nil {
		return []TerraformContract{}, nil
	}
	executorProvided := map[string]bool{}
	for _, name := range inspector.ExecutorVariables() {
		executorProvided[name] = true
	}

	contracts := []TerraformContract{}
	for _, key := range b.descriptors() {
		node := b.nodes[key]
		match, ok := matches[key]
		if !ok || match.DriverType != resource.DriverTerraform {
			continue
		}
		def, ok := matcher.definition(match.DefinitionKey)
		if !ok {
			continue
		}
		contract, err := inspector.Inspect(moduleName(def))
		if err != nil {
			return nil, fmt.Errorf("planning: definition %q: %w", def.Key, err)
		}

		variables, err := resolveContextTree(def.Variables(), b.ctx, node)
		if err != nil {
			return nil, fmt.Errorf("planning: definition %q driver variables: %w", def.Key, err)
		}
		values := map[string]any{}
		provided := map[string]string{}
		for name, value := range variables {
			provided[name] = "driver-input"
			values[name] = value
		}
		for name, value := range node.Params {
			provided[name] = "resource-input"
			values[name] = value
		}
		// An executor variable only counts as provided when the module declares
		// it; the executor never passes a variable a module does not accept.
		for name := range executorProvided {
			if _, declared := contract.Variables[name]; !declared {
				continue
			}
			if _, ok := provided[name]; !ok {
				provided[name] = "executor"
			}
		}

		names := map[string]bool{}
		for name := range provided {
			names[name] = true
		}
		for name := range contract.Variables {
			names[name] = true
		}
		ordered := make([]string, 0, len(names))
		for name := range names {
			ordered = append(ordered, name)
		}
		sort.Strings(ordered)

		inputs := make([]ContractInput, 0, len(contract.Variables))
		for _, name := range ordered {
			variable, declared := contract.Variables[name]
			source, isProvided := provided[name]
			if isProvided && !declared {
				return nil, fmt.Errorf("planning: %s passes %q which the Terraform module %q does not declare",
					node.Descriptor, name, contract.Module)
			}
			if declared && !isProvided && !variable.HasDefault {
				return nil, fmt.Errorf("planning: %s does not provide the required Terraform variable %q of module %q",
					node.Descriptor, name, contract.Module)
			}
			if !declared {
				continue
			}
			value := values[name]
			if !isProvided {
				source = "terraform-default"
				value = variable.Default
			}
			inputs = append(inputs, ContractInput{Name: name, Type: variable.Type, Source: source, Value: value})
		}

		if typ, ok := b.catalog.Types[node.ResourceType]; ok {
			available := append(contract.Outputs, inspector.ExecutorOutputs(contract.Module)...)
			for _, output := range typ.Outputs {
				if !contains(available, output.Name) {
					return nil, fmt.Errorf("planning: Terraform module %q does not declare output %q required by resource type %q",
						contract.Module, output.Name, typ.Key)
				}
			}
		}

		contracts = append(contracts, TerraformContract{
			Descriptor:    node.Descriptor,
			DefinitionKey: def.Key,
			Module:        contract.Module,
			Fingerprint:   contract.Fingerprint,
			Outputs:       contract.Outputs,
			Inputs:        inputs,
		})
	}
	return contracts, nil
}

// moduleName identifies the Terraform module a Definition points at. The
// embedded modules use a plain name; a remote source is identified by
// url[@rev][/path].
// resolveContextTree replaces ${context.*} in driver variables and leaves every
// other placeholder exactly as written, mirroring the planner reference.
func resolveContextTree(variables map[string]any, ctx Context, node *Node) (map[string]any, error) {
	out := map[string]any{}
	for name, value := range variables {
		resolved, err := resolveContextValue(value, ctx, node)
		if err != nil {
			return nil, err
		}
		out[name] = resolved
	}
	return out, nil
}

func resolveContextValue(v any, ctx Context, node *Node) (any, error) {
	switch t := v.(type) {
	case string:
		return resolveContextString(t, ctx, node)
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, value := range t {
			resolved, err := resolveContextValue(value, ctx, node)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, value := range t {
			resolved, err := resolveContextValue(value, ctx, node)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}

func resolveContextString(text string, ctx Context, node *Node) (any, error) {
	matches := placeholder.Pattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	resolver := nodeContext{ctx: ctx, node: node}
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(text) && text[:2] != "$$" {
		body := text[matches[0][2]:matches[0][3]]
		if !isContextBody(body) {
			return text, nil
		}
		return resolver.ResolveContext(strings.TrimPrefix(strings.TrimSpace(body), "context."))
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(text[last:m[0]])
		raw := text[m[0]:m[1]]
		body := text[m[2]:m[3]]
		if raw[:2] == "$$" || !isContextBody(body) {
			b.WriteString(raw)
		} else {
			value, err := resolver.ResolveContext(strings.TrimPrefix(strings.TrimSpace(body), "context."))
			if err != nil {
				return nil, err
			}
			b.WriteString(placeholder.Stringify(value))
		}
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String(), nil
}

func isContextBody(body string) bool {
	return strings.HasPrefix(strings.TrimSpace(body), "context.")
}

// nodeContext resolves context values, including the res.* values of the node
// that owns the driver inputs.
type nodeContext struct {
	ctx  Context
	node *Node
}

func (r nodeContext) ResolveContext(path string) (any, error) {
	switch path {
	case "res.id":
		return descriptorID(r.node.Descriptor), nil
	case "res.class":
		return r.node.Class, nil
	case "res.type":
		return r.node.ResourceType, nil
	}
	return r.ctx.ResolveContext(path)
}

func moduleName(def resource.Definition) string {
	source := def.Source()
	if name, _ := source["module"].(string); name != "" {
		return name
	}
	url, _ := source["url"].(string)
	if url == "" {
		return ""
	}
	key := url
	if rev, _ := source["rev"].(string); rev != "" {
		key += "@" + rev
	}
	if path, _ := source["path"].(string); path != "" {
		key += "/" + path
	}
	return key
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
