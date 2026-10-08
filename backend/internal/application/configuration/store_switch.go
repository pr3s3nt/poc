package configuration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/domain/configuration"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

// SwitchCommand selects the Environment secret store (ADR-012).
type SwitchCommand struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, StoreKey string
	ExpectedVersion, ExpectedConfigVersion                    int64
}

// SwitchResult reports the committed selection. Changed is false for an
// idempotent same-store request at the current versions.
type SwitchResult struct {
	Environment environment.Environment
	Changed     bool
	Copied      int
	// PendingWorkloads is the number of workloads that must be redeployed
	// because their delivery store (or a copied Secret ref) changed; the exact
	// list comes from the next Preview.
	Revision string
}

// SetSecretStore claims the Environment, copies every Secret of the desired
// revision into immutable attempt-owned objects of the new store (reading each
// original through its owning store), validates the copies, then commits the
// selection and the new revision together. Any failure leaves the selection,
// revision and old values untouched and removes only this attempt's objects.
func (s *Service) SetSecretStore(ctx context.Context, cmd SwitchCommand) (SwitchResult, error) {
	if cmd.StoreKey == "" || cmd.ExpectedVersion <= 0 {
		return SwitchResult{}, ErrInvalid
	}
	l, err := s.load(ctx, cmd.ApplicationKey, cmd.EnvironmentKey)
	if err != nil {
		return SwitchResult{}, err
	}
	if l.org != cmd.OrganizationKey {
		return SwitchResult{}, fmt.Errorf("%w: application %q", persistence.ErrNotFound, cmd.ApplicationKey)
	}
	if l.env.Busy() {
		return SwitchResult{}, persistence.Busy(cmd.ApplicationKey, cmd.EnvironmentKey)
	}
	if l.env.Version != cmd.ExpectedVersion || l.scope.Version != cmd.ExpectedConfigVersion {
		return SwitchResult{}, persistence.ErrVersionConflict
	}
	if l.env.SecretStoreKey == cmd.StoreKey {
		return SwitchResult{Environment: l.env}, nil
	}
	target, err := s.store.GetSecretStore(ctx, l.org, cmd.StoreKey)
	if err != nil || target.OrganizationKey != l.org || target.Status != secretstore.StatusReady {
		return SwitchResult{}, fmt.Errorf("%w: secret store %q", ErrStoreUnavailable, cmd.StoreKey)
	}
	newProvider, err := s.registry.Provider(ctx, l.org, cmd.StoreKey)
	if err != nil {
		return SwitchResult{}, fmt.Errorf("%w: secret store %q", ErrStoreUnavailable, cmd.StoreKey)
	}

	lease, err := s.ops.Begin(ctx, claimFor(cmd, l))
	if err != nil {
		return SwitchResult{}, err
	}
	var created []string
	fail := func(cause error) (SwitchResult, error) {
		// Only this attempt's objects are removed, with a bounded context that
		// a cancelled request cannot abort. A failed removal stays visible.
		orphaned := s.discardAll(newProvider, created)
		failure := safeFailure(cause)
		if len(orphaned) > 0 {
			_ = lease.Stage("CLEANUP", map[string]any{"orphanedObjects": orphaned, "from": l.env.SecretStoreKey, "to": cmd.StoreKey})
			failure += fmt.Sprintf("; %d attempt-owned object(s) in the new store could not be removed (ids: %s)", len(orphaned), strings.Join(orphaned, ","))
			cause = fmt.Errorf("%w: %d new object(s) could not be removed", cause, len(orphaned))
		}
		_ = lease.End(environment.OpFailed, failure)
		return SwitchResult{}, cause
	}

	// Re-read under the claim: nothing may change while the claim is held.
	entries := copyEntries(l.revision.Entries)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	copied := 0
	changed := false
	for index, name := range names {
		entry := entries[name]
		if err := lease.Stage(fmt.Sprintf("COPY %d/%d", index+1, len(names)), nil); err != nil {
			return fail(err)
		}
		switch {
		case entry.Kind == configuration.Secret:
			value, err := s.readEntry(lease.Ctx, l.org, entry)
			if err != nil {
				return fail(fmt.Errorf("%w: a secret could not be read from its current store", ErrStoreCopy))
			}
			ref, err := newProvider.WriteValue(lease.Ctx, cmd.ApplicationKey, cmd.EnvironmentKey, value)
			if ref != "" && err == nil {
				created = append(created, ref)
			}
			if err != nil {
				return fail(ErrStoreCopy)
			}
			check, err := newProvider.ReadValue(lease.Ctx, ref)
			if err != nil || check != value {
				return fail(fmt.Errorf("%w: copy verification failed", ErrStoreCopy))
			}
			entries[name] = configuration.Entry{Kind: configuration.Secret, ValueRef: ref, StoreKey: cmd.StoreKey}
			copied++
			changed = true
		case entry.LegacyVariable():
			value, err := s.readEntry(lease.Ctx, l.org, entry)
			if err != nil {
				return fail(err)
			}
			entries[name] = configuration.Entry{Kind: configuration.Variable, Value: value}
			changed = true
		}
	}

	var out environment.Environment
	var revisionID string
	commitErr := s.store.Transact(lease.Ctx, func(ctx context.Context) error {
		if changed {
			revisionID = ids.New()
			if err := s.store.CommitConfigurationRevision(ctx, cmd.ExpectedConfigVersion, configuration.Revision{
				ID: revisionID, ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Version: cmd.ExpectedConfigVersion + 1, Entries: entries,
			}); err != nil {
				return err
			}
		}
		var err error
		out, err = s.store.SelectSecretStore(ctx, persistence.SecretStoreSelection{
			ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, StoreKey: cmd.StoreKey, ExpectedVersion: cmd.ExpectedVersion,
		})
		return err
	})
	if commitErr != nil {
		return fail(commitErr)
	}
	if err := lease.End(environment.OpSucceeded, ""); err != nil {
		return SwitchResult{}, err
	}
	if released, err := s.store.GetEnvironment(ctx, cmd.ApplicationKey, cmd.EnvironmentKey); err == nil {
		out = released
	}
	return SwitchResult{Environment: out, Changed: true, Copied: copied, Revision: revisionID}, nil
}

// ErrStoreUnavailable reports an unknown, foreign or not READY target store.
var ErrStoreUnavailable = configport.ErrStoreUnavailable

func claimFor(cmd SwitchCommand, l loaded) envops.Claim {
	return envops.Claim{
		ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, Kind: environment.OpStoreCopy,
		Pins: environment.OperationPins{
			CheckEnvVersion: true, EnvVersion: cmd.ExpectedVersion,
			CheckConfigVersion: true, ConfigVersion: cmd.ExpectedConfigVersion,
			CheckRevision: true, DesiredRevisionID: l.scope.DesiredRevisionID,
			CheckStore: true, SecretStoreKey: l.env.SecretStoreKey,
		},
		Detail: map[string]any{"from": l.env.SecretStoreKey, "to": cmd.StoreKey},
	}
}

// discardAll removes attempt-owned objects and returns the opaque object IDs
// (never values or full refs) of those that could not be removed.
func (s *Service) discardAll(provider configport.Provider, refs []string) []string {
	var orphaned []string
	deleter, ok := provider.(ValueDeleter)
	for _, ref := range refs {
		id := ref[strings.LastIndex(ref, "/")+1:]
		if !ok {
			orphaned = append(orphaned, id)
			continue
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := deleter.DeleteValue(cleanup, ref); err != nil {
			orphaned = append(orphaned, id)
		}
		cancel()
	}
	return orphaned
}

// safeFailure reduces an error to a persistable sentence without remote text.
func safeFailure(err error) string {
	switch {
	case errors.Is(err, ErrStoreCopy):
		return ErrStoreCopy.Error()
	case errors.Is(err, persistence.ErrVersionConflict):
		return "the environment or its configuration changed during the copy"
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		return "the environment is busy"
	case errors.Is(err, ErrLegacyUnreadable):
		return ErrLegacyUnreadable.Error()
	}
	return "the secret store change failed"
}
