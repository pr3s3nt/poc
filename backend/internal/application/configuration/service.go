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
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/score"
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
	// ErrStoreCopy reports a failed secret copy; selection and revision are unchanged.
	ErrStoreCopy = errors.New("configuration: secrets could not be copied to the new store; the selection was not changed")
	// ErrReferenceRewriteUnsafe is the 422 failure of a rename whose desired
	// references cannot be rewritten without losing configuration.
	ErrReferenceRewriteUnsafe = workloadconfig.ErrReferenceRewriteUnsafe
)

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type KeyView struct {
	Name       string             `json:"name"`
	Kind       configuration.Kind `json:"kind"`
	Value      *string            `json:"value,omitempty"`
	Configured bool               `json:"configured"`
	UsedBy     []string           `json:"usedBy"`
	// StoreKey names the store owning a Secret.
	StoreKey string `json:"storeKey,omitempty"`
}

type View struct {
	ApplicationKey string `json:"applicationKey"`
	EnvironmentKey string `json:"environmentKey"`
	RevisionID     string `json:"revisionId,omitempty"`
	Version        int64  `json:"version"`
	SecretStoreKey string `json:"secretStoreKey,omitempty"`
	// EnvironmentVersion and DraftVersion are read in the same snapshot as
	// Version/RevisionID; an explicit reference rename sends them back as guards.
	EnvironmentVersion int64     `json:"environmentVersion"`
	DraftVersion       int64     `json:"draftVersion"`
	Keys               []KeyView `json:"keys"`
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

// List reads the view from one consistent snapshot, so configuration, Environment
// and draft versions always describe the same moment.
func (s *Service) List(ctx context.Context, app, env string) (View, error) {
	var out View
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		var err error
		out, err = (&Service{store: view, registry: s.registry, ops: s.ops}).list(ctx, app, env)
		return err
	})
	return out, err
}

func (s *Service) list(ctx context.Context, app, env string) (View, error) {
	l, err := s.load(ctx, app, env)
	if err != nil {
		return View{}, err
	}
	out := View{ApplicationKey: app, EnvironmentKey: env, RevisionID: l.scope.DesiredRevisionID, Version: l.scope.Version, SecretStoreKey: l.env.SecretStoreKey,
		EnvironmentVersion: l.env.Version, DraftVersion: l.env.DraftVersion, Keys: []KeyView{}}
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
			value := entry.Value
			item.Value = &value
		}
		out.Keys = append(out.Keys, item)
	}
	return out, nil
}

var errSecretUnreadable = errors.New("configuration: a secret could not be read from its store")

// readEntry reads one Secret through its explicit owning store.
func (s *Service) readEntry(ctx context.Context, org string, entry configuration.Entry) (string, error) {
	if entry.StoreKey == "" {
		return "", errSecretUnreadable
	}
	provider, err := s.registry.Provider(ctx, org, entry.StoreKey)
	if err != nil {
		return "", errSecretUnreadable
	}
	value, err := provider.ReadValue(ctx, entry.ValueRef)
	if err != nil {
		return "", errSecretUnreadable
	}
	return value, nil
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

// ReferenceGuards are the Environment and draft versions a client read
// together with the configuration view. They are only used when UpdateReferences
// is set; a missing guard is never replaced by zero (the delivery layer rejects it).
type ReferenceGuards struct {
	UpdateReferences   bool
	EnvironmentVersion int64
	DraftVersion       int64
}

// Rename renames one key and keeps every workload reference unchanged.
func (s *Service) Rename(ctx context.Context, app, env, oldName, newName string, expectedVersion int64) (View, error) {
	return s.RenameKey(ctx, app, env, oldName, newName, expectedVersion, ReferenceGuards{})
}

// RenameKey renames one key. With UpdateReferences it also rewrites the exact
// references in desired workload drafts and commits the configuration revision
// and all changed drafts atomically (OC-12 T30). It never reads Secret values,
// calls a provider or touches the current Deployment Set or runtime.
func (s *Service) RenameKey(ctx context.Context, app, env, oldName, newName string, expectedVersion int64, guards ReferenceGuards) (View, error) {
	if !keyPattern.MatchString(newName) || oldName == newName {
		return View{}, ErrInvalid
	}
	if guards.UpdateReferences {
		return s.renameWithReferences(ctx, app, env, oldName, newName, expectedVersion, guards)
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
	entries[newName] = entries[oldName]
	delete(entries, oldName)
	if err := s.commit(ctx, app, env, l.scope.Version, entries); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

type renamePlan struct {
	command persistence.DesiredRename
}

func (s *Service) renameWithReferences(ctx context.Context, app, env, oldName, newName string, expectedVersion int64, guards ReferenceGuards) (View, error) {
	var plan renamePlan
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		snapshot := &Service{store: view, registry: s.registry, ops: s.ops}
		var err error
		plan, err = snapshot.prepareRename(ctx, app, env, oldName, newName, expectedVersion, guards)
		return err
	})
	if err != nil {
		return View{}, err
	}
	if err := s.store.CommitDesiredRename(ctx, plan.command); err != nil {
		return View{}, err
	}
	return s.List(ctx, app, env)
}

// prepareRename runs every guard and builds the complete desired change from
// one snapshot; nothing is written. The commit repeats the guards atomically.
func (s *Service) prepareRename(ctx context.Context, app, env, oldName, newName string, expectedVersion int64, guards ReferenceGuards) (renamePlan, error) {
	l, err := s.load(ctx, app, env)
	if err != nil {
		return renamePlan{}, err
	}
	if err := s.checkVersion(l, expectedVersion); err != nil {
		return renamePlan{}, err
	}
	if l.env.Version != guards.EnvironmentVersion || l.env.DraftVersion != guards.DraftVersion {
		return renamePlan{}, persistence.ErrVersionConflict
	}
	if _, ok := l.revision.Entries[oldName]; !ok {
		return renamePlan{}, persistence.ErrNotFound
	}
	if _, duplicate := l.revision.Entries[newName]; duplicate {
		return renamePlan{}, ErrConflict
	}
	drafts, err := s.rewriteDrafts(ctx, app, env, l, oldName, newName)
	if err != nil {
		return renamePlan{}, err
	}
	entries := copyEntries(l.revision.Entries)
	entries[newName] = entries[oldName]
	delete(entries, oldName)
	return renamePlan{command: persistence.DesiredRename{
		ApplicationKey: app, EnvironmentKey: env,
		ExpectedConfigVersion: l.scope.Version, ExpectedEnvironmentVersion: l.env.Version, ExpectedDraftVersion: l.env.DraftVersion,
		ExpectedCurrentSetID: l.env.CurrentDeploymentSetID,
		Revision:             configuration.Revision{ID: ids.New(), ApplicationKey: app, EnvironmentKey: env, Version: l.scope.Version + 1, Entries: entries},
		Drafts:               drafts,
	}}, nil
}

// rewriteDrafts prepares the desired upsert drafts that carry the renamed key.
// An upsert draft supersedes its deployed module; a pending delete is skipped;
// a deployed module without a draft is rebuilt losslessly first. Any shape that
// cannot be rewritten without loss rejects the whole rename.
func (s *Service) rewriteDrafts(ctx context.Context, app, env string, l loaded, oldName, newName string) ([]environment.WorkloadDraft, error) {
	existing, err := s.store.ListWorkloadDrafts(ctx, app, env)
	if err != nil {
		return nil, err
	}
	byID := map[string]environment.WorkloadDraft{}
	for _, draft := range existing {
		byID[draft.WorkloadID] = draft
	}
	var out []environment.WorkloadDraft
	for id, draft := range byID {
		if draft.State != environment.DraftUpsert || !workloadconfig.ReferencesEnvironmentKey(draft.Score, oldName) {
			continue
		}
		rewritten, changed, err := workloadconfig.RewriteEnvironmentKey(draft.Score, oldName, newName)
		if err != nil || !changed {
			return nil, ErrReferenceRewriteUnsafe
		}
		if _, err := score.FromMap(rewritten); err != nil {
			return nil, ErrReferenceRewriteUnsafe
		}
		out = append(out, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: id, State: environment.DraftUpsert, Score: rewritten})
	}
	if l.env.CurrentDeploymentSetID != "" {
		set, err := s.store.GetDeploymentSet(ctx, l.env.CurrentDeploymentSetID)
		if err != nil {
			return nil, err
		}
		var types map[string]resource.Type
		for id, module := range set.Document.Modules {
			if _, hasDraft := byID[id]; hasDraft || !workloadconfig.ModuleReferencesKey(module, set.Document, oldName) {
				continue
			}
			if types == nil {
				list, err := s.store.ListResourceTypes(ctx, l.org)
				if err != nil {
					return nil, err
				}
				types = make(map[string]resource.Type, len(list))
				for _, typ := range list {
					types[typ.Key] = typ
				}
			}
			raw, err := workloadconfig.ReconstructVerified(id, module, set.Document, types)
			if err != nil {
				return nil, ErrReferenceRewriteUnsafe
			}
			rewritten, changed, err := workloadconfig.RewriteEnvironmentKey(raw, oldName, newName)
			if err != nil || !changed {
				return nil, ErrReferenceRewriteUnsafe
			}
			if err := workloadconfig.VerifyRewritten(id, rewritten, module, set.Document, types, oldName, newName); err != nil {
				return nil, ErrReferenceRewriteUnsafe
			}
			out = append(out, environment.WorkloadDraft{ApplicationKey: app, EnvironmentKey: env, WorkloadID: id, State: environment.DraftUpsert, Score: rewritten})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkloadID < out[j].WorkloadID })
	return out, nil
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

// affected lists the workloads whose desired configuration references the key:
// an upsert draft supersedes the deployed module, a pending delete is excluded.
func (s *Service) affected(ctx context.Context, app, env, name string) ([]string, error) {
	drafts, err := s.store.ListWorkloadDrafts(ctx, app, env)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	hasDraft := map[string]bool{}
	for _, draft := range drafts {
		hasDraft[draft.WorkloadID] = true
		if draft.State == environment.DraftUpsert && workloadconfig.ReferencesEnvironmentKey(draft.Score, name) {
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
			if !hasDraft[workload] && workloadconfig.ModuleReferencesKey(module, set.Document, name) {
				seen[workload] = true
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
