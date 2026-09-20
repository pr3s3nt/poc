package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type sourceEntry struct {
	URL       string
	Rev       string
	Directory string
}

// terraformCache reads each referenced module at most once.
type terraformCache struct {
	root    string
	sources []sourceEntry
	modules map[string]*TerraformModule
}

func newTerraformCache(c *LoadedCase) *terraformCache {
	tc := &terraformCache{root: c.Root, modules: map[string]*TerraformModule{}}
	for _, entry := range c.Manifest.Spec.TerraformSourceMap {
		tc.sources = append(tc.sources, sourceEntry{URL: entry.URL, Rev: entry.Rev, Directory: entry.Directory})
	}
	return tc
}

func definitionSource(def *Definition) Document {
	return mapAt(mapAt(def.DriverInputs, "values"), "source")
}

func driverVariables(def *Definition) Document {
	return mapAt(mapAt(def.DriverInputs, "values"), "variables")
}

// moduleFor resolves the Terraform module a Definition points at.
func (tc *terraformCache) moduleFor(def *Definition, descriptor string) (*TerraformModule, error) {
	source := definitionSource(def)
	if source == nil {
		return nil, newError("terraform", "SOURCE_REQUIRED", descriptor)
	}
	url := stringAt(source, "url")
	if url == "" {
		return nil, newError("terraform", "SOURCE_REQUIRED", descriptor)
	}
	rev := stringAt(source, "rev")

	directory := ""
	for _, entry := range tc.sources {
		if entry.URL == url && entry.Rev == rev {
			directory = entry.Directory
			break
		}
	}
	if directory == "" {
		return nil, newError("terraform", "SOURCE_NOT_FOUND", descriptor)
	}
	relative := path.Join(directory, stringAt(source, "path"))
	if cached, ok := tc.modules[relative]; ok {
		return cached, nil
	}

	module, err := readTerraformModule(filepath.Join(tc.root, filepath.FromSlash(relative)), relative)
	if err != nil {
		return nil, newError("terraform", "SOURCE_NOT_FOUND", descriptor)
	}
	tc.modules[relative] = module
	return module, nil
}

// readTerraformModule parses every *.tf file below dir.
func readTerraformModule(dir, relative string) (*TerraformModule, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".tf") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fs.ErrNotExist
	}

	type entry struct {
		rel     string
		content []byte
	}
	var contents []entry
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil, err
		}
		contents = append(contents, entry{rel: filepath.ToSlash(rel), content: data})
	}
	sort.Slice(contents, func(i, j int) bool { return contents[i].rel < contents[j].rel })

	module := &TerraformModule{Directory: relative, Variables: map[string]TerraformVariable{}}
	digest := sha256.New()
	parser := hclparse.NewParser()
	for _, item := range contents {
		digest.Write([]byte(item.rel))
		digest.Write([]byte{0})
		digest.Write(item.content)
		digest.Write([]byte{0})

		file, diags := parser.ParseHCL(item.content, item.rel)
		if diags.HasErrors() {
			return nil, diags
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, block := range body.Blocks {
			if len(block.Labels) == 0 {
				continue
			}
			name := block.Labels[0]
			switch block.Type {
			case "variable":
				module.Variables[name] = readVariableBlock(name, block, item.content)
			case "output":
				module.Outputs = append(module.Outputs, name)
			}
		}
	}
	sort.Strings(module.Outputs)
	module.Fingerprint = "sha256:" + hex.EncodeToString(digest.Sum(nil))
	return module, nil
}

func readVariableBlock(name string, block *hclsyntax.Block, source []byte) TerraformVariable {
	variable := TerraformVariable{Name: name, Type: "string"}
	if attr, ok := block.Body.Attributes["type"]; ok {
		variable.Type = renderTypeExpression(attr.Expr, source)
	}
	if attr, ok := block.Body.Attributes["default"]; ok {
		variable.HasDefault = true
		if value, diags := attr.Expr.Value(nil); !diags.HasErrors() {
			variable.Default = ctyToGo(value)
		}
	}
	return variable
}

func renderTypeExpression(expr hclsyntax.Expression, source []byte) string {
	if traversal, ok := expr.(*hclsyntax.ScopeTraversalExpr); ok && len(traversal.Traversal) == 1 {
		if root, ok := traversal.Traversal[0].(hcl.TraverseRoot); ok {
			return root.Name
		}
	}
	rng := expr.Range()
	if rng.Start.Byte >= 0 && rng.End.Byte <= len(source) {
		return strings.TrimSpace(string(source[rng.Start.Byte:rng.End.Byte]))
	}
	return "string"
}

func ctyToGo(value cty.Value) any {
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
		if float == float64(int64(float)) {
			return int(float)
		}
		return float
	case value.Type().IsTupleType() || value.Type().IsListType() || value.Type().IsSetType():
		out := []any{}
		for it := value.ElementIterator(); it.Next(); {
			_, element := it.Element()
			out = append(out, ctyToGo(element))
		}
		return out
	case value.Type().IsObjectType() || value.Type().IsMapType():
		out := Document{}
		for it := value.ElementIterator(); it.Next(); {
			key, element := it.Element()
			out[key.AsString()] = ctyToGo(element)
		}
		return out
	}
	return nil
}

// resolvedDriverVariables applies ${context.*} substitution to every string
// leaf of the Definition supplied Terraform variables, including values nested
// in maps and lists. Resource references stay raw bindings.
func resolvedDriverVariables(def *Definition, node *Node, ctx Document) Document {
	out := Document{}
	for key, value := range driverVariables(def) {
		resolved, err := mapStrings(deepCopy(value), "", func(_ string, text string) (any, error) {
			return resolveContext(text, ctx, node), nil
		})
		if err != nil {
			resolved = deepCopy(value)
		}
		out[key] = resolved
	}
	return out
}

// buildTerraformRecord resolves one node's Terraform invocation contract.
func buildTerraformRecord(node *Node, module *TerraformModule, ctx Document, types map[string]*ResourceType) (*TerraformRecord, error) {
	desc := node.Descriptor()
	params := node.ResourceInputs
	driver := resolvedDriverVariables(node.Definition, node, ctx)

	provided := Document{}
	for key := range params {
		provided[key] = true
	}
	for key := range driver {
		provided[key] = true
	}
	names := map[string]bool{}
	for key := range provided {
		names[key] = true
	}
	for key := range module.Variables {
		names[key] = true
	}
	ordered := make([]string, 0, len(names))
	for key := range names {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	for _, name := range ordered {
		variable, declared := module.Variables[name]
		if provided[name] == true && !declared {
			return nil, newError("terraform", "UNKNOWN_TERRAFORM_INPUT", desc+":"+name)
		}
		if declared && provided[name] != true && !variable.HasDefault {
			return nil, newError("terraform", "MISSING_TERRAFORM_INPUT", desc+":"+name)
		}
	}

	if rt, ok := types[node.Type]; ok {
		for _, name := range sortedKeys(rt.Outputs) {
			if !contains(module.Outputs, name) {
				return nil, newError("terraform", "MISSING_TERRAFORM_OUTPUT", desc+":"+name)
			}
		}
	}

	record := &TerraformRecord{
		Resource:     desc,
		DefinitionID: node.Definition.ID,
		Directory:    module.Directory,
		Fingerprint:  module.Fingerprint,
		Outputs:      module.Outputs,
		Inputs:       []Document{},
	}
	source := definitionSource(node.Definition)
	record.Source = Document{"url": stringAt(source, "url")}
	if rev := stringAt(source, "rev"); rev != "" {
		record.Source["rev"] = rev
	}
	if sub := stringAt(source, "path"); sub != "" {
		record.Source["path"] = sub
	}

	variableNames := make([]string, 0, len(module.Variables))
	for name := range module.Variables {
		variableNames = append(variableNames, name)
	}
	sort.Strings(variableNames)
	for _, name := range variableNames {
		variable := module.Variables[name]
		input := Document{"name": name, "type": variable.Type}
		switch {
		case hasKey(params, name):
			input["source"] = "resource-input"
			input["value"] = params[name]
		case hasKey(driver, name):
			input["source"] = "driver-input"
			input["value"] = driver[name]
		default:
			input["source"] = "terraform-default"
			input["value"] = variable.Default
		}
		record.Inputs = append(record.Inputs, input)
	}
	return record, nil
}

func hasKey(doc Document, key string) bool {
	_, ok := doc[key]
	return ok
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
