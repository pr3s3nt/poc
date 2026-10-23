package deploymentreview

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
)

// Property is a bounded desired-configuration fact, never a resolved secret or
// observed provider output. The planner has already removed sensitive inputs.
type Property struct {
	Key          string `json:"key"`
	Availability string `json:"availability"`
	Value        string `json:"value,omitempty"`
	Source       string `json:"source"`
}

var publicIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,95}$`)
var publicSize = regexp.MustCompile(`^[0-9]{1,6}(Mi|Gi|Ti)$`)
var publicVersion = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9A-Za-z-]{1,12}){0,3}$`)

func (a Assembler) properties(ctx context.Context, node planning.Node, contract planning.TerraformContract) []Property {
	fields := map[string]string{}
	switch node.ResourceType {
	case "postgres":
		fields = map[string]string{"database": "database", "namespace": "namespace", "storage": "storage", "engine_version": "engineVersion"}
	case "redis":
		fields = map[string]string{"namespace": "namespace", "storage": "storage", "engine_version": "engineVersion"}
	case "k8s-namespace":
		fields = map[string]string{"name": "namespace"}
	case "k8s-cluster", "vpc":
		fields = map[string]string{"name": "name"}
	}
	result := []Property{}
	for _, input := range contract.Inputs {
		key, allowed := fields[input.Name]
		if !allowed || input.Type != "string" || input.Value == nil || (input.Source != "resource-input" && input.Source != "driver-input" && input.Source != "terraform-default") {
			continue
		}
		value, ok := input.Value.(string)
		if !ok {
			continue
		}
		property := Property{Key: key, Availability: "UNKNOWN", Source: "configured"}
		if strings.Contains(value, "${") {
			result = append(result, property)
			continue // Never copy an unresolved expression or its references.
		}
		valid := publicIdentifier.MatchString(value)
		if key == "storage" {
			valid = publicSize.MatchString(value)
		}
		if key == "engineVersion" {
			valid = publicVersion.MatchString(value)
		}
		if valid {
			property.Availability, property.Value = "AVAILABLE", value
			result = append(result, property)
		}
	}
	if node.ResourceType == "postgres" || node.ResourceType == "redis" {
		if property, ok := a.portProperty(ctx, contract); ok {
			result = append(result, property)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}

func (a Assembler) portProperty(ctx context.Context, contract planning.TerraformContract) (Property, bool) {
	property := Property{Key: "port", Availability: "UNKNOWN", Source: "module-output"}
	unknown := property
	moduleOutput := ""
	for module, output := range contract.OutputMapping {
		if output == "port" {
			if moduleOutput != "" {
				return property, false
			}
			moduleOutput = module
		}
	}
	if moduleOutput == "" {
		return property, false
	}
	for _, secret := range contract.SecretOutputs {
		if secret == "port" || secret == moduleOutput {
			return property, false
		}
	}
	if a.Sources == nil || contract.Source == nil {
		return property, true
	}
	files, err := a.Sources.ReadPinned(ctx, tfmodule.SourceRef(*contract.Source))
	if err != nil {
		return property, true
	}
	found := false
	for name, data := range files {
		if path.Dir(name) != "." || path.Ext(name) != ".tf" || len(data) > MaxSideBytes {
			continue
		}
		file, diagnostics := hclsyntax.ParseConfig(data, name, hcl.InitialPos)
		if diagnostics.HasErrors() {
			return unknown, true
		}
		for _, block := range file.Body.(*hclsyntax.Body).Blocks {
			if block.Type != "output" || len(block.Labels) != 1 || block.Labels[0] != moduleOutput {
				continue
			}
			if found {
				return Property{Key: "port", Availability: "UNKNOWN", Source: "module-output"}, true
			}
			found = true
			if sensitive, ok := block.Body.Attributes["sensitive"]; ok {
				literal, ok := sensitive.Expr.(*hclsyntax.LiteralValueExpr)
				if !ok || !literal.Val.RawEquals(cty.False) {
					return property, false
				}
			}
			value, ok := block.Body.Attributes["value"]
			if !ok {
				continue
			}
			literal, ok := value.Expr.(*hclsyntax.LiteralValueExpr)
			if !ok || !literal.Val.Type().Equals(cty.Number) {
				continue
			}
			number, accuracy := literal.Val.AsBigFloat().Int64()
			if accuracy == 0 && number > 0 && number <= 65535 {
				property.Availability, property.Value = "AVAILABLE", strconv.FormatInt(number, 10)
			}
		}
	}
	return property, true
}
