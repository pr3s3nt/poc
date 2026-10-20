package target_test

import (
	"context"
	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/target"
	"orchestrator/internal/domain/application"
	"testing"
)

func TestAccountsReturnsOrganizationScopedCatalog(t *testing.T) {
	st := store.New()
	ctx := context.Background()
	for _, c := range []application.Connection{
		{ID: "a1", Key: "aws-a", OrganizationKey: "acme", Name: "AWS A", Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady},
		{ID: "a2", Key: "aws-b", OrganizationKey: "globex", Name: "AWS B", Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady},
	} {
		if err := st.SaveConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := target.Accounts(ctx, st, "acme")
	if err != nil || len(got) != 1 || got["aws-a"].Name != "AWS A" {
		t.Fatalf("accounts=%+v err=%v", got, err)
	}
}
