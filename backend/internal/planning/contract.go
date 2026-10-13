package planning

import (
	"errors"
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
			contract, err := inspector.Contract(def)
			if err != nil {
				return fmt.Errorf("planning: definition %q: %w", def.Key, err)
			}
			if contract.OutputMapping == nil {
				// Planner-reference fixtures bind module outputs by name.
				if !contains(contract.Outputs, ref.Output) {
					return fmt.Errorf("planning: %s binds output %q which the Terraform module %q does not declare",
						ref.Path, ref.Output, contract.Module)
				}
				continue
			}
			if !mapsTo(contract.OutputMapping, ref.Output) {
				return fmt.Errorf("planning: %s binds output %q which the Terraform module %q does not map to the %q output contract",
					ref.Path, ref.Output, contract.Module, provider.ResourceType)
			}
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
// verified Terraform module contract and snapshots the bundle pins into the
// plan (UC-06 BR-09). Params reach module variables only through the
// Definition input mapping; the order is param, then Definition value, then
// executor-owned value, then module default.
func (b *graphBuilder) inspectTerraform(
	matches map[string]Match,
	matcher *definitionMatcher,
	inspector ModuleInspector,
) ([]TerraformContract, error) {
	if inspector == nil {
		return []TerraformContract{}, nil
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
		contract, err := inspector.Contract(def)
		if err != nil {
			return nil, fmt.Errorf("planning: definition %q: %w", def.Key, err)
		}
		executorOwned := map[string]bool{}
		for _, name := range contract.ExecutorOwned {
			executorOwned[name] = true
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
		if contract.InputMapping == nil {
			// Planner-reference fixtures pass params by variable name.
			for name, value := range node.Params {
				provided[name] = "resource-input"
				values[name] = value
			}
		} else {
			consumed := map[string]bool{}
			for variable, param := range contract.InputMapping {
				consumed[param] = true
				if value, ok := node.Params[param]; ok {
					provided[variable] = "resource-input"
					values[variable] = value
				}
			}
			for name := range node.Params {
				if !consumed[name] {
					return nil, fmt.Errorf("planning: %s passes param %q which definition %q does not map to a variable of module %q",
						node.Descriptor, name, def.Key, contract.Module)
				}
			}
		}
		// An executor variable only counts as provided when the module declares
		// it; the executor never passes a variable a module does not accept.
		for name := range executorOwned {
			if _, declared := contract.Variables[name]; !declared {
				continue
			}
			if source, ok := provided[name]; ok && contract.InputMapping != nil {
				return nil, fmt.Errorf("planning: %s sets executor-owned variable %q of module %q from %s",
					node.Descriptor, name, contract.Module, source)
			}
			if _, ok := provided[name]; !ok {
				provided[name] = "executor"
			}
		}

		// A secret output (Resource Type OR module classification) never feeds a
		// module input: author-provided inputs are nonsecret and are persisted in
		// attempt snapshots. List/map/concatenated placeholders are covered.
		for _, name := range sortedKeys(values) {
			if err := b.rejectSecretSources(node, name, contract.Module, values[name], matches, matcher, inspector); err != nil {
				return nil, err
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
			if source == "executor" || variable.Sensitive {
				value = nil // never snapshot an executor-owned or sensitive value
			}
			inputs = append(inputs, ContractInput{Name: name, Type: variable.Type, Source: source, Value: value})
		}

		if typ, ok := b.catalog.Types[node.ResourceType]; ok {
			for _, output := range typ.Outputs {
				if contract.OutputMapping == nil {
					if !contains(contract.Outputs, output.Name) {
						return nil, fmt.Errorf("planning: Terraform module %q does not declare output %q required by resource type %q",
							contract.Module, output.Name, typ.Key)
					}
					continue
				}
				if output.Required && !mapsTo(contract.OutputMapping, output.Name) {
					return nil, fmt.Errorf("planning: definition %q does not map module %q to output %q required by resource type %q",
						def.Key, contract.Module, output.Name, typ.Key)
				}
			}
		}

		pins, err := b.providerPins(node, def, contract)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, TerraformContract{
			Descriptor:         node.Descriptor,
			DefinitionKey:      def.Key,
			Module:             contract.Module,
			Fingerprint:        contract.Fingerprint,
			Outputs:            contract.Outputs,
			Inputs:             inputs,
			Source:             contract.Source,
			BundleID:           contract.BundleID,
			TerraformVersion:   contract.TerraformVersion,
			ProviderLockDigest: contract.ProviderLockDigest,
			OutputMapping:      contract.OutputMapping,
			SecretOutputs:      contract.SecretOutputs,
			ProviderBindings:   pins,
		})
	}
	return contracts, nil
}

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

func mapsTo(mapping map[string]string, target string) bool {
	for _, value := range mapping {
		if value == target {
			return true
		}
	}
	return false
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// ErrSecretInput means a secret output would feed a nonsecret Terraform input.
var ErrSecretInput = errors.New("planning: a secret output cannot feed a nonsecret Terraform input")

// rejectSecretSources fails when any placeholder inside value (nested in
// lists, maps or concatenated strings) names an output classified secret by
// its Resource Type or by the producer's module output mapping. The message
// names descriptors and variables only, never a value.
func (b *graphBuilder) rejectSecretSources(node *Node, variable, module string, value any, matches map[string]Match, matcher *definitionMatcher, inspector ModuleInspector) error {
	return placeholder.WalkStrings(value, "", func(_, text string) error {
		refs, err := placeholder.Refs(text)
		if err != nil {
			return nil // malformed references are reported by their own validation
		}
		for _, ref := range refs {
			if ref.Kind != placeholder.KindResource {
				continue
			}
			provider, ok := node.Bindings[ref.Resource]
			if !ok {
				continue
			}
			secret, err := b.outputIsSecret(provider, ref.OutputKey, matches, matcher, inspector)
			if err != nil {
				return err
			}
			if secret {
				return fmt.Errorf("%w: %s output %q of %s into variable %q of module %q", ErrSecretInput, node.Descriptor, ref.OutputKey, provider, variable, module)
			}
		}
		return nil
	})
}

// outputIsSecret reports the effective classification known at planning time.
func (b *graphBuilder) outputIsSecret(provider, output string, matches map[string]Match, matcher *definitionMatcher, inspector ModuleInspector) (bool, error) {
	node, ok := b.nodes[provider]
	if !ok {
		return false, nil
	}
	if typ, ok := b.catalog.Types[node.ResourceType]; ok {
		if field, ok := typ.Output(output); ok && field.Secret {
			return true, nil
		}
	}
	match, ok := matches[provider]
	if !ok || match.DriverType != resource.DriverTerraform {
		return false, nil
	}
	def, ok := matcher.definition(match.DefinitionKey)
	if !ok || inspector == nil {
		return false, nil
	}
	contract, err := inspector.Contract(def)
	if err != nil {
		return false, fmt.Errorf("planning: definition %q: %w", def.Key, err)
	}
	return contains(contract.SecretOutputs, output), nil
}
