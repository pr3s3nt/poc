package catalogrelease

import (
	"context"
	"orchestrator/internal/ports/persistence"
	"sort"
)

// Usage exposes only scoped selection identity; configuration/account data stays private.
type Usage struct {
	ApplicationKey string `json:"applicationKey"`
	EnvironmentKey string `json:"environmentKey"`
	Tag            string `json:"tag"`
	Desired        bool   `json:"desired"`
	Applied        bool   `json:"applied"`
	Status         string `json:"status"`
}

func (s *Service) Usage(ctx context.Context, org, source string) ([]Usage, error) {
	out := []Usage{}
	err := s.Store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		applications, err := view.ListApplications(ctx)
		if err != nil {
			return err
		}
		for _, app := range applications {
			if app.OrganizationKey != org {
				continue
			}
			envs, err := view.ListEnvironments(ctx, app.Key)
			if err != nil {
				return err
			}
			for _, env := range envs {
				binding, err := view.GetCatalogEnvironment(ctx, app.Key, env.Key)
				if err != nil {
					return err
				}
				byTag := map[string]Usage{}
				if binding.Desired != nil && binding.Desired.SourceID == source {
					byTag[binding.Desired.Tag] = Usage{ApplicationKey: app.Key, EnvironmentKey: env.Key, Tag: binding.Desired.Tag, Desired: true, Status: binding.Status}
				}
				if binding.Applied != nil && binding.Applied.SourceID == source {
					u := byTag[binding.Applied.Tag]
					u.ApplicationKey = app.Key
					u.EnvironmentKey = env.Key
					u.Tag = binding.Applied.Tag
					u.Applied = true
					u.Status = binding.Status
					byTag[u.Tag] = u
				}
				for _, usage := range byTag {
					out = append(out, usage)
				}
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ApplicationKey != b.ApplicationKey {
			return a.ApplicationKey < b.ApplicationKey
		}
		if a.EnvironmentKey != b.EnvironmentKey {
			return a.EnvironmentKey < b.EnvironmentKey
		}
		return a.Tag < b.Tag
	})
	return out, err
}
