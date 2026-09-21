package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/planning"
)

// FixtureInspector resolves the Terraform modules of the challenge fixtures.
// The product inspector reads modules embedded in the binary; the fixtures ship
// local checkouts addressed by url@rev[/path], so the conformance run supplies
// its own inspector instead of changing the product one.
type FixtureInspector struct {
	Sources map[string]string

	mu     sync.Mutex
	cached map[string]planning.ModuleContract
}

// ExecutorVariables reports none: fixture definitions declare every variable.
func (i *FixtureInspector) ExecutorVariables() []string { return nil }

// ExecutorOutputs reports none for the same reason.
func (i *FixtureInspector) ExecutorOutputs(string) []string { return nil }

// Inspect parses the checkout a module key points at.
func (i *FixtureInspector) Inspect(module string) (planning.ModuleContract, error) {
	if module == "" {
		return planning.ModuleContract{}, fmt.Errorf("conformance: definition has no Terraform source")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cached == nil {
		i.cached = map[string]planning.ModuleContract{}
	}
	if cached, ok := i.cached[module]; ok {
		return cached, nil
	}

	key, sub := module, ""
	if idx := strings.Index(module, "/"); idx >= 0 && strings.Contains(module[:idx], "@") {
		key, sub = module[:idx], module[idx+1:]
	}
	dir, ok := i.Sources[key]
	if !ok {
		return planning.ModuleContract{}, fmt.Errorf("conformance: no checkout for Terraform source %q", module)
	}
	if sub != "" {
		dir = filepath.Join(dir, filepath.FromSlash(sub))
	}

	contract, err := readModule(dir, module)
	if err != nil {
		return planning.ModuleContract{}, err
	}
	i.cached[module] = contract
	return contract, nil
}

func readModule(dir, module string) (planning.ModuleContract, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return planning.ModuleContract{}, fmt.Errorf("conformance: read module %q: %w", module, err)
	}
	if len(files) == 0 {
		return planning.ModuleContract{}, fmt.Errorf("conformance: module %q has no .tf file", module)
	}

	type entry struct {
		rel     string
		content []byte
	}
	contents := make([]entry, 0, len(files))
	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			return planning.ModuleContract{}, err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return planning.ModuleContract{}, err
		}
		contents = append(contents, entry{rel: filepath.ToSlash(rel), content: payload})
	}
	sort.Slice(contents, func(i, j int) bool { return contents[i].rel < contents[j].rel })

	contract := planning.ModuleContract{Module: module, Variables: map[string]planning.ModuleVariable{}}
	digest := sha256.New()
	parser := hclparse.NewParser()
	for _, item := range contents {
		digest.Write([]byte(item.rel))
		digest.Write([]byte{0})
		digest.Write(item.content)
		digest.Write([]byte{0})

		file, diags := parser.ParseHCL(item.content, item.rel)
		if diags.HasErrors() {
			return planning.ModuleContract{}, fmt.Errorf("conformance: parse %s: %s", item.rel, diags.Error())
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
				variable := planning.ModuleVariable{Name: label, Type: "string"}
				if attr, ok := block.Body.Attributes["type"]; ok {
					variable.Type = renderType(attr.Expr, item.content)
				}
				if attr, ok := block.Body.Attributes["default"]; ok {
					variable.HasDefault = true
					if value, diags := attr.Expr.Value(nil); !diags.HasErrors() {
						variable.Default = terraform.CtyToGo(value)
					}
				}
				contract.Variables[label] = variable
			case "output":
				contract.Outputs = append(contract.Outputs, label)
			}
		}
	}
	sort.Strings(contract.Outputs)
	contract.Fingerprint = "sha256:" + hex.EncodeToString(digest.Sum(nil))
	return contract, nil
}

func renderType(expr hclsyntax.Expression, source []byte) string {
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

var _ planning.ModuleInspector = (*FixtureInspector)(nil)
