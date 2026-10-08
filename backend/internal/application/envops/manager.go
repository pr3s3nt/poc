package envops

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// ErrBusy wraps persistence.ErrEnvironmentBusy with the visible operation.
type ErrBusy struct {
	Operation environment.Operation
}

func (e *ErrBusy) Error() string { return "environment is busy with another operation" }
func (e *ErrBusy) Unwrap() error { return persistence.ErrEnvironmentBusy }

// Defaults for heartbeats and stale detection. Staleness only flags an
// operation INTERRUPTED; it never releases the claim.
const (
	DefaultHeartbeat  = 5 * time.Second
	DefaultStaleAfter = 30 * time.Second
	DefaultDeadline   = 4 * time.Hour
)

// DeadlineFor bounds how long one kind of operation may hold an Environment.
func DeadlineFor(kind environment.OperationKind) time.Duration {
	switch kind {
	case environment.OpDeploy, environment.OpRemove, environment.OpRoutes:
		return 45 * time.Minute
	case environment.OpStoreCopy:
		return 15 * time.Minute
	case environment.OpTransition:
		return 3 * time.Hour
	case environment.OpCleanup, environment.OpRecovery:
		return 30 * time.Minute
	}
	return DefaultDeadline
}

// Manager claims Environments and keeps their heartbeats alive.
type Manager struct {
	store      persistence.Store
	instanceID string
	heartbeat  time.Duration
	staleAfter time.Duration
}

// NewManager returns a manager with a fresh process identity.
func NewManager(store persistence.Store) *Manager {
	return &Manager{store: store, instanceID: ids.New(), heartbeat: DefaultHeartbeat, staleAfter: DefaultStaleAfter}
}

// SetTiming overrides heartbeat and stale thresholds (tests).
func (m *Manager) SetTiming(heartbeat, staleAfter time.Duration) {
	m.heartbeat, m.staleAfter = heartbeat, staleAfter
}

// InstanceID identifies this backend process in claims.
func (m *Manager) InstanceID() string { return m.instanceID }

// StaleAfter is the heartbeat age after which a claim reads INTERRUPTED.
func (m *Manager) StaleAfter() time.Duration { return m.staleAfter }

// Sweep flags claims whose heartbeat is stale as INTERRUPTED. Claims stay held.
func (m *Manager) Sweep(ctx context.Context) (int, error) {
	return m.store.MarkInterrupted(ctx, time.Now().UTC().Add(-m.staleAfter))
}

// Claim describes what the caller wants to own and which snapshot it verified.
type Claim struct {
	ApplicationKey, EnvironmentKey string
	Kind                           environment.OperationKind
	Pins                           environment.OperationPins
	Detail                         map[string]any
	Deadline                       time.Duration
}

// Lease is a held claim. Ctx carries the owner for repository writes and is
// cancelled when the claim is lost or ended.
type Lease struct {
	Operation environment.Operation
	Ctx       context.Context
	owner     persistence.Owner
	manager   *Manager
	cancel    context.CancelFunc
	stop      chan struct{}
	done      sync.WaitGroup
	mu        sync.Mutex
	stage     string
	detail    map[string]any
	lost      bool
	ended     bool
}

// Begin claims the Environment. ErrBusy (wrapping ErrEnvironmentBusy) reports
// the visible owner; ErrVersionConflict reports a stale snapshot. No executor
// side effect may precede a successful Begin.
func (m *Manager) Begin(ctx context.Context, c Claim) (*Lease, error) {
	deadline := c.Deadline
	if deadline <= 0 {
		deadline = DeadlineFor(c.Kind)
	}
	_, _ = m.Sweep(ctx)
	op, err := m.store.ClaimEnvironment(ctx, persistence.OperationClaim{
		ID: ids.New(), ApplicationKey: c.ApplicationKey, EnvironmentKey: c.EnvironmentKey, Owner: m.instanceID,
		Kind: c.Kind, Pins: c.Pins, Detail: c.Detail, Deadline: time.Now().UTC().Add(deadline),
	})
	if errors.Is(err, persistence.ErrEnvironmentBusy) {
		active, ok, getErr := m.store.ActiveOperation(ctx, c.ApplicationKey, c.EnvironmentKey)
		if getErr == nil && ok {
			return nil, &ErrBusy{Operation: active}
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return m.start(ctx, op), nil
}

// Adopt wraps an operation already moved to RECOVERING for this process.
func (m *Manager) Adopt(ctx context.Context, op environment.Operation) *Lease {
	return m.start(ctx, op)
}

func (m *Manager) start(parent context.Context, op environment.Operation) *Lease {
	owner := persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}
	base := persistence.WithOwner(context.WithoutCancel(parent), owner)
	// The recorded deadline bounds every executor call made under the lease.
	ctx, cancel := context.WithDeadline(base, op.Deadline)
	lease := &Lease{Operation: op, Ctx: ctx, owner: owner, manager: m, cancel: cancel, stop: make(chan struct{}), detail: op.Detail}
	lease.done.Add(1)
	go lease.beat()
	return lease
}

// ioTimeout bounds every claim bookkeeping call so a storage outage cannot hang
// the owner while its authority silently lapses.
const ioTimeout = 10 * time.Second

// Detached returns a bounded context that keeps the lease owner (so fenced
// writes still prove ownership) but is not cancelled with the executor
// context. Final and cleanup records use it.
func (l *Lease) Detached() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(l.Ctx), ioTimeout)
}

func (l *Lease) markLost() {
	l.mu.Lock()
	l.lost = true
	l.mu.Unlock()
	l.cancel()
}

func (l *Lease) heartbeatNow(stage string, detail map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), ioTimeout)
	defer cancel()
	return l.manager.store.HeartbeatOperation(ctx, l.owner, stage, detail)
}

func (l *Lease) beat() {
	defer l.done.Done()
	ticker := time.NewTicker(l.manager.heartbeat)
	defer ticker.Stop()
	lastOK := time.Now()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.mu.Lock()
			stage, detail := l.stage, l.detail
			l.mu.Unlock()
			err := l.heartbeatNow(stage, detail)
			switch {
			case err == nil:
				lastOK = time.Now()
			case errors.Is(err, persistence.ErrOperationLost):
				l.markLost()
				return
			case time.Since(lastOK) > l.manager.staleAfter/2:
				// Authority can no longer be verified and another backend may soon
				// mark this claim INTERRUPTED: stop external work instead of
				// continuing under unverified authority.
				l.markLost()
				return
			}
		}
	}
}

// Lost reports whether recovery took the claim away from this owner.
func (l *Lease) Lost() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lost
}

// Stage persists the current stage and nonsecret detail immediately.
func (l *Lease) Stage(stage string, detail map[string]any) error {
	l.mu.Lock()
	l.stage = stage
	if detail != nil {
		l.detail = detail
	}
	d := l.detail
	l.mu.Unlock()
	err := l.heartbeatNow(stage, d)
	if errors.Is(err, persistence.ErrOperationLost) {
		l.markLost()
	}
	return err
}

// End stops the heartbeat and releases the Environment with a terminal status.
func (l *Lease) End(status environment.OperationStatus, failure string) error {
	l.mu.Lock()
	if l.ended {
		l.mu.Unlock()
		return nil
	}
	l.ended = true
	lost := l.lost
	l.mu.Unlock()
	expired := errors.Is(l.Ctx.Err(), context.DeadlineExceeded)
	close(l.stop)
	l.done.Wait()
	l.cancel()
	if lost {
		return fmt.Errorf("%w: operation %s", persistence.ErrOperationLost, l.Operation.ID)
	}
	if expired && status == environment.OpSucceeded {
		status, failure = environment.OpFailed, "the operation exceeded its deadline"
	} else if expired && failure == "" {
		failure = "the operation exceeded its deadline"
	}
	ctx, cancel := context.WithTimeout(context.Background(), ioTimeout)
	defer cancel()
	return l.manager.store.ReleaseOperation(ctx, l.owner, status, failure)
}

// Suspend stops the heartbeat and returns the claim to INTERRUPTED with a
// visible failure, keeping the Environment held: used when compensation or
// recovery could not complete and an operator must act.
func (l *Lease) Suspend(failure string) error {
	l.mu.Lock()
	if l.ended {
		l.mu.Unlock()
		return nil
	}
	l.ended = true
	l.mu.Unlock()
	close(l.stop)
	l.done.Wait()
	l.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), ioTimeout)
	defer cancel()
	return l.manager.store.SuspendOperation(ctx, l.owner, failure)
}
