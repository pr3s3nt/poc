// Package configuration implements UC-12 Application Variables & Secrets.
package configuration

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

var (
	ErrInvalid  = errors.New("configuration: invalid input")
	ErrConflict = errors.New("configuration: conflict")
)

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type KeyView struct {
	Name       string             `json:"name"`
	Kind       configuration.Kind `json:"kind"`
	Value      *string            `json:"value,omitempty"`
	Configured bool               `json:"configured"`
	UsedBy     []string           `json:"usedBy"`
}

type View struct {
	ApplicationKey string    `json:"applicationKey"`
	EnvironmentKey string    `json:"environmentKey"`
	RevisionID     string    `json:"revisionId,omitempty"`
	Version        int64     `json:"version"`
	Keys           []KeyView `json:"keys"`
}

type Service struct {
	store    persistence.Store
	provider configport.Provider
}

func NewService(store persistence.Store, provider configport.Provider) *Service {
	return &Service{store: store, provider: provider}
}

func (s *Service) List(ctx context.Context, app, env string) (View, error) {
	scope, revision, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	out := View{ApplicationKey: app, EnvironmentKey: env, RevisionID: scope.DesiredRevisionID, Version: scope.Version, Keys: []KeyView{}}
	names := make([]string, 0, len(revision.Entries))
	for name := range revision.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := revision.Entries[name]
		usedBy, err := s.affected(ctx, app, env, name)
		if err != nil {
			return View{}, err
		}
		item := KeyView{Name: name, Kind: entry.Kind, Configured: true, UsedBy: usedBy}
		if entry.Kind == configuration.Variable {
			value, err := s.provider.ReadValue(ctx, entry.ValueRef)
			if err != nil {
				return View{}, err
			}
			item.Value = &value
		}
		out.Keys = append(out.Keys, item)
	}
	return out, nil
}

func (s *Service) Put(ctx context.Context, app, env, name string, kind configuration.Kind, value string, expectedVersion int64) (View, error) {
	if !keyPattern.MatchString(name) || (kind != configuration.Variable && kind != configuration.Secret) || value == "" || strings.IndexByte(value, 0) >= 0 {
		return View{}, ErrInvalid
	}
	scope, prior, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if scope.Version != expectedVersion {
		return View{}, persistence.ErrVersionConflict
	}
	if existing, ok := prior.Entries[name]; ok && existing.Kind != kind {
		return View{}, fmt.Errorf("%w: key %q already exists with another type", ErrConflict, name)
	}
	ref, err := s.provider.WriteValue(ctx, app, env, value)
	if err != nil {
		return View{}, err
	}
	entries := copyEntries(prior.Entries)
	entries[name] = configuration.Entry{Kind: kind, ValueRef: ref}
	if err := s.commit(ctx, app, env, scope.Version, entries); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

func (s *Service) Rename(ctx context.Context, app, env, oldName, newName string, expectedVersion int64) (View, error) {
	if !keyPattern.MatchString(newName) || oldName == newName {
		return View{}, ErrInvalid
	}
	scope, prior, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if scope.Version != expectedVersion {
		return View{}, persistence.ErrVersionConflict
	}
	entry, ok := prior.Entries[oldName]
	if !ok {
		return View{}, persistence.ErrNotFound
	}
	if _, duplicate := prior.Entries[newName]; duplicate {
		return View{}, ErrConflict
	}
	entries := copyEntries(prior.Entries)
	delete(entries, oldName)
	entries[newName] = entry
	if err := s.commit(ctx, app, env, scope.Version, entries); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

func (s *Service) Delete(ctx context.Context, app, env, name string, expectedVersion int64) (View, error) {
	scope, prior, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if scope.Version != expectedVersion {
		return View{}, persistence.ErrVersionConflict
	}
	if _, ok := prior.Entries[name]; !ok {
		return View{}, persistence.ErrNotFound
	}
	entries := copyEntries(prior.Entries)
	delete(entries, name)
	if err := s.commit(ctx, app, env, scope.Version, entries); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

func (s *Service) load(ctx context.Context, app, env string) (configuration.Scope, configuration.Revision, error) {
	application, err := s.store.GetApplication(ctx, app)
	if err != nil {
		return configuration.Scope{}, configuration.Revision{}, err
	}
	if application.ConfigurationProvider != "" && application.ConfigurationProvider != "vault" {
		return configuration.Scope{}, configuration.Revision{}, fmt.Errorf("configuration: unsupported provider")
	}
	scope, err := s.store.GetConfigurationScope(ctx, app, env)
	if err != nil {
		return configuration.Scope{}, configuration.Revision{}, err
	}
	if scope.DesiredRevisionID == "" {
		return scope, configuration.Revision{Entries: map[string]configuration.Entry{}}, nil
	}
	revision, err := s.store.GetConfigurationRevision(ctx, scope.DesiredRevisionID)
	return scope, revision, err
}

func (s *Service) commit(ctx context.Context, app, env string, expectedVersion int64, entries map[string]configuration.Entry) error {
	return s.store.CommitConfigurationRevision(ctx, expectedVersion, configuration.Revision{
		ID: ids.New(), ApplicationKey: app, EnvironmentKey: env, Version: expectedVersion + 1, Entries: entries,
	})
}

func copyEntries(in map[string]configuration.Entry) map[string]configuration.Entry {
	out := make(map[string]configuration.Entry, len(in))
	for name, entry := range in {
		out[name] = entry
	}
	return out
}

func (s *Service) affected(ctx context.Context, app, env, name string) ([]string, error) {
	drafts, err := s.store.ListWorkloadDrafts(ctx, app, env)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, draft := range drafts {
		if draft.State == environment.DraftUpsert && treeHasReference(draft.Score, name) {
			seen[draft.WorkloadID] = true
		}
	}
	e, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return nil, err
	}
	if e.CurrentDeploymentSetID != "" {
		set, err := s.store.GetDeploymentSet(ctx, e.CurrentDeploymentSetID)
		if err != nil {
			return nil, err
		}
		for workload, module := range set.Document.Modules {
			for _, container := range module.Spec.Containers {
				for _, value := range container.Variables {
					if strings.Contains(value, "${externals.env."+name+"}") || strings.Contains(value, "${resources.env."+name+"}") {
						seen[workload] = true
					}
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for workload := range seen {
		out = append(out, workload)
	}
	sort.Strings(out)
	return out, nil
}

func treeHasReference(value any, key string) bool {
	switch t := value.(type) {
	case string:
		return strings.Contains(t, "${resources.env."+key+"}")
	case map[string]any:
		for _, child := range t {
			if treeHasReference(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if treeHasReference(child, key) {
				return true
			}
		}
	}
	return false
}
