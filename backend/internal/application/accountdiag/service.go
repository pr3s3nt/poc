// Package accountdiag implements Infrastructure Account diagnostics (ADR-022,
// UC-04 DIAG-01..14): a durable, Organization/account scoped report aggregate,
// an asynchronous worker with a bounded lease, and provider probes that use
// the account's saved credential. A diagnostic never changes the account's
// Connection record.
package accountdiag

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/platform/ids"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/persistence"
)

// Repository is the report store port.
type Repository = probes.Repository

// ListQuery and Page are the history paging types.
type (
	ListQuery = probes.ListQuery
	Page      = probes.Page
)

// Accounts reads the saved account (the Connection record).
type Accounts interface {
	GetAccount(ctx context.Context, org, key string) (application.Connection, error)
}

// Config bounds a run. Zero values take the documented defaults.
type Config struct {
	// Instance names this process in owner tokens (no secrets).
	Instance string
	// LeaseTTL is how long an owner holds a run without a heartbeat.
	LeaseTTL time.Duration
	// Heartbeat is the lease renewal period (default LeaseTTL/3).
	Heartbeat time.Duration
	// CheckTimeout bounds one conditions probe; ExecutionTimeout one execution probe.
	CheckTimeout, ExecutionTimeout time.Duration
	// RunTimeout bounds the whole run, cleanup excluded.
	RunTimeout time.Duration
	// CleanupTimeout bounds the independent cleanup context.
	CleanupTimeout time.Duration
	// SweepEvery is the interval of lease expiry and cleanup recovery.
	SweepEvery time.Duration
	// SecretDelivery is the platform's workload secret delivery mode.
	SecretDelivery string
	// BaseDomain forms the public host of an Application (subdomain.base).
	BaseDomain string
	Now        func() time.Time
}

func (c Config) withDefaults() Config {
	if c.Instance == "" {
		c.Instance = "console"
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 45 * time.Second
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = c.LeaseTTL / 3
	}
	if c.CheckTimeout <= 0 {
		c.CheckTimeout = 45 * time.Second
	}
	if c.ExecutionTimeout <= 0 {
		c.ExecutionTimeout = 3 * time.Minute
	}
	if c.RunTimeout <= 0 {
		c.RunTimeout = 15 * time.Minute
	}
	if c.CleanupTimeout <= 0 {
		c.CleanupTimeout = 3 * time.Minute
	}
	if c.SweepEvery <= 0 {
		c.SweepEvery = 15 * time.Second
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Service is the diagnostics application service.
type Service struct {
	repo      Repository
	accounts  Accounts
	context   ContextStore
	planner   Planner
	stores    probes.StoreInspector
	routes    probes.RouteInspector
	providers map[string]probes.Provider
	cfg       Config

	mu     sync.Mutex
	runs   map[string]*runHandle
	wg     sync.WaitGroup
	closed bool
}

// New returns the service. providers is keyed by account type, "INTERNAL" or "AWS".
func New(repo Repository, accounts Accounts, store ContextStore, providers map[string]probes.Provider, cfg Config) *Service {
	return &Service{repo: repo, accounts: accounts, context: store, providers: providers, cfg: cfg.withDefaults(), runs: map[string]*runHandle{}}
}

// SetPlanner wires the desired-state planning pipeline used to resolve the
// requirements of a selected Application and Environment.
func (s *Service) SetPlanner(p Planner) { s.planner = p }

// SetInspectors wires the real secret-store and public-route evidence readers.
func (s *Service) SetInspectors(stores probes.StoreInspector, routes probes.RouteInspector) {
	s.stores, s.routes = stores, routes
}

// StartCommand is the validated input of a run.
type StartCommand struct {
	Organization, UserID, AccountKey string
	Request                          domain.Request
}

// ErrUnavailable means no provider exists for the account type.
var ErrUnavailable = errors.New("accountdiag: diagnostics are not available for this account")

func accountType(c application.Connection) string {
	if c.Kind == application.ConnectionKubernetes {
		return "INTERNAL"
	}
	return "AWS"
}

// scopeIdentity copies only safe target facts of the saved account.
func scopeIdentity(c application.Connection) map[string]string {
	identity := map[string]string{}
	put := func(key, value string) {
		if value != "" && domain.ValidIdentity(key, value) {
			identity[key] = value
		}
	}
	if c.Kind == application.ConnectionKubernetes {
		put("cluster", c.ConfigString("cluster"))
		if u, err := url.Parse(c.ConfigString("endpoint")); err == nil {
			put("endpoint", u.Host)
		}
	} else {
		put("accountId", c.ConfigString("accountId"))
		put("authentication", string(c.AuthenticationType))
		put("roleArn", c.ConfigString("roleArn"))
	}
	if len(identity) == 0 {
		return nil
	}
	return identity
}

// Start validates the request, persists a QUEUED report with its owner lease
// and launches the worker. A conflicting active run is domain.ErrBusy; a stale
// account version is persistence.ErrVersionConflict. It never repeats on an
// uncertain outcome: the caller reconciles with Get.
func (s *Service) Start(ctx context.Context, cmd StartCommand) (domain.Report, error) {
	acct, err := s.accounts.GetAccount(ctx, cmd.Organization, cmd.AccountKey)
	if err != nil {
		return domain.Report{}, err
	}
	typ := accountType(acct)
	provider := s.providers[typ]
	if provider == nil {
		return domain.Report{}, ErrUnavailable
	}
	req, err := cmd.Request.Normalize(typ)
	if err != nil {
		return domain.Report{}, err
	}
	if req.Version != acct.Version {
		return domain.Report{}, fmt.Errorf("%w: account version", persistence.ErrVersionConflict)
	}
	ref, need, err := s.resolveContext(ctx, cmd.Organization, req)
	if err != nil {
		return domain.Report{}, err
	}
	scope := domain.Scope{
		Purpose: req.Purpose, Mode: req.Mode, Region: req.Region, UseCases: req.UseCases, Context: ref,
		AccountKey: acct.Key, AccountName: accountName(acct), AccountType: typ, AccountVersion: acct.Version, Identity: scopeIdentity(acct),
	}
	plan := s.plan(provider, scope, need)
	now := s.cfg.Now().UTC()
	lease := now.Add(s.cfg.LeaseTTL)
	report := domain.Report{
		ID: ids.New(), OrganizationKey: cmd.Organization, AccountKey: acct.Key, CreatedBy: cmd.UserID, AccountVersion: acct.Version,
		Purpose: req.Purpose, Mode: req.Mode, Scope: scope, Status: domain.StatusQueued, Revision: 1,
		Owner: s.cfg.Instance + "/" + ids.New(), LeaseExpiresAt: &lease, TotalChecks: len(plan),
		CleanupStatus: domain.CleanupNotRequired, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.repo.CreateReport(ctx, report)
	if err != nil {
		return domain.Report{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	handle := &runHandle{root: cancel}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return created, nil
	}
	s.runs[created.ID] = handle
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.runs, created.ID)
			s.mu.Unlock()
			cancel()
		}()
		s.execute(runCtx, handle, created, acct, provider, plan, need)
	}()
	return created, nil
}

func accountName(c application.Connection) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

// Get returns one report of the account.
func (s *Service) Get(ctx context.Context, org, accountKey, id string) (domain.Report, error) {
	return s.repo.GetReport(ctx, org, accountKey, id)
}

// List returns one history page, newest first.
func (s *Service) List(ctx context.Context, org, accountKey string, q ListQuery) (Page, error) {
	return s.repo.ListReports(ctx, org, accountKey, q)
}

// Latest returns the newest report of every account of the Organization.
func (s *Service) Latest(ctx context.Context, org string) (map[string]domain.Report, error) {
	return s.repo.LatestReports(ctx, org)
}

// Cancel records a cancellation request and, when this process runs the
// report, interrupts its probes at once. It is idempotent and never replays a
// start.
func (s *Service) Cancel(ctx context.Context, org, accountKey, id string) (domain.Report, error) {
	report, err := s.repo.RequestCancel(ctx, org, accountKey, id, s.cfg.Now().UTC())
	if err != nil {
		return domain.Report{}, err
	}
	if report.Status == domain.StatusCancelling {
		s.mu.Lock()
		handle := s.runs[id]
		s.mu.Unlock()
		if handle != nil {
			handle.stopProbes()
		}
	}
	return report, nil
}

// Close stops accepting runs, cancels running probes and waits for the
// workers to leave. Interrupted reports are reconciled by the lease sweep.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	for _, handle := range s.runs {
		handle.root()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// Wait blocks until every worker of this process has finished (tests).
func (s *Service) Wait() { s.wg.Wait() }

// runHandle lets Cancel stop a run's probes (the report then ends CANCELLED)
// apart from Close, which stops the whole process's runs (the report is then
// left to the lease sweep as INTERRUPTED).
type runHandle struct {
	root   context.CancelFunc
	mu     sync.Mutex
	probes context.CancelFunc
}

func (h *runHandle) setProbes(c context.CancelFunc) {
	h.mu.Lock()
	h.probes = c
	h.mu.Unlock()
}

func (h *runHandle) stopProbes() {
	h.mu.Lock()
	c := h.probes
	h.mu.Unlock()
	if c != nil {
		c()
	}
}
