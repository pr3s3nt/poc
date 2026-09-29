// Package store implements the persistence ports with an in-memory aggregate map
// and an optional JSON snapshot file. It keeps the logical identity, lifecycle and
// plan snapshots defined by architecture/database/schema.md. Secret values are never
// written here; only opaque references are.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/identity"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

type state struct {
	Organizations     map[string]application.Organization           `json:"organizations"`
	UserAccounts      map[string]identity.UserAccount               `json:"userAccounts"`
	Sessions          map[string]identity.Session                   `json:"sessions"`
	Applications      map[string]application.Application            `json:"applications"`
	Connections       map[string]application.Connection             `json:"connections"`
	Environments      map[string]environment.Environment            `json:"environments"`
	DeploymentSets    map[string]environment.DeploymentSet          `json:"deploymentSets"`
	ResourceTypes     map[string]resource.Type                      `json:"resourceTypes"`
	Definitions       map[string]resource.Definition                `json:"resourceDefinitions"`
	Deployments       map[string]deployment.Deployment              `json:"deployments"`
	DeltaSnapshots    map[string]deployment.DeploymentDeltaSnapshot `json:"deploymentDeltaSnapshots"`
	Plans             map[string]map[string]any                     `json:"deploymentPlans"`
	DeployResources   map[string][]deployment.Resource              `json:"deploymentResources"`
	ActiveResources   map[string]resource.ActiveResource            `json:"activeResources"`
	WorkloadInstances map[string]deployment.WorkloadInstance        `json:"workloadInstances"`
	ConfigScopes      map[string]configuration.Scope                `json:"configurationScopes"`
	ConfigRevisions   map[string]configuration.Revision             `json:"configurationRevisions"`
	WorkloadDrafts    map[string]environment.WorkloadDraft          `json:"workloadDrafts"`
}

func newState() *state {
	return &state{
		Organizations:     map[string]application.Organization{},
		UserAccounts:      map[string]identity.UserAccount{},
		Sessions:          map[string]identity.Session{},
		Applications:      map[string]application.Application{},
		Connections:       map[string]application.Connection{},
		Environments:      map[string]environment.Environment{},
		DeploymentSets:    map[string]environment.DeploymentSet{},
		ResourceTypes:     map[string]resource.Type{},
		Definitions:       map[string]resource.Definition{},
		Deployments:       map[string]deployment.Deployment{},
		DeltaSnapshots:    map[string]deployment.DeploymentDeltaSnapshot{},
		Plans:             map[string]map[string]any{},
		DeployResources:   map[string][]deployment.Resource{},
		ActiveResources:   map[string]resource.ActiveResource{},
		WorkloadInstances: map[string]deployment.WorkloadInstance{},
		ConfigScopes:      map[string]configuration.Scope{},
		ConfigRevisions:   map[string]configuration.Revision{},
		WorkloadDrafts:    map[string]environment.WorkloadDraft{},
	}
}

func (s *state) ensureMaps() {
	if s.Organizations == nil {
		s.Organizations = map[string]application.Organization{}
	}
	if s.UserAccounts == nil {
		s.UserAccounts = map[string]identity.UserAccount{}
	}
	if s.Sessions == nil {
		s.Sessions = map[string]identity.Session{}
	}
	if s.ConfigScopes == nil {
		s.ConfigScopes = map[string]configuration.Scope{}
	}
	if s.ConfigRevisions == nil {
		s.ConfigRevisions = map[string]configuration.Revision{}
	}
	if s.WorkloadDrafts == nil {
		s.WorkloadDrafts = map[string]environment.WorkloadDraft{}
	}
}

func draftKey(app, env, workload string) string { return envKey(app, env) + "/" + workload }

func (s *Store) ListWorkloadDrafts(_ context.Context, app, env string) ([]environment.WorkloadDraft, error) {
	defer s.rlock()()
	if _, ok := s.state.Environments[envKey(app, env)]; !ok {
		return nil, fmt.Errorf("%w: environment %s/%s", persistence.ErrNotFound, app, env)
	}
	var out []environment.WorkloadDraft
	for _, draft := range s.state.WorkloadDrafts {
		if draft.ApplicationKey == app && draft.EnvironmentKey == env {
			out = append(out, cloneWorkloadDraft(draft))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkloadID < out[j].WorkloadID })
	return out, nil
}

func (s *Store) GetWorkloadDraft(_ context.Context, app, env, workload string) (environment.WorkloadDraft, error) {
	defer s.rlock()()
	draft, ok := s.state.WorkloadDrafts[draftKey(app, env, workload)]
	if !ok {
		return environment.WorkloadDraft{}, fmt.Errorf("%w: workload draft %s/%s/%s", persistence.ErrNotFound, app, env, workload)
	}
	return cloneWorkloadDraft(draft), nil
}

func (s *Store) SaveWorkloadDraft(_ context.Context, expected int64, draft environment.WorkloadDraft) error {
	defer s.lock()()
	key := envKey(draft.ApplicationKey, draft.EnvironmentKey)
	env, ok := s.state.Environments[key]
	if !ok {
		return fmt.Errorf("%w: environment %s", persistence.ErrNotFound, key)
	}
	if env.DraftVersion != expected {
		return persistence.ErrVersionConflict
	}
	if draft.WorkloadID == "" || (draft.State != environment.DraftUpsert && draft.State != environment.DraftDelete) || (draft.State == environment.DraftUpsert && draft.Score == nil) {
		return fmt.Errorf("store: invalid workload draft")
	}
	s.state.WorkloadDrafts[draftKey(draft.ApplicationKey, draft.EnvironmentKey, draft.WorkloadID)] = cloneWorkloadDraft(draft)
	env.DraftVersion++
	s.state.Environments[key] = env
	return nil
}

func (s *Store) DeleteWorkloadDraft(_ context.Context, app, envKeyName, workload string, expected int64) error {
	defer s.lock()()
	key := envKey(app, envKeyName)
	env, ok := s.state.Environments[key]
	if !ok {
		return fmt.Errorf("%w: environment %s", persistence.ErrNotFound, key)
	}
	if env.DraftVersion != expected {
		return persistence.ErrVersionConflict
	}
	if _, ok := s.state.WorkloadDrafts[draftKey(app, envKeyName, workload)]; !ok {
		return fmt.Errorf("%w: workload draft %s", persistence.ErrNotFound, workload)
	}
	delete(s.state.WorkloadDrafts, draftKey(app, envKeyName, workload))
	env.DraftVersion++
	s.state.Environments[key] = env
	return nil
}

func cloneWorkloadDraft(in environment.WorkloadDraft) environment.WorkloadDraft {
	out := in
	if in.Score != nil {
		b, _ := json.Marshal(in.Score)
		_ = json.Unmarshal(b, &out.Score)
	}
	return out
}

func (s *Store) GetConfigurationScope(_ context.Context, app, env string) (configuration.Scope, error) {
	defer s.rlock()()
	if _, ok := s.state.Environments[envKey(app, env)]; !ok {
		return configuration.Scope{}, fmt.Errorf("%w: environment %s/%s", persistence.ErrNotFound, app, env)
	}
	if scope, ok := s.state.ConfigScopes[envKey(app, env)]; ok {
		return scope, nil
	}
	return configuration.Scope{ApplicationKey: app, EnvironmentKey: env}, nil
}

func (s *Store) GetConfigurationRevision(_ context.Context, id string) (configuration.Revision, error) {
	defer s.rlock()()
	revision, ok := s.state.ConfigRevisions[id]
	if !ok {
		return configuration.Revision{}, fmt.Errorf("%w: configuration revision %q", persistence.ErrNotFound, id)
	}
	return copyConfigurationRevision(revision), nil
}

func (s *Store) CommitConfigurationRevision(_ context.Context, expectedVersion int64, revision configuration.Revision) error {
	defer s.lock()()
	if err := revision.Validate(); err != nil {
		return err
	}
	key := envKey(revision.ApplicationKey, revision.EnvironmentKey)
	if _, ok := s.state.Environments[key]; !ok {
		return fmt.Errorf("%w: environment %s", persistence.ErrNotFound, key)
	}
	scope := s.state.ConfigScopes[key]
	if scope.Version != expectedVersion || revision.Version != expectedVersion+1 {
		return persistence.ErrVersionConflict
	}
	if _, ok := s.state.ConfigRevisions[revision.ID]; ok {
		return persistence.ErrImmutable
	}
	s.state.ConfigRevisions[revision.ID] = copyConfigurationRevision(revision)
	s.state.ConfigScopes[key] = configuration.Scope{ApplicationKey: revision.ApplicationKey, EnvironmentKey: revision.EnvironmentKey, DesiredRevisionID: revision.ID, Version: revision.Version}
	return nil
}

func copyConfigurationRevision(in configuration.Revision) configuration.Revision {
	out := in
	out.Entries = make(map[string]configuration.Entry, len(in.Entries))
	for key, entry := range in.Entries {
		out.Entries[key] = entry
	}
	return out
}

// Store is the Phase 6 state store.
type Store struct {
	mu           sync.Mutex
	state        *state
	snapshotPath string
	inTx         bool
}

// New returns an empty in-memory store.
func New() *Store { return &Store{state: newState()} }

// NewWithSnapshot returns a store backed by a JSON snapshot file.
func NewWithSnapshot(path string) (*Store, error) {
	s := &Store{state: newState(), snapshotPath: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("store: read snapshot: %w", err)
	}
	loaded := newState()
	if err := json.Unmarshal(b, loaded); err != nil {
		return nil, fmt.Errorf("store: parse snapshot: %w", err)
	}
	s.state = loaded
	s.state.ensureMaps()
	return s, nil
}

func (s *Store) GetOrganization(_ context.Context, key string) (application.Organization, error) {
	defer s.rlock()()
	org, ok := s.state.Organizations[key]
	if !ok {
		return application.Organization{}, fmt.Errorf("%w: organization %q", persistence.ErrNotFound, key)
	}
	return org, nil
}
func (s *Store) SaveOrganization(_ context.Context, org application.Organization) error {
	defer s.lock()()
	s.state.Organizations[org.Key] = org
	return nil
}
func (s *Store) GetUserAccountByUsername(_ context.Context, username string) (identity.UserAccount, error) {
	defer s.rlock()()
	for _, a := range s.state.UserAccounts {
		if a.Username == username {
			return a, nil
		}
	}
	return identity.UserAccount{}, fmt.Errorf("%w: user %q", persistence.ErrNotFound, username)
}
func (s *Store) GetUserAccount(_ context.Context, id string) (identity.UserAccount, error) {
	defer s.rlock()()
	a, ok := s.state.UserAccounts[id]
	if !ok {
		return identity.UserAccount{}, fmt.Errorf("%w: user %q", persistence.ErrNotFound, id)
	}
	return a, nil
}
func (s *Store) SaveUserAccount(_ context.Context, account identity.UserAccount) error {
	defer s.lock()()
	s.state.UserAccounts[account.ID] = account
	return nil
}
func (s *Store) SaveSession(_ context.Context, session identity.Session) error {
	defer s.lock()()
	s.state.Sessions[session.ID] = session
	return nil
}
func (s *Store) GetSessionByTokenHash(_ context.Context, tokenHash string) (identity.Session, error) {
	defer s.rlock()()
	for _, session := range s.state.Sessions {
		if session.TokenHash == tokenHash {
			return session, nil
		}
	}
	return identity.Session{}, fmt.Errorf("%w: session", persistence.ErrNotFound)
}

func (s *Store) lock() func() {
	if s.inTx {
		return func() {}
	}
	s.mu.Lock()
	return func() {
		s.persistLocked()
		s.mu.Unlock()
	}
}

func (s *Store) rlock() func() {
	if s.inTx {
		return func() {}
	}
	s.mu.Lock()
	return s.mu.Unlock
}

func (s *Store) persistLocked() {
	if s.snapshotPath == "" || s.inTx {
		return
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.snapshotPath), 0o755)
	_ = os.WriteFile(s.snapshotPath, b, 0o600)
}

// Transact runs fn atomically; state is restored when fn returns an error.
func (s *Store) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	backup, err := cloneState(s.state)
	if err != nil {
		return err
	}
	s.inTx = true
	err = fn(ctx)
	s.inTx = false
	if err != nil {
		s.state = backup
		return err
	}
	s.persistLocked()
	return nil
}

func cloneState(in *state) (*state, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("store: snapshot: %w", err)
	}
	out := newState()
	if err := json.Unmarshal(b, out); err != nil {
		return nil, fmt.Errorf("store: snapshot: %w", err)
	}
	return out, nil
}

// ListApplications returns every Application ordered by key.
func (s *Store) ListApplications(context.Context) ([]application.Application, error) {
	defer s.rlock()()
	out := make([]application.Application, 0, len(s.state.Applications))
	for _, a := range s.state.Applications {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// GetApplication reads one Application.
func (s *Store) GetApplication(_ context.Context, key string) (application.Application, error) {
	defer s.rlock()()
	a, ok := s.state.Applications[key]
	if !ok {
		return application.Application{}, fmt.Errorf("%w: application %q", persistence.ErrNotFound, key)
	}
	return a, nil
}

// SaveApplication inserts or updates an Application.
func (s *Store) SaveApplication(_ context.Context, app application.Application) error {
	defer s.lock()()
	if app.Version == 0 {
		app.Version = 1
	}
	s.state.Applications[app.Key] = app
	return nil
}

// GetConnection reads one Connection.
func (s *Store) GetConnection(_ context.Context, key string) (application.Connection, error) {
	defer s.rlock()()
	c, ok := s.state.Connections[key]
	if !ok {
		return application.Connection{}, fmt.Errorf("%w: connection %q", persistence.ErrNotFound, key)
	}
	return c, nil
}

// ListConnections returns every Connection ordered by key.
func (s *Store) ListConnections(context.Context) ([]application.Connection, error) {
	defer s.rlock()()
	out := make([]application.Connection, 0, len(s.state.Connections))
	for _, c := range s.state.Connections {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// SaveConnection inserts or updates a Connection.
func (s *Store) SaveConnection(_ context.Context, conn application.Connection) error {
	defer s.lock()()
	s.state.Connections[conn.Key] = conn
	return nil
}

func envKey(applicationKey, environmentKey string) string {
	return applicationKey + "/" + environmentKey
}

// ListEnvironments returns the Environments of one Application.
func (s *Store) ListEnvironments(_ context.Context, applicationKey string) ([]environment.Environment, error) {
	defer s.rlock()()
	var out []environment.Environment
	for _, e := range s.state.Environments {
		if e.ApplicationKey == applicationKey {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// GetEnvironment reads one Environment.
func (s *Store) GetEnvironment(_ context.Context, applicationKey, environmentKey string) (environment.Environment, error) {
	defer s.rlock()()
	e, ok := s.state.Environments[envKey(applicationKey, environmentKey)]
	if !ok {
		return environment.Environment{}, fmt.Errorf("%w: environment %s/%s", persistence.ErrNotFound, applicationKey, environmentKey)
	}
	return e, nil
}

// SaveEnvironment inserts or updates an Environment.
func (s *Store) SaveEnvironment(_ context.Context, env environment.Environment) error {
	defer s.lock()()
	if env.Version == 0 {
		env.Version = 1
	}
	s.state.Environments[envKey(env.ApplicationKey, env.Key)] = env
	return nil
}

// GetDeploymentSet reads one immutable Deployment Set.
func (s *Store) GetDeploymentSet(_ context.Context, id string) (environment.DeploymentSet, error) {
	defer s.rlock()()
	set, ok := s.state.DeploymentSets[id]
	if !ok {
		return environment.DeploymentSet{}, fmt.Errorf("%w: deployment set %q", persistence.ErrNotFound, id)
	}
	return set, nil
}

// SaveDeploymentSet stores an immutable Deployment Set.
func (s *Store) SaveDeploymentSet(_ context.Context, set environment.DeploymentSet) error {
	defer s.lock()()
	if set.ID == "" {
		return fmt.Errorf("store: deployment set needs an id")
	}
	s.state.DeploymentSets[set.ID] = set
	return nil
}

// CompareVersionAndSetCurrent atomically moves the current-set pointer forward.
func (s *Store) CompareVersionAndSetCurrent(_ context.Context, applicationKey, environmentKey string, expectedVersion int64, setID string) error {
	defer s.lock()()
	key := envKey(applicationKey, environmentKey)
	env, ok := s.state.Environments[key]
	if !ok {
		return fmt.Errorf("%w: environment %s", persistence.ErrNotFound, key)
	}
	if env.Version != expectedVersion {
		return fmt.Errorf("%w: environment %s is at version %d, plan used %d", persistence.ErrVersionConflict, key, env.Version, expectedVersion)
	}
	if _, ok := s.state.DeploymentSets[setID]; !ok {
		return fmt.Errorf("%w: deployment set %q", persistence.ErrNotFound, setID)
	}
	env.CurrentDeploymentSetID = setID
	env.Version = expectedVersion + 1
	s.state.Environments[key] = env
	return nil
}

func catalogKey(organizationKey, key string) string { return organizationKey + "/" + key }

// ListResourceTypes returns this Organization's registered Resource Types.
func (s *Store) ListResourceTypes(_ context.Context, organizationKey string) ([]resource.Type, error) {
	defer s.rlock()()
	out := make([]resource.Type, 0)
	for key, t := range s.state.ResourceTypes {
		if strings.HasPrefix(key, organizationKey+"/") {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ListResourceDefinitions returns this Organization's registered Definitions.
func (s *Store) ListResourceDefinitions(_ context.Context, organizationKey string) ([]resource.Definition, error) {
	defer s.rlock()()
	out := make([]resource.Definition, 0)
	for key, d := range s.state.Definitions {
		if strings.HasPrefix(key, organizationKey+"/") {
			out = append(out, d)
		}
	}
	resource.SortDefinitions(out)
	return out, nil
}

// SaveResourceType registers a Resource Type for one Organization.
func (s *Store) SaveResourceType(_ context.Context, organizationKey string, t resource.Type) error {
	defer s.lock()()
	if organizationKey == "" {
		return fmt.Errorf("store: resource type needs an organization")
	}
	if err := t.Validate(); err != nil {
		return err
	}
	s.state.ResourceTypes[catalogKey(organizationKey, t.Key)] = t
	return nil
}

// SaveResourceDefinition registers a Resource Definition for one Organization.
func (s *Store) SaveResourceDefinition(_ context.Context, organizationKey string, d resource.Definition) error {
	defer s.lock()()
	if organizationKey == "" {
		return fmt.Errorf("store: definition needs an organization")
	}
	if err := d.Validate(); err != nil {
		return err
	}
	s.state.Definitions[catalogKey(organizationKey, d.Key)] = d
	return nil
}

// SaveDeployment inserts or updates a Deployment record.
func (s *Store) SaveDeployment(_ context.Context, d deployment.Deployment) error {
	defer s.lock()()
	if d.ID == "" {
		return fmt.Errorf("store: deployment needs an id")
	}
	if err := s.checkDeltaSnapshotLocked(d); err != nil {
		return err
	}
	s.state.Deployments[d.ID] = d
	return nil
}

// checkDeltaSnapshotLocked enforces the one-to-one Deployment/Delta Snapshot
// association: the Snapshot exists, belongs to no other Deployment, never
// changes once set and is present before the Deployment leaves PLANNING.
func (s *Store) checkDeltaSnapshotLocked(d deployment.Deployment) error {
	if existing, ok := s.state.Deployments[d.ID]; ok && existing.DeltaSnapshotID != "" && existing.DeltaSnapshotID != d.DeltaSnapshotID {
		return fmt.Errorf("%w: deployment %q already references delta snapshot %q", persistence.ErrImmutable, d.ID, existing.DeltaSnapshotID)
	}
	if d.DeltaSnapshotID == "" {
		switch d.Status {
		case deployment.StatusProvisioning, deployment.StatusDeploying, deployment.StatusSucceeded:
			return fmt.Errorf("store: deployment %q cannot be %s without a delta snapshot", d.ID, d.Status)
		}
		return nil
	}
	if _, ok := s.state.DeltaSnapshots[d.DeltaSnapshotID]; !ok {
		return fmt.Errorf("%w: delta snapshot %q", persistence.ErrNotFound, d.DeltaSnapshotID)
	}
	for id, other := range s.state.Deployments {
		if id != d.ID && other.DeltaSnapshotID == d.DeltaSnapshotID {
			return fmt.Errorf("%w: delta snapshot %q already belongs to deployment %q", persistence.ErrImmutable, d.DeltaSnapshotID, id)
		}
	}
	return nil
}

// SaveDeltaSnapshot stores an immutable Deployment Delta Snapshot once. The
// store keeps its own deep copy, so later changes to the caller's value cannot
// reach the stored Snapshot.
func (s *Store) SaveDeltaSnapshot(_ context.Context, snapshot deployment.DeploymentDeltaSnapshot) error {
	defer s.lock()()
	stored, err := cloneDeltaSnapshot(snapshot)
	if err != nil {
		return err
	}
	if err := stored.Validate(); err != nil {
		return err
	}
	if _, ok := s.state.DeltaSnapshots[stored.ID]; ok {
		return fmt.Errorf("%w: delta snapshot %q already exists", persistence.ErrImmutable, stored.ID)
	}
	s.state.DeltaSnapshots[stored.ID] = stored
	return nil
}

// GetDeltaSnapshot reads one Deployment Delta Snapshot.
func (s *Store) GetDeltaSnapshot(_ context.Context, id string) (deployment.DeploymentDeltaSnapshot, error) {
	defer s.rlock()()
	snapshot, ok := s.state.DeltaSnapshots[id]
	if !ok {
		return deployment.DeploymentDeltaSnapshot{}, fmt.Errorf("%w: delta snapshot %q", persistence.ErrNotFound, id)
	}
	return cloneDeltaSnapshot(snapshot)
}

// cloneDeltaSnapshot deep-copies a Snapshot through its JSON form, the same
// form the snapshot file persists. Every nested map, slice and any value of the
// Delta document is rebuilt, so the copy shares no memory with the original,
// and the canonical encoding, hence the document hash, is unchanged.
func cloneDeltaSnapshot(in deployment.DeploymentDeltaSnapshot) (deployment.DeploymentDeltaSnapshot, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return deployment.DeploymentDeltaSnapshot{}, fmt.Errorf("store: copy delta snapshot %q: %w", in.ID, err)
	}
	var out deployment.DeploymentDeltaSnapshot
	if err := json.Unmarshal(b, &out); err != nil {
		return deployment.DeploymentDeltaSnapshot{}, fmt.Errorf("store: copy delta snapshot %q: %w", in.ID, err)
	}
	return out, nil
}

// GetDeployment reads one Deployment record.
func (s *Store) GetDeployment(_ context.Context, id string) (deployment.Deployment, error) {
	defer s.rlock()()
	d, ok := s.state.Deployments[id]
	if !ok {
		return deployment.Deployment{}, fmt.Errorf("%w: deployment %q", persistence.ErrNotFound, id)
	}
	return d, nil
}

// ListDeployments returns deployments newest first, optionally filtered.
func (s *Store) ListDeployments(_ context.Context, applicationKey, environmentKey string) ([]deployment.Deployment, error) {
	defer s.rlock()()
	var out []deployment.Deployment
	for _, d := range s.state.Deployments {
		if applicationKey != "" && d.ApplicationKey != applicationKey {
			continue
		}
		if environmentKey != "" && d.EnvironmentKey != environmentKey {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

// SavePlan stores the immutable plan snapshot of a Deployment.
func (s *Store) SavePlan(_ context.Context, deploymentID string, plan map[string]any) error {
	defer s.lock()()
	s.state.Plans[deploymentID] = plan
	return nil
}

// GetPlan reads the plan snapshot of a Deployment.
func (s *Store) GetPlan(_ context.Context, deploymentID string) (map[string]any, error) {
	defer s.rlock()()
	p, ok := s.state.Plans[deploymentID]
	if !ok {
		return nil, fmt.Errorf("%w: plan for deployment %q", persistence.ErrNotFound, deploymentID)
	}
	return p, nil
}

// SaveDeploymentResource records per-node progress of a Deployment.
func (s *Store) SaveDeploymentResource(_ context.Context, r deployment.Resource) error {
	defer s.lock()()
	list := s.state.DeployResources[r.DeploymentID]
	for i, existing := range list {
		if existing.NodeDescriptor == r.NodeDescriptor {
			list[i] = r
			s.state.DeployResources[r.DeploymentID] = list
			return nil
		}
	}
	s.state.DeployResources[r.DeploymentID] = append(list, r)
	return nil
}

// ListDeploymentResources returns node progress ordered by batch then descriptor.
func (s *Store) ListDeploymentResources(_ context.Context, deploymentID string) ([]deployment.Resource, error) {
	defer s.rlock()()
	list := append([]deployment.Resource(nil), s.state.DeployResources[deploymentID]...)
	sort.Slice(list, func(i, j int) bool {
		if list[i].BatchIndex != list[j].BatchIndex {
			return list[i].BatchIndex < list[j].BatchIndex
		}
		return list[i].NodeDescriptor < list[j].NodeDescriptor
	})
	return list, nil
}

// FindByLogicalIdentity resolves an Active Resource by organization, descriptor and scope.
func (s *Store) FindByLogicalIdentity(_ context.Context, organizationKey string, descriptor resource.Descriptor, scope resource.Scope) (resource.ActiveResource, error) {
	defer s.rlock()()
	a, ok := s.state.ActiveResources[resource.LogicalKey(organizationKey, descriptor, scope)]
	if !ok {
		return resource.ActiveResource{}, fmt.Errorf("%w: active resource %s", persistence.ErrNotFound, descriptor)
	}
	return a, nil
}

// ListActiveResources returns every Active Resource of an Organization.
func (s *Store) ListActiveResources(_ context.Context, organizationKey string) ([]resource.ActiveResource, error) {
	defer s.rlock()()
	var out []resource.ActiveResource
	for _, a := range s.state.ActiveResources {
		if a.OrganizationKey == organizationKey {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LogicalKey() < out[j].LogicalKey() })
	return out, nil
}

// UpsertActiveResource inserts or updates an Active Resource by logical identity.
func (s *Store) UpsertActiveResource(_ context.Context, a resource.ActiveResource) (resource.ActiveResource, error) {
	defer s.lock()()
	key := a.LogicalKey()
	existing, ok := s.state.ActiveResources[key]
	now := time.Now().UTC()
	if ok {
		a.ID = existing.ID
		a.CreatedAt = existing.CreatedAt
		a.Version = existing.Version + 1
	} else {
		if a.ID == "" {
			a.ID = ids.New()
		}
		a.CreatedAt = now
		a.Version = 1
	}
	a.UpdatedAt = now
	s.state.ActiveResources[key] = a
	return a, nil
}

// UpsertWorkloadInstance records applied workload state.
func (s *Store) UpsertWorkloadInstance(_ context.Context, w deployment.WorkloadInstance) error {
	defer s.lock()()
	key := w.EnvironmentKey + "/" + w.WorkloadID
	existing, ok := s.state.WorkloadInstances[key]
	if ok {
		w.ID = existing.ID
	} else if w.ID == "" {
		w.ID = ids.New()
	}
	s.state.WorkloadInstances[key] = w
	return nil
}

// ListWorkloadInstances returns the workload instances of an Environment.
func (s *Store) ListWorkloadInstances(_ context.Context, environmentKey string) ([]deployment.WorkloadInstance, error) {
	defer s.rlock()()
	var out []deployment.WorkloadInstance
	for _, w := range s.state.WorkloadInstances {
		if w.EnvironmentKey == environmentKey {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkloadID < out[j].WorkloadID })
	return out, nil
}

var _ persistence.Store = (*Store)(nil)
