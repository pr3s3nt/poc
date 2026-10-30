// Package drift coordinates independent Terraform scans. Stored Console reads
// never invoke providers, Terraform or plan rendering.
package drift

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/reconcilebudget"
	"orchestrator/internal/application/tfexec"
	domain "orchestrator/internal/domain/drift"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

type Store interface {
	persistence.Store
	persistence.DriftScanRepository
}
type Instances interface {
	ListInstances(context.Context, string) ([]tfresource.Instance, error)
	GetInstance(context.Context, string) (tfresource.Instance, error)
	LatestExecutionBaseline(context.Context, string, string) (terraformstate.ExecutionBaseline, error)
}
type Inspector interface {
	InspectInstance(context.Context, string, string) (domain.Report, error)
}
type Service struct {
	Store      Store
	Instances  Instances
	Inspector  Inspector
	Operations *envops.Manager
	Interval   time.Duration
	mu         sync.Mutex
	inflight   map[string]bool
}
type QueueCommand struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, RequestID string
	ExpectedEnvironmentVersion                                 int64
}

func (s *Service) scope(ctx context.Context, orgKey, appKey, envKey string) (string, environment.Environment, error) {
	app, err := s.Store.GetApplication(ctx, appKey)
	if err != nil || app.OrganizationKey != orgKey || app.Lifecycle() != "ACTIVE" {
		return "", environment.Environment{}, persistence.ErrNotFound
	}
	env, err := s.Store.GetEnvironment(ctx, appKey, envKey)
	if err != nil {
		return "", env, err
	}
	org, err := s.Store.GetOrganization(ctx, orgKey)
	return org.ID, env, err
}
func (s *Service) Queue(ctx context.Context, cmd QueueCommand) (domain.Scan, error) {
	if s == nil || s.Store == nil || s.Instances == nil || s.Inspector == nil || s.Operations == nil {
		return domain.Scan{}, tfexec.ErrBaselineUnavailable
	}
	if !ids.ValidUUID(cmd.RequestID) || cmd.ExpectedEnvironmentVersion < 1 {
		return domain.Scan{}, domain.ErrScan
	}
	org, env, err := s.scope(ctx, cmd.OrganizationKey, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return domain.Scan{}, err
	}
	instances, err := s.Instances.ListInstances(ctx, env.ID)
	if err != nil {
		return domain.Scan{}, err
	}
	selected := []domain.InstancePin{}
	for _, instance := range instances {
		if instance.Identity.OrganizationID != org || instance.Identity.TargetGeneration != env.TargetGeneration || instance.Status == tfresource.InstanceDestroyed {
			continue
		}
		pin := domain.InstancePin{InstanceID: instance.ID, InstanceVersion: instance.Version}
		baseline, err := s.Instances.LatestExecutionBaseline(ctx, org, instance.ID)
		if err == nil {
			pin.BaselineID = baseline.ID
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return domain.Scan{}, err
		}
		selected = append(selected, pin)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].InstanceID < selected[j].InstanceID })
	// Idempotency binds the original user command. Server-computed scope is
	// checked at execution, rather than changing an existing request on retry.
	raw, _ := json.Marshal([]any{org, env.ID, cmd.ExpectedEnvironmentVersion})
	sum := sha256.Sum256(raw)
	return s.Store.QueueDriftScan(ctx, domain.Scan{ID: ids.New(), OrganizationID: org, EnvironmentID: env.ID, ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, RequestID: cmd.RequestID, RequestDigest: hex.EncodeToString(sum[:]), EnvironmentVersion: cmd.ExpectedEnvironmentVersion, Status: domain.Queued, Result: domain.Unknown, Scope: selected, Findings: []domain.InstanceResult{}, CreatedAt: time.Now().UTC()})
}
func (s *Service) List(ctx context.Context, orgKey, appKey, envKey, cursor string, limit int) ([]domain.Scan, error) {
	org, env, err := s.scope(ctx, orgKey, appKey, envKey)
	if err != nil {
		return nil, err
	}
	return s.Store.ListDriftScans(ctx, org, env.ID, cursor, limit)
}
func (s *Service) Get(ctx context.Context, orgKey, appKey, envKey, id string) (domain.Scan, error) {
	org, env, err := s.scope(ctx, orgKey, appKey, envKey)
	if err != nil {
		return domain.Scan{}, err
	}
	return s.Store.GetDriftScan(ctx, org, env.ID, id)
}

// ProcessQueued may race another backend. Claim/CAS determines the sole worker;
// no provider call precedes the Environment claim and persisted scan owner.
func (s *Service) ProcessQueued(ctx context.Context) {
	rows, err := s.Store.ListQueuedDriftScans(ctx, 100)
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, row := range rows {
		s.mu.Lock()
		if s.inflight == nil {
			s.inflight = map[string]bool{}
		}
		busy := s.inflight[row.EnvironmentID]
		if !busy {
			s.inflight[row.EnvironmentID] = true
		}
		s.mu.Unlock()
		if busy {
			continue
		}
		release, err := reconcilebudget.Terraform(ctx)
		if err != nil {
			s.mu.Lock()
			delete(s.inflight, row.EnvironmentID)
			s.mu.Unlock()
			break
		}
		wg.Add(1)
		go func(row domain.Scan) {
			defer wg.Done()
			defer release()
			defer func() { s.mu.Lock(); delete(s.inflight, row.EnvironmentID); s.mu.Unlock() }()
			s.run(ctx, row)
		}(row)
	}
	wg.Wait()
}
func (s *Service) run(ctx context.Context, row domain.Scan) {
	app, err := s.Store.GetApplication(ctx, row.ApplicationKey)
	if err != nil || app.Lifecycle() != "ACTIVE" {
		_ = s.Store.SkipQueuedDriftScan(ctx, row.OrganizationID, row.ID, row.Version, "APPLICATION_UNAVAILABLE")
		return
	}
	env, err := s.Store.GetEnvironment(ctx, row.ApplicationKey, row.EnvironmentKey)
	if err != nil || env.ID != row.EnvironmentID || env.Version != row.EnvironmentVersion {
		_ = s.Store.SkipQueuedDriftScan(ctx, row.OrganizationID, row.ID, row.Version, "BASELINE_CHANGED")
		return
	}
	if env.Busy() {
		_ = s.Store.SkipQueuedDriftScan(ctx, row.OrganizationID, row.ID, row.Version, "ENVIRONMENT_BUSY")
		return
	}
	lease, err := s.Operations.Begin(ctx, envops.Claim{ApplicationKey: row.ApplicationKey, EnvironmentKey: row.EnvironmentKey, Kind: environment.OpDriftScan, Pins: environment.OperationPins{CheckEnvVersion: true, EnvVersion: row.EnvironmentVersion}, Detail: map[string]any{"driftScanId": row.ID}, Deadline: 10 * time.Minute})
	if err != nil {
		_ = s.Store.SkipQueuedDriftScan(ctx, row.OrganizationID, row.ID, row.Version, "ENVIRONMENT_BUSY")
		return
	}
	running, err := s.Store.StartDriftScan(lease.Ctx, row.OrganizationID, row.ID, row.Version)
	if err != nil {
		_ = lease.End(environment.OpFailed, "scan admission changed")
		return
	}
	row = running
	work, cancelWork := context.WithCancel(lease.Ctx)
	stopParent := context.AfterFunc(ctx, cancelWork)
	defer func() { stopParent(); cancelWork() }()
	if err := lease.Stage("DRIFT_SCAN", map[string]any{"driftScanId": row.ID}); err != nil {
		_ = lease.Suspend("scan ownership unavailable")
		return
	}
	row.Result = domain.NotApplicable
	row.Status = domain.Succeeded
	for _, pin := range row.Scope {
		finding := domain.InstanceResult{InstanceID: pin.InstanceID, Result: domain.Unknown}
		instance, err := s.Instances.GetInstance(lease.Ctx, pin.InstanceID)
		if err != nil || instance.Identity.EnvironmentID != row.EnvironmentID || instance.Identity.OrganizationID != row.OrganizationID || instance.Version != pin.InstanceVersion {
			finding.ReasonCode = "BASELINE_CHANGED"
			row.Status = domain.Failed
		} else {
			finding.Descriptor = instance.Identity.Descriptor
			if instance.Status != tfresource.InstanceReady && instance.Status != tfresource.InstanceUnreferenced {
				finding.Result = domain.NotApplicable
				finding.ReasonCode = "INSTANCE_NOT_ACKNOWLEDGED"
			} else if pin.BaselineID == "" {
				finding.Result = domain.BaselineUnavailable
				finding.ReasonCode = "BASELINE_UNAVAILABLE"
			} else {
				baseline, err := s.Instances.LatestExecutionBaseline(lease.Ctx, row.OrganizationID, pin.InstanceID)
				if err != nil || baseline.ID != pin.BaselineID {
					finding.ReasonCode = "BASELINE_CHANGED"
					row.Status = domain.Failed
				} else {
					report, err := s.Inspector.InspectInstance(work, row.OrganizationID, pin.InstanceID)
					if err == nil {
						report.PrivateChangeDigest = ""
						finding.Report = report
						finding.Result = report.Result()
					} else if errors.Is(err, tfexec.ErrBaselineUnavailable) {
						finding.Result = domain.BaselineUnavailable
						finding.ReasonCode = "BASELINE_UNAVAILABLE"
					} else {
						finding.ReasonCode = "PROVIDER_READ_FAILED"
						row.Status = domain.Failed
					}
				}
			}
		}
		row.Findings = append(row.Findings, finding)
		row.Result = mergeResult(row.Result, finding.Result)
		if persistence.Held(lease.Ctx) {
			row.Status = domain.Interrupted
			row.Result = domain.Unknown
			row.ReasonCode = "PROCESS_OUTCOME_UNKNOWN"
			break
		}
	}
	now := time.Now().UTC()
	row.FinishedAt = &now
	persist, cancel := lease.Detached()
	defer cancel()
	if err := s.Store.FinishDriftScan(persist, row, row.Version); err != nil {
		_ = lease.Suspend("scan result could not be persisted")
		return
	}
	if row.Status == domain.Interrupted {
		_ = lease.Suspend("scan process outcome requires recovery")
		return
	}
	status := environment.OpSucceeded
	if row.Status == domain.Failed {
		status = environment.OpFailed
	}
	if err := lease.End(status, ""); err != nil {
		_ = lease.Suspend("scan lock release could not be confirmed")
	}
}
func mergeResult(a, b domain.Result) domain.Result {
	rank := map[domain.Result]int{domain.NotApplicable: 0, domain.InSync: 1, domain.Drifted: 2, domain.BaselineUnavailable: 3, domain.Unknown: 4}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func (s *Service) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			s.Schedule(ctx, time.Now().UTC())
			s.ProcessQueued(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}

// Schedule is enabled only by platform bootstrap's opt-in. Stable jitter and
// request IDs deduplicate concurrent backends; skipped work never refreshes a
// previous report's observation time.
func (s *Service) Schedule(ctx context.Context, now time.Time) {
	interval := s.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	apps, err := s.Store.ListApplications(ctx)
	if err != nil {
		return
	}
	for _, app := range apps {
		if app.Lifecycle() != "ACTIVE" {
			continue
		}
		envs, err := s.Store.ListEnvironments(ctx, app.Key)
		if err != nil {
			continue
		}
		org, err := s.Store.GetOrganization(ctx, app.OrganizationKey)
		if err != nil {
			continue
		}
		for _, env := range envs {
			if env.Busy() {
				continue
			}
			instances, err := s.Instances.ListInstances(ctx, env.ID)
			if err != nil || len(instances) == 0 {
				continue
			}
			rows, err := s.Store.ListDriftScans(ctx, org.ID, env.ID, "", 1)
			if err != nil {
				continue
			}
			sum := sha256.Sum256([]byte(env.ID))
			jitter := time.Duration(binary.BigEndian.Uint64(sum[:8]) % uint64(interval/10+1))
			if len(rows) > 0 && now.Sub(rows[0].CreatedAt) < interval+jitter {
				continue
			}
			// New environments wait one full cadence after creation of their
			// first managed resource, avoiding provider reads during setup.
			latest := time.Time{}
			for _, instance := range instances {
				if instance.CreatedAt.After(latest) {
					latest = instance.CreatedAt
				}
			}
			if now.Sub(latest) < interval+jitter {
				continue
			}
			bucket := now.Add(-jitter).UnixNano() / int64(interval)
			request := sha256.Sum256([]byte(fmt.Sprint("drift-schedule/", env.ID, "/", bucket)))
			request[6] = (request[6] & 15) | 80
			request[8] = (request[8] & 63) | 128
			hexID := hex.EncodeToString(request[:16])
			id := hexID[:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:]
			_, _ = s.Queue(ctx, QueueCommand{OrganizationKey: app.OrganizationKey, ApplicationKey: app.Key, EnvironmentKey: env.Key, RequestID: id, ExpectedEnvironmentVersion: env.Version})
		}
	}
}
