// Package application implements the self-service UC-01 creation transaction.
package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/application/envops"
	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

var subdomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Sentinel errors let delivery map UC-01 failures without parsing messages.
var (
	ErrInvalid        = errors.New("application: invalid input")
	ErrDuplicate      = errors.New("application: already exists")
	ErrTargetNotReady = errors.New("application: execution target is unavailable")
)

// connectionKeyField names the request field reported for target failures.
const connectionKeyField = "connectionKey"

// targetUnavailableMsg is the single safe message for a missing, foreign,
// not-ready or unsupported Connection; it never reveals which case applied.
const targetUnavailableMsg = "the selected connection is not available"

// FieldError names the Developer-editable field (name, subdomain or
// connectionKey) that failed.
type FieldError struct {
	Field string
	Err   error
	msg   string
}

func (e *FieldError) Error() string { return e.msg }
func (e *FieldError) Unwrap() error { return e.Err }

func fieldError(field string, err error, msg string) error {
	return &FieldError{Field: field, Err: err, msg: "application: " + msg}
}

// CreateCommand carries session-derived Organization and Developer input.
// Create takes no Connection: Environments start UNCONFIGURED (ADR-011).
type CreateCommand struct {
	OrganizationKey, Name, Subdomain, BaseDomain string
}

// Choice is the safe projection of a selectable Connection.
type Choice struct {
	Key    string
	Name   string
	Kind   appdomain.ConnectionKind
	Status appdomain.ConnectionStatus
}

// Choices lists READY supported Connections and the Organization default.
type Choices struct {
	Connections          []Choice
	DefaultConnectionKey string
}

func supportedKind(k appdomain.ConnectionKind) bool {
	return k == appdomain.ConnectionKubernetes || k == appdomain.ConnectionAWS
}

// eligible reports whether a Connection can bind a new Application.
func eligible(org string, c appdomain.Connection) bool {
	if c.OrganizationKey != org || c.Status != appdomain.ConnectionReady || !supportedKind(c.Kind) {
		return false
	}
	return c.Kind != appdomain.ConnectionAWS || c.ConfigString("region") != ""
}

// ListChoices returns the selectable Connections of the Organization. The
// default key is reported only when it is itself eligible.
func (s *Service) ListChoices(ctx context.Context, organizationKey string) (Choices, error) {
	org, err := s.store.GetOrganization(ctx, organizationKey)
	if err != nil {
		return Choices{}, err
	}
	conns, err := s.store.ListConnections(ctx, org.Key)
	if err != nil {
		return Choices{}, err
	}
	out := Choices{Connections: []Choice{}}
	for _, c := range conns {
		if !eligible(org.Key, c) {
			continue
		}
		name := c.Name
		if name == "" {
			name = c.Key
		}
		out.Connections = append(out.Connections, Choice{Key: c.Key, Name: name, Kind: c.Kind, Status: c.Status})
		if c.Key == org.DefaultConnectionKey {
			out.DefaultConnectionKey = c.Key
		}
	}
	return out, nil
}

type Result struct {
	Application  appdomain.Application
	Environments []environment.Environment
}
type Service struct{ store persistence.Store }

func NewService(store persistence.Store) *Service { return &Service{store: store} }
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Result, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.Subdomain = strings.ToLower(strings.TrimSpace(cmd.Subdomain))
	if cmd.Name == "" {
		return Result{}, fieldError("name", ErrInvalid, "application name is required")
	}
	if !subdomainPattern.MatchString(cmd.Subdomain) {
		return Result{}, fieldError("subdomain", ErrInvalid, "subdomain must be a DNS label of lowercase letters, numbers and hyphens")
	}
	var result Result
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetOrganization(ctx, cmd.OrganizationKey); err != nil {
			return err
		}
		apps, err := s.store.ListApplications(ctx)
		if err != nil {
			return err
		}
		for _, existing := range apps {
			if existing.OrganizationKey == cmd.OrganizationKey && strings.EqualFold(existing.Name, cmd.Name) {
				return fieldError("name", ErrDuplicate, "application name already exists in the organization")
			}
			if existing.Subdomain == cmd.Subdomain {
				return fieldError("subdomain", ErrDuplicate, "subdomain is already in use")
			}
		}
		appID := ids.New()
		app := appdomain.Application{ID: appID, Key: appID, OrganizationKey: cmd.OrganizationKey, Name: cmd.Name, Subdomain: cmd.Subdomain, RuntimeStatus: appdomain.RuntimeUnconfigured, ConfigurationProvider: "vault"}
		if err := app.Validate(); err != nil {
			return err
		}
		if err := s.store.SaveApplication(ctx, app); err != nil {
			// Unique Name/Subdomain constraints are the final duplicate guard.
			if errors.Is(err, persistence.ErrImmutable) {
				return fmt.Errorf("%w: %v", ErrDuplicate, err)
			}
			return err
		}
		for _, key := range []string{"staging", "production"} {
			envID := ids.New()
			set := environment.DeploymentSet{ID: ids.New(), EnvironmentID: envID, EnvironmentKey: app.Key + "/" + key, Document: environment.NewDocument(), DocumentHash: "empty", CreatedAt: time.Now().UTC()}
			env := environment.Environment{ID: envID, Key: key, ApplicationID: app.ID, ApplicationKey: app.Key, Name: strings.Title(key), Type: key, NamespaceIdentity: "app-" + app.Key + "-" + key, CurrentDeploymentSetID: set.ID, Version: 1, RuntimeStatus: appdomain.RuntimeUnconfigured, InfrastructureScope: environment.ScopeEnvironment}
			if err := s.store.SaveDeploymentSet(ctx, set); err != nil {
				return err
			}
			if err := s.store.SaveEnvironment(ctx, env); err != nil {
				return err
			}
			result.Environments = append(result.Environments, env)
		}
		result.Application = app
		return nil
	})
	return result, err
}

// Failures of SetConnection that delivery maps to 409.
var (
	// ErrRuntimeExists means the Environment already has runtime state, so the
	// target can only change through the explicit transition flow.
	ErrRuntimeExists = errors.New("application: environment has runtime resources; use a connection transition")
	ErrStaleVersion  = errors.New("application: environment changed; reload it")
	ErrNotFound      = persistence.ErrNotFound
	// ErrBusy wraps persistence.ErrEnvironmentBusy for delivery.
	ErrBusy = persistence.ErrEnvironmentBusy
)

// SetConnectionCommand carries session-derived Organization and the request.
type SetConnectionCommand struct {
	OrganizationKey, ApplicationKey, EnvironmentKey, ConnectionKey string
	ExpectedVersion                                                int64
}

// SetConnection selects or changes the execution Connection of one Environment
// before any runtime exists (UC-01 ES-03..06, ADR-012). Selecting the same key
// at the current version is an idempotent no-op. It is one versioned metadata
// write: no provisioning and no external call.
func (s *Service) SetConnection(ctx context.Context, cmd SetConnectionCommand) (environment.Environment, error) {
	cmd.ConnectionKey = strings.TrimSpace(cmd.ConnectionKey)
	if cmd.ConnectionKey == "" {
		return environment.Environment{}, fieldError(connectionKeyField, ErrInvalid, "connectionKey must not be blank")
	}
	if cmd.ExpectedVersion <= 0 {
		return environment.Environment{}, fieldError("expectedVersion", ErrInvalid, "expectedVersion is required")
	}
	var out environment.Environment
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		app, err := s.store.GetApplication(ctx, cmd.ApplicationKey)
		if err != nil {
			return err
		}
		if app.OrganizationKey != cmd.OrganizationKey {
			return fmt.Errorf("%w: application %q", persistence.ErrNotFound, cmd.ApplicationKey)
		}
		env, err := s.store.GetEnvironment(ctx, cmd.ApplicationKey, cmd.EnvironmentKey)
		if err != nil {
			return err
		}
		if env.Busy() {
			return persistence.Busy(cmd.ApplicationKey, cmd.EnvironmentKey)
		}
		if env.Version != cmd.ExpectedVersion {
			return ErrStaleVersion
		}
		if env.Configured() && env.ConnectionKey == cmd.ConnectionKey {
			// Same key at the current version: nothing changes, nothing is rechecked.
			out = env
			return nil
		}
		conn, err := s.store.GetConnection(ctx, cmd.OrganizationKey, cmd.ConnectionKey)
		if errors.Is(err, persistence.ErrNotFound) {
			return fieldError(connectionKeyField, ErrTargetNotReady, targetUnavailableMsg)
		}
		if err != nil {
			return err
		}
		if !eligible(cmd.OrganizationKey, conn) {
			return fieldError(connectionKeyField, ErrTargetNotReady, targetUnavailableMsg)
		}
		if env.Configured() {
			exists, err := envops.HasRuntime(ctx, s.store, cmd.OrganizationKey, env)
			if err != nil {
				return err
			}
			if exists {
				return ErrRuntimeExists
			}
		}
		profile, region, status := appdomain.ProfileInternalK8s, "", appdomain.RuntimeReady
		if conn.Kind == appdomain.ConnectionAWS {
			profile, region, status = appdomain.ProfileAWSEKS, conn.ConfigString("region"), appdomain.RuntimePending
		}
		bound, err := s.store.BindEnvironment(ctx, persistence.EnvironmentBinding{
			ApplicationKey: cmd.ApplicationKey, EnvironmentKey: cmd.EnvironmentKey, ConnectionKey: conn.Key,
			Profile: profile, Region: region, RuntimeStatus: status, Scope: environment.ScopeEnvironment,
			Generation: env.TargetGeneration, ExpectedVersion: cmd.ExpectedVersion,
		})
		switch {
		case errors.Is(err, persistence.ErrVersionConflict):
			return ErrStaleVersion
		case errors.Is(err, persistence.ErrNotFound):
			return fieldError(connectionKeyField, ErrTargetNotReady, targetUnavailableMsg)
		case err != nil:
			return err
		}
		out = bound
		return nil
	})
	return out, err
}
