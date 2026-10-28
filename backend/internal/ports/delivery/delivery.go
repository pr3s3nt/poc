// Package delivery defines CD integration without exposing concrete controller types.
package delivery

import (
	"context"
	"time"

	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
)

type BindingVersion struct {
	ID              string
	Version         int64
	OrganizationID  string
	ProviderKind    string
	ContractVersion string
	Capabilities    domain.Capabilities
	// Config and credential refs are private integration data, never API projections.
	Config              map[string]string
	ManagementAccountID string
	WriterCredentialRef string
}

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
	Owner            Owner
	Binding          BindingVersion
	Revision         domain.Revision
	Target           execution.Target
	ExpectedRevision string
}

func (i Intent) Validate() error {
	if err := i.Owner.Validate(); err != nil {
		return err
	}
	if err := i.Revision.Validate(); err != nil {
		return err
	}
	if i.ID == "" || !i.Target.Explicit() || i.Target.Organization != i.Revision.Scope.OrganizationID {
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
	PrepareTransfer(context.Context, Intent, Receipt) (Prepared, error)
	Release(context.Context, Intent, Receipt) (WithdrawalProof, error)
	Adopt(context.Context, Intent, Prepared) (Receipt, error)
}
