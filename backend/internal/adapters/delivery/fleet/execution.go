package fleet

import (
	"context"
	"encoding/json"

	"orchestrator/internal/adapters/delivery/artifact"
	"orchestrator/internal/ports/execution"
)

// SameExecution ignores only platform-generated provenance. Business labels,
// annotations, selectors, Job and Pod specs remain part of the comparison.
func (p *Provider) SameExecution(_ context.Context, previous []byte, proposed []execution.Manifest) (bool, error) {
	var pkg Package
	if json.Unmarshal(previous, &pkg) != nil {
		return false, ErrConfig
	}
	prior := []map[string]any{}
	for name, raw := range pkg.Publication.Files {
		if name == "fleet.yaml" {
			continue
		}
		var object map[string]any
		if json.Unmarshal(raw, &object) != nil || object["kind"] != "Job" {
			return false, ErrConfig
		}
		prior = append(prior, object)
	}
	next := []map[string]any{}
	for _, m := range proposed {
		if m.Kind != "Job" {
			return false, ErrConfig
		}
		raw, err := json.Marshal(m.Object)
		if err != nil {
			return false, ErrConfig
		}
		var object map[string]any
		if json.Unmarshal(raw, &object) != nil {
			return false, ErrConfig
		}
		next = append(next, object)
	}
	return artifact.SameExecution(prior, next)
}

func (p *Provider) ExportArtifact(_ context.Context, raw []byte) ([]execution.Manifest, error) {
	var pkg Package
	if json.Unmarshal(raw, &pkg) != nil {
		return nil, ErrConfig
	}
	objects := []map[string]any{}
	for name, body := range pkg.Publication.Files {
		if name == "fleet.yaml" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal(body, &obj) != nil {
			return nil, ErrConfig
		}
		objects = append(objects, obj)
	}
	return artifact.Export(objects)
}
