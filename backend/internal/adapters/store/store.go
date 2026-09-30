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
	Organizations       map[string]application.Organization           `json:"organizations"`
	UserAccounts        map[string]identity.UserAccount               `json:"userAccounts"`
	Sessions            map[string]identity.Session                   `json:"sessions"`
	Applications        map[string]application.Application            `json:"applications"`
	Connections         map[string]application.Connection             `json:"connections"`
	Environments        map[string]environment.Environment            `json:"environments"`
	DeploymentSets      map[string]environment.DeploymentSet          `json:"deploymentSets"`
	ResourceTypes       map[string]resource.Type                      `json:"resourceTypes"`
	Definitions         map[string]resource.Definition                `json:"resourceDefinitions"`
	Deployments         map[string]deployment.Deployment              `json:"deployments"`
	DeltaSnapshots      map[string]deployment.DeploymentDeltaSnapshot `json:"deploymentDeltaSnapshots"`
	Plans               map[string]map[string]any                     `json:"deploymentPlans"`
	DeployResources     map[string][]deployment.Resource              `json:"deploymentResources"`
	ActiveResources     map[string]resource.ActiveResource            `json:"activeResources"`
	WorkloadInstances   map[string]deployment.WorkloadInstance        `json:"workloadInstances"`
	DeploymentWorkloads map[string]deployment.WorkloadSnapshot        `json:"deploymentWorkloads"`
	ConfigScopes        map[string]configuration.Scope                `json:"configurationScopes"`
	ConfigRevisions     map[string]configuration.Revision             `json:"configurationRevisions"`
	WorkloadDrafts      map[string]environment.WorkloadDraft          `json:"workloadDrafts"`
}

func newState() *state {
	return &state{
		Organizations:       map[string]application.Organization{},
		UserAccounts:        map[string]identity.UserAccount{},
		Sessions:            map[string]identity.Session{},
		Applications:        map[string]application.Application{},
		Connections:         map[string]application.Connection{},
		Environments:        map[string]environment.Environment{},
		DeploymentSets:      map[string]environment.DeploymentSet{},
		ResourceTypes:       map[string]resource.Type{},
		Definitions:         map[string]resource.Definition{},
		Deployments:         map[string]deployment.Deployment{},
		DeltaSnapshots:      map[string]deployment.DeploymentDeltaSnapshot{},
		Plans:               map[string]map[string]any{},
		DeployResources:     map[string][]deployment.Resource{},
		ActiveResources:     map[string]resource.ActiveResource{},
		WorkloadInstances:   map[string]deployment.WorkloadInstance{},
		DeploymentWorkloads: map[string]deployment.WorkloadSnapshot{},
		ConfigScopes:        map[string]configuration.Scope{},
		ConfigRevisions:     map[string]configuration.Revision{},
		WorkloadDrafts:      map[string]environment.WorkloadDraft{},
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
	if s.DeploymentWorkloads == nil {
		s.DeploymentWorkloads = map[string]deployment.WorkloadSnapshot{}
	}
}

func draftKey(app, env, workload string) string { return envKey(app, env) + "/" + workload }

func (s *Store) ListWorkloadDrafts(ctx context.Context, app, env string) ([]environment.WorkloadDraft, error) {
	defer s.rlock(ctx)()
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

func (s *Store) GetWorkloadDraft(ctx context.Context, app, env, workload string) (environment.WorkloadDraft, error) {
	defer s.rlock(ctx)()
	draft, ok := s.state.WorkloadDrafts[draftKey(app, env, workload)]
	if !ok {
		return environment.WorkloadDraft{}, fmt.Errorf("%w: workload draft %s/%s/%s", persistence.ErrNotFound, app, env, workload)
	}
	return cloneWorkloadDraft(draft), nil
}

func (s *Store) SaveWorkloadDraft(ctx context.Context, expected int64, draft environment.WorkloadDraft) error {
	defer s.lock(ctx)()
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

func (s *Store) DeleteWorkloadDraft(ctx context.Context, app, envKeyName, workload string, expected int64) error {
	defer s.lock(ctx)()
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

func (s *Store) GetConfigurationScope(ctx context.Context, app, env string) (configuration.Scope, error) {
	defer s.rlock(ctx)()
	if _, ok := s.state.Environments[envKey(app, env)]; !ok {
		return configuration.Scope{}, fmt.Errorf("%w: environment %s/%s", persistence.ErrNotFound, app, env)
	}
	if scope, ok := s.state.ConfigScopes[envKey(app, env)]; ok {
		return scope, nil
	}
	return configuration.Scope{ApplicationKey: app, EnvironmentKey: env}, nil
}

func (s *Store) GetConfigurationRevision(ctx context.Context, id string) (configuration.Revision, error) {
	defer s.rlock(ctx)()
	revision, ok := s.state.ConfigRevisions[id]
	if !ok {
		return configuration.Revision{}, fmt.Errorf("%w: configuration revision %q", persistence.ErrNotFound, id)
	}
	return copyConfigurationRevision(revision), nil
}

func (s *Store) CommitConfigurationRevision(ctx context.Context, expectedVersion int64, revision configuration.Revision) error {
	defer s.lock(ctx)()
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, fmt.Errorf("store: parse snapshot: %w", err)
	}
	loaded := newState()
	if err := json.Unmarshal(b, loaded); err != nil {
		return nil, fmt.Errorf("store: parse snapshot: %w", err)
	}
	s.state = loaded
	s.state.ensureMaps()
	if history, ok := fields["deploymentWorkloads"]; !ok || string(history) == "null" {
		s.state.backfillLatestWorkloadSnapshots()
	}
	return s, nil
}

// backfillLatestWorkloadSnapshots upgrades a JSON snapshot written before
// per-Deployment workload history existed. That history was never recorded
// and is not reconstructed: each current Workload Instance row becomes the
// snapshot of its last Deployment only (latest-only backfill), and every older
// Deployment keeps no workload rows. It runs only when the field is absent;
// a present, even empty, history is authoritative.
func (s *state) backfillLatestWorkloadSnapshots() {
	for _, instance := range s.WorkloadInstances {
		if instance.LastDeploymentID == "" {
			continue
		}
		snapshot := workloadSnapshot(instance)
		s.DeploymentWorkloads[deploymentWorkloadKey(snapshot.DeploymentID, snapshot.WorkloadID)] = snapshot
	}
}

func (s *Store) GetOrganization(ctx context.Context, key string) (application.Organization, error) {
	defer s.rlock(ctx)()
	org, ok := s.state.Organizations[key]
	if !ok {
		return application.Organization{}, fmt.Errorf("%w: organization %q", persistence.ErrNotFound, key)
	}
	return org, nil
}
func (s *Store) SaveOrganization(ctx context.Context, org application.Organization) error {
	defer s.lock(ctx)()
	if org.ID == "" {
		org.ID = ids.New()
	}
	s.state.Organizations[org.Key] = org
	return nil
}
func (s *Store) GetUserAccountByUsername(ctx context.Context, username string) (identity.UserAccount, error) {
	defer s.rlock(ctx)()
	for _, a := range s.state.UserAccounts {
		if a.Username == username {
			return a, nil
		}
	}
	return identity.UserAccount{}, fmt.Errorf("%w: user %q", persistence.ErrNotFound, username)
}
func (s *Store) GetUserAccount(ctx context.Context, id string) (identity.UserAccount, error) {
	defer s.rlock(ctx)()
	a, ok := s.state.UserAccounts[id]
	if !ok {
		return identity.UserAccount{}, fmt.Errorf("%w: user %q", persistence.ErrNotFound, id)
	}
	return a, nil
}
func (s *Store) SaveUserAccount(ctx context.Context, account identity.UserAccount) error {
	defer s.lock(ctx)()
	s.state.UserAccounts[account.ID] = account
	return nil
}
func (s *Store) SaveSession(ctx context.Context, session identity.Session) error {
	defer s.lock(ctx)()
	s.state.Sessions[session.ID] = session
	return nil
}
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (identity.Session, error) {
	defer s.rlock(ctx)()
	for _, session := range s.state.Sessions {
		if session.TokenHash == tokenHash {
			return session, nil
		}
	}
	return identity.Session{}, fmt.Errorf("%w: session", persistence.ErrNotFound)
}

// inTx reports whether ctx belongs to a Transact already holding s.mu. The
// marker travels with ctx, so other goroutines still wait for the lock.
func (s *Store) inTx(ctx context.Context) bool {
	owner, ok := ctx.Value(txKey{}).(*Store)
	return ok && owner == s
}

func (s *Store) lock(ctx context.Context) func() {
	if s.inTx(ctx) {
		return func() {}
	}
	s.mu.Lock()
	return func() {
		_ = s.persistLocked(context.Background())
		s.mu.Unlock()
	}
}

func (s *Store) rlock(ctx context.Context) func() {
	if s.inTx(ctx) {
		return func() {}
	}
	s.mu.Lock()
	return s.mu.Unlock
}

func (s *Store) persistLocked(ctx context.Context) error {
	if s.snapshotPath == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.snapshotPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.snapshotPath, b, 0o600)
}

// Transact runs fn atomically; state is restored when fn returns an error.
func (s *Store) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	backup, err := cloneState(s.state)
	if err != nil {
		return err
	}
	err = fn(context.WithValue(ctx, txKey{}, s))
	if err != nil {
		s.state = backup
		return err
	}
	if err := s.persistLocked(ctx); err != nil {
		s.state = backup
		return err
	}
	return nil
}

type txKey struct{}

// ReadSnapshot passes fn a private deep copy of committed state, taken under
// the store lock, so every read in fn sees one consistent point in time.
// Writes through the copy are discarded: it has no snapshot path and is never
// swapped back. Inside Transact, fn reads the transaction's own state.
func (s *Store) ReadSnapshot(ctx context.Context, fn func(context.Context, persistence.Store) error) error {
	if s.inTx(ctx) {
		return fn(ctx, s)
	}
	s.mu.Lock()
	view, err := cloneState(s.state)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return fn(ctx, &Store{state: view})
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
func (s *Store) ListApplications(ctx context.Context) ([]application.Application, error) {
	defer s.rlock(ctx)()
	out := make([]application.Application, 0, len(s.state.Applications))
	for _, a := range s.state.Applications {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// GetApplication reads one Application.
func (s *Store) GetApplication(ctx context.Context, key string) (application.Application, error) {
	defer s.rlock(ctx)()
	a, ok := s.state.Applications[key]
	if !ok {
		return application.Application{}, fmt.Errorf("%w: application %q", persistence.ErrNotFound, key)
	}
	return a, nil
}

// SaveApplication inserts or updates an Application.
func (s *Store) SaveApplication(ctx context.Context, app application.Application) error {
	defer s.lock(ctx)()
	if app.ID == "" {
		app.ID = ids.New()
	}
	if app.Version == 0 {
		app.Version = 1
	}
	s.state.Applications[app.Key] = app
	return nil
}

// GetConnection reads one Connection.
func (s *Store) GetConnection(ctx context.Context, organizationKey, key string) (application.Connection, error) {
	defer s.rlock(ctx)()
	c, ok := s.state.Connections[catalogKey(organizationKey, key)]
	if !ok {
		return application.Connection{}, fmt.Errorf("%w: connection %q", persistence.ErrNotFound, key)
	}
	return c, nil
}

// ListConnections returns every Connection ordered by key.
func (s *Store) ListConnections(ctx context.Context, organizationKey string) ([]application.Connection, error) {
	defer s.rlock(ctx)()
	out := make([]application.Connection, 0, len(s.state.Connections))
	for key, c := range s.state.Connections {
		if strings.HasPrefix(key, organizationKey+"/") {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// SaveConnection inserts or updates a Connection.
func (s *Store) SaveConnection(ctx context.Context, conn application.Connection) error {
	defer s.lock(ctx)()
	if conn.OrganizationKey == "" || conn.Key == "" {
		return fmt.Errorf("store: connection needs organization and key")
	}
	if conn.ID == "" {
		conn.ID = ids.New()
	}
	s.state.Connections[catalogKey(conn.OrganizationKey, conn.Key)] = conn
	return nil
}

func envKey(applicationKey, environmentKey string) string {
	return applicationKey + "/" + environmentKey
}

// ListEnvironments returns the Environments of one Application.
func (s *Store) ListEnvironments(ctx context.Context, applicationKey string) ([]environment.Environment, error) {
	defer s.rlock(ctx)()
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
func (s *Store) GetEnvironment(ctx context.Context, applicationKey, environmentKey string) (environment.Environment, error) {
	defer s.rlock(ctx)()
	e, ok := s.state.Environments[envKey(applicationKey, environmentKey)]
	if !ok {
		return environment.Environment{}, fmt.Errorf("%w: environment %s/%s", persistence.ErrNotFound, applicationKey, environmentKey)
	}
	return e, nil
}

// SaveEnvironment inserts or updates an Environment.
func (s *Store) SaveEnvironment(ctx context.Context, env environment.Environment) error {
	defer s.lock(ctx)()
	if env.ID == "" {
		env.ID = ids.New()
	}
	if env.ApplicationID == "" {
		if app, ok := s.state.Applications[env.ApplicationKey]; ok {
			env.ApplicationID = app.ID
		}
	}
	if env.Version == 0 {
		env.Version = 1
	}
	s.state.Environments[envKey(env.ApplicationKey, env.Key)] = env
	return nil
}

// GetDeploymentSet reads one immutable Deployment Set.
func (s *Store) GetDeploymentSet(ctx context.Context, id string) (environment.DeploymentSet, error) {
	defer s.rlock(ctx)()
	set, ok := s.state.DeploymentSets[id]
	if !ok {
		return environment.DeploymentSet{}, fmt.Errorf("%w: deployment set %q", persistence.ErrNotFound, id)
	}
	return set, nil
}

// SaveDeploymentSet stores an immutable Deployment Set.
func (s *Store) SaveDeploymentSet(ctx context.Context, set environment.DeploymentSet) error {
	defer s.lock(ctx)()
	if set.ID == "" {
		return fmt.Errorf("store: deployment set needs an id")
	}
	s.state.DeploymentSets[set.ID] = set
	return nil
}

// CompareVersionAndSetCurrent atomically moves the current-set pointer forward.
func (s *Store) CompareVersionAndSetCurrent(ctx context.Context, applicationKey, environmentKey string, expectedVersion int64, setID string) error {
	defer s.lock(ctx)()
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
func (s *Store) ListResourceTypes(ctx context.Context, organizationKey string) ([]resource.Type, error) {
	defer s.rlock(ctx)()
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
func (s *Store) ListResourceDefinitions(ctx context.Context, organizationKey string) ([]resource.Definition, error) {
	defer s.rlock(ctx)()
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
func (s *Store) SaveResourceType(ctx context.Context, organizationKey string, t resource.Type) error {
	defer s.lock(ctx)()
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
func (s *Store) SaveResourceDefinition(ctx context.Context, organizationKey string, d resource.Definition) error {
	defer s.lock(ctx)()
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
func (s *Store) SaveDeployment(ctx context.Context, d deployment.Deployment) error {
	defer s.lock(ctx)()
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
	snapshot, ok := s.state.DeltaSnapshots[d.DeltaSnapshotID]
	if !ok {
		return fmt.Errorf("%w: delta snapshot %q", persistence.ErrNotFound, d.DeltaSnapshotID)
	}
	if snapshot.DeploymentID != d.ID {
		return fmt.Errorf("%w: delta snapshot %q belongs to deployment %q", persistence.ErrImmutable, d.DeltaSnapshotID, snapshot.DeploymentID)
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
func (s *Store) SaveDeltaSnapshot(ctx context.Context, snapshot deployment.DeploymentDeltaSnapshot) error {
	defer s.lock(ctx)()
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
	if _, ok := s.state.Deployments[stored.DeploymentID]; !ok {
		return fmt.Errorf("%w: deployment %q", persistence.ErrNotFound, stored.DeploymentID)
	}
	for _, other := range s.state.DeltaSnapshots {
		if other.DeploymentID == stored.DeploymentID {
			return fmt.Errorf("%w: deployment %q already owns a delta snapshot", persistence.ErrImmutable, stored.DeploymentID)
		}
	}
	s.state.DeltaSnapshots[stored.ID] = stored
	return nil
}

// GetDeltaSnapshot reads one Deployment Delta Snapshot.
func (s *Store) GetDeltaSnapshot(ctx context.Context, id string) (deployment.DeploymentDeltaSnapshot, error) {
	defer s.rlock(ctx)()
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
func (s *Store) GetDeployment(ctx context.Context, id string) (deployment.Deployment, error) {
	defer s.rlock(ctx)()
	d, ok := s.state.Deployments[id]
	if !ok {
		return deployment.Deployment{}, fmt.Errorf("%w: deployment %q", persistence.ErrNotFound, id)
	}
	return d, nil
}

// ListDeployments returns deployments newest first, optionally filtered.
func (s *Store) ListDeployments(ctx context.Context, applicationKey, environmentKey string) ([]deployment.Deployment, error) {
	defer s.rlock(ctx)()
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
func (s *Store) SavePlan(ctx context.Context, deploymentID string, plan map[string]any) error {
	defer s.lock(ctx)()
	s.state.Plans[deploymentID] = plan
	return nil
}

// GetPlan reads the plan snapshot of a Deployment.
func (s *Store) GetPlan(ctx context.Context, deploymentID string) (map[string]any, error) {
	defer s.rlock(ctx)()
	p, ok := s.state.Plans[deploymentID]
	if !ok {
		return nil, fmt.Errorf("%w: plan for deployment %q", persistence.ErrNotFound, deploymentID)
	}
	return p, nil
}

// SaveDeploymentResource records per-node progress of a Deployment.
func (s *Store) SaveDeploymentResource(ctx context.Context, r deployment.Resource) error {
	defer s.lock(ctx)()
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
func (s *Store) ListDeploymentResources(ctx context.Context, deploymentID string) ([]deployment.Resource, error) {
	defer s.rlock(ctx)()
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
func (s *Store) FindByLogicalIdentity(ctx context.Context, organizationKey string, descriptor resource.Descriptor, scope resource.Scope) (resource.ActiveResource, error) {
	defer s.rlock(ctx)()
	a, ok := s.state.ActiveResources[resource.LogicalKey(organizationKey, descriptor, scope)]
	if !ok {
		return resource.ActiveResource{}, fmt.Errorf("%w: active resource %s", persistence.ErrNotFound, descriptor)
	}
	return a, nil
}

// ListActiveResources returns every Active Resource of an Organization.
func (s *Store) ListActiveResources(ctx context.Context, organizationKey string) ([]resource.ActiveResource, error) {
	defer s.rlock(ctx)()
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
func (s *Store) UpsertActiveResource(ctx context.Context, a resource.ActiveResource) (resource.ActiveResource, error) {
	defer s.lock(ctx)()
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
func (s *Store) UpsertWorkloadInstance(ctx context.Context, w deployment.WorkloadInstance) error {
	defer s.lock(ctx)()
	key := w.EnvironmentKey + "/" + w.WorkloadID
	existing, ok := s.state.WorkloadInstances[key]
	if ok {
		w.ID = existing.ID
	} else if w.ID == "" {
		w.ID = ids.New()
	}
	w.TargetRef = copyMap(w.TargetRef)
	s.state.WorkloadInstances[key] = w
	return nil
}

// UpsertWorkloadProgress atomically records current and per-Deployment state.
func (s *Store) UpsertWorkloadProgress(ctx context.Context, w deployment.WorkloadInstance) error {
	defer s.lock(ctx)()
	record, ok := s.state.Deployments[w.LastDeploymentID]
	if !ok || envKey(record.ApplicationKey, record.EnvironmentKey) != w.EnvironmentKey {
		return fmt.Errorf("%w: deployment %q in environment %q", persistence.ErrNotFound, w.LastDeploymentID, w.EnvironmentKey)
	}
	if record.Status == deployment.StatusSucceeded || record.Status == deployment.StatusFailed {
		return fmt.Errorf("%w: workload snapshot for terminal deployment %q", persistence.ErrImmutable, record.ID)
	}
	key := w.EnvironmentKey + "/" + w.WorkloadID
	existing, ok := s.state.WorkloadInstances[key]
	if ok {
		w.ID = existing.ID
	} else if w.ID == "" {
		w.ID = ids.New()
	}
	snapshot := workloadSnapshot(w)
	w.TargetRef = copyMap(w.TargetRef)
	s.state.WorkloadInstances[key] = w
	s.state.DeploymentWorkloads[deploymentWorkloadKey(snapshot.DeploymentID, snapshot.WorkloadID)] = snapshot
	return nil
}

// ListWorkloadInstances returns the workload instances of an Environment.
func (s *Store) ListWorkloadInstances(ctx context.Context, environmentKey string) ([]deployment.WorkloadInstance, error) {
	defer s.rlock(ctx)()
	var out []deployment.WorkloadInstance
	for _, w := range s.state.WorkloadInstances {
		if w.EnvironmentKey == environmentKey {
			w.TargetRef = copyMap(w.TargetRef)
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkloadID < out[j].WorkloadID })
	return out, nil
}

// ListDeploymentWorkloads returns immutable history for exactly one run.
func (s *Store) ListDeploymentWorkloads(ctx context.Context, deploymentID string) ([]deployment.WorkloadSnapshot, error) {
	defer s.rlock(ctx)()
	var out []deployment.WorkloadSnapshot
	for _, snapshot := range s.state.DeploymentWorkloads {
		if snapshot.DeploymentID == deploymentID {
			snapshot.TargetRef = copyMap(snapshot.TargetRef)
			out = append(out, snapshot)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkloadID < out[j].WorkloadID })
	return out, nil
}

func deploymentWorkloadKey(deploymentID, workloadID string) string {
	return deploymentID + "/" + workloadID
}

func workloadSnapshot(w deployment.WorkloadInstance) deployment.WorkloadSnapshot {
	return deployment.WorkloadSnapshot{
		DeploymentID: w.LastDeploymentID, WorkloadID: w.WorkloadID,
		AppliedConfigRevisionID: w.AppliedConfigRevisionID,
		TargetRef:               copyMap(w.TargetRef), ManifestDigest: w.ManifestDigest,
		Status: w.Status, ObservedAt: w.ObservedAt,
	}
}

// copyMap deep-copies JSON-shaped values so stored rows never alias caller or
// returned maps.
func copyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = copyValue(value)
	}
	return out
}

func copyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return copyMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = copyValue(item)
		}
		return out
	default:
		return v
	}
}

var _ persistence.Store = (*Store)(nil)
