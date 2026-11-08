package planning

import (
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
)

// moduleSpecReferenceTree excludes literal file content from reference discovery.
// GW-06 ConfigMap values are opaque text, including shell ${...} and text that
// resembles a platform reference. The authored spec itself stays untouched.
func moduleSpecReferenceTree(spec environment.ModuleSpec) (map[string]any, error) {
	tree, err := canon.Map(spec)
	if err != nil {
		return nil, err
	}
	execution, _ := tree["workloadExecution"].(map[string]any)
	volumes, _ := execution["volumes"].([]any)
	for _, raw := range volumes {
		volume, _ := raw.(map[string]any)
		config, _ := volume["configMap"].(map[string]any)
		files, _ := config["files"].([]any)
		for _, rawFile := range files {
			file, _ := rawFile.(map[string]any)
			delete(file, "value")
		}
	}
	return tree, nil
}
