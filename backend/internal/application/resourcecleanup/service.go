package resourcecleanup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/deployment"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/canon"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

// RedactedValue replaces an output the Resource Type does not declare
// non-secret (same marker as the Deployment view).
const RedactedValue = "***redacted***"

// PreviewTTL bounds how long a deletion preview may be confirmed.
const PreviewTTL = 15 * time.Minute

// Request bounds.
const (
	maxSelected   = 64
	maxIDLength   = 64
	maxNameLength = 512
)

// Engine is the cleanup engine the admitted operation runs on: the existing
// tfexec service (CleanupSource, persisted admission, owner/fence and
// physical absence proofs). There is no separate production engine.
type Engine interface {
	RunCleanup(ctx context.Context, organizationKey string, source tfexec.CleanupSource, report tfexec.StageReporter) (tfexec.CleanupResult, error)
}

// InstanceSource reads the full instance, including its immutable original
// target reference, on the State login. Only the admission uses it, to pin the
// original target of a selected resource; no query path does.
type InstanceSource interface {
	GetInstance(ctx context.Context, id string) (tfresource.Instance, error)
}

// Service implements ResourceQueryService, CleanupPlanner and
// ResourceCleanupService of UC-07/UC-09.
type Service struct {
	store  persistence.Store
	reader terraformstate.ResourceReader
	claims terraformstate.CleanupRepository
	state  InstanceSource
	engine Engine
	ops    *envops.Manager
	now    func() time.Time
	wg     sync.WaitGroup
}

// New builds the service over the Console store. Without PostgreSQL the store
// has no safe resource reader or cleanup repository and the service answers
// ErrUnavailable; it never falls back to another store.
func New(store persistence.Store, ops *envops.Manager) *Service {
	s := &Service{store: store, ops: ops, now: func() time.Time { return time.Now().UTC() }}
	if r, ok := store.(terraformstate.ResourceReader); ok {
		s.reader = r
	}
	if c, ok := store.(terraformstate.CleanupRepository); ok {
		s.claims = c
	}
	return s
}

// SetState enables the admission to pin original targets.
func (s *Service) SetState(src InstanceSource) { s.state = src }

// SetEngine enables the execution of admitted operations.
func (s *Service) SetEngine(e Engine) { s.engine = e }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Wait blocks until every admitted operation started by this process has
// finished (tests and graceful shutdown).
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) readable() bool { return s.reader != nil }

func (s *Service) mutable() bool {
	return s.reader != nil && s.claims != nil && s.state != nil && s.engine != nil && s.ops != nil
}

// GetResource returns the safe detail of one resource of the Environment
// (UC-09 RES-01). A foreign or unknown ID is ErrNotFound.
func (s *Service) GetResource(ctx context.Context, sc Scope, id string) (ResourceView, error) {
	if !s.readable() {
		return ResourceView{}, ErrUnavailable
	}
	var out ResourceView
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		p, err := buildPlan(snap, id)
		if err != nil {
			return err
		}
		out = s.resourceView(snap, p)
		return nil
	})
	return out, err
}

// ListResources returns the Environment's resources, every generation, newest
// generation first, for navigation to a detail. Platform resources are not
// part of the Environment's own resources and are not listed.
func (s *Service) ListResources(ctx context.Context, sc Scope) ([]SummaryView, error) {
	if !s.readable() {
		return nil, ErrUnavailable
	}
	out := []SummaryView{}
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		for _, id := range snap.order {
			rec := snap.records[id]
			if platformResource(rec) {
				continue
			}
			d, _ := resource.ParseDescriptor(rec.Instance.Identity.Descriptor)
			out = append(out, SummaryView{ID: id, Descriptor: rec.Instance.Identity.Descriptor, Type: d.Type, Class: d.Class, ResourceID: d.ID,
				Status: safeInstanceStatus(rec.Instance.Status), Scope: string(rec.Instance.Identity.Scope.Type), Generation: rec.Instance.Identity.TargetGeneration, Module: safeToken(rec.ModuleID)})
		}
		return nil
	})
	return out, err
}

func (s *Service) resourceView(snap *snapshot, p *plan) ResourceView {
	rec := snap.records[p.root]
	in := rec.Instance
	d, _ := resource.ParseDescriptor(in.Identity.Descriptor)
	it := p.items[p.root]
	out := ResourceView{
		ID: in.ID, Application: snap.scope.ApplicationKey, Environment: snap.scope.EnvironmentKey,
		Descriptor: in.Identity.Descriptor, Type: d.Type, Class: d.Class, ResourceID: d.ID,
		Status: safeInstanceStatus(in.Status), Scope: ScopeView{Type: string(in.Identity.Scope.Type), ID: in.Identity.Scope.ID},
		Owner: string(in.Identity.Scope.Type), Generation: in.Identity.TargetGeneration,
		Module:  ModuleView{ID: safeToken(rec.ModuleID), Revision: safeToken(rec.ModuleRevision)},
		Version: in.Version, Outputs: map[string]any{}, Consumers: it.consumers, Eligibility: p.eligibility(),
	}
	if out.Consumers == nil {
		out.Consumers = []ConsumerView{}
	}
	if a, ok := snap.activeOf(rec); ok {
		out.Outputs = redactOutputs(a.Outputs, snap.types[d.Type])
	}
	if rec.LastAttempt != nil {
		safe := domain.ResourceAttempt{Descriptor: in.Identity.Descriptor, ScopeType: string(in.Identity.Scope.Type), Number: rec.LastAttempt.Number, Action: string(rec.LastAttempt.Action),
			Ownership: string(rec.LastAttempt.Ownership), Stage: rec.LastAttempt.Stage, Status: string(rec.LastAttempt.Status), SafeErrorCode: rec.LastAttempt.SafeErrorCode,
			ModuleID: rec.ModuleID, ModuleRevision: rec.ModuleRevision}.Safe()
		out.LastAttempt = &AttemptView{Number: safe.Number, Action: safe.Action, Stage: safe.Stage, Status: safe.Status, ErrorCode: safe.SafeErrorCode, UpdatedAt: rec.LastAttempt.UpdatedAt}
	}
	for _, c := range snap.cleanups {
		for _, selected := range c.SelectedIDs {
			if selected == in.ID {
				out.Cleanup = &CleanupSummary{OperationID: c.OperationID, Status: safeOperationStatus(c.Status), Stage: safeStage(c.Stage)}
				break
			}
		}
		if out.Cleanup != nil {
			break
		}
	}
	return out
}

// redactOutputs fails closed: an output is shown only when the Resource Type
// declares it non-secret and no nested value is a secret reference.
func redactOutputs(outputs map[string]any, typ resource.Type) map[string]any {
	visible := map[string]bool{}
	for _, f := range typ.Outputs {
		if !f.Secret {
			visible[f.Name] = true
		}
	}
	out := map[string]any{}
	for name, value := range outputs {
		if !visible[name] || containsSecretRef(value) {
			out[name] = RedactedValue
			continue
		}
		out[name] = value
	}
	return out
}

func containsSecretRef(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "secretRef" || containsSecretRef(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsSecretRef(item) {
				return true
			}
		}
	}
	return false
}

var tokenShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func safeToken(v string) string {
	if tokenShape.MatchString(v) {
		return v
	}
	return domain.AttemptUnknown
}

func safeInstanceStatus(s tfresource.InstanceStatus) string {
	if s.Valid() {
		return string(s)
	}
	return domain.AttemptUnknown
}

func safeOperationStatus(v string) string {
	switch environment.OperationStatus(v) {
	case environment.OpActive, environment.OpInterrupted, environment.OpRecovering, environment.OpSucceeded, environment.OpFailed, environment.OpRecovered:
		return v
	}
	return domain.AttemptUnknown
}

func safeStage(v string) string {
	switch v {
	case tfexec.CleanupRequested, tfexec.CleanupChecking, tfexec.CleanupDestroying, tfexec.CleanupVerifying:
		return v
	}
	return domain.AttemptUnknown
}

func candidateView(p *plan, id string) CandidateView {
	it := p.items[id]
	d, _ := resource.ParseDescriptor(it.rec.Instance.Identity.Descriptor)
	v := CandidateView{ID: id, Descriptor: it.rec.Instance.Identity.Descriptor, Type: d.Type, Class: d.Class, Status: safeInstanceStatus(it.rec.Instance.Status),
		Version: it.rec.Instance.Version, Selectable: len(it.reasons) == 0, Reasons: it.reasons, RequiredBy: it.requiredBy, Consumers: it.consumers}
	if v.Reasons == nil {
		v.Reasons = []string{}
	}
	if v.RequiredBy == nil {
		v.RequiredBy = []string{}
	}
	if v.Consumers == nil {
		v.Consumers = []ConsumerView{}
	}
	return v
}

// PrepareDeletion returns the deletion scope of a root (OC-19 PrepareDeletion).
// It reads persisted metadata only: no provider, no secret and no write.
func (s *Service) PrepareDeletion(ctx context.Context, sc Scope, actor Actor, rootID string) (PreviewView, error) {
	var out PreviewView
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		if !s.readable() {
			return ErrUnavailable
		}
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		p, err := buildPlan(snap, rootID)
		if err != nil {
			return err
		}
		if !actor.MayDelete {
			return ErrForbidden
		}
		if !s.mutable() {
			return ErrUnavailable
		}
		if p.busy {
			return persistence.Busy(sc.ApplicationKey, sc.EnvironmentKey)
		}
		if r := p.rootRefusal(); r != "" {
			return &RefusedError{Reason: r}
		}
		targets, err := s.targetDigests(ctx, snap, p)
		if err != nil {
			return err
		}
		hash, err := p.tokenHash(actor.Username, targets)
		if err != nil {
			return err
		}
		expires := s.now().Add(PreviewTTL)
		out = PreviewView{Token: "rd1." + strconv.FormatInt(expires.Unix(), 10) + "." + hash, ExpiresAt: time.Unix(expires.Unix(), 0).UTC(),
			Application: sc.ApplicationKey, Environment: sc.EnvironmentKey, Generation: snap.records[rootID].Instance.Identity.TargetGeneration,
			ConfirmationName: snap.records[rootID].Instance.Identity.Descriptor, Root: candidateView(p, rootID), Candidates: []CandidateView{}}
		for _, id := range p.order[1:] {
			out.Candidates = append(out.Candidates, candidateView(p, id))
		}
		return nil
	})
	return out, err
}

// parseToken splits a preview token into its expiry and pinned-state hash.
func parseToken(token string) (expires time.Time, hash string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "rd1" || len(parts[2]) != 64 {
		return time.Time{}, "", invalid("token")
	}
	unix, perr := strconv.ParseInt(parts[1], 10, 64)
	if perr != nil || unix <= 0 {
		return time.Time{}, "", invalid("token")
	}
	if _, derr := hex.DecodeString(parts[2]); derr != nil {
		return time.Time{}, "", invalid("token")
	}
	return time.Unix(unix, 0).UTC(), parts[2], nil
}

// Input is a confirmed deletion request.
type Input struct {
	Token            string
	ConfirmationName string
	Selected         []string
}

// CleanupResources admits a confirmed deletion (OC-19 CleanupResources). The
// whole check - session scope and role, preview token, versions, UNREFERENCED,
// ownership, consumers (current, pending and retained), the selected set and
// the absence of any active operation - runs in the single short transaction
// that writes the claim and the confirmed scope; nothing is held across a
// provider call. The engine runs only after that transaction committed.
func (s *Service) CleanupResources(ctx context.Context, sc Scope, actor Actor, rootID string, in Input) (AdmissionView, error) {
	if len(in.Selected) == 0 || len(in.Selected) > maxSelected {
		return AdmissionView{}, invalid("selectedResourceIds")
	}
	for _, id := range in.Selected {
		if id == "" || len(id) > maxIDLength {
			return AdmissionView{}, invalid("selectedResourceIds")
		}
	}
	if in.ConfirmationName == "" || len(in.ConfirmationName) > maxNameLength {
		return AdmissionView{}, ErrConfirmationMismatch
	}
	expires, tokenHash, err := parseToken(in.Token)
	if err != nil {
		return AdmissionView{}, err
	}

	// Read-only pre-check outside the transaction: scope, role, the exact
	// name and the original targets to pin. Everything it decides is decided
	// again inside the admission transaction.
	var (
		pre     *snapshot
		prePlan *plan
	)
	if err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		if !s.readable() {
			return ErrUnavailable
		}
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		p, err := buildPlan(snap, rootID)
		if err != nil {
			return err
		}
		pre, prePlan = snap, p
		return nil
	}); err != nil {
		return AdmissionView{}, err
	}
	if !actor.MayDelete {
		return AdmissionView{}, ErrForbidden
	}
	if !s.mutable() {
		return AdmissionView{}, ErrUnavailable
	}
	rootName := pre.records[rootID].Instance.Identity.Descriptor
	if in.ConfirmationName != rootName {
		return AdmissionView{}, ErrConfirmationMismatch
	}
	if !s.now().Before(expires) {
		return AdmissionView{}, ErrStalePreview
	}

	requestHash := requestHashOf(sc.OrganizationKey, in.Token)
	targets, err := s.targetDigests(ctx, pre, prePlan)
	if err != nil {
		return AdmissionView{}, err
	}
	check := s.admissionCheck(sc, actor, rootID, tokenHash, in.Selected, pre.env.ID, targets)

	// Pin the original target of every selected resource. target_ref is
	// immutable, so reading it outside the transaction cannot race; the
	// versions below are compared again by the claim.
	items := make([]tfresource.CleanupItem, 0, len(in.Selected))
	for _, id := range in.Selected {
		rec, ok := pre.records[id]
		if !ok {
			return AdmissionView{}, invalid("selectedResourceIds")
		}
		inst, err := s.state.GetInstance(ctx, id)
		if err != nil || inst.Identity.EnvironmentID != pre.env.ID || inst.Identity.OrganizationID != rec.Instance.Identity.OrganizationID {
			return AdmissionView{}, ErrUnavailable
		}
		digest, err := tfresource.TargetDigest(inst.TargetRef)
		if err != nil {
			return AdmissionView{}, ErrUnavailable
		}
		// Only a digest of the original target is persisted in the
		// Console-readable operation record; the engine re-reads the target
		// itself and compares.
		items = append(items, tfresource.CleanupItem{InstanceID: id, Version: inst.Version, StateKey: inst.StateKey, ModuleBundleID: inst.ModuleBundleID, TargetDigest: digest})
	}
	candidates := make([]string, 0, len(prePlan.order))
	retained := map[string]string{}
	chosen := map[string]bool{}
	for _, id := range in.Selected {
		chosen[id] = true
	}
	for _, id := range prePlan.order {
		candidates = append(candidates, id)
		if !chosen[id] {
			retained[id] = "NOT_SELECTED"
		}
	}
	req := tfresource.CleanupRequest{
		Mode: tfresource.CleanupUserConfirmed, ActorRef: actor.Username, RootInstanceID: rootID, CandidateIDs: candidates,
		Scope:       tfresource.ConfirmedScope{SelectedResourceIDs: append([]string(nil), in.Selected...), Items: items, RetainedReasons: retained},
		RequestHash: requestHash, ConfirmationName: rootName,
		ScopeSnapshot: map[string]any{"environmentVersion": pre.env.Version, "draftVersion": pre.env.DraftVersion, "currentSetId": pre.env.CurrentDeploymentSetID,
			"generation": pre.records[rootID].Instance.Identity.TargetGeneration, "previewExpiresAt": expires.Format(time.RFC3339)},
	}
	if err := req.Validate(); err != nil {
		return AdmissionView{}, invalid("selectedResourceIds")
	}
	claim := persistence.OperationClaim{
		ID: ids.New(), ApplicationKey: sc.ApplicationKey, EnvironmentKey: sc.EnvironmentKey, Owner: s.ops.InstanceID(),
		Kind: environment.OpResourceCleanup, Deadline: s.now().Add(envops.DeadlineFor(environment.OpResourceCleanup)),
		Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: pre.env.Version, CheckDraftVersion: true, DraftVersion: pre.env.DraftVersion,
			CheckSet: true, CurrentSetID: pre.env.CurrentDeploymentSetID},
		Detail: map[string]any{"rootId": rootID},
	}
	op, replayed, err := s.claims.ClaimResourceCleanup(ctx, claim, req, check)
	switch {
	case errors.Is(err, terraformstate.ErrCleanupSelectionChanged), errors.Is(err, persistence.ErrVersionConflict):
		return AdmissionView{}, ErrStalePreview
	case err != nil:
		return AdmissionView{}, err
	}
	view := AdmissionView{OperationID: op.ID, Status: safeOperationStatus(string(op.Status)), Stage: safeStage(op.Stage), Replayed: replayed}
	if view.Stage == domain.AttemptUnknown {
		view.Stage = tfexec.CleanupRequested
	}
	if replayed {
		return view, nil
	}
	view.Stage = tfexec.CleanupRequested
	actives := make([]string, 0, len(in.Selected))
	for _, id := range in.Selected {
		if aid := pre.records[id].Instance.ActiveResourceID; aid != "" {
			actives = append(actives, aid)
		}
	}
	s.wg.Add(1)
	go s.run(op, sc, in.Selected, actives)
	return view, nil
}

// targetDigests reads the immutable original target of every item of the plan
// on the State login and keeps only a one-way digest, so the token pins the
// exact original targets without carrying them. Admission and preview both
// need the State port; without it nothing is previewed or admitted.
func (s *Service) targetDigests(ctx context.Context, snap *snapshot, p *plan) (map[string]string, error) {
	out := make(map[string]string, len(p.order))
	for _, id := range p.order {
		rec := snap.records[id]
		inst, err := s.state.GetInstance(ctx, id)
		if err != nil || inst.Identity.EnvironmentID != snap.env.ID || inst.Identity.OrganizationID != rec.Instance.Identity.OrganizationID || inst.StateKey != rec.Instance.StateKey {
			return nil, ErrUnavailable
		}
		digest, err := canon.Hash(inst.TargetRef)
		if err != nil {
			return nil, ErrUnavailable
		}
		out[id] = digest
	}
	return out, nil
}

func requestHashOf(org, token string) string {
	sum := sha256.Sum256([]byte("orchestrator-resource-deletion-v1|" + org + "|" + token))
	return hex.EncodeToString(sum[:])
}

// admissionCheck is the atomic part of the admission. It reloads everything
// through the claim transaction (Environment row locked), so a deploy, a draft
// write, a consumer edge or a status change cannot commit between this check
// and the claim. Any failure rejects the whole request before any effect.
func (s *Service) admissionCheck(sc Scope, actor Actor, rootID, tokenHash string, selected []string, envID string, targets map[string]string) terraformstate.AdmissionCheck {
	return func(ctx context.Context) error {
		if !actor.MayDelete {
			return ErrForbidden
		}
		snap, err := loadSnapshot(ctx, s.store, s.reader, sc)
		if err != nil {
			return err
		}
		if snap.env.ID != envID {
			return ErrStalePreview
		}
		p, err := buildPlan(snap, rootID)
		if errors.Is(err, persistence.ErrNotFound) {
			return ErrStalePreview
		}
		if err != nil {
			return err
		}
		if p.busy {
			return persistence.Busy(sc.ApplicationKey, sc.EnvironmentKey)
		}
		hash, err := p.tokenHash(actor.Username, targets)
		if err != nil {
			return err
		}
		if hash != tokenHash {
			return ErrStalePreview
		}
		return p.validate(selected)
	}
}

// progress accumulates the safe operation detail.
type progress struct {
	selected map[string]bool
	current  string
}

func (g *progress) detail(detail map[string]any) map[string]any {
	if id, ok := detail["resource"].(string); ok && g.selected[id] {
		g.current = id
	}
	out := map[string]any{"kind": "RESOURCE_CLEANUP"}
	if g.current != "" {
		out["current"] = g.current
	}
	return out
}

// run executes an admitted operation under its Environment claim. It never
// holds a database transaction across the engine, and it never retries or
// replays a mutation.
func (s *Service) run(op environment.Operation, sc Scope, selected, actives []string) {
	defer s.wg.Done()
	lease := s.ops.Adopt(context.Background(), op)
	track := &progress{selected: map[string]bool{}}
	for _, id := range selected {
		track.selected[id] = true
	}
	s.setActive(lease.Ctx, sc, actives, resource.StatusDestroying)
	lastStage := tfexec.CleanupRequested
	report := func(stage string, detail map[string]any) error {
		if safeStage(stage) != domain.AttemptUnknown {
			lastStage = stage
		}
		return lease.Stage(lastStage, track.detail(detail))
	}
	result, err := s.engine.RunCleanup(lease.Ctx, sc.OrganizationKey, s.claims, report)

	ctx, cancel := lease.Detached()
	defer cancel()
	if err != nil {
		code := "CLEANUP_ERROR"
		if errors.Is(err, tfexec.ErrCleanupRejected) {
			code = "CLEANUP_REJECTED"
		}
		_ = lease.Stage(lastStage, map[string]any{"kind": "RESOURCE_CLEANUP", "terminal": string(tfexec.CleanupFailed)})
		s.setActive(ctx, sc, actives, resource.StatusUnreferenced)
		_ = lease.End(environment.OpFailed, code)
		return
	}
	_ = lease.Stage(lastStage, resultDetail(result))
	destroyed := map[string]bool{}
	keep := map[string]bool{}
	for _, r := range result.Instances {
		if r.Destroyed {
			destroyed[r.InstanceID] = true
		}
		if r.Code == tfexec.CodeNeedsAttention {
			keep[r.InstanceID] = true
		}
	}
	s.finalizeActive(ctx, sc, selected, actives, destroyed, keep)
	switch result.Terminal {
	case tfexec.CleanupSucceeded:
		_ = lease.End(environment.OpSucceeded, "")
	case tfexec.CleanupNeedsAttention:
		_ = lease.Suspend("CLEANUP_NEEDS_ATTENTION")
	default:
		_ = lease.End(environment.OpFailed, "CLEANUP_INCOMPLETE")
	}
}

// finalizeActive applies the Active Resource state machine after the engine:
// verified-absent resources become DESTROYED; a known failure returns the
// resource to UNREFERENCED; an unknown outcome stays DESTROYING.
func (s *Service) finalizeActive(ctx context.Context, sc Scope, selected, actives []string, destroyed, keep map[string]bool) {
	byInstance := map[string]string{}
	rec, err := s.reader.ListResourceRecords(ctx, sc.ApplicationKey, sc.EnvironmentKey)
	if err != nil {
		return
	}
	for _, r := range rec {
		byInstance[r.Instance.ID] = r.Instance.ActiveResourceID
	}
	toDestroyed, toUnref := []string{}, []string{}
	for _, id := range selected {
		aid := byInstance[id]
		switch {
		case aid == "":
		case destroyed[id]:
			toDestroyed = append(toDestroyed, aid)
		case keep[id]:
		default:
			toUnref = append(toUnref, aid)
		}
	}
	s.setActive(ctx, sc, toDestroyed, resource.StatusDestroyed)
	s.setActive(ctx, sc, toUnref, resource.StatusUnreferenced)
}

// setActive moves the listed Active Resources to status. It re-reads each
// record so outputs and fingerprints written elsewhere are preserved.
func (s *Service) setActive(ctx context.Context, sc Scope, activeIDs []string, status resource.Status) {
	if len(activeIDs) == 0 {
		return
	}
	want := map[string]bool{}
	for _, id := range activeIDs {
		want[id] = true
	}
	list, err := s.store.ListActiveResources(ctx, sc.OrganizationKey)
	if err != nil {
		return
	}
	for _, a := range list {
		if !want[a.ID] || a.Status == status {
			continue
		}
		a.Status = status
		_, _ = s.store.UpsertActiveResource(ctx, a)
	}
}

// resultDetail is the allowlisted persisted form of an engine result.
func resultDetail(r tfexec.CleanupResult) map[string]any {
	results := make([]any, 0, len(r.Instances))
	for _, i := range r.Instances {
		results = append(results, map[string]any{
			"id": i.InstanceID, "destroyed": i.Destroyed, "remaining": sanitizeIDs(i.Remaining),
			"remainingUnknown": i.RemainingUnknown, "code": allowCode(i.Code),
		})
	}
	return map[string]any{"kind": "RESOURCE_CLEANUP", "terminal": allowTerminal(string(r.Terminal)), "results": results}
}

var physicalID = regexp.MustCompile(`^[A-Za-z0-9_.:/@=+\-\[\]"]{1,256}$`)

// sanitizeIDs keeps only physical IDs of a plain shape; anything else is
// counted as hidden instead of being shown.
func sanitizeIDs(in []string) []string {
	out := []string{}
	for _, v := range in {
		if physicalID.MatchString(v) {
			out = append(out, v)
		}
	}
	return out
}

var resultCodes = map[string]bool{
	tfexec.CodeNotAttempted: true, tfexec.CodeRetainedConsumer: true, tfexec.CodeNeedsAttention: true, tfexec.CodeInstanceHeld: true,
	tfexec.CodeTargetUnreach: true, tfexec.CodeDestructivePlan: true, tfexec.CodeNamespaceContent: true, tfexec.CodeDestroyFailed: true,
}

func allowCode(code string) string {
	if code == "" || resultCodes[code] {
		return code
	}
	return "UNKNOWN_ERROR"
}

func allowTerminal(v string) string {
	switch tfexec.CleanupTerminal(v) {
	case tfexec.CleanupSucceeded, tfexec.CleanupFailed, tfexec.CleanupNeedsAttention:
		return v
	}
	return ""
}

var failureCodes = map[string]bool{
	"CLEANUP_INCOMPLETE": true, "CLEANUP_REJECTED": true, "CLEANUP_ERROR": true, "CLEANUP_NEEDS_ATTENTION": true,
}

// GetCleanup returns the safe persisted progress of one cleanup operation
// (UC-09 RES-03). It reads persistence only: no provider, no refresh.
func (s *Service) GetCleanup(ctx context.Context, sc Scope, operationID string) (CleanupView, error) {
	if !s.readable() {
		return CleanupView{}, ErrUnavailable
	}
	var out CleanupView
	err := s.store.ReadSnapshot(ctx, func(ctx context.Context, view persistence.Store) error {
		snap, err := loadSnapshot(ctx, view, s.reader, sc)
		if err != nil {
			return err
		}
		rec, err := s.reader.GetCleanupRecord(ctx, sc.ApplicationKey, sc.EnvironmentKey, operationID)
		if err != nil {
			return err
		}
		out = projectCleanup(snap, rec)
		return nil
	})
	return out, err
}

type storedResult struct {
	destroyed bool
	remaining []string
	unknown   bool
	code      string
}

func projectCleanup(snap *snapshot, rec tfresource.CleanupRecord) CleanupView {
	status := safeOperationStatus(rec.Status)
	view := CleanupView{OperationID: rec.OperationID, Application: snap.scope.ApplicationKey, Environment: snap.scope.EnvironmentKey,
		Status: status, Stage: safeStage(rec.Stage), Mode: string(rec.Mode), Actor: rec.ActorRef, RootID: rec.RootID,
		Resources: []CleanupResourceView{}, RemainingID: []string{}, StartedAt: rec.StartedAt, UpdatedAt: rec.UpdatedAt, FinishedAt: rec.FinishedAt}
	if view.Mode != string(tfresource.CleanupUserConfirmed) && view.Mode != string(tfresource.CleanupRunCleanup) {
		view.Mode = domain.AttemptUnknown
	}
	if rec.Failure != "" {
		view.FailureCode = "CLEANUP_FAILED"
		if failureCodes[rec.Failure] {
			view.FailureCode = rec.Failure
		}
	}
	selected := map[string]bool{}
	for _, id := range rec.SelectedIDs {
		selected[id] = true
	}
	if t, ok := rec.Detail["terminal"].(string); ok {
		view.Terminal = allowTerminal(t)
	}
	current, _ := rec.Detail["current"].(string)
	stored := map[string]storedResult{}
	if list, ok := rec.Detail["results"].([]any); ok {
		for _, raw := range list {
			m, _ := raw.(map[string]any)
			id, _ := m["id"].(string)
			if !selected[id] {
				continue
			}
			r := storedResult{}
			r.destroyed, _ = m["destroyed"].(bool)
			r.unknown, _ = m["remainingUnknown"].(bool)
			code, _ := m["code"].(string)
			r.code = allowCode(code)
			if remaining, ok := m["remaining"].([]any); ok {
				for _, v := range remaining {
					if str, ok := v.(string); ok {
						r.remaining = append(r.remaining, str)
					}
				}
			}
			r.remaining = sanitizeIDs(r.remaining)
			stored[id] = r
		}
	}
	order := append([]string(nil), rec.SelectedIDs...)
	// Root first, then by descriptor, so the display order is stable.
	sortIDs(order, snap, rec.RootID)
	held := status == string(environment.OpActive) || status == string(environment.OpRecovering)
	allVerified := len(order) > 0
	remainingSeen := map[string]bool{}
	for _, id := range order {
		r, known := snap.records[id]
		item := CleanupResourceView{ID: id, Descriptor: "unknown", Status: domain.AttemptUnknown, Outcome: "UNKNOWN", Remaining: []string{}}
		if known {
			item.Descriptor = r.Instance.Identity.Descriptor
			item.Status = safeInstanceStatus(r.Instance.Status)
		}
		res, hasResult := stored[id]
		persistedGone := known && r.Instance.Status == tfresource.InstanceDestroyed
		switch {
		case persistedGone:
			item.Outcome, item.Verified = "DESTROYED", true
		case hasResult && res.code == tfexec.CodeNotAttempted, hasResult && res.code == tfexec.CodeRetainedConsumer:
			item.Outcome, item.ReasonCode = "RETAINED", res.code
		case hasResult && res.code == tfexec.CodeNeedsAttention:
			item.Outcome, item.ReasonCode = "NEEDS_ATTENTION", res.code
		case hasResult:
			item.Outcome, item.ReasonCode = "REMAINING", res.code
		case known && r.Instance.Status == tfresource.InstanceDestroying, held && id == current:
			item.Outcome = "DESTROYING"
		case held:
			item.Outcome = "PENDING"
		}
		if hasResult && !persistedGone {
			item.Remaining = res.remaining
			item.RemainingUnknown = res.unknown || (len(res.remaining) == 0)
		} else if !persistedGone && !held {
			// Not verified absent and no result recorded: absence is unknown.
			item.RemainingUnknown = true
		}
		if !item.Verified {
			allVerified = false
			for _, v := range item.Remaining {
				if !remainingSeen[v] {
					remainingSeen[v] = true
					view.RemainingID = append(view.RemainingID, v)
				}
			}
		}
		view.Resources = append(view.Resources, item)
	}
	sortStrings(view.RemainingID)
	view.AllVerified = allVerified && status == string(environment.OpSucceeded)
	return view
}
