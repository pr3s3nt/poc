package drift

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

type RepairEngine interface {
	PrepareInstanceRepair(context.Context, string, string) (tfexec.RepairPreparation, error)
	RepairInstance(context.Context, string, string, string) error
	SealReviewToken(context.Context, string, string) (persistence.CipherArtifact, string, error)
	OpenReviewToken(context.Context, string, string, persistence.CipherArtifact) (string, error)
}
type PrepareRepairCommand struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, ActorID, RequestID, ScanID string
	InstanceIDs                                                                 []string
	ExpectedEnvironmentVersion                                                  int64
	PreviousReviewID                                                            string
}
type RepairView struct {
	Review      domain.RepairReview `json:"review"`
	ReviewToken string              `json:"reviewToken,omitempty"`
}

func (s *Service) repairPorts() (RepairEngine, terraformstate.RepairReviewRepository, error) {
	engine, ok := s.Inspector.(RepairEngine)
	if !ok {
		return nil, nil, tfexec.ErrBaselineUnavailable
	}
	repo, ok := s.Instances.(terraformstate.RepairReviewRepository)
	if !ok {
		return nil, nil, tfexec.ErrBaselineUnavailable
	}
	return engine, repo, nil
}
func (s *Service) PrepareRepair(ctx context.Context, cmd PrepareRepairCommand) (out RepairView, err error) {
	engine, repo, err := s.repairPorts()
	if err != nil {
		return out, err
	}
	if !ids.ValidUUID(cmd.ActorID) || !ids.ValidUUID(cmd.RequestID) || !ids.ValidUUID(cmd.ScanID) || len(cmd.InstanceIDs) == 0 || len(cmd.InstanceIDs) > 20 || cmd.ExpectedEnvironmentVersion < 1 || (cmd.PreviousReviewID != "" && !ids.ValidUUID(cmd.PreviousReviewID)) {
		return out, domain.ErrScan
	}
	selected := append([]string(nil), cmd.InstanceIDs...)
	sort.Strings(selected)
	for n, id := range selected {
		if !ids.ValidUUID(id) || (n > 0 && selected[n-1] == id) {
			return out, domain.ErrScan
		}
	}
	org, env, err := s.scope(ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return out, err
	}
	raw, _ := json.Marshal([]any{org, env.ID, cmd.ActorID, cmd.ScanID, selected, cmd.ExpectedEnvironmentVersion, cmd.PreviousReviewID})
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	prior, err := repo.FindRepairReviewRequest(ctx, org, env.ID, cmd.ActorID, cmd.RequestID)
	if err == nil {
		if prior.Review.RequestDigest != digest {
			return out, persistence.ErrVersionConflict
		}
		return s.repairView(ctx, engine, prior, cmd.ActorID)
	}
	if !errors.Is(err, persistence.ErrNotFound) {
		return out, err
	}
	if env.Version != cmd.ExpectedEnvironmentVersion {
		return out, persistence.ErrVersionConflict
	}

	var members []domain.RepairInstance
	var consumers []domain.RepairConsumer
	var dependencies map[string][]string
	roots := selected
	if cmd.PreviousReviewID == "" {
		scan, err := s.Store.GetDriftScan(ctx, org, env.ID, cmd.ScanID)
		if err != nil {
			return out, err
		}
		if scan.Status != domain.Succeeded || scan.Result != domain.Drifted {
			return out, tfexec.ErrRepairBlocked
		}
		covered := map[string]domain.InstancePin{}
		for _, pin := range scan.Scope {
			covered[pin.InstanceID] = pin
		}
		for _, id := range selected {
			pin, ok := covered[id]
			if !ok || pin.BaselineID == "" {
				return out, tfexec.ErrBaselineUnavailable
			}
			inst, e := s.Instances.GetInstance(ctx, id)
			b, f := s.Instances.LatestExecutionBaseline(ctx, org, id)
			if e != nil || f != nil || inst.Version != pin.InstanceVersion || b.ID != pin.BaselineID {
				return out, tfexec.ErrRepairStale
			}
		}
		members, err = s.repairScope(ctx, org, env, roots)
		if err != nil {
			return out, err
		}
		if topology, ok := s.Consumers.(interface {
			RepairTopology(context.Context, string, string, string, []string) (map[string][]string, []string, error)
		}); ok {
			descriptors := []string{}
			for _, m := range members {
				descriptors = append(descriptors, m.Descriptor)
			}
			var expanded []string
			dependencies, expanded, err = topology.RepairTopology(ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey, descriptors)
			if err != nil {
				return out, err
			}
			members, err = s.repairScope(ctx, org, env, append(append([]string{}, roots...), expanded...))
			if err != nil {
				return out, err
			}
			members, err = orderRepairMembers(members, dependencies)
			if err != nil {
				return out, err
			}
		}
		if s.Consumers != nil {
			scope := []string{}
			for _, m := range members {
				scope = append(scope, m.Descriptor)
			}
			consumers, err = s.Consumers.PreviewRepairConsumers(ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey, scope)
			if err != nil {
				return out, err
			}
		}
	} else {
		parent, err := repo.GetRepairReview(ctx, org, env.ID, cmd.PreviousReviewID)
		if err != nil {
			return out, err
		}
		if parent.Review.ActorID != cmd.ActorID || parent.Review.ScanID != cmd.ScanID || parent.Review.EnvironmentVersion != env.Version || parent.Review.Status != domain.Consumed {
			return out, tfexec.ErrRepairStale
		}
		op, err := s.Store.GetOperation(ctx, parent.Review.ConsumedOperationID)
		if err != nil || op.Kind != environment.OpDriftRepair || op.Status != environment.OpFailed || op.Stage != "DRIFT_REPAIR_REVIEW_REQUIRED" || op.Detail["repairReviewId"] != parent.Review.ID {
			return out, tfexec.ErrRepairStale
		}
		members, err = decodeRepairCheckpoint(op.Detail["checkpoint"])
		if err != nil {
			return out, err
		}
		if err = s.verifyRepairPins(ctx, org, members); err != nil {
			return out, err
		}
		roots = parent.Review.RootIDs
		consumers = parent.Review.Consumers
		dependencies = parent.Review.Dependencies
		if raw, exists := op.Detail["consumerCheckpoint"]; exists {
			body, e := json.Marshal(raw)
			if e != nil || json.Unmarshal(body, &consumers) != nil {
				return out, tfexec.ErrRepairStale
			}
		}
		expected := append([]string{}, roots...)
		sort.Strings(expected)
		if !equalRepairJSON(expected, selected) {
			return out, tfexec.ErrRepairStale
		}
	}
	lease, err := s.Operations.Begin(ctx, envops.Claim{ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Kind: environment.OpDriftRepair, Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version}, Detail: map[string]any{"repairPreparation": true}, Deadline: 2 * time.Hour})
	if err != nil {
		return out, err
	}
	now := time.Now().UTC()
	review := domain.RepairReview{ID: ids.New(), OrganizationID: org, EnvironmentID: env.ID, ScanID: cmd.ScanID, ActorID: cmd.ActorID, RequestID: cmd.RequestID, RequestDigest: digest, EnvironmentVersion: env.Version, Status: domain.Preparing, Instances: members, PreviousReviewID: cmd.PreviousReviewID, RootIDs: roots, Consumers: consumers, Dependencies: dependencies, Blockers: []string{}, ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now, OperationID: lease.Operation.ID, Version: 1}
	token, hash, err := engine.SealReviewToken(lease.Ctx, org, review.ID)
	if err != nil {
		_ = lease.End(environment.OpFailed, "repair token unavailable")
		return out, err
	}
	row := terraformstate.PrivateRepairReview{Review: review, Token: token, TokenHash: hash}
	_ = lease.Stage("DRIFT_REPAIR_PREVIEW", map[string]any{"repairReviewId": review.ID, "repairPreparation": true})
	row.Review.Status = domain.Ready

	// Recheck the entire scope under the owner claim before any provider planning.
	if err := s.verifyRepairPins(lease.Ctx, org, row.Review.Instances); err != nil {
		_ = lease.End(environment.OpFailed, "repair scope changed")
		return out, err
	}
	changes := map[string]bool{}
	for _, c := range row.Review.Consumers {
		if !c.Completed {
			changes[c.Descriptor] = true
		}
	}
	for n, instance := range row.Review.Instances {
		if instance.Completed {
			continue
		}
		deferred, e := s.deferredBy(lease.Ctx, instance.InstanceID, changes)
		if len(row.Review.Dependencies) > 0 {
			deferred = repairDependsOnChange(instance.Descriptor, row.Review.Dependencies, changes, map[string]bool{})
		}
		if e != nil {
			row.Review.Status = domain.RepairFailed
			row.Review.Blockers = append(row.Review.Blockers, "PREPARATION_UNAVAILABLE")
			break
		}
		row.Review.Instances[n].Deferred = deferred
		if deferred {
			changes[instance.InstanceID] = true
			changes[instance.Descriptor] = true
			continue
		}
		var prepared tfexec.RepairPreparation
		if memberEngine, ok := engine.(interface {
			PrepareRepairMember(context.Context, string, string) (tfexec.RepairPreparation, error)
		}); ok {
			prepared, err = memberEngine.PrepareRepairMember(lease.Ctx, org, instance.InstanceID)
		} else {
			prepared, err = engine.PrepareInstanceRepair(lease.Ctx, org, instance.InstanceID)
		}
		if err != nil {
			row.Review.Status = domain.RepairFailed
			row.Review.Blockers = append(row.Review.Blockers, "PREPARATION_UNAVAILABLE")
			break
		}
		if prepared.BaselineID != instance.BaselineID || prepared.InstanceVersion != instance.InstanceVersion {
			row.Review.Status = domain.Blocked
			row.Review.Blockers = append(row.Review.Blockers, "BASELINE_CHANGED")
			break
		}
		row.Review.Instances[n].Report = prepared.Report
		row.Review.Instances[n].ArtifactID = prepared.ArtifactID
		row.Review.Instances[n].Blockers = prepared.Blockers
		changes[instance.InstanceID] = prepared.Report.ChangesOutputs()
		changes[instance.Descriptor] = prepared.Report.ChangesOutputs()
		if len(prepared.Blockers) > 0 || prepared.ArtifactID == "" {
			row.Review.Status = domain.Blocked
		}
	}
	// The ten-minute user review window starts after planning completes.
	// Preparation itself remains recorded by the fenced Environment operation.
	row.Review.CreatedAt = time.Now().UTC()
	row.Review.ExpiresAt = row.Review.CreatedAt.Add(10 * time.Minute)
	initial := row
	initial.Review = row.Review
	initial.Review.Status = domain.Preparing
	if err := repo.CreateRepairReview(lease.Ctx, initial); err != nil {
		_ = lease.End(environment.OpFailed, "repair preparation not admitted")
		return out, err
	}
	persist, cancel := lease.Detached()
	defer cancel()
	if err := repo.UpdateRepairReview(persist, row, review.Version); err != nil {
		_ = lease.Suspend("repair review persistence unavailable")
		return out, err
	}
	if persistence.Held(lease.Ctx) {
		_ = lease.Suspend("repair preparation process requires recovery")
		return out, tfexec.ErrNeedsAttention
	}
	status := environment.OpSucceeded
	if row.Review.Status == domain.RepairFailed {
		status = environment.OpFailed
	}
	if err := lease.End(status, ""); err != nil {
		return out, err
	}
	stored, err := repo.GetRepairReview(ctx, org, env.ID, review.ID)
	if err != nil {
		return out, err
	}
	return s.repairView(ctx, engine, stored, cmd.ActorID)
}
func (s *Service) repairView(ctx context.Context, engine RepairEngine, row terraformstate.PrivateRepairReview, actor string) (RepairView, error) {
	out := RepairView{Review: row.Review}
	if row.Review.ActorID != actor {
		return out, persistence.ErrNotFound
	}
	if out.Review.Status == domain.Ready {
		if !time.Now().UTC().Before(out.Review.ExpiresAt) {
			out.Review.Status = domain.Expired
			return out, nil
		}
		token, err := engine.OpenReviewToken(ctx, row.Review.OrganizationID, row.Review.ID, row.Token)
		if err != nil {
			return out, err
		}
		out.ReviewToken = token
	}
	return out, nil
}
func (s *Service) GetRepair(ctx context.Context, orgKey, appKey, envKey, actor, id string) (RepairView, error) {
	engine, repo, err := s.repairPorts()
	if err != nil {
		return RepairView{}, err
	}
	org, env, err := s.scope(ctx, orgKey, appKey, envKey)
	if err != nil {
		return RepairView{}, err
	}
	row, err := repo.GetRepairReview(ctx, org, env.ID, id)
	if err != nil {
		return RepairView{}, err
	}
	return s.repairView(ctx, engine, row, actor)
}

type ConfirmRepairCommand struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, ActorID, ReviewID, ReviewToken, RequestID string
	ConfirmDestructive                                                                         bool
}

func (s *Service) ConfirmRepair(ctx context.Context, cmd ConfirmRepairCommand) (string, error) {
	engine, repo, err := s.repairPorts()
	if err != nil {
		return "", err
	}
	if !ids.ValidUUID(cmd.ReviewID) || !ids.ValidUUID(cmd.ActorID) || !ids.ValidUUID(cmd.RequestID) || len(cmd.ReviewToken) != 64 {
		return "", domain.ErrScan
	}
	org, env, err := s.scope(ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return "", err
	}
	row, err := repo.GetRepairReview(ctx, org, env.ID, cmd.ReviewID)
	if err != nil {
		return "", err
	}
	if row.Review.ActorID != cmd.ActorID {
		return "", persistence.ErrNotFound
	}
	hash := sha256.Sum256([]byte(cmd.ReviewToken))
	tokenHash := hex.EncodeToString(hash[:])
	if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(row.TokenHash)) != 1 {
		return "", persistence.ErrVersionConflict
	}
	if row.Review.Status == domain.Consumed {
		if row.Review.ConfirmationRequestID == cmd.RequestID {
			return row.Review.ConsumedOperationID, nil
		}
		return "", persistence.ErrVersionConflict
	}
	if row.Review.Status != domain.Ready || !time.Now().UTC().Before(row.Review.ExpiresAt) || row.Review.EnvironmentVersion != env.Version {
		return "", tfexec.ErrRepairStale
	}
	if !cmd.ConfirmDestructive {
		for _, i := range row.Review.Instances {
			if !i.Completed && !i.Deferred && i.Report.Destructive() {
				return "", tfexec.ErrRepairBlocked
			}
		}
	}
	lease, err := s.Operations.Begin(ctx, envops.Claim{ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Kind: environment.OpDriftRepair, Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version}, Detail: map[string]any{"repairReviewId": row.Review.ID}, Deadline: 2 * time.Hour})
	if err != nil {
		return "", err
	}
	row, err = repo.ConsumeRepairReview(lease.Ctx, org, env.ID, row.Review.ID, cmd.ActorID, cmd.RequestID, tokenHash, row.Review.Version)
	if err != nil {
		_ = lease.End(environment.OpFailed, "repair admission changed")
		return "", err
	}

	opID := lease.Operation.ID
	if err := s.verifyRepairPins(lease.Ctx, org, row.Review.Instances); err != nil {
		_ = lease.End(environment.OpFailed, "repair scope changed")
		return opID, err
	}
	if s.Consumers != nil {
		descriptors := []string{}
		for _, m := range row.Review.Instances {
			descriptors = append(descriptors, m.Descriptor)
		}
		current, e := s.Consumers.PreviewRepairConsumers(lease.Ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey, descriptors)
		if e != nil || len(current) != len(row.Review.Consumers) || (len(current) > 0 && !equalRepairConsumers(current, row.Review.Consumers)) {
			_ = lease.End(environment.OpFailed, "repair consumers changed")
			return opID, tfexec.ErrRepairStale
		}
	}
	checkpoint := append([]domain.RepairInstance{}, row.Review.Instances...)
	applied := false
	remaining := false
	for n, instance := range row.Review.Instances {
		if instance.Completed {
			continue
		}
		if instance.Deferred {
			remaining = true
			continue
		}
		if instance.ArtifactID == "" {
			_ = lease.End(environment.OpFailed, "repair artifact unavailable")
			return opID, tfexec.ErrRepairBlocked
		}
		detail := map[string]any{"repairReviewId": row.Review.ID, "instanceId": instance.InstanceID, "checkpoint": checkpoint}
		if err := lease.Stage("DRIFT_REPAIR", detail); err != nil {
			_ = lease.Suspend("repair ownership unavailable")
			return opID, err
		}
		if err := engine.RepairInstance(lease.Ctx, org, instance.InstanceID, instance.ArtifactID); err != nil {
			if persistence.Held(lease.Ctx) || applied {
				_ = lease.Suspend("repair partially completed or outcome requires recovery")
			} else {
				_ = lease.End(environment.OpFailed, "reviewed repair did not complete")
			}
			return opID, err
		}
		applied = true
		inst, e := s.Instances.GetInstance(lease.Ctx, instance.InstanceID)
		b, f := s.Instances.LatestExecutionBaseline(lease.Ctx, org, instance.InstanceID)
		if e != nil || f != nil {
			_ = lease.Suspend("repair checkpoint unavailable")
			return opID, tfexec.ErrNeedsAttention
		}
		checkpoint[n].Completed = true
		checkpoint[n].Deferred = false
		checkpoint[n].ArtifactID = ""
		checkpoint[n].Report = domain.Report{}
		checkpoint[n].InstanceVersion = inst.Version
		checkpoint[n].BaselineID = b.ID
		if err := lease.Stage("DRIFT_REPAIR_CHECKPOINT", map[string]any{"repairReviewId": row.Review.ID, "checkpoint": checkpoint}); err != nil {
			_ = lease.Suspend("repair checkpoint unavailable")
			return opID, err
		}
	}

	consumerCheckpoint := append([]domain.RepairConsumer{}, row.Review.Consumers...)
	if s.Consumers != nil && len(consumerCheckpoint) > 0 {
		completed := map[string]bool{}
		for _, m := range checkpoint {
			if m.Completed {
				completed[m.Descriptor] = true
			}
		}
		for _, c := range consumerCheckpoint {
			if c.Completed {
				completed[c.Descriptor] = true
			}
		}
		ready := []domain.RepairConsumer{}
		// Fixed point admits non-Terraform chains only after all reviewed providers
		// have completed. A Terraform node behind such a chain gets a fresh preview.
		for changed := true; changed; {
			changed = false
			for _, c := range consumerCheckpoint {
				if c.Completed || completed[c.Descriptor] {
					continue
				}
				available := true
				for _, provider := range row.Review.Dependencies[c.Descriptor] {
					if !completed[provider] {
						available = false
						break
					}
				}
				if available {
					ready = append(ready, c)
					completed[c.Descriptor] = true
					changed = true
				}
			}
		}
		if len(ready) > 0 {
			if err := lease.Stage("DRIFT_REPAIR_CONSUMERS", map[string]any{"repairReviewId": row.Review.ID, "checkpoint": checkpoint, "consumerCheckpoint": consumerCheckpoint}); err != nil {
				_ = lease.Suspend("consumer progress unavailable")
				return opID, err
			}
			if err := s.Consumers.RefreshRepairConsumers(lease.Ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey, cmd.ActorID, ready); err != nil {
				_ = lease.Suspend("repair consumer refresh requires recovery")
				return opID, tfexec.ErrNeedsAttention
			}
			descriptors := []string{}
			for _, m := range checkpoint {
				descriptors = append(descriptors, m.Descriptor)
			}
			fresh, err := s.Consumers.PreviewRepairConsumers(lease.Ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey, descriptors)
			if err != nil {
				_ = lease.Suspend("consumer checkpoint unavailable")
				return opID, tfexec.ErrNeedsAttention
			}
			for n, c := range consumerCheckpoint {
				for _, now := range fresh {
					if c.Descriptor == now.Descriptor {
						now.Completed = completed[c.Descriptor]
						consumerCheckpoint[n] = now
					}
				}
			}
		}
		for _, c := range consumerCheckpoint {
			if !c.Completed {
				remaining = true
			}
		}
	}
	if remaining {
		if err := lease.Stage("DRIFT_REPAIR_REVIEW_REQUIRED", map[string]any{"repairReviewId": row.Review.ID, "checkpoint": checkpoint, "consumerCheckpoint": consumerCheckpoint}); err != nil {
			_ = lease.Suspend("repair checkpoint unavailable")
			return opID, err
		}
		if err := lease.End(environment.OpFailed, "remaining consumers require a new repair preview"); err != nil {
			return opID, err
		}
		return opID, nil
	}
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		_ = lease.Suspend("repair completion unavailable")
		return opID, err
	}
	// Queue a new read-only observation; it cannot rewrite the old scan or Set.
	current, err := s.Store.GetEnvironment(ctx, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err == nil {
		_, _ = s.Queue(ctx, QueueCommand{OrganizationKey: cmd.OrganizationKey, ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, RequestID: ids.New(), ExpectedEnvironmentVersion: current.Version})
	}
	return opID, nil
}

func equalRepairJSON(a, b any) bool {
	left, e := json.Marshal(a)
	right, f := json.Marshal(b)
	return e == nil && f == nil && string(left) == string(right)
}

func equalRepairConsumers(current, expected []domain.RepairConsumer) bool {
	if len(current) != len(expected) {
		return false
	}
	cloned := append([]domain.RepairConsumer{}, expected...)
	for n := range cloned {
		cloned[n].Completed = false
	}
	return len(current) == 0 || equalRepairJSON(current, cloned)
}
