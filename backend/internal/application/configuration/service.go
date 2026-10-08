// Package configuration implements UC-12 Application Variables & Secrets.
package configuration

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

var (
	ErrInvalid  = errors.New("configuration: invalid input")
	ErrConflict = errors.New("configuration: conflict")
	// ErrNoStore is the 422 secretStoreKey failure of a Secret write without
	// a selected store.
	ErrNoStore = configport.ErrNoStore
	// ErrLegacyUnreadable reports a legacy Variable whose explicit legacy
	// store cannot be read; state is preserved and the edit is refused.
	ErrLegacyUnreadable = errors.New("configuration: a legacy value could not be read from its store; restore access to that store and retry")
	// ErrStoreCopy reports a failed secret copy; selection and revision are unchanged.
	ErrStoreCopy = errors.New("configuration: secrets could not be copied to the new store; the selection was not changed")
)

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type KeyView struct {
	Name       string             `json:"name"`
	Kind       configuration.Kind `json:"kind"`
	Value      *string            `json:"value,omitempty"`
	Configured bool               `json:"configured"`
	UsedBy     []string           `json:"usedBy"`
	// StoreKey names the store owning a Secret or unmigrated legacy Variable.
	StoreKey string `json:"storeKey,omitempty"`
	// Legacy marks a Variable still read through its legacy store ref.
	Legacy bool `json:"legacy,omitempty"`
}

type View struct {
	ApplicationKey string    `json:"applicationKey"`
	EnvironmentKey string    `json:"environmentKey"`
	RevisionID     string    `json:"revisionId,omitempty"`
	Version        int64     `json:"version"`
	SecretStoreKey string    `json:"secretStoreKey,omitempty"`
	Keys           []KeyView `json:"keys"`
}

// ValueDeleter is implemented by providers that can remove an attempt-owned
// value object (copy and write rollback).
type ValueDeleter interface {
	DeleteValue(ctx context.Context, ref string) error
}

type Service struct {
	store    persistence.Store
	registry configport.Registry
	ops      *envops.Manager
}

// NewService wires the store registry and the Environment claim manager.
func NewService(store persistence.Store, registry configport.Registry, ops *envops.Manager) *Service {
	if ops == nil {
		ops = envops.NewManager(store)
	}
	return &Service{store: store, registry: registry, ops: ops}
}

type loaded struct {
	org      string
	env      environment.Environment
	scope    configuration.Scope
	revision configuration.Revision
}

func (s *Service) load(ctx context.Context, app, env string) (loaded, error) {
	application, err := s.store.GetApplication(ctx, app)
	if err != nil {
		return loaded{}, err
	}
	record, err := s.store.GetEnvironment(ctx, app, env)
	if err != nil {
		return loaded{}, err
	}
	scope, err := s.store.GetConfigurationScope(ctx, app, env)
	if err != nil {
		return loaded{}, err
	}
	out := loaded{org: application.OrganizationKey, env: record, scope: scope, revision: configuration.Revision{Entries: map[string]configuration.Entry{}}}
	if scope.DesiredRevisionID != "" {
		out.revision, err = s.store.GetConfigurationRevision(ctx, scope.DesiredRevisionID)
		if err != nil {
			return loaded{}, err
		}
	}
	return out, nil
}

func (s *Service) List(ctx context.Context, app, env string) (View, error) {
	l, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	out := View{ApplicationKey: app, EnvironmentKey: env, RevisionID: l.scope.DesiredRevisionID, Version: l.scope.Version, SecretStoreKey: l.env.SecretStoreKey, Keys: []KeyView{}}
	names := make([]string, 0, len(l.revision.Entries))
	for name := range l.revision.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := l.revision.Entries[name]
		usedBy, err := s.affected(ctx, app, env, name)
		if err != nil {
			return View{}, err
		}
		item := KeyView{Name: name, Kind: entry.Kind, Configured: true, UsedBy: usedBy, StoreKey: entry.StoreKey}
		if entry.Kind == configuration.Variable {
			if entry.LegacyVariable() {
				item.Legacy = true
				// A legacy value is read through its explicit store; an
				// unreadable one shows no value instead of failing the list.
				if value, err := s.readEntry(ctx, l.org, entry); err == nil {
					item.Value = &value
				}
			} else {
				value := entry.Value
				item.Value = &value
			}
		}
		out.Keys = append(out.Keys, item)
	}
	return out, nil
}

// readEntry reads one store-backed entry through its owning store.
func (s *Service) readEntry(ctx context.Context, org string, entry configuration.Entry) (string, error) {
	if entry.StoreKey == "" {
		return "", ErrLegacyUnreadable
	}
	provider, err := s.registry.Provider(ctx, org, entry.StoreKey)
	if err != nil {
		return "", ErrLegacyUnreadable
	}
	value, err := provider.ReadValue(ctx, entry.ValueRef)
	if err != nil {
		return "", ErrLegacyUnreadable
	}
	return value, nil
}

// materialize converts every legacy Variable of entries into an ordinary
// Variable value. Reads happen before any transaction; a failure refuses the
// edit and changes nothing.
func (s *Service) materialize(ctx context.Context, org string, entries map[string]configuration.Entry) error {
	for name, entry := range entries {
		if !entry.LegacyVariable() {
			continue
		}
		value, err := s.readEntry(ctx, org, entry)
		if err != nil {
			return err
		}
		entries[name] = configuration.Entry{Kind: configuration.Variable, Value: value}
	}
	return nil
}

func (s *Service) checkVersion(l loaded, expected int64) error {
	if l.env.Busy() {
		return persistence.Busy(l.env.ApplicationKey, l.env.Key)
	}
	if l.scope.Version != expected {
		return persistence.ErrVersionConflict
	}
	return nil
}

func (s *Service) Put(ctx context.Context, app, env, name string, kind configuration.Kind, value string, expectedVersion int64) (View, error) {
	if !keyPattern.MatchString(name) || (kind != configuration.Variable && kind != configuration.Secret) || value == "" || strings.IndexByte(value, 0) >= 0 {
		return View{}, ErrInvalid
	}
	l, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if err := s.checkVersion(l, expectedVersion); err != nil {
		return View{}, err
	}
	if existing, ok := l.revision.Entries[name]; ok && existing.Kind != kind {
		return View{}, fmt.Errorf("%w: key %q already exists with another type", ErrConflict, name)
	}
	entries := copyEntries(l.revision.Entries)
	if err := s.materialize(ctx, l.org, entries); err != nil {
		return View{}, err
	}
	var written string
	var provider configport.Provider
	if kind == configuration.Secret {
		if l.env.SecretStoreKey == "" {
			return View{}, ErrNoStore
		}
		provider, err = s.registry.Provider(ctx, l.org, l.env.SecretStoreKey)
		if err != nil {
			return View{}, err
		}
		written, err = provider.WriteValue(ctx, app, env, value)
		if err != nil {
			return View{}, err
		}
		entries[name] = configuration.Entry{Kind: configuration.Secret, ValueRef: written, StoreKey: l.env.SecretStoreKey}
	} else {
		entries[name] = configuration.Entry{Kind: configuration.Variable, Value: value}
	}
	if err := s.commit(ctx, app, env, l.scope.Version, entries); err != nil {
		s.discard(ctx, provider, written)
		return View{}, err
	}
	return s.List(ctx, app, env)
}

// discard removes an attempt-owned value that never became part of a revision.
func (s *Service) discard(ctx context.Context, provider configport.Provider, ref string) {
	if provider == nil || ref == "" {
		return
	}
	if deleter, ok := provider.(ValueDeleter); ok {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_ = deleter.DeleteValue(cleanup, ref)
	}
}

func (s *Service) Rename(ctx context.Context, app, env, oldName, newName string, expectedVersion int64) (View, error) {
	if !keyPattern.MatchString(newName) || oldName == newName {
		return View{}, ErrInvalid
	}
	l, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if err := s.checkVersion(l, expectedVersion); err != nil {
		return View{}, err
	}
	if _, ok := l.revision.Entries[oldName]; !ok {
		return View{}, persistence.ErrNotFound
	}
	if _, duplicate := l.revision.Entries[newName]; duplicate {
		return View{}, ErrConflict
	}
	entries := copyEntries(l.revision.Entries)
	if err := s.materialize(ctx, l.org, entries); err != nil {
		return View{}, err
	}
	entries[newName] = entries[oldName]
	delete(entries, oldName)
	if err := s.commit(ctx, app, env, l.scope.Version, entries); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

func (s *Service) Delete(ctx context.Context, app, env, name string, expectedVersion int64) (View, error) {
	l, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	if err := s.checkVersion(l, expectedVersion); err != nil {
		return View{}, err
	}
	if _, ok := l.revision.Entries[name]; !ok {
		return View{}, persistence.ErrNotFound
	}
	entries := copyEntries(l.revision.Entries)
	delete(entries, name)
	if err := s.materialize(ctx, l.org, entries); err != nil {
		return View{}, err
	}
	if err := s.commit(ctx, app, env, l.scope.Version, entries); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
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
