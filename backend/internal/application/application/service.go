// Package application implements the self-service UC-01 creation transaction.
package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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
// ConnectionKey nil means the caller omitted it (legacy default); a non-nil
// blank value is invalid and never falls back to the default.
type CreateCommand struct {
	OrganizationKey, Name, Subdomain, BaseDomain string
	ConnectionKey                                *string
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
	if cmd.ConnectionKey != nil && strings.TrimSpace(*cmd.ConnectionKey) == "" {
		return Result{}, fieldError(connectionKeyField, ErrInvalid, "connectionKey must not be blank")
	}
	var result Result
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		org, err := s.store.GetOrganization(ctx, cmd.OrganizationKey)
		if err != nil {
			return err
		}
		wanted := org.DefaultConnectionKey
		if cmd.ConnectionKey != nil {
			wanted = strings.TrimSpace(*cmd.ConnectionKey)
		}
		conn, err := s.store.GetConnection(ctx, org.Key, wanted)
		if errors.Is(err, persistence.ErrNotFound) {
			return fieldError(connectionKeyField, ErrTargetNotReady, targetUnavailableMsg)
		}
		if err != nil {
			return err
		}
		if !eligible(org.Key, conn) {
			return fieldError(connectionKeyField, ErrTargetNotReady, targetUnavailableMsg)
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
		profile, region, status := appdomain.ProfileInternalK8s, "", appdomain.RuntimeReady
		if conn.Kind == appdomain.ConnectionAWS {
			profile, region, status = appdomain.ProfileAWSEKS, conn.ConfigString("region"), appdomain.RuntimePending
		}
		appID := ids.New()
		app := appdomain.Application{ID: appID, Key: appID, OrganizationKey: cmd.OrganizationKey, Name: cmd.Name, Subdomain: cmd.Subdomain, Profile: profile, ConnectionKey: conn.Key, Region: region, RuntimeStatus: status, ConfigurationProvider: "vault"}
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
			env := environment.Environment{ID: envID, Key: key, ApplicationID: app.ID, ApplicationKey: app.Key, Name: strings.Title(key), Type: key, NamespaceIdentity: "app-" + app.Key + "-" + key, CurrentDeploymentSetID: set.ID}
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
