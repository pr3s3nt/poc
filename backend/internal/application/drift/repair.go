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
	if !ids.ValidUUID(cmd.ActorID) || !ids.ValidUUID(cmd.RequestID) || !ids.ValidUUID(cmd.ScanID) || len(cmd.InstanceIDs) == 0 || len(cmd.InstanceIDs) > 20 || cmd.ExpectedEnvironmentVersion < 1 {
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
	raw, _ := json.Marshal([]any{org, env.ID, cmd.ActorID, cmd.ScanID, selected, cmd.ExpectedEnvironmentVersion})
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
		instance, err := s.Instances.GetInstance(ctx, id)
		if err != nil || instance.Version != pin.InstanceVersion {
			return out, tfexec.ErrRepairStale
		}
	}
	lease, err := s.Operations.Begin(ctx, envops.Claim{ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Kind: environment.OpDriftRepair, Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version}, Detail: map[string]any{"repairPreparation": true}, Deadline: 10 * time.Minute})
	if err != nil {
		return out, err
	}
	now := time.Now().UTC()
	review := domain.RepairReview{ID: ids.New(), OrganizationID: org, EnvironmentID: env.ID, ScanID: cmd.ScanID, ActorID: cmd.ActorID, RequestID: cmd.RequestID, RequestDigest: digest, EnvironmentVersion: env.Version, Status: domain.Preparing, Instances: []domain.RepairInstance{}, Blockers: []string{}, ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now, OperationID: lease.Operation.ID, Version: 1}
	for _, id := range selected {
		pin := covered[id]
		review.Instances = append(review.Instances, domain.RepairInstance{InstanceID: id, InstanceVersion: pin.InstanceVersion, BaselineID: pin.BaselineID, Blockers: []string{}})
	}
	token, hash, err := engine.SealReviewToken(lease.Ctx, org, review.ID)
	if err != nil {
		_ = lease.End(environment.OpFailed, "repair token unavailable")
		return out, err
	}
	row := terraformstate.PrivateRepairReview{Review: review, Token: token, TokenHash: hash}
	if err := repo.CreateRepairReview(lease.Ctx, row); err != nil {
		_ = lease.End(environment.OpFailed, "repair preparation not admitted")
		return out, err
	}
	_ = lease.Stage("DRIFT_REPAIR_PREVIEW", map[string]any{"repairReviewId": review.ID, "repairPreparation": true})
	row.Review.Status = domain.Ready
	for n, instance := range row.Review.Instances {
		prepared, err := engine.PrepareInstanceRepair(lease.Ctx, org, instance.InstanceID)
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
		if len(prepared.Blockers) > 0 || prepared.ArtifactID == "" {
			row.Review.Status = domain.Blocked
		}
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
	lease, err := s.Operations.Begin(ctx, envops.Claim{ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Kind: environment.OpDriftRepair, Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: env.Version}, Detail: map[string]any{"repairReviewId": row.Review.ID}, Deadline: 20 * time.Minute})
	if err != nil {
		return "", err
	}
	row, err = repo.ConsumeRepairReview(lease.Ctx, org, env.ID, row.Review.ID, cmd.ActorID, cmd.RequestID, tokenHash, row.Review.Version)
	if err != nil {
		_ = lease.End(environment.OpFailed, "repair admission changed")
		return "", err
	}
	opID := lease.Operation.ID
	for _, instance := range row.Review.Instances {
		if instance.ArtifactID == "" {
			_ = lease.End(environment.OpFailed, "repair artifact unavailable")
			return opID, tfexec.ErrRepairBlocked
		}
		if err := lease.Stage("DRIFT_REPAIR", map[string]any{"repairReviewId": row.Review.ID, "instanceId": instance.InstanceID}); err != nil {
			_ = lease.Suspend("repair ownership unavailable")
			return opID, err
		}
		if err := engine.RepairInstance(lease.Ctx, org, instance.InstanceID, instance.ArtifactID); err != nil {
			if persistence.Held(lease.Ctx) {
				_ = lease.Suspend("repair outcome requires recovery")
			} else {
				_ = lease.End(environment.OpFailed, "reviewed repair did not complete")
			}
			return opID, err
		}
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
