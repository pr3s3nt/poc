package planning

import (
	"fmt"
	"reflect"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
)

func storageCapability(typ resource.Type) bool {
	fields := map[string]bool{}
	for _, output := range typ.Outputs {
		if !output.Secret && output.Type == "string" {
			fields[output.Name] = true
		}
	}
	return fields["claimName"] && fields["namespace"]
}

// Storage parameters are immutable in v1. Expansion needs explicit provider /
// StorageClass capability, which v1 does not advertise; never guess support.
func validateStorageIntent(before, after environment.Document, catalog Catalog) error {
	check := func(old, next environment.ResourceEntry) error {
		if old.Type != next.Type || old.Class != next.Class || !storageCapability(catalog.Types[old.Type]) {
			return nil
		}
		changed := false
		for _, key := range []string{"storage", "size", "storage_class", "storageClass", "access_mode", "accessMode", "volume_mode", "volumeMode", "namespace"} {
			changed = changed || !reflect.DeepEqual(old.Params[key], next.Params[key])
		}
		if changed {
			return &StageError{Stage: StageCandidate, Reason: ReasonStorageImmutable, Err: fmt.Errorf("storage parameters require an explicit migration or verified expansion capability")}
		}
		return nil
	}
	for id, next := range after.Shared {
		if old, ok := before.Shared[id]; ok {
			if err := check(old, next); err != nil {
				return err
			}
		}
	}
	for id, module := range after.Modules {
		oldModule, ok := before.Modules[id]
		if !ok {
			continue
		}
		for alias, next := range module.Externals {
			if old, ok := oldModule.Externals[alias]; ok {
				if err := check(old, next); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
