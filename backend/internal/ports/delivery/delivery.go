// Package delivery defines CD integration without exposing concrete controller types.
package delivery

import (
	"context"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
)

type BindingVersion = domain.BindingVersion

type Owner struct {
	OperationID, Instance string
	Fence                 int64
}

func (o Owner) Validate() error {
	if o.OperationID == "" || o.Instance == "" || o.Fence < 1 {
		return domain.ErrInvalid
	}
	return nil
}

type Intent struct {
	ID               string
	OrganizationKey  string
	Owner            Owner
	Binding          BindingVersion
	Revision         domain.Revision
	Target           execution.Target
	ExpectedRevision string
	SourceInventory  []AppliedObject
	PolicyOnly       bool
}

func (i Intent) Validate() error {
	if err := i.Owner.Validate(); err != nil {
		return err
	}
	if err := i.Revision.Validate(); err != nil {
		return err
	}
	if i.ID == "" || !i.Target.Explicit() || i.OrganizationKey == "" || i.Target.Organization != i.OrganizationKey {
		return domain.ErrInvalid
	}
	s := i.Revision.Selection.Effective()
	if s.Mode == domain.CD {
		if i.Binding.ID != s.BindingID || i.Binding.Version != s.BindingVersion || i.Binding.OrganizationID != i.Revision.Scope.OrganizationID {
			return domain.ErrInvalid
		}
		return i.Binding.Capabilities.ValidateSelection(s)
	}
	return nil
}

type Prepared struct {
	Revision domain.Revision
	Objects  []execution.ObjectRef
	// Payload is a private deterministic adapter envelope persisted encrypted by
	// the coordinator; never an HTTP DTO or a pointer to process-local state.
	Payload []byte `json:"-"`
}
type Receipt struct {
	IntentID, RevisionID, PayloadDigest string
	// Handle contains safe resource identities/UIDs, never authentication material.
	Handle     map[string]string
	ObservedAt time.Time
}
type Verification struct {
	Ready        bool
	Capabilities domain.Capabilities
	CheckedAt    time.Time
	ReasonCode   string
}
type WithdrawalProof struct {
	Receipt           Receipt
	ManagementStopped bool
	RetainedObjects   []execution.ObjectRef
}

// Provider operations are idempotent by Intent.ID. Unknown mutation outcomes
// must be observed before retry; lease expiry never authorizes another writer.
type Provider interface {
	Describe(context.Context, BindingVersion) (domain.Capabilities, error)
	Verify(context.Context, BindingVersion) (Verification, error)
	Prepare(context.Context, Intent, []execution.Manifest) (Prepared, error)
	Submit(context.Context, Intent, Prepared) (Receipt, error)
	Observe(context.Context, Intent, Receipt) (domain.Observation, error)
	Reconcile(context.Context, Intent, Receipt) (Receipt, error)
	Withdraw(context.Context, Intent, Receipt) (WithdrawalProof, error)
}

// OwnershipTransfer is deliberately optional; an unsupported manager transition
// must fail before changing either controller's authority.
type OwnershipTransfer interface {
	ValidateTransfer(context.Context, Intent, Receipt, []AppliedObject) error
	Adopt(context.Context, Intent, Prepared, TransferSource) (Receipt, error)
}

// InventoryValidator proves the exact admitted incarnations before a policy
// update. Configuration drift may be present; foreign/replaced UIDs are refused.
type InventoryValidator interface {
	ValidateInventory(context.Context, Intent, Receipt, []AppliedObject) error
}

// SubmissionRecovery resolves an already receipted submission after explicit
// worker-stop confirmation/fencing. It may release handoff metadata field
// ownership; it cannot publish/apply, replace objects or replay a Job.
type SubmissionRecovery interface {
	RecoverSubmission(context.Context, Intent, Prepared, Receipt, TransferSource) (Receipt, error)
}

// TransferSource is private coordinator input backed by immutable source
// artifacts and acknowledged incarnation receipts. It is never accepted from UI.
type TransferSource struct {
	Revision  domain.Revision
	Receipt   Receipt
	Inventory []AppliedObject
}

// ConfigurationValidator performs local nonsecret schema validation. Pure
// Preview never calls Verify or external controllers.
type ConfigurationValidator interface{ ValidateConfiguration(BindingVersion) error }

// PublicationIdentity exposes only the canonical destination lease key; the core
// never interprets provider-specific configuration such as Fleet GitRepo fields.
type PublicationIdentity interface {
	PublicationDestination(BindingVersion) (string, error)
}

// AppliedObject binds cleanup to a proven incarnation of an admitted object.
type AppliedObject struct {
	execution.ObjectRef
	UID     string
	Release string
}
type GuardedRemoval interface {
	RemoveRetainedObjects(context.Context, Intent, Receipt, []AppliedObject) error
}

// GuardedPrune removes only objects omitted by an acknowledged newer revision
// of the same release, using incarnation proof from the prior revision.
type GuardedPrune interface {
	PruneReleasedObjects(context.Context, Intent, Receipt, []AppliedObject) error
}

// ImmutableExecution compares the private acknowledged execution artifact with
// a proposed Job set without contacting a controller. An equal set reuses its
// completion receipt; a different set requires explicit removal/new identity.
type ImmutableExecution interface {
	SameExecution(context.Context, []byte, []execution.Manifest) (bool, error)
}

// ArtifactExporter reads only the encrypted, admitted artifact supplied by the
// coordinator. Transfer must not rerender a template or infer intent from live
// resources. Returned objects exclude provider ownership/provenance metadata.
type ArtifactExporter interface {
	ExportArtifact(context.Context, []byte) ([]execution.Manifest, error)
}

// ConfigurationField is nonsecret form metadata supplied by an adapter. It does
// not authorize publication or expose credential references.
type ConfigurationField struct {
	Key              string                `json:"key"`
	LabelKey         string                `json:"labelKey"`
	Kind             string                `json:"kind"`
	Required         bool                  `json:"required"`
	Default          string                `json:"default,omitempty"`
	Advanced         bool                  `json:"advanced,omitempty"`
	ReadOnly         bool                  `json:"readOnly,omitempty"`
	VisibleWhenKey   string                `json:"visibleWhenKey,omitempty"`
	VisibleWhenValue string                `json:"visibleWhenValue,omitempty"`
	Choices          []ConfigurationChoice `json:"choices,omitempty"`
}
type ConfigurationChoice struct {
	Value    string `json:"value"`
	LabelKey string `json:"labelKey"`
}
type ConfigurationDescriptor interface{ ConfigurationFields() []ConfigurationField }

// Discovery returns safe named options from an explicitly selected management
// account. It never enrolls a cluster, writes resources or reads Secret bytes.
type ConfigurationOption struct {
	Name   string            `json:"name"`
	Value  string            `json:"value"`
	Values map[string]string `json:"values"`
}
type ConfigurationDiscovery struct {
	Options map[string][]ConfigurationOption `json:"options"`
}
type ConfigurationInspector interface {
	InspectConfiguration(context.Context, BindingVersion) (ConfigurationDiscovery, error)
}
