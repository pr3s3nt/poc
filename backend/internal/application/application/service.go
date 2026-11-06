// Package application implements the self-service UC-01 creation transaction.
package application

import (
	"context"
	"errors"
	"strings"
	"time"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// Sentinel errors let delivery map UC-01 failures without parsing messages.
var (
	ErrInvalid      = errors.New("application: invalid input")
	ErrDuplicate    = errors.New("application: already exists")
	ErrStaleVersion = errors.New("application: environment changed; reload it")
)

// FieldError names the Developer-editable field (name) that failed.
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
// Create takes no Connection: placement comes from Resource Definitions (ADR-017).
type CreateCommand struct {
	OrganizationKey, Name string
}

type Result struct {
	Application  appdomain.Application
	Environments []environment.Environment
}
type DeliveryDefaults interface {
	DefaultSelection(context.Context, string) (delivery.Selection, error)
}
type Service struct {
	store    persistence.Store
	delivery DeliveryDefaults
	newKey   func(string, string) (string, error)
}

func (s *Service) SetDeliveryDefaults(d DeliveryDefaults) { s.delivery = d }

func NewService(store persistence.Store) *Service {
	return &Service{store: store, newKey: appdomain.NewKey}
}
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Result, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.Name == "" {
		return Result{}, fieldError("name", ErrInvalid, "application name is required")
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
		}
		appKey, err := s.availableKey(ctx, cmd.OrganizationKey, cmd.Name)
		if err != nil {
			return err
		}
		var selection delivery.Selection
		if s.delivery != nil {
			selection, err = s.delivery.DefaultSelection(ctx, cmd.OrganizationKey)
			if err != nil {
				return err
			}
		}
		appID := ids.New()
		app := appdomain.Application{ID: appID, Key: appKey, OrganizationKey: cmd.OrganizationKey, Name: cmd.Name}
		if err := app.Validate(); err != nil {
			return err
		}
		if err := s.store.SaveApplication(ctx, app); err != nil {
			// Unique Name/key constraints are the final concurrent creation guard.
			if errors.Is(err, persistence.ErrImmutable) {
				return fieldError("name", ErrDuplicate, "application name or code already exists; choose a different name")
			}
			return err
		}
		for _, key := range []string{"staging", "production"} {
			envID := ids.New()
			set := environment.DeploymentSet{ID: ids.New(), EnvironmentID: envID, EnvironmentKey: app.Key + "/" + key, Document: environment.NewDocument(), DocumentHash: "empty", CreatedAt: time.Now().UTC()}
			namespace, _ := appdomain.ManagedResourceName(app.Key, key)
			env := environment.Environment{DesiredDelivery: selection, AppliedDelivery: selection, ID: envID, Key: key, ApplicationID: app.ID, ApplicationKey: app.Key, Name: strings.Title(key), Type: key, NamespaceIdentity: namespace, CurrentDeploymentSetID: set.ID, Version: 1, InfrastructureScope: environment.ScopeEnvironment}
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

func (s *Service) availableKey(ctx context.Context, org, name string) (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		key, err := s.newKey(org, name)
		if err != nil {
			return "", fieldError("name", ErrInvalid, err.Error())
		}
		// Existing keys, including tombstones, never grant reuse authority.
		if _, err := s.store.GetApplication(ctx, key); errors.Is(err, persistence.ErrNotFound) {
			return key, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fieldError("name", ErrDuplicate, "could not allocate an application code; retry creation")
}
