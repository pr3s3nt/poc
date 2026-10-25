package awsaccess

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/internal/adapters/awsaccess/awsfake"
	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
	"orchestrator/internal/ports/terraformexec"
)

// FAKE AWS (httptest): these tests prove the diagnostics provider's selection,
// signed-request, credential-source and classification behaviour. They are not
// evidence that a real AWS account is ready; live read-only AWS acceptance is a
// separate record.

type awsFixture struct {
	fake    *awsfake.Server
	prov    Diagnostics
	acct    application.Connection
	scope   domain.Scope
	session probes.Session
}

func newAWSFixture(t *testing.T, mutate func(*awsfake.Server)) *awsFixture {
	t.Helper()
	// The host credential chain must never be consulted.
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAHOSTCHAINKEY00001")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "host-chain-secret")
	t.Setenv("AWS_PROFILE", "host")
	f := newFakeAWS()
	if mutate != nil {
		mutate(f)
	}
	creds := credentialmemory.New()
	raw, _ := json.Marshal(map[string]string{"accessKeyId": fakeKeyID, "secretAccessKey": fakeSecret})
	ref, err := creds.Put(context.Background(), "acme", "cloud", raw)
	if err != nil {
		t.Fatal(err)
	}
	acct := application.Connection{Key: "cloud", Name: "Cloud", OrganizationKey: "acme", Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey, SecretRef: ref,
		Config: map[string]any{"accountId": fakeAccount, "verificationRegions": []string{"us-east-1"}}}
	return &awsFixture{fake: f, prov: Diagnostics{Client: client(t, f), Credentials: creds}, acct: acct,
		scope: domain.Scope{Purpose: domain.PurposeGeneral, Mode: domain.ModeConditions, Region: "us-east-1", AccountKey: "cloud", AccountType: "AWS", AccountName: "Cloud", AccountVersion: 1}}
}

// run executes the provider's whole plan for scope/req in order.
func (a *awsFixture) run(t *testing.T, scope domain.Scope, req *probes.Requirements) map[string]domain.Finding {
	t.Helper()
	session := a.prov.Open(context.Background(), a.acct, scope)
	defer session.Close()
	got := map[string]domain.Finding{}
	for _, c := range a.prov.Plan(scope, req) {
		got[c.ID] = session.Run(context.Background(), c, got, req, nil)
	}
	return got
}

func (a *awsFixture) callsTo(service string) int {
	n := 0
	for _, call := range a.fake.Calls() {
		if strings.HasPrefix(call, service+" ") {
			n++
		}
	}
	return n
}

func cacheRequirements(region string) *probes.Requirements {
	return &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{{Descriptor: "d", Type: "redis", Module: "elasticache", Packaged: true,
		Providers: []probes.ProviderTarget{{Kind: "aws", Connection: "cloud", Region: region}}}}}
}

func useCase(region string, uses ...string) domain.Scope {
	return domain.Scope{Purpose: domain.PurposeUseCase, Mode: domain.ModeConditions, UseCases: uses, Region: region, AccountKey: "cloud", AccountType: "AWS", AccountName: "Cloud", AccountVersion: 1}
}

func TestDiagnosticsSignWithTheSavedScopedKeyAndNeverTheHostChain(t *testing.T) {
	a := newAWSFixture(t, nil)
	got := a.run(t, a.scope, nil)
	for _, id := range []string{"aws.credential", "aws.identity", "aws.region", "aws.services", "aws.capability.vpc", "aws.capability.eks", "aws.capability.elasticache", "aws.capability.aurora"} {
		if got[id].Verdict != domain.VerdictPass {
			t.Errorf("%s: %+v", id, got[id])
		}
	}
	if len(a.fake.Authorizations()) == 0 {
		t.Fatal("no signed request was made")
	}
	for _, auth := range a.fake.Authorizations() {
		if !strings.Contains(auth, "Credential="+fakeKeyID+"/") || strings.Contains(auth, "HOSTCHAIN") || strings.Contains(auth, fakeSecret) {
			t.Fatalf("authorization %q", auth)
		}
	}
	// Policy evidence and unmeasured quota are labelled, never presented as proof.
	if got["aws.capability.vpc"].Facts["evidence"] != "POLICY_SIMULATION" || got["aws.quota"].Verdict != domain.VerdictUnknown || got["aws.quota"].Reason != "QUOTA_EVIDENCE_UNAVAILABLE" {
		t.Errorf("evidence labels: %+v %+v", got["aws.capability.vpc"], got["aws.quota"])
	}
	// Read-only: no call other than STS, regional reads and IAM simulation.
	for _, call := range a.fake.Calls() {
		if strings.HasPrefix(call, "sts ") || strings.HasPrefix(call, "iam ") || strings.Contains(call, "Describe") || strings.HasPrefix(call, "eks GET") {
			continue
		}
		t.Errorf("unexpected AWS call %q", call)
	}
}

func TestOnlyTheSelectedFamilyIsProbedAndSimulated(t *testing.T) {
	a := newAWSFixture(t, func(f *awsfake.Server) { f.DenyRead = map[string]string{"rds": "AccessDenied", "eks": "AccessDenied"} })
	got := a.run(t, useCase("us-east-1", "cache"), cacheRequirements("us-east-1"))
	if got["aws.services"].Verdict != domain.VerdictPass || got["aws.capability.elasticache"].Verdict != domain.VerdictPass {
		t.Fatalf("an unrelated denied service must not fail a cache-only selection: %+v %+v", got["aws.services"], got["aws.capability.elasticache"])
	}
	if a.callsTo("rds") != 0 || a.callsTo("eks") != 0 || a.callsTo("elasticache") == 0 || a.callsTo("ec2") == 0 {
		t.Fatalf("service reads: %v", a.fake.Calls())
	}
	for _, id := range []string{"aws.capability.aurora", "aws.capability.eks", "aws.capability.vpc"} {
		if _, planned := got[id]; planned {
			t.Errorf("%s was planned for a cache-only selection", id)
		}
	}
	for _, req := range a.fake.SimulationRequests() {
		for _, action := range req.Actions {
			if strings.HasPrefix(action, "rds:") || strings.HasPrefix(action, "eks:") {
				t.Fatalf("unrelated action simulated: %s", action)
			}
		}
	}
	// The same denial does fail a general inventory.
	general := newAWSFixture(t, func(f *awsfake.Server) { f.DenyRead = map[string]string{"rds": "AccessDenied"} })
	if f := general.run(t, general.scope, nil)["aws.services"]; f.Verdict != domain.VerdictFail || f.Reason != "SERVICE_ACCESS_DENIED" || f.Facts["services"] != "rds" {
		t.Fatalf("general inventory: %+v", f)
	}
}

func TestSelectionWithoutResolvedAwsWorkIsNotReadiness(t *testing.T) {
	a := newAWSFixture(t, nil)
	if got := a.run(t, useCase("us-east-1", "cache"), nil); got["aws.capability.unresolved"].Verdict != domain.VerdictUnknown || got["aws.capability.unresolved"].Reason != "REQUIREMENTS_UNRESOLVED" || got["aws.services"].Reason != "" {
		t.Fatalf("no context: %+v", got)
	}
	elsewhere := &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{{Type: "redis", Module: "elasticache", Packaged: true, Providers: []probes.ProviderTarget{{Kind: "aws", Connection: "another-account", Region: "us-east-1"}}}}}
	if got := a.run(t, useCase("us-east-1", "cache"), elsewhere); got["aws.usecase.scope"].Verdict != domain.VerdictNA || a.callsTo("elasticache") != 0 {
		t.Fatalf("a resource of another account: %+v", got)
	}
	custom := &probes.Requirements{Resolved: true, Resources: []probes.ResourceNeed{{Type: "redis", Module: "my-redis", Packaged: false, Providers: []probes.ProviderTarget{{Kind: "aws", Connection: "cloud", Region: "us-east-1"}}}}}
	if got := a.run(t, useCase("us-east-1", "cache"), custom); got["aws.capability.undeclared"].Verdict != domain.VerdictUnknown {
		t.Fatalf("a custom module declares no groups: %+v", got)
	}
	exec := useCase("us-east-1", "cache")
	exec.Mode = domain.ModeExecution
	if got := a.run(t, exec, cacheRequirements("us-east-1")); got["aws.exec.unsupported"].Verdict != domain.VerdictUnknown || got["aws.exec.unsupported"].Reason != "EXECUTION_UNSUPPORTED" {
		t.Fatalf("execution on AWS: %+v", got["aws.exec.unsupported"])
	}
}

func TestTypedFailuresAreClassifiedWithoutLeakingAwsText(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*awsfake.Server)
		scope   func(domain.Scope) domain.Scope
		id      string
		verdict domain.Verdict
		reason  string
	}{
		{"key rejected", func(f *awsfake.Server) { f.Rules = map[string]awsfake.Rule{fakeKeyID: awsfake.RuleReject} }, nil, "aws.identity", domain.VerdictFail, "CREDENTIAL_REJECTED"},
		{"key of another account", func(f *awsfake.Server) { f.KeyAccount = map[string]string{fakeKeyID: "999999999999"} }, nil, "aws.identity", domain.VerdictFail, "ACCOUNT_MISMATCH"},
		{"region disabled", func(f *awsfake.Server) { f.Disabled = map[string]bool{"eu-west-1": true} }, func(s domain.Scope) domain.Scope { s.Region = "eu-west-1"; return s }, "aws.region", domain.VerdictFail, "REGION_UNAVAILABLE"},
		{"region reports another account", func(f *awsfake.Server) { f.RegionAccount = map[string]string{"eu-west-1": "999999999999"} }, func(s domain.Scope) domain.Scope { s.Region = "eu-west-1"; return s }, "aws.region", domain.VerdictFail, "REGION_UNAVAILABLE"},
		{"policy denies the actions", func(f *awsfake.Server) { f.Rules = map[string]awsfake.Rule{fakeKeyID: awsfake.RuleNoCapabilities} }, nil, "aws.capability.vpc", domain.VerdictFail, "CAPABILITY_DENIED"},
		{"principal may not simulate", func(f *awsfake.Server) { f.Rules = map[string]awsfake.Rule{fakeKeyID: awsfake.RuleNoSimulate} }, nil, "aws.capability.vpc", domain.VerdictUnknown, "SIMULATION_UNAVAILABLE"},
		{"a service read is denied", func(f *awsfake.Server) { f.DenyRead = map[string]string{"ec2": "UnauthorizedOperation"} }, nil, "aws.services", domain.VerdictFail, "SERVICE_ACCESS_DENIED"},
	}
	for _, c := range cases {
		a := newAWSFixture(t, c.mutate)
		scope := a.scope
		if c.scope != nil {
			scope = c.scope(scope)
		}
		got := a.run(t, scope, nil)[c.id]
		if got.Verdict != c.verdict || got.Reason != c.reason {
			t.Errorf("%s: %+v", c.name, got)
		}
		for k, v := range got.Facts {
			if !domain.ValidFact(k, v) || strings.Contains(v, "SECRET-AWS-DETAIL") {
				t.Errorf("%s: unsafe fact %s=%s", c.name, k, v)
			}
		}
	}
	// Dependent checks of a failed identity are blocked by the service; the
	// provider itself never reports them as passed.
	a := newAWSFixture(t, func(f *awsfake.Server) { f.Rules = map[string]awsfake.Rule{fakeKeyID: awsfake.RuleReject} })
	if a.run(t, a.scope, nil)["aws.capability.vpc"].Verdict == domain.VerdictPass {
		t.Fatal("capability passed without identity")
	}
}

func TestMissingOrChangedCredentialNeverFallsBackToTheHost(t *testing.T) {
	a := newAWSFixture(t, nil)
	a.acct.SecretRef = "memory://missing"
	got := a.run(t, a.scope, nil)
	if got["aws.credential"].Verdict != domain.VerdictFail || got["aws.credential"].Reason != "CREDENTIAL_UNAVAILABLE" || got["aws.identity"].Verdict != domain.VerdictBlocked {
		t.Fatalf("missing credential: %+v", got)
	}
	if len(a.fake.Calls()) != 0 {
		t.Fatalf("a call was made without the saved credential: %v", a.fake.Calls())
	}

	role := newAWSFixture(t, nil)
	role.acct.AuthenticationType, role.acct.SecretRef = application.AuthAWSEC2Role, ""
	role.acct.Config = map[string]any{"accountId": fakeAccount, "roleArn": "arn:aws:iam::123456789012:role/orch", "roleId": "AROAEXAMPLE"}
	role.prov.EC2 = fakeRole{conn: terraformexec.AWSConnection{AccountID: "999999999999", RoleARN: "arn:aws:iam::123456789012:role/orch", RoleID: "AROAEXAMPLE", AccessKeyID: fakeKeyID, SecretAccessKey: fakeSecret}}
	if got := role.run(t, role.scope, nil); got["aws.credential"].Reason != "TARGET_CHANGED" || got["aws.credential"].Verdict != domain.VerdictFail {
		t.Fatalf("a role of another account: %+v", got["aws.credential"])
	}
	role.prov.EC2 = fakeRole{conn: terraformexec.AWSConnection{AccountID: fakeAccount, RoleARN: "arn:aws:iam::123456789012:role/orch", RoleID: "AROAEXAMPLE", AccessKeyID: fakeKeyID, SecretAccessKey: fakeSecret, SessionToken: "role-session-token"}}
	if got := role.run(t, role.scope, nil); got["aws.credential"].Verdict != domain.VerdictPass || got["aws.credential"].Facts["role"] != "EC2_ROLE" {
		t.Fatalf("role credential: %+v", got["aws.credential"])
	}
}

type connectionVerification = connection.AWSVerification

type fakeRole struct{ conn terraformexec.AWSConnection }

func (f fakeRole) ResolveEC2Role(context.Context, string, string) (terraformexec.AWSConnection, error) {
	return f.conn, nil
}
func (f fakeRole) VerifyEC2Role(context.Context, string, []string) (connectionVerification, error) {
	return connectionVerification{}, nil
}
