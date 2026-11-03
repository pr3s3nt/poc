package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	moduleregistry "orchestrator/internal/adapters/terraform/registry"
)

// Retain the product's filesystem-only installation policy. Arbitrary custom CLI
// policies remain operator-managed and are never rewritten or broadened.
func retainProviderInstallation(opts *Options, release moduleregistry.Release) error {
	if opts.TerraformCLIConfigFile == "" || opts.TerraformModuleCacheDir == "" {
		return nil
	}
	raw, err := os.ReadFile(opts.TerraformCLIConfigFile)
	if err != nil {
		return err
	}
	file, diags := hclsyntax.ParseConfig(raw, "provider-policy", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("bootstrap: invalid provider installation policy")
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok || len(body.Attributes) != 0 || len(body.Blocks) != 1 || body.Blocks[0].Type != "provider_installation" {
		return nil
	}
	install := body.Blocks[0].Body
	if len(install.Attributes) != 0 || len(install.Blocks) != 1 || install.Blocks[0].Type != "filesystem_mirror" {
		return nil
	}
	mirror := install.Blocks[0].Body
	if len(mirror.Attributes) != 1 || mirror.Attributes["path"] == nil || len(mirror.Blocks) != 0 {
		return nil
	}
	value, diags := mirror.Attributes["path"].Expr.Value(nil)
	if diags.HasErrors() || value.Type() != cty.String || !value.IsKnown() || value.IsNull() || !filepath.IsAbs(value.AsString()) {
		return fmt.Errorf("bootstrap: provider mirror requires a literal absolute path")
	}
	destination := filepath.Join(opts.TerraformModuleCacheDir, "provider-mirror")
	if err := moduleregistry.RetainProviderMirror(release, value.AsString(), destination); err != nil {
		return err
	}
	config := []byte("provider_installation {\n filesystem_mirror {\n  path = " + strconv.Quote(destination) + "\n }\n}\n")
	name := filepath.Join(opts.TerraformModuleCacheDir, "retained-providers.tfrc")
	if err := os.WriteFile(name, config, 0600); err != nil {
		return err
	}
	opts.TerraformCLIConfigFile = name
	return nil
}
