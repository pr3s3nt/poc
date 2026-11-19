package catalog

import (
	"context"
	"sort"
	"strings"

	"orchestrator/internal/application/definitionversions"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/endpoint"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
)

type DeletionBlocked struct {
	References []string
}

func (e *DeletionBlocked) Error() string { return "catalog: deletion is blocked" }

func (s *Service) DeleteEntry(ctx context.Context, org, key string, definition bool) error {
	if definition && key == planning.BuiltinClusterKey {
		return &DeletionBlocked{References: []string{"builtin"}}
	}
	if !definition {
		switch key {
		case planning.TypeWorkload, planning.TypeCluster, planning.TypeNamespace, planning.TypeVPC, endpoint.GatewayType, "endpoint", "postgres", "redis":
			return &DeletionBlocked{References: []string{"builtin"}}
		}
	}
	return s.store.DeleteCatalogEntry(ctx, org, key, definition, func(ctx context.Context) error {
		return s.checkDeletionReferences(ctx, org, key, definition)
	})
}

func (s *Service) checkDeletionReferences(ctx context.Context, org, key string, definition bool) error {
	references := map[string]bool{}
	if !definition {
		defs, err := s.store.ListResourceDefinitions(ctx, org)
		if err != nil {
			return err
		}
		for _, def := range defs {
			used := def.ResourceTypeKey == key || documentUsesType(def.DriverInputs, key)
			for target := range def.Provision {
				used = used || descriptorUsesType(target, key)
			}
			if used {
				references["definition/"+def.Key] = true
			}
		}
	}
	resources, err := s.store.ListActiveResources(ctx, org)
	if err != nil {
		return err
	}
	for _, active := range resources {
		if (definition && active.CatalogSource == "" && active.DefinitionKey == key) || (!definition && active.Descriptor.Type == key) {
			references["resource/"+active.Descriptor.String()] = true
		}
	}
	apps, err := s.store.ListApplications(ctx)
	if err != nil {
		return err
	}
	for _, app := range apps {
		if app.OrganizationKey != org || app.Lifecycle() == appdomain.StatusDeleted {
			continue
		}
		envs, err := s.store.ListEnvironments(ctx, app.Key)
		if err != nil {
			return err
		}
		for _, env := range envs {
			label := app.Key + "/" + env.Key
			if env.ActiveOperationID != "" {
				references[label+"/operation"] = true
			}
			selection, err := definitionversions.Effective(ctx, s.store, app.Key, env.Key)
			if err != nil {
				return err
			}
			for _, snapshots := range [][]resource.Definition{selection.Baseline, selection.Desired, selection.Applied} {
				for _, def := range snapshots {
					if (definition && def.Key == key) || (!definition && def.ResourceTypeKey == key) {
						references[label+"/selection"] = true
					}
				}
			}
			if definition {
				continue
			}
			if env.CurrentDeploymentSetID != "" {
				set, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
				if err != nil {
					return err
				}
				for _, entry := range set.Document.Shared {
					if entry.Type == key {
						references[label] = true
					}
				}
				for workload, module := range set.Document.Modules {
					for _, entry := range module.Externals {
						if entry.Type == key {
							references[label+"/"+workload] = true
						}
					}
				}
			}
			drafts, err := s.store.ListWorkloadDrafts(ctx, app.Key, env.Key)
			if err != nil {
				return err
			}
			for _, draft := range drafts {
				if documentUsesType(draft.Workload, key) {
					references[label+"/"+draft.WorkloadID] = true
				}
			}
		}
	}
	if len(references) == 0 {
		return nil
	}
	out := &DeletionBlocked{}
	for reference := range references {
		out.References = append(out.References, reference)
	}
	sort.Strings(out.References)
	return out
}

func descriptorUsesType(descriptor, key string) bool {
	return descriptor == key || strings.HasPrefix(descriptor, key+".") || strings.HasPrefix(descriptor, key+"#")
}

func documentUsesType(value any, key string) bool {
	switch value := value.(type) {
	case map[string]any:
		for name, field := range value {
			if text, ok := field.(string); ok {
				switch name {
				case "type", "resourceType":
					if text == key {
						return true
					}
				case "resource_ref":
					if descriptorUsesType(text, key) {
						return true
					}
				}
			}
			if documentUsesType(field, key) {
				return true
			}
		}
	case []any:
		for _, entry := range value {
			if documentUsesType(entry, key) {
				return true
			}
		}
	}
	return false
}
