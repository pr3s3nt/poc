// Package delivery coordinates provider-neutral delivery intent and observations.
package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/credentials"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/persistence"
)

var ErrUnavailable = errors.New("delivery: service unavailable")

type bindingStore interface {
	persistence.Store
	persistence.DeliveryBindingRepository
	persistence.DeliverySelectionRepository
}
type Bindings struct {
	store       bindingStore
	registry    *port.Registry
	now         func() time.Time
	credentials credentials.Store
}

func NewBindings(st bindingStore, registry *port.Registry) *Bindings {
	return &Bindings{store: st, registry: registry, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Bindings) SetCredentials(store credentials.Store) { s.credentials = store }

type RegisterBinding struct {
	// Internal versioning pins; public handlers derive BindingID from the path.
	BindingID                string
	PreviousVersion          int64
	ExpectedRevision         int64
	RetainWriterCredential   bool
	WriterUsername           string
	WriterPassword           string
	Name                     string
	ProviderKind             string
	ManagementAccountKey     string
	ManagementAccountVersion int64
	Config                   map[string]string
	// Already stored via the private credential service, never accepted from a
	// public opaque-reference field. Registration HTTP must persist bytes first.
	WriterCredentialRef string
}

func (s *Bindings) Register(ctx context.Context, orgKey string, cmd RegisterBinding) (domain.BindingVersion, error) {
	if s.registry == nil {
		return domain.BindingVersion{}, ErrUnavailable
	}
	provider, err := s.registry.Resolve(cmd.ProviderKind)
	if err != nil {
		return domain.BindingVersion{}, err
	}
	validator, ok := provider.(port.ConfigurationValidator)
	if !ok {
		return domain.BindingVersion{}, domain.ErrUnsupported
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, "\x00\r\n") {
		return domain.BindingVersion{}, domain.ErrInvalid
	}
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return domain.BindingVersion{}, err
	}
	account, err := s.store.GetConnection(ctx, orgKey, cmd.ManagementAccountKey)
	if err != nil || account.Status != application.ConnectionReady || account.Kind != application.ConnectionKubernetes || account.Version != cmd.ManagementAccountVersion {
		return domain.BindingVersion{}, persistence.ErrNotFound
	}
	b := domain.BindingVersion{ID: ids.New(), Version: 1, OrganizationID: org.ID, Name: name, ProviderKind: cmd.ProviderKind, ContractVersion: "v1", ManagementAccountID: account.ID, ManagementAccountVersion: account.Version, Config: cmd.Config, WriterCredentialRef: cmd.WriterCredentialRef, Status: domain.Unverified, Revision: 1, CreatedAt: s.now()}
	if cmd.BindingID != "" {
		previous, err := s.store.GetDeliveryBinding(ctx, org.ID, cmd.BindingID, cmd.PreviousVersion)
		if err != nil {
			return domain.BindingVersion{}, err
		}
		if previous.Revision != cmd.ExpectedRevision {
			return domain.BindingVersion{}, persistence.ErrVersionConflict
		}
		if previous.ProviderKind != cmd.ProviderKind {
			return domain.BindingVersion{}, domain.ErrInvalid
		}
		b.ID, b.Version = previous.ID, previous.Version+1
		versions, err := s.store.ListDeliveryBindings(ctx, org.ID)
		if err != nil {
			return domain.BindingVersion{}, err
		}
		for _, version := range versions {
			if version.ID == b.ID && version.Version >= b.Version {
				b.Version = version.Version + 1
			}
		}
		if cmd.RetainWriterCredential {
			if cmd.WriterUsername != "" || cmd.WriterPassword != "" {
				return domain.BindingVersion{}, domain.ErrInvalid
			}
			b.WriterCredentialRef = previous.WriterCredentialRef
		}
	} else if cmd.RetainWriterCredential {
		return domain.BindingVersion{}, domain.ErrInvalid
	}
	if err := validator.ValidateConfiguration(b); err != nil {
		return domain.BindingVersion{}, domain.ErrInvalid
	}
	createdCredential := false
	if cmd.WriterUsername != "" || cmd.WriterPassword != "" {
		if s.credentials == nil || !s.credentials.Durable() || cmd.WriterUsername == "" || cmd.WriterPassword == "" || strings.ContainsAny(cmd.WriterUsername+cmd.WriterPassword, "\x00\r\n") {
			return domain.BindingVersion{}, domain.ErrInvalid
		}
		raw, _ := json.Marshal(map[string]string{"username": cmd.WriterUsername, "password": cmd.WriterPassword})
		ref, err := s.credentials.Put(ctx, orgKey, b.ID, raw)
		if err != nil {
			if ref != "" {
				_ = s.credentials.Delete(ctx, orgKey, b.ID, ref)
			}
			return domain.BindingVersion{}, ErrUnavailable
		}
		b.WriterCredentialRef = ref
		createdCredential = true
	}
	if err := s.store.CreateDeliveryBinding(ctx, b); err != nil {
		if createdCredential && !errors.Is(err, persistence.ErrCommitUncertain) {
			_ = s.credentials.Delete(ctx, orgKey, b.ID, b.WriterCredentialRef)
		}
		return domain.BindingVersion{}, err
	}
	return b.Clone(), nil
}
func (s *Bindings) Verify(ctx context.Context, orgKey, id string, version, expected int64) (domain.BindingVersion, error) {
	if s.registry == nil {
		return domain.BindingVersion{}, ErrUnavailable
	}
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return domain.BindingVersion{}, err
	}
	b, err := s.store.GetDeliveryBinding(ctx, org.ID, id, version)
	if err != nil {
		return domain.BindingVersion{}, err
	}
	if b.Revision != expected {
		return domain.BindingVersion{}, persistence.ErrVersionConflict
	}
	provider, err := s.registry.Resolve(b.ProviderKind)
	if err != nil {
		return domain.BindingVersion{}, err
	}
	result, verifyErr := provider.Verify(ctx, b)
	status := domain.Rejected
	if verifyErr == nil && result.Ready {
		status = domain.BindingReady
	}
	if status != domain.BindingReady {
		// A failed re-verification blocks new admission, but must not erase the
		// last verified facts needed to retire an already admitted controller.
		result.Capabilities = b.Capabilities
	}
	// Only a bounded code from the known adapter result is persisted. Exceptions
	// never become public provider text or credential-bearing diagnostics.
	reason := "VERIFICATION_FAILED"
	if status == domain.BindingReady {
		reason = ""
	}
	if safeCode(result.ReasonCode) {
		reason = result.ReasonCode
	}
	return s.store.VerifyDeliveryBinding(ctx, org.ID, id, version, expected, status, result.Capabilities, s.now(), reason)
}
func safeCode(code string) bool {
	switch code {
	case "CONTROLLER_UNAVAILABLE", "VERSION_UNSUPPORTED", "TARGET_MAPPING_UNSUPPORTED", "VERIFICATION_FAILED", "PERMISSION_DENIED":
		return true
	}
	return false
}
func (s *Bindings) Selection(ctx context.Context, orgKey, appKey, envKey string) (environment.Environment, error) {
	app, err := s.store.GetApplication(ctx, appKey)
	if err != nil || app.OrganizationKey != orgKey {
		return environment.Environment{}, persistence.ErrNotFound
	}
	return s.store.GetEnvironment(ctx, appKey, envKey)
}
func (s *Bindings) Select(ctx context.Context, orgKey string, cmd persistence.DeliverySelectionCommand) (environment.Environment, error) {
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return environment.Environment{}, err
	}
	cmd.OrganizationID = org.ID
	return s.store.SelectDelivery(ctx, cmd)
}

func (s *Bindings) List(ctx context.Context, orgKey string) ([]domain.BindingVersion, error) {
	org, err := s.store.GetOrganization(ctx, orgKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListDeliveryBindings(ctx, org.ID)
}

type ProviderDescriptor struct {
	Kind         string                    `json:"kind"`
	Capabilities domain.Capabilities       `json:"capabilities"`
	Fields       []port.ConfigurationField `json:"fields"`
}

func (s *Bindings) Providers(ctx context.Context) ([]ProviderDescriptor, error) {
	out := []ProviderDescriptor{}
	if s.registry == nil {
		return out, nil
	}
	for _, kind := range s.registry.Kinds() {
		if kind == "direct" {
			continue
		}
		provider, err := s.registry.Resolve(kind)
		if err != nil {
			return nil, err
		}
		caps, err := provider.Describe(ctx, domain.BindingVersion{})
		if err != nil {
			return nil, err
		}
		fields := []port.ConfigurationField{}
		if schema, ok := provider.(port.ConfigurationDescriptor); ok {
			fields = schema.ConfigurationFields()
		}
		out = append(out, ProviderDescriptor{Kind: kind, Capabilities: caps, Fields: fields})
	}
	return out, nil
}
