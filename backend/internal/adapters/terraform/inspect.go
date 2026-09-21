package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"orchestrator/internal/planning"
)

// executorVariables are supplied by the executor itself, not by a Resource
// Definition: the run region, the run tags and the generated master password.
var executorVariables = []string{"region", "tags", "master_password"}

// Inspector parses the embedded Terraform modules so planning can check the
// resource inputs, the driver variables and the module outputs before anything
// is applied (UC-06 BR-09).
type Inspector struct {
	mu     sync.Mutex
	cached map[string]planning.ModuleContract
}

// NewInspector returns a Terraform module inspector.
func NewInspector() *Inspector {
	return &Inspector{cached: map[string]planning.ModuleContract{}}
}

// executorOutputs are produced by the executor after apply, not by Terraform.
var executorOutputs = map[string][]string{
	ModuleEKS: {"kubeContext", "kubeconfig"},
}

// ExecutorOutputs lists the outputs the executor adds after apply.
func (i *Inspector) ExecutorOutputs(module string) []string {
	return append([]string(nil), executorOutputs[module]...)
}

// ExecutorVariables lists the variables the executor provides.
func (i *Inspector) ExecutorVariables() []string {
	return append([]string(nil), executorVariables...)
}

// Inspect parses one embedded module and returns its contract.
func (i *Inspector) Inspect(module string) (planning.ModuleContract, error) {
	if module == "" {
		return planning.ModuleContract{}, fmt.Errorf("terraform: the definition source has no module name")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if cached, ok := i.cached[module]; ok {
		return cached, nil
	}

	dir := path.Join("modules", module)
	entries, err := fs.ReadDir(moduleFS, dir)
	if err != nil {
		return planning.ModuleContract{}, fmt.Errorf("terraform: unknown module %q", module)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return planning.ModuleContract{}, fmt.Errorf("terraform: module %q has no .tf file", module)
	}
	sort.Strings(names)

	contract := planning.ModuleContract{Module: module, Variables: map[string]planning.ModuleVariable{}}
	digest := sha256.New()
	parser := hclparse.NewParser()
	for _, name := range names {
		content, err := moduleFS.ReadFile(path.Join(dir, name))
		if err != nil {
			return planning.ModuleContract{}, fmt.Errorf("terraform: read module %q: %w", module, err)
		}
		digest.Write([]byte(name))
		digest.Write([]byte{0})
		digest.Write(content)
		digest.Write([]byte{0})

		file, diags := parser.ParseHCL(content, name)
		if diags.HasErrors() {
			return planning.ModuleContract{}, fmt.Errorf("terraform: parse module %q: %s", module, diags.Error())
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, block := range body.Blocks {
			if len(block.Labels) == 0 {
				continue
			}
			label := block.Labels[0]
			switch block.Type {
			case "variable":
				contract.Variables[label] = readVariable(label, block, content)
			case "output":
				contract.Outputs = append(contract.Outputs, label)
			}
		}
	}
	sort.Strings(contract.Outputs)
	contract.Fingerprint = "sha256:" + hex.EncodeToString(digest.Sum(nil))
	i.cached[module] = contract
	return contract, nil
}

func readVariable(name string, block *hclsyntax.Block, source []byte) planning.ModuleVariable {
	variable := planning.ModuleVariable{Name: name, Type: "string"}
	if attr, ok := block.Body.Attributes["type"]; ok {
		variable.Type = renderTypeExpression(attr.Expr, source)
	}
	if value, ok := variableDefault(block); ok {
		variable.HasDefault = true
		variable.Default = value
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

var _ planning.ModuleInspector = (*Inspector)(nil)
