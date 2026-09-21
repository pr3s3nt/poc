package bootstrap

import (
	tf "orchestrator/internal/adapters/terraform"
	"orchestrator/internal/ports/execution"
)

// newTerraformExecutor wires the Terraform executor used by the aws-eks profile.
func newTerraformExecutor(opts Options) (execution.ResourceExecutor, error) {
	return tf.New(tf.Options{
		BinaryPath:  opts.TerraformPath,
		Root:        opts.TerraformRoot,
		Region:      opts.Region,
		Tags:        opts.Tags,
		PluginCache: opts.TerraformPluginCache,
	})
}
