package delivery

import (
	"encoding/json"
	"time"
)

type BindingStatus string

const (
	Unverified   BindingStatus = "UNVERIFIED"
	BindingReady BindingStatus = "READY"
	Rejected     BindingStatus = "REJECTED"
	Disabled     BindingStatus = "DISABLED"
)

// BindingVersion is an immutable provider configuration. Verification can change
// only status/facts and its CAS revision, never account or publication identity.
type BindingVersion struct {
	ID                       string            `json:"id"`
	Version                  int64             `json:"version"`
	OrganizationID           string            `json:"organizationId"`
	Name                     string            `json:"name"`
	ProviderKind             string            `json:"providerKind"`
	ContractVersion          string            `json:"contractVersion"`
	Capabilities             Capabilities      `json:"capabilities"`
	ManagementAccountID      string            `json:"managementAccountId"`
	ManagementAccountVersion int64             `json:"managementAccountVersion"`
	Config                   map[string]string `json:"config"`
	// This is an opaque private reference, never the credential bytes.
	WriterCredentialRef string        `json:"-"`
	Status              BindingStatus `json:"status"`
	Revision            int64         `json:"revision"`
	VerifiedAt          *time.Time    `json:"verifiedAt,omitempty"`
	ReasonCode          string        `json:"reasonCode,omitempty"`
	CreatedAt           time.Time     `json:"createdAt"`
}

func (b BindingVersion) Validate() error {
	if b.ID == "" || b.OrganizationID == "" || b.Version < 1 || b.Revision < 1 || b.Name == "" || b.ProviderKind == "" || b.ContractVersion == "" || b.ManagementAccountID == "" || b.ManagementAccountVersion < 1 || b.CreatedAt.IsZero() || b.Config == nil {
		return ErrInvalid
	}
	switch b.Status {
	case Unverified, BindingReady, Rejected, Disabled:
	default:
		return ErrInvalid
	}
	if b.Status == BindingReady && (b.VerifiedAt == nil || b.VerifiedAt.IsZero() || b.Capabilities.ContractVersion != b.ContractVersion) {
		return ErrInvalid
	}
	return nil
}
func (b BindingVersion) Clone() BindingVersion {
	out := b
	out.Config = make(map[string]string, len(b.Config))
	for k, v := range b.Config {
		out.Config[k] = v
	}
	if b.VerifiedAt != nil {
		t := *b.VerifiedAt
		out.VerifiedAt = &t
	}
	return out
}
func (b BindingVersion) SameConfiguration(other BindingVersion) bool {
	b.CreatedAt, other.CreatedAt = b.CreatedAt.UTC(), other.CreatedAt.UTC()
	b.Status, other.Status = "", ""
	b.Revision, other.Revision = 0, 0
	b.VerifiedAt, other.VerifiedAt = nil, nil
	b.Capabilities, other.Capabilities = Capabilities{}, Capabilities{}
	b.ReasonCode, other.ReasonCode = "", ""
	a, _ := json.Marshal(b)
	c, _ := json.Marshal(other)
	return string(a) == string(c) && b.WriterCredentialRef == other.WriterCredentialRef
}
