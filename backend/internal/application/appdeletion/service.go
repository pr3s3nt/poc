package appdeletion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/application/workloadconfig"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

// Workloads is the UC-16 pending workload editor.
type Workloads interface {
	List(ctx context.Context, app, env string) (workloadconfig.View, error)
	Delete(ctx context.Context, app, env, workload string, expectedVersion int64) (workloadconfig.View, error)
}

// Pending is the UC-16 pending Preview/Deploy path.
type Pending interface {
	Preview(ctx context.Context, appKey, envKey string) (pending.Preview, error)
	Deploy(ctx context.Context, appKey, envKey, actor, token string) (pending.DeployReport, error)
}

// Cleanup is the confirmed resource cleanup of UC-07 DEL.
type Cleanup interface {
	EnvironmentInventory(ctx context.Context, sc resourcecleanup.Scope) (resourcecleanup.Inventory, error)
	AdmitEnvironmentCleanup(ctx context.Context, sc resourcecleanup.Scope, actor resourcecleanup.Actor, in resourcecleanup.EnvironmentCleanupInput) (resourcecleanup.AdmissionView, error)
	WaitCleanup(ctx context.Context, sc resourcecleanup.Scope, operationID string, every time.Duration) (resourcecleanup.CleanupView, error)
}

// Service implements UC-18.
type Service struct {
	store     persistence.Store
	life      persistence.ApplicationLifecycleRepository
	workloads Workloads
	pending   Pending
	cleanup   Cleanup
	secrets   configport.Registry
	now       func() time.Time
	poll      time.Duration

	mu      sync.Mutex
	running map[string]bool
	wg      sync.WaitGroup
}

// New builds the service. Without a lifecycle repository, workload, pending
// or cleanup service it answers ErrUnavailable.
func New(store persistence.Store, workloads Workloads, pendingSvc Pending, cleanup Cleanup, secrets configport.Registry) *Service {
	s := &Service{store: store, workloads: workloads, pending: pendingSvc, cleanup: cleanup, secrets: secrets,
		now: func() time.Time { return time.Now().UTC() }, poll: 2 * time.Second, running: map[string]bool{}}
	if l, ok := store.(persistence.ApplicationLifecycleRepository); ok {
		s.life = l
	}
	return s
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetPoll replaces the cleanup polling interval (tests).
func (s *Service) SetPoll(every time.Duration) { s.poll = every }

// Wait blocks until every run started by this process has ended.
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) available() bool {
	return s.life != nil && s.workloads != nil && s.pending != nil && s.cleanup != nil
}

func (s *Service) isRunning(app string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[app]
}

// scoped reads an Application of the Organization. A foreign, unknown or
// DELETED Application is ErrNotFound.
func (s *Service) scoped(ctx context.Context, org, appKey string) (appdomain.Application, error) {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return app, err
	}
	if org == "" || app.OrganizationKey != org || app.Lifecycle() == appdomain.StatusDeleted {
		return appdomain.Application{}, persistence.ErrNotFound
	}
	return app, nil
}

// tokenBody is everything a preview pins.
type tokenBody struct {
	Version     int               `json:"v"`
	Actor       string            `json:"actor"`
	Org         string            `json:"org"`
	Application string            `json:"application"`
	AppVersion  int64             `json:"appVersion"`
	Status      string            `json:"status"`
	Expires     int64             `json:"expires"`
	Nonce       string            `json:"nonce"`
	Envs        []tokenEnvironent `json:"envs"`
}

type tokenEnvironent struct {
	Key          string   `json:"key"`
	ID           string   `json:"id"`
	Version      int64    `json:"version"`
	DraftVersion int64    `json:"draftVersion"`
	SetID        string   `json:"setId"`
	Operation    string   `json:"operation"`
	Workloads    []string `json:"workloads"`
	Resources    []string `json:"resources"`
}

type snapshot struct {
	app       appdomain.Application
	envs      []environment.Environment
	preview   PreviewView
	pins      []tokenEnvironent
	refs      map[string][]storeRefs
	deployIDs map[string][]string
}

type storeRefs struct {
	store string
	refs  []string
}

// load builds the preview body and its pins. It reads persisted metadata only;
// any change between two loads changes the pins, so the admission compares the
// token of a fresh load and the lifecycle write is a version compare-and-set.
func (s *Service) load(ctx context.Context, org, appKey string) (*snapshot, error) {
	app, err := s.scoped(ctx, org, appKey)
	if err != nil {
		return nil, err
	}
	envs, err := s.store.ListEnvironments(ctx, appKey)
	if err != nil {
		return nil, err
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Key < envs[j].Key })
	snap := &snapshot{app: app, envs: envs, refs: map[string][]storeRefs{}, deployIDs: map[string][]string{}}
	view := PreviewView{Application: app.Key, Status: string(app.Lifecycle()), ConfirmationName: app.Name,
		Environments: []EnvironmentPreview{}, Kept: Kept{SecretStores: []string{}, Definitions: []string{}}, Warnings: []string{}}
	stores := map[string]bool{}
	running := s.isRunning(app.Key)
	for _, env := range envs {
		ep := EnvironmentPreview{Key: env.Key, Workloads: []string{}, PendingDrafts: []string{}, Resources: []resourcecleanup.InventoryItem{}, Blockers: []Blocker{}}
		if env.CurrentDeploymentSetID != "" {
			applied, err := s.store.GetDeploymentSet(ctx, env.CurrentDeploymentSetID)
			if err != nil {
				return nil, err
			}
			ep.Blockers = append(ep.Blockers, scheduledDeletionBlockers(applied.Document)...)
		}
		pin := tokenEnvironent{Key: env.Key, ID: env.ID, Version: env.Version, DraftVersion: env.DraftVersion, SetID: env.CurrentDeploymentSetID, Operation: env.ActiveOperationID, Workloads: []string{}, Resources: []string{}}
		list, err := s.workloads.List(ctx, appKey, env.Key)
		if err != nil {
			return nil, err
		}
		for _, w := range list.Workloads {
			if w.State == "" || w.State == environment.DraftDelete {
				ep.Workloads = append(ep.Workloads, w.ID)
			} else {
				ep.PendingDrafts = append(ep.PendingDrafts, w.ID)
			}
			pin.Workloads = append(pin.Workloads, w.ID+"|"+string(w.State))
		}
		sort.Strings(ep.Workloads)
		sort.Strings(ep.PendingDrafts)
		sort.Strings(pin.Workloads)
		inv, err := s.cleanup.EnvironmentInventory(ctx, resourcecleanup.Scope{OrganizationKey: org, ApplicationKey: appKey, EnvironmentKey: env.Key})
		if err != nil {
			return nil, err
		}
		view.Kept.PlatformResources += inv.Platform
		ep.Resources = inv.Items
		for _, item := range inv.Items {
			pin.Resources = append(pin.Resources, item.ID+"|"+item.Status)
			if item.Blocker != "" {
				ep.Blockers = append(ep.Blockers, Blocker{Code: item.Blocker, ResourceID: item.ID, Descriptor: item.Descriptor})
			}
		}
		if env.ActiveOperationID != "" {
			ep.Blockers = append([]Blocker{{Code: BlockerEnvironmentBusy, OperationID: env.ActiveOperationID}}, ep.Blockers...)
		}
		revisions, err := s.life.ListConfigurationRevisions(ctx, appKey, env.Key)
		if err != nil {
			return nil, err
		}
		perStore := map[string]map[string]bool{}
		for _, r := range revisions {
			for _, entry := range r.Entries {
				if entry.Kind != configuration.Secret || entry.ValueRef == "" || entry.StoreKey == "" {
					continue
				}
				if perStore[entry.StoreKey] == nil {
					perStore[entry.StoreKey] = map[string]bool{}
				}
				perStore[entry.StoreKey][entry.ValueRef] = true
			}
		}
		if env.SecretStoreKey != "" && perStore[env.SecretStoreKey] == nil {
			perStore[env.SecretStoreKey] = map[string]bool{}
		}
		for key, refs := range perStore {
			stores[key] = true
			sr := storeRefs{store: key}
			for ref := range refs {
				sr.refs = append(sr.refs, ref)
			}
			sort.Strings(sr.refs)
			ep.SecretValues += len(sr.refs)
			snap.refs[env.Key] = append(snap.refs[env.Key], sr)
		}
		sort.Slice(snap.refs[env.Key], func(i, j int) bool { return snap.refs[env.Key][i].store < snap.refs[env.Key][j].store })
		deployments, err := s.store.ListDeployments(ctx, appKey, env.Key)
		if err != nil {
			return nil, err
		}
		for _, d := range deployments {
			snap.deployIDs[env.Key] = append(snap.deployIDs[env.Key], d.ID)
		}
		sort.Strings(snap.deployIDs[env.Key])
		ep.Deployments = len(deployments)
		view.Environments = append(view.Environments, ep)
		snap.pins = append(snap.pins, pin)
	}
	for key := range stores {
		view.Kept.SecretStores = append(view.Kept.SecretStores, key)
	}
	sort.Strings(view.Kept.SecretStores)
	defs, err := s.store.ListResourceDefinitions(ctx, org)
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		for _, c := range d.Criteria {
			if c.ApplicationID == app.Key {
				view.Kept.Definitions = append(view.Kept.Definitions, d.Key)
				break
			}
		}
	}
	sort.Strings(view.Kept.Definitions)
	if len(view.Kept.Definitions) > 0 {
		view.Warnings = append(view.Warnings, WarningMatchingCriteria)
	}
	if running {
		view.Blocked = true
		for i := range view.Environments {
			view.Environments[i].Blockers = append([]Blocker{{Code: BlockerDeletionRunning}}, view.Environments[i].Blockers...)
		}
	}
	for _, ep := range view.Environments {
		if len(ep.Blockers) > 0 {
			view.Blocked = true
		}
	}
	if app.Deletion != nil {
		progress := s.viewOf(app)
		view.Progress = &progress
	}
	snap.preview = view
	return snap, nil
}

func (s *Service) hashOf(snap *snapshot, actor, org string, expires int64, nonce string) (string, error) {
	return canon.Hash(tokenBody{Version: 1, Actor: actor, Org: org, Application: snap.app.Key, AppVersion: snap.app.Version,
		Status: string(snap.app.Lifecycle()), Expires: expires, Nonce: nonce, Envs: snap.pins})
}

// Preview returns the deletion scope (UC-18 MS-01..02). It reads persisted
// metadata only: no provider call and no write. A blocked preview carries no
// token.
func (s *Service) Preview(ctx context.Context, org, appKey string, actor Actor) (PreviewView, error) {
	if _, err := s.scoped(ctx, org, appKey); err != nil {
		return PreviewView{}, err
	}
	if !actor.MayDelete || actor.Username == "" {
		return PreviewView{}, ErrForbidden
	}
	if !s.available() {
		return PreviewView{}, ErrUnavailable
	}
	snap, err := s.load(ctx, org, appKey)
	if err != nil {
		return PreviewView{}, err
	}
	out := snap.preview
	if out.Blocked {
		return out, nil
	}
	expires := time.Unix(s.now().Add(PreviewTTL).Unix(), 0).UTC()
	nonce := strings.ReplaceAll(ids.New(), "-", "")[:16]
	hash, err := s.hashOf(snap, actor.Username, org, expires.Unix(), nonce)
	if err != nil {
		return PreviewView{}, err
	}
	out.Token = "ad1." + strconv.FormatInt(expires.Unix(), 10) + "." + nonce + "." + hash
	out.ExpiresAt = &expires
	return out, nil
}

func parseToken(token string) (time.Time, string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != "ad1" || len(parts[2]) != 16 || len(parts[3]) != 64 {
		return time.Time{}, "", "", ErrInvalid
	}
	unix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || unix <= 0 {
		return time.Time{}, "", "", ErrInvalid
	}
	for _, part := range parts[2:] {
		if _, err := hex.DecodeString(part); err != nil {
			return time.Time{}, "", "", ErrInvalid
		}
	}
	return time.Unix(unix, 0).UTC(), parts[2], parts[3], nil
}

func requestHashOf(org, token string) string {
	sum := sha256.Sum256([]byte("orchestrator-application-deletion-v1|" + org + "|" + token))
	return hex.EncodeToString(sum[:])
}

// Delete admits a confirmed deletion or a Retry (UC-18 MS-03..04, VAR-02).
// The whole check runs against one consistent read and the lifecycle write is
// a compare-and-set on the Application version, so any change after the
// preview makes the request stale. The run starts only after the write.
func (s *Service) Delete(ctx context.Context, org, appKey string, actor Actor, token, confirmationName string) (AdmissionView, error) {
	app, err := s.scoped(ctx, org, appKey)
	if err != nil {
		return AdmissionView{}, err
	}
	if !actor.MayDelete || actor.Username == "" {
		return AdmissionView{}, ErrForbidden
	}
	if confirmationName == "" || len(confirmationName) > 512 {
		return AdmissionView{}, ErrConfirmationMismatch
	}
	expires, nonce, hash, err := parseToken(token)
	if err != nil {
		return AdmissionView{}, err
	}
	if !s.available() {
		return AdmissionView{}, ErrUnavailable
	}
	requestHash := requestHashOf(org, token)
	if d := app.Deletion; d != nil && d.RequestHash == requestHash && d.Actor == actor.Username {
		if confirmationName != app.Name {
			return AdmissionView{}, ErrConfirmationMismatch
		}
		return AdmissionView{OperationID: d.OperationID, Status: d.Status, Replayed: true}, nil
	}
	if confirmationName != app.Name {
		return AdmissionView{}, ErrConfirmationMismatch
	}
	if !s.now().Before(expires) {
		return AdmissionView{}, ErrStalePreview
	}
	snap, err := s.load(ctx, org, appKey)
	if err != nil {
		return AdmissionView{}, err
	}
	again, err := s.hashOf(snap, actor.Username, org, expires.Unix(), nonce)
	if err != nil {
		return AdmissionView{}, err
	}
	if again != hash {
		return AdmissionView{}, ErrStalePreview
	}
	if snap.preview.Blocked {
		codes := []string{}
		for _, ep := range snap.preview.Environments {
			for _, b := range ep.Blockers {
				codes = append(codes, b.Code)
			}
		}
		return AdmissionView{}, &BlockedError{Codes: codes}
	}
	s.mu.Lock()
	if s.running[appKey] {
		s.mu.Unlock()
		return AdmissionView{}, ErrDeleting
	}
	s.running[appKey] = true
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		delete(s.running, appKey)
		s.mu.Unlock()
	}
	now := s.now()
	next := &appdomain.Deletion{OperationID: ids.New(), RequestHash: requestHash, Actor: actor.Username, Status: appdomain.DeletionRunning,
		Attempt: 1, RequestedAt: now, UpdatedAt: now}
	if prior := snap.app.Deletion; prior != nil {
		// A Retry keeps the operation identity and the verified stages.
		next.OperationID, next.Attempt, next.RequestedAt = prior.OperationID, prior.Attempt+1, prior.RequestedAt
		next.Environments = prior.Clone().Environments
	}
	byKey := map[string]bool{}
	for i := range next.Environments {
		byKey[next.Environments[i].Key] = true
		if next.Environments[i].Stage != appdomain.StageDone {
			next.Environments[i].Status, next.Environments[i].FailureCode = appdomain.DeletionRunning, ""
		}
	}
	for _, env := range snap.envs {
		if !byKey[env.Key] {
			next.Environments = append(next.Environments, appdomain.DeletionEnvironment{Key: env.Key, Stage: appdomain.StageRequested, Status: appdomain.DeletionRunning,
				RemovedWorkloads: []string{}, CleanupIDs: []string{}, Resources: []appdomain.DeletionResource{}})
		}
	}
	stored, err := s.life.TransitionApplication(ctx, appKey, snap.app.Version, appdomain.StatusDeleting, next)
	if errors.Is(err, persistence.ErrVersionConflict) {
		release()
		return AdmissionView{}, ErrStalePreview
	}
	if err != nil {
		release()
		return AdmissionView{}, err
	}
	s.wg.Add(1)
	go s.run(org, stored, actor.Username, snap)
	return AdmissionView{OperationID: next.OperationID, Status: next.Status}, nil
}

// Get returns the persisted progress of a deletion. It stays readable after
// the Application became DELETED, for the actor's Organization only.
func (s *Service) Get(ctx context.Context, org, appKey, operationID string) (DeletionView, error) {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil {
		return DeletionView{}, err
	}
	if org == "" || app.OrganizationKey != org || app.Deletion == nil || app.Deletion.OperationID != operationID {
		return DeletionView{}, persistence.ErrNotFound
	}
	return s.viewOf(app), nil
}

func (s *Service) viewOf(app appdomain.Application) DeletionView {
	d := app.Deletion.Clone()
	view := DeletionView{OperationID: d.OperationID, Application: app.Key, ApplicationStatus: string(app.Lifecycle()), Status: d.Status, FailureCode: d.FailureCode,
		Actor: d.Actor, Attempt: d.Attempt, Running: s.isRunning(app.Key), RequestedAt: d.RequestedAt, UpdatedAt: d.UpdatedAt, FinishedAt: d.FinishedAt,
		Environments: d.Environments}
	if view.Environments == nil {
		view.Environments = []appdomain.DeletionEnvironment{}
	}
	if d.Status == appdomain.DeletionRunning && !view.Running {
		// The process that ran it is gone: the outcome of the stage it was in
		// is unknown, and only a Retry continues.
		view.Status, view.FailureCode = appdomain.DeletionFailed, FailInterrupted
	}
	return view
}

// run executes the stages of every Environment, then finalizes.
func (s *Service) run(org string, app appdomain.Application, actor string, snap *snapshot) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.running, app.Key)
		s.mu.Unlock()
	}()
	ctx := context.Background()
	r := &runner{s: s, org: org, appKey: app.Key, actor: actor, snap: snap, version: app.Version, deletion: app.Deletion.Clone()}
	failed := false
	for i := range r.deletion.Environments {
		if r.deletion.Environments[i].Stage == appdomain.StageDone {
			continue
		}
		if code := r.environment(ctx, i); code != "" {
			failed = true
			r.update(ctx, func(d *appdomain.Deletion) {
				d.Environments[i].Status, d.Environments[i].FailureCode = appdomain.DeletionFailed, code
			})
		}
	}
	finished := s.now()
	if failed {
		r.update(ctx, func(d *appdomain.Deletion) {
			d.Status, d.FailureCode, d.FinishedAt = appdomain.DeletionFailed, firstFailure(d), &finished
		})
		return
	}
	r.deletion.Status, r.deletion.FailureCode, r.deletion.FinishedAt = appdomain.DeletionSucceeded, "", &finished
	r.deletion.UpdatedAt = finished
	if _, err := s.life.TransitionApplication(ctx, app.Key, r.version, appdomain.StatusDeleted, r.deletion); err != nil {
		log.Printf("appdeletion: finalize %s: %v", app.Key, err)
		if fresh, gerr := s.store.GetApplication(ctx, app.Key); gerr == nil {
			r.version = fresh.Version
			if _, err := s.life.TransitionApplication(ctx, app.Key, r.version, appdomain.StatusDeleted, r.deletion); err != nil {
				log.Printf("appdeletion: finalize retry %s: %v", app.Key, err)
			}
		}
	}
}

func firstFailure(d *appdomain.Deletion) string {
	for _, e := range d.Environments {
		if e.FailureCode != "" {
			return e.FailureCode
		}
	}
	return FailInternal
}

type runner struct {
	s        *Service
	org      string
	appKey   string
	actor    string
	snap     *snapshot
	version  int64
	deletion *appdomain.Deletion
}

// update persists one progress change; the Application stays DELETING.
func (r *runner) update(ctx context.Context, change func(d *appdomain.Deletion)) {
	change(r.deletion)
	r.deletion.UpdatedAt = r.s.now()
	for attempt := 0; attempt < 3; attempt++ {
		stored, err := r.s.life.TransitionApplication(ctx, r.appKey, r.version, appdomain.StatusDeleting, r.deletion)
		if err == nil {
			r.version = stored.Version
			return
		}
		if !errors.Is(err, persistence.ErrVersionConflict) {
			log.Printf("appdeletion: persist progress %s: %v", r.appKey, err)
			return
		}
		fresh, gerr := r.s.store.GetApplication(ctx, r.appKey)
		if gerr != nil {
			return
		}
		r.version = fresh.Version
	}
}

func (r *runner) scope(env string) resourcecleanup.Scope {
	return resourcecleanup.Scope{OrganizationKey: r.org, ApplicationKey: r.appKey, EnvironmentKey: env}
}

// environment runs the remaining stages of one Environment and returns a
// failure code, or "" when it is DONE.
func (r *runner) environment(ctx context.Context, i int) string {
	env := r.deletion.Environments[i].Key
	stages := []string{appdomain.StageRemovingWorkload, appdomain.StageDestroying, appdomain.StageVerifying, appdomain.StagePurgingSecrets}
	start := 0
	for k, st := range stages {
		if r.deletion.Environments[i].Stage == st {
			start = k
		}
	}
	for _, stage := range stages[start:] {
		r.update(ctx, func(d *appdomain.Deletion) {
			d.Environments[i].Stage, d.Environments[i].Status, d.Environments[i].FailureCode = stage, appdomain.DeletionRunning, ""
		})
		var code string
		switch stage {
		case appdomain.StageRemovingWorkload:
			code = r.removeWorkloads(ctx, i, env)
		case appdomain.StageDestroying:
			code = r.destroy(ctx, i, env)
		case appdomain.StageVerifying:
			code = r.verify(ctx, i, env)
		case appdomain.StagePurgingSecrets:
			code = r.purge(ctx, i, env)
		}
		if code != "" {
			return code
		}
	}
	r.update(ctx, func(d *appdomain.Deletion) {
		d.Environments[i].Stage, d.Environments[i].Status = appdomain.StageDone, appdomain.DeletionSucceeded
	})
	return ""
}

// removeWorkloads applies an empty Deployment Set through the UC-16 pending
// path (UC-07 VAR-02): every deployed workload is marked for deletion, every
// undeployed draft is discarded, then one Preview and Deploy run.
func (r *runner) removeWorkloads(ctx context.Context, i int, env string) string {
	view, err := r.s.workloads.List(ctx, r.appKey, env)
	if err != nil {
		log.Printf("appdeletion: list workloads %s/%s: %v", r.appKey, env, err)
		return FailWorkloadRemoval
	}
	removed := []string{}
	for _, w := range view.Workloads {
		if w.State == environment.DraftDelete {
			removed = append(removed, w.ID)
			continue
		}
		next, err := r.s.workloads.Delete(ctx, r.appKey, env, w.ID, view.DraftVersion)
		if err != nil {
			log.Printf("appdeletion: mark workload %s/%s/%s: %v", r.appKey, env, w.ID, err)
			if errors.Is(err, persistence.ErrEnvironmentBusy) {
				return FailEnvironmentBusy
			}
			return FailWorkloadRemoval
		}
		view = next
		if w.State == "" {
			removed = append(removed, w.ID)
		}
	}
	sort.Strings(removed)
	preview, err := r.s.pending.Preview(ctx, r.appKey, env)
	if err != nil {
		log.Printf("appdeletion: preview removal %s/%s: %v", r.appKey, env, err)
		return FailWorkloadRemoval
	}
	deploymentID := ""
	if len(preview.Changes) > 0 || preview.RoutePending {
		report, err := r.s.pending.Deploy(ctx, r.appKey, env, r.actor, preview.Token)
		if err != nil || report.Status != "SUCCEEDED" {
			log.Printf("appdeletion: deploy removal %s/%s: status=%s err=%v", r.appKey, env, report.Status, err)
			if errors.Is(err, persistence.ErrEnvironmentBusy) {
				return FailEnvironmentBusy
			}
			return FailWorkloadRemoval
		}
		for _, result := range report.Results {
			if result.DeploymentID != "" {
				deploymentID = result.DeploymentID
			}
		}
	}
	after, err := r.s.workloads.List(ctx, r.appKey, env)
	if err != nil || len(after.Workloads) != 0 {
		return FailWorkloadRemoval
	}
	r.update(ctx, func(d *appdomain.Deletion) {
		d.Environments[i].RemovedWorkloads = mergeSorted(d.Environments[i].RemovedWorkloads, removed)
		if deploymentID != "" {
			d.Environments[i].DeploymentID = deploymentID
		}
	})
	return ""
}

// destroy admits one cleanup per remaining generation and waits for it.
func (r *runner) destroy(ctx context.Context, i int, env string) string {
	sc := r.scope(env)
	actor := resourcecleanup.Actor{Username: r.actor, MayDelete: true}
	for round := 0; round < 16; round++ {
		inv, err := r.s.cleanup.EnvironmentInventory(ctx, sc)
		if err != nil {
			return FailCleanup
		}
		if len(inv.Items) == 0 {
			return ""
		}
		for _, item := range inv.Items {
			if item.Blocker != "" {
				return FailCleanupRefused
			}
		}
		generation := inv.Items[0].Generation
		admission, err := r.s.cleanup.AdmitEnvironmentCleanup(ctx, sc, actor, resourcecleanup.EnvironmentCleanupInput{
			DeletionID: r.deletion.OperationID, Attempt: r.deletion.Attempt*100 + round, Generation: generation})
		if err != nil {
			log.Printf("appdeletion: admit cleanup %s/%s gen %d: %v", r.appKey, env, generation, err)
			if errors.Is(err, persistence.ErrEnvironmentBusy) {
				return FailEnvironmentBusy
			}
			return FailCleanupRefused
		}
		r.update(ctx, func(d *appdomain.Deletion) {
			d.Environments[i].CleanupIDs = append(d.Environments[i].CleanupIDs, admission.OperationID)
		})
		result, err := r.s.cleanup.WaitCleanup(ctx, sc, admission.OperationID, r.s.poll)
		if err != nil {
			return FailCleanup
		}
		r.record(ctx, i, result)
		if result.Status != string(environment.OpSucceeded) || !result.AllVerified {
			return FailCleanup
		}
	}
	return FailCleanup
}

// record keeps the per-resource outcome of one cleanup operation.
func (r *runner) record(ctx context.Context, i int, view resourcecleanup.CleanupView) {
	r.update(ctx, func(d *appdomain.Deletion) {
		index := map[string]int{}
		for k, res := range d.Environments[i].Resources {
			index[res.ID] = k
		}
		for _, res := range view.Resources {
			item := appdomain.DeletionResource{ID: res.ID, Descriptor: res.Descriptor, Outcome: res.Outcome, Remaining: append([]string(nil), res.Remaining...)}
			if k, ok := index[res.ID]; ok {
				d.Environments[i].Resources[k] = item
			} else {
				d.Environments[i].Resources = append(d.Environments[i].Resources, item)
			}
		}
	})
}

// verify checks that no owned instance and no workload remains.
func (r *runner) verify(ctx context.Context, i int, env string) string {
	inv, err := r.s.cleanup.EnvironmentInventory(ctx, r.scope(env))
	if err != nil || len(inv.Items) != 0 {
		return FailVerification
	}
	view, err := r.s.workloads.List(ctx, r.appKey, env)
	if err != nil || len(view.Workloads) != 0 {
		return FailVerification
	}
	return ""
}

// Purger deletes exact value objects of one Application Environment. The
// Vault registry implements it; a registry without it cannot purge.
type Purger interface {
	PurgeValues(ctx context.Context, organizationKey, storeKey, applicationKey, environmentKey string, refs, deploymentIDs []string) (int, error)
}

// purge deletes the Environment's secret values and delivery bundles by
// exact reference (UC-18 MS-08, BR-07).
func (r *runner) purge(ctx context.Context, i int, env string) string {
	stores := r.snap.refs[env]
	if len(stores) == 0 {
		return ""
	}
	purger, ok := r.s.secrets.(Purger)
	if r.s.secrets == nil || !ok {
		return FailSecretPurge
	}
	total := 0
	for _, sr := range stores {
		n, err := purger.PurgeValues(ctx, r.org, sr.store, r.appKey, env, sr.refs, r.snap.deployIDs[env])
		if err != nil {
			log.Printf("appdeletion: purge %s/%s store %s: %v", r.appKey, env, sr.store, err)
			return FailSecretPurge
		}
		total += n
	}
	r.update(ctx, func(d *appdomain.Deletion) { d.Environments[i].SecretsPurged = total })
	return ""
}

func mergeSorted(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range append(append([]string{}, a...), b...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// A pending Suspend draft is insufficient: only the applied schedule can make
// removal safe. Reject before freezing the Application's workload editor.
func scheduledDeletionBlockers(doc environment.Document) []Blocker {
	out := []Blocker{}
	for _, id := range doc.ModuleIDs() {
		e := doc.Modules[id].Spec.Execution
		if e != nil && e.Kind == "CronJob" && (e.Schedule == nil || !e.Schedule.Suspend) {
			out = append(out, Blocker{Code: BlockerScheduleNotSuspended, Descriptor: id})
		}
	}
	return out
}
