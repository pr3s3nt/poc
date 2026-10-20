package connection_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

const ec2ARN = "arn:aws:iam::123456789012:role/platform/Orchestrator"

type ec2Provider struct {
	roleID string
	err    error
	calls  int
}

func (p *ec2Provider) ResolveEC2Role(_ context.Context, arn, region string) (terraformexec.AWSConnection, error) {
	p.calls++
	return terraformexec.AWSConnection{AccountID: "123456789012", RoleARN: arn, RoleID: p.roleID, Region: region, AccessKeyID: "temporary-key", SecretAccessKey: "temporary-secret", SessionToken: "temporary-token", Expiration: time.Now().Add(time.Hour)}, p.err
}
func (p *ec2Provider) VerifyEC2Role(_ context.Context, arn string, regions []string) (connection.AWSVerification, error) {
	p.calls++
	return connection.AWSVerification{AccountID: "123456789012", PrincipalARN: arn, RoleID: p.roleID, Regions: regions, Capabilities: connection.RequiredAWSCapabilities}, p.err
}

func TestEC2AccountRegistrationVerifyAndResolver(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t)
	provider := &ec2Provider{roleID: "AROAORIGINAL"}
	f.svc.SetEC2RoleProvider(provider)
	cmd := connection.RegisterEC2RoleCommand{Name: "EC2 role", RoleARN: ec2ARN, VerificationRegions: []string{"us-east-1"}}
	acct, err := f.svc.RegisterEC2Role(ctx, "acme", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if acct.SecretRef != "" || f.creds.Len() != 0 || acct.Version != 1 || acct.AuthenticationType != application.AuthAWSEC2Role || acct.ConfigString("roleId") != provider.roleID {
		t.Fatal("incorrect safe account")
	}
	renamed, err := f.svc.RenameAccount(ctx, "acme", acct.Key, 1, "Renamed")
	if err != nil || renamed.ConfigString("roleArn") != ec2ARN {
		t.Fatal("rename changed role", err)
	}
	if _, err = f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 1}); !errors.Is(err, persistence.ErrVersionConflict) {
		t.Fatal("stale accepted", err)
	}
	verified, err := f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 2, VerificationRegions: []string{"eu-west-1"}})
	if err != nil || verified.Version != 3 {
		t.Fatal("verify", err)
	}
	if _, err = f.svc.ReplaceAWSCredential(ctx, "acme", acct.Key, connection.ReplaceAWSCommand{Version: 3}); !errors.Is(err, connection.ErrAccountIncompatible) {
		t.Fatal("key replacement accepted", err)
	}
	resolver := connection.NewCredentialResolver(f.st, nil)
	resolver.SetEC2RoleProvider(provider)
	resolved, err := resolver.ResolveAWS(ctx, "acme", acct.Key)
	if err != nil || resolved.Refresh == nil || resolved.SessionToken == "" {
		t.Fatal("role resolution", err)
	}
	if _, err = resolver.ResolveAWS(ctx, "other", acct.Key); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatal("foreign role accepted", err)
	}
	provider.roleID = "AROARECREATED"
	if _, err = resolved.Refresh(ctx); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatal("recreated role accepted", err)
	}
	if _, err = f.svc.VerifyAccount(ctx, "acme", acct.Key, connection.VerifyAccountCommand{Version: 3}); !errors.Is(err, connection.ErrAccountIncompatible) {
		t.Fatal("changed identity accepted", err)
	}
	current, _ := f.st.GetConnection(ctx, "acme", acct.Key)
	if current.Version != 3 || current.ConfigString("roleId") != "AROAORIGINAL" {
		t.Fatal("failed verify altered record")
	}
}

func TestEC2AccountFailsClosedWithoutProviderOrRole(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	cmd := connection.RegisterEC2RoleCommand{Name: "EC2 role", RoleARN: ec2ARN, VerificationRegions: []string{"us-east-1"}}
	if _, err := f.svc.RegisterEC2Role(ctx, "acme", cmd); !errors.Is(err, connection.ErrVerification) {
		t.Fatal(err)
	}
	p := &ec2Provider{roleID: "AROAID", err: connection.ErrAuthentication}
	f.svc.SetEC2RoleProvider(p)
	if _, err := f.svc.RegisterEC2Role(ctx, "acme", cmd); !errors.Is(err, connection.ErrVerification) {
		t.Fatal(err)
	}
	cmd.RoleARN = "arn:aws:sts::123456789012:assumed-role/Orchestrator/session"
	calls := p.calls
	if _, err := f.svc.RegisterEC2Role(ctx, "acme", cmd); !errors.Is(err, connection.ErrInvalid) || p.calls != calls {
		t.Fatal("invalid ARN reached provider", err)
	}
	accounts, _ := f.st.ListConnections(ctx, "acme")
	if len(accounts) != 0 || f.creds.Len() != 0 {
		t.Fatal("failed verification persisted data")
	}
}
