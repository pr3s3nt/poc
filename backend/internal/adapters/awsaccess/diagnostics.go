package awsaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	credstore "orchestrator/internal/ports/credentials"
	probes "orchestrator/internal/ports/diagnostics"
)

// Diagnostics implements the diagnostic provider for AWS accounts. It signs
// every call with the account's saved access key, or with the registered EC2
// role resolved through IMDSv2; it never reads the host credential chain.
// Capability checks are IAM policy simulation: policy evidence, not a real
// provisioning attempt, and the findings say so.
type Diagnostics struct {
	Client      *Client
	Credentials credstore.Store
	EC2         connection.EC2RoleProvider
}

var _ probes.Provider = Diagnostics{}

// packagedGroup maps the platform's own packaged AWS modules to the IAM
// capability group derived from their resources. A module that is not
// packaged has no known group.
var packagedGroup = map[string]string{"vpc": "VPC", "eks": "EKS", "elasticache": "ElastiCache", "aurora": "Aurora"}

var allGroups = []string{"Aurora", "EKS", "ElastiCache", "VPC"}

// neededGroups derives the capability groups of the selected work from the
// planned resources placed on this account. A GENERAL check covers every
// platform-supported family. unresolved means the requirements are unknown;
// undeclared means a selected resource uses a module with no known groups.
func neededGroups(scope domain.Scope, req *probes.Requirements) (groups []string, unresolved, undeclared bool) {
	if scope.Purpose == domain.PurposeGeneral {
		return allGroups, false, false
	}
	if req == nil || !req.Resolved {
		return nil, true, false
	}
	selected := map[string]bool{}
	for _, u := range scope.UseCases {
		selected[u] = true
	}
	set := map[string]bool{}
	for _, r := range req.Resources {
		capability := domain.CapabilityOf(r.Type)
		if !selected[capability] && !(selected["deploy"] && capability == "") {
			continue
		}
		for _, p := range r.Providers {
			if p.Kind != "aws" || p.Connection != scope.AccountKey {
				continue
			}
			if g, ok := packagedGroup[r.Module]; ok && r.Packaged {
				set[g] = true
			} else {
				undeclared = true
			}
		}
	}
	for g := range set {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return groups, false, undeclared
}

func checkID(group string) string { return "aws.capability." + strings.ToLower(group) }

// Plan implements probes.Provider.
func (Diagnostics) Plan(scope domain.Scope, req *probes.Requirements) []probes.Check {
	affect := scope.Affects
	all := []string{"deploy", "database", "cache"}
	checks := []probes.Check{
		{ID: "aws.credential", Group: groupAccess, AffectedUseCases: affect(all...)},
		{ID: "aws.identity", Group: groupAccess, DependsOn: []string{"aws.credential"}, AffectedUseCases: affect(all...)},
		{ID: "aws.region", Group: groupAccess, DependsOn: []string{"aws.identity"}, AffectedUseCases: affect(all...)},
	}
	groups, unresolved, undeclared := neededGroups(scope, req)
	switch {
	case unresolved:
		checks = append(checks, probes.Check{ID: "aws.capability.unresolved", Group: "capability", AffectedUseCases: scope.UseCases})
	case len(groups) == 0 && !undeclared:
		checks = append(checks, probes.Check{ID: "aws.usecase.scope", Group: groupAccess, AffectedUseCases: scope.UseCases})
	default:
		checks = append(checks, probes.Check{ID: "aws.services", Group: groupAccess, DependsOn: []string{"aws.region"}, AffectedUseCases: affect(all...)})
		for _, g := range groups {
			checks = append(checks, probes.Check{ID: checkID(g), Group: "capability", DependsOn: []string{"aws.identity", "aws.region"}, AffectedUseCases: affect(all...)})
		}
		if undeclared {
			checks = append(checks, probes.Check{ID: "aws.capability.undeclared", Group: "capability", AffectedUseCases: affect(all...)})
		}
		checks = append(checks, probes.Check{ID: "aws.quota", Group: "capability", DependsOn: []string{"aws.identity"}, AffectedUseCases: affect(all...)})
	}
	if scope.Mode == domain.ModeExecution {
		checks = append(checks, probes.Check{ID: "aws.exec.unsupported", Group: "execution", AffectedUseCases: scope.UseCases, Execution: true})
	}
	return checks
}

type awsSession struct {
	d         Diagnostics
	creds     Credentials
	region    string
	expected  string // saved AWS account ID
	role      string // registered role ARN for EC2 accounts
	reason    string
	principal string // ARN used for policy simulation
	ec2       bool
	scope     domain.Scope
}

// Open implements probes.Provider.
func (d Diagnostics) Open(ctx context.Context, acct application.Connection, scope domain.Scope) probes.Session {
	s := &awsSession{d: d, region: scope.Region, expected: acct.ConfigString("accountId"), scope: scope}
	if d.Client == nil || acct.Kind != application.ConnectionAWS || s.expected == "" {
		s.reason = "CREDENTIAL_UNAVAILABLE"
		return s
	}
	switch acct.AuthenticationType {
	case application.AuthAWSEC2Role:
		s.ec2, s.role = true, acct.ConfigString("roleArn")
		if d.EC2 == nil || !connection.ValidEC2RoleARN(s.role) {
			s.reason = "CREDENTIAL_UNAVAILABLE"
			return s
		}
		fresh, err := d.EC2.ResolveEC2Role(ctx, s.role, scope.Region)
		if err != nil {
			s.reason = "ROLE_CREDENTIAL_UNAVAILABLE"
			return s
		}
		if fresh.AccountID != s.expected || fresh.RoleARN != s.role || fresh.RoleID != acct.ConfigString("roleId") {
			s.reason = "TARGET_CHANGED"
			return s
		}
		s.creds = Credentials{AccessKeyID: fresh.AccessKeyID, SecretAccessKey: fresh.SecretAccessKey, SessionToken: fresh.SessionToken}
		s.principal = s.role
	case application.AuthAWSAccessKey:
		if d.Credentials == nil || acct.SecretRef == "" {
			s.reason = "CREDENTIAL_UNAVAILABLE"
			return s
		}
		raw, err := d.Credentials.Get(ctx, acct.OrganizationKey, acct.Key, acct.SecretRef)
		var value struct {
			AccessKeyID     string `json:"accessKeyId"`
			SecretAccessKey string `json:"secretAccessKey"`
			SessionToken    string `json:"sessionToken,omitempty"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err != nil || decoder.Decode(&value) != nil || value.AccessKeyID == "" || value.SecretAccessKey == "" {
			s.reason = "CREDENTIAL_UNAVAILABLE"
			return s
		}
		s.creds = Credentials{AccessKeyID: value.AccessKeyID, SecretAccessKey: value.SecretAccessKey, SessionToken: value.SessionToken}
	default:
		s.reason = "CREDENTIAL_UNAVAILABLE"
	}
	return s
}

func (s *awsSession) Close() {}
func (s *awsSession) Cleanup(_ context.Context, r []domain.ProbeResource, _ string) []domain.ProbeResource {
	return r
}

func fact(kv ...string) map[string]string {
	out := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func finding(v domain.Verdict, reason, remediation string, facts map[string]string) domain.Finding {
	return domain.Finding{Verdict: v, Reason: reason, Remediation: remediation, Facts: facts}
}

// callFailure maps a typed verification error to a verdict. Authentication
// rejection is observed; everything else is not proof.
func callFailure(err error) domain.Finding {
	switch {
	case errors.Is(err, connection.ErrAuthentication):
		return finding(domain.VerdictFail, "CREDENTIAL_REJECTED", "replace-credential", nil)
	case errors.Is(err, connection.ErrRegionUnavailable):
		return finding(domain.VerdictFail, "REGION_UNAVAILABLE", "enable-region", nil)
	case errors.Is(err, connection.ErrPermissionDenied):
		return finding(domain.VerdictFail, "PERMISSION_DENIED", "grant-iam", nil)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return finding(domain.VerdictUnknown, "CHECK_TIMEOUT", "rerun", nil)
	case errors.Is(err, connection.ErrClusterUnreachable):
		return finding(domain.VerdictUnknown, "ENDPOINT_UNREACHABLE", "check-network", nil)
	}
	return finding(domain.VerdictUnknown, "CALL_FAILED", "rerun", nil)
}

// Run implements probes.Session.
func (s *awsSession) Run(ctx context.Context, c probes.Check, prior map[string]domain.Finding, req *probes.Requirements, rec probes.Recorder) domain.Finding {
	switch c.ID {
	case "aws.credential":
		if s.reason != "" {
			return finding(domain.VerdictFail, s.reason, "restore-credential", nil)
		}
		kind := "ACCESS_KEY"
		if s.ec2 {
			kind = "EC2_ROLE"
		}
		return finding(domain.VerdictPass, "CREDENTIAL_RESOLVED", "", fact("accountId", s.expected, "role", kind, "region", s.region))
	case "aws.capability.unresolved":
		return finding(domain.VerdictUnknown, "REQUIREMENTS_UNRESOLVED", "select-context", fact("evidence", "NONE"))
	case "aws.capability.undeclared":
		return finding(domain.VerdictUnknown, "MODULE_CAPABILITIES_UNDECLARED", "", fact("evidence", "NONE"))
	case "aws.usecase.scope":
		return finding(domain.VerdictNA, "ACCOUNT_NOT_USED_BY_SELECTION", "", nil)
	case "aws.exec.unsupported":
		return finding(domain.VerdictUnknown, "EXECUTION_UNSUPPORTED", "", fact("evidence", "NONE"))
	case "aws.quota":
		return finding(domain.VerdictUnknown, "QUOTA_EVIDENCE_UNAVAILABLE", "", fact("evidence", "NONE"))
	}
	if s.reason != "" {
		return finding(domain.VerdictBlocked, "PREREQUISITE_FAILED", "", nil)
	}
	switch c.ID {
	case "aws.identity":
		return s.identity(ctx)
	case "aws.region":
		return s.regionCheck(ctx)
	case "aws.services":
		return s.services(ctx, req)
	}
	for _, g := range []string{"Aurora", "EKS", "ElastiCache", "VPC"} {
		if c.ID == checkID(g) {
			return s.capability(ctx, g)
		}
	}
	return finding(domain.VerdictUnknown, "PROBE_ERROR", "", nil)
}

func (s *awsSession) identity(ctx context.Context) domain.Finding {
	id, err := s.d.Client.callerIdentity(ctx, s.creds, identityRegion, false)
	if err != nil {
		return callFailure(err)
	}
	if id.account != s.expected {
		return finding(domain.VerdictFail, "ACCOUNT_MISMATCH", "replace-credential", fact("accountId", id.account))
	}
	m := principalPattern.FindStringSubmatch(id.arn)
	kind := "OTHER"
	if m != nil {
		kind = strings.ToUpper(m[1])
	} else if strings.Contains(id.arn, ":assumed-role/") {
		kind = "ASSUMED_ROLE"
	}
	facts := fact("accountId", id.account, "principalType", kind)
	if domain.ValidFact("principalArn", id.arn) {
		facts["principalArn"] = id.arn
	}
	if kind == "ROOT" {
		return finding(domain.VerdictFail, "UNSUPPORTED_PRINCIPAL", "replace-credential", facts)
	}
	if !s.ec2 {
		s.principal = id.arn
		if kind != "USER" {
			return finding(domain.VerdictFail, "UNSUPPORTED_PRINCIPAL", "replace-credential", facts)
		}
	}
	return finding(domain.VerdictPass, "IDENTITY_VERIFIED", "", facts)
}

func (s *awsSession) regionCheck(ctx context.Context) domain.Finding {
	id, err := s.d.Client.callerIdentity(ctx, s.creds, s.region, true)
	if err != nil {
		f := callFailure(err)
		f.Facts = fact("region", s.region)
		return f
	}
	if id.account != s.expected {
		return finding(domain.VerdictFail, "REGION_UNAVAILABLE", "enable-region", fact("region", s.region))
	}
	return finding(domain.VerdictPass, "REGION_USABLE", "", fact("region", s.region))
}

// servicesFor lists the service read probes of the selected permission
// families: the services their IAM actions name (IAM itself is not probed).
// A GENERAL check inventories every family.
func servicesFor(scope domain.Scope, req *probes.Requirements) map[string]bool {
	groups, _, _ := neededGroups(scope, req)
	set := map[string]bool{}
	for _, g := range capabilityGroups {
		for _, want := range groups {
			if g.Name != want {
				continue
			}
			for _, action := range g.Actions {
				if prefix, _, ok := strings.Cut(action, ":"); ok && prefix != "iam" {
					set[prefix] = true
				}
			}
		}
	}
	return set
}

func (s *awsSession) services(ctx context.Context, req *probes.Requirements) domain.Finding {
	selected := servicesFor(s.scope, req)
	var denied, unknown []string
	probed := 0
	for _, probe := range regionProbes {
		if !selected[probe.service] {
			continue
		}
		probed++
		if err := s.d.Client.probeRegion(ctx, s.creds, s.region, probe); err != nil {
			switch {
			case errors.Is(err, connection.ErrPermissionDenied):
				denied = append(denied, probe.service)
			case errors.Is(err, connection.ErrRegionUnavailable), errors.Is(err, connection.ErrAuthentication):
				return callFailure(err)
			default:
				unknown = append(unknown, probe.service)
			}
		}
	}
	facts := fact("servicesChecked", domain.Itoa(probed), "region", s.region)
	if len(denied) > 0 {
		sort.Strings(denied)
		facts["services"] = domain.JoinList(denied)
		return finding(domain.VerdictFail, "SERVICE_ACCESS_DENIED", "grant-iam", facts)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		facts["services"] = domain.JoinList(unknown)
		return finding(domain.VerdictUnknown, "SERVICE_PROBE_FAILED", "rerun", facts)
	}
	return finding(domain.VerdictPass, "SERVICES_READABLE", "", facts)
}

// capability simulates the IAM actions of one module family. Global IAM
// actions use the region IAM really sees (us-east-1), regional ones the
// scoped region. The result is policy evidence only.
func (s *awsSession) capability(ctx context.Context, name string) domain.Finding {
	var actions []string
	for _, g := range capabilityGroups {
		if g.Name == name {
			actions = g.Actions
		}
	}
	if s.principal == "" || len(actions) == 0 {
		return finding(domain.VerdictUnknown, "SIMULATION_UNAVAILABLE", "grant-iam-simulate", fact("group", name))
	}
	var global, regional []string
	for _, a := range actions {
		if isGlobalAction(a) {
			global = append(global, a)
		} else {
			regional = append(regional, a)
		}
	}
	var total simulation
	run := func(region string, list []string) error {
		if len(list) == 0 {
			return nil
		}
		r, err := s.d.Client.simulateActions(ctx, s.creds, s.principal, region, list)
		total.denied = total.denied || r.denied
		total.unproven = total.unproven || r.unproven
		return err
	}
	facts := fact("group", name, "actions", domain.Itoa(len(actions)), "region", s.region, "evidence", "POLICY_SIMULATION")
	if err := run(identityRegion, global); err != nil {
		return capabilityError(err, facts)
	}
	if err := run(s.region, regional); err != nil {
		return capabilityError(err, facts)
	}
	switch {
	case total.denied:
		facts["deniedGroups"] = name
		return finding(domain.VerdictFail, "CAPABILITY_DENIED", "grant-iam", facts)
	case total.unproven:
		facts["unprovenGroups"] = name
		return finding(domain.VerdictUnknown, "CAPABILITY_UNPROVEN", "grant-iam-simulate", facts)
	}
	return finding(domain.VerdictPass, "POLICY_ALLOWS", "", facts)
}

func capabilityError(err error, facts map[string]string) domain.Finding {
	if errors.Is(err, connection.ErrCapabilityUnverifiable) {
		return finding(domain.VerdictUnknown, "SIMULATION_UNAVAILABLE", "grant-iam-simulate", facts)
	}
	f := callFailure(err)
	f.Facts = facts
	return f
}

const groupAccess = "access"

var principalPattern = regexp.MustCompile(`^arn:aws:iam::[0-9]{12}:(user|role|root)(/|$)`)
