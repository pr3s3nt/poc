package delivery

import (
	"testing"
	"time"
)

func TestBindingConfigurationComparisonPreservesImmutableIdentity(t *testing.T) {
	b := BindingVersion{ID: "b", OrganizationID: "org", Version: 1, Revision: 1, Name: "Fleet", ProviderKind: "fleet", ContractVersion: "v1", ManagementAccountID: "account", ManagementAccountVersion: 1, Config: map[string]string{"workspace": "fleet-local"}, WriterCredentialRef: "private-ref", Status: Unverified, CreatedAt: time.Now().UTC()}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := b.Clone()
	at := time.Now()
	copy.VerifiedAt = &at
	copy.Status = BindingReady
	copy.Revision = 2
	copy.Capabilities.ContractVersion = "v1"
	if !b.SameConfiguration(copy) {
		t.Fatal("verification changed config identity")
	}
	copy.Config["workspace"] = "other"
	if b.SameConfiguration(copy) || b.Config["workspace"] != "fleet-local" {
		t.Fatal("mutable binding config")
	}
	copy = b.Clone()
	copy.WriterCredentialRef = "other"
	if b.SameConfiguration(copy) {
		t.Fatal("credential ref changed undetected")
	}
}
