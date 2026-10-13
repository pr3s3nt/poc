package awsaccess

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"orchestrator/internal/adapters/awsaccess/awsfake"
	"orchestrator/internal/application/connection"
)

// FAKE AWS: these tests prove the verifier's fail-closed handling of the
// IAM simulation (per-region context, missing context, organization denials,
// paging), redirect refusal and the coverage of the module resources. They are
// not evidence about a real account.

// simulatedBy groups the simulated actions by the region context they were
// asked with, and reports how many requests each region needed.
func simulatedBy(f *awsfake.Server) (actions map[string][]string) {
	actions = map[string][]string{}
	for _, request := range f.SimulationRequests() {
		actions[request.Region] = append(actions[request.Region], request.Actions...)
	}
	return actions
}

func TestGlobalIAMActionsAreSimulatedOnlyInUSEast1AndRegionalOnesPerAllowedRegion(t *testing.T) {
	f := newFakeAWS()
	regions := []string{"eu-west-1", "ap-south-1"} // deliberately not us-east-1
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), regions); err != nil {
		t.Fatal(err)
	}
	global, regional := partitionActions()
	if len(global) == 0 || len(regional) == 0 {
		t.Fatalf("partition: %d global, %d regional", len(global), len(regional))
	}
	got := simulatedBy(f)
	if len(got) != 3 {
		t.Fatalf("regions used as context: %v", got)
	}
	// Global (iam:) actions: exactly once, only with us-east-1.
	count := func(list []string) map[string]int {
		out := map[string]int{}
		for _, a := range list {
			out[a]++
		}
		return out
	}
	usEast := count(got["us-east-1"])
	for _, action := range global {
		if usEast[action] != 1 {
			t.Errorf("global action %s simulated %d times in us-east-1", action, usEast[action])
		}
	}
	for region, list := range got {
		for _, action := range list {
			if isGlobalAction(action) != (region == "us-east-1") {
				t.Errorf("action %s was simulated with aws:RequestedRegion=%s", action, region)
			}
		}
	}
	// Regional actions: each exactly once per allowed region and not in us-east-1.
	for _, region := range regions {
		seen := count(got[region])
		for _, action := range regional {
			if seen[action] != 1 {
				t.Errorf("regional action %s simulated %d times for %s", action, seen[action], region)
			}
		}
	}
	for _, action := range regional {
		if usEast[action] != 0 {
			t.Errorf("regional action %s was simulated for us-east-1, which is not allowed", action)
		}
	}
	// Both partitions together are exactly the required set: no group is dropped.
	if len(global)+len(regional) != len(requiredActions()) {
		t.Fatal("partition lost an action")
	}
}

// iamOnlyUnder returns a policy model: iam: actions are allowed only when the
// request carries aws:RequestedRegion=region, everything else is always allowed.
func iamOnlyUnder(region string) func(string, []string) (int, string) {
	return func(requested string, actions []string) (int, string) {
		var b strings.Builder
		b.WriteString(`<SimulatePrincipalPolicyResponse><SimulatePrincipalPolicyResult><IsTruncated>false</IsTruncated><EvaluationResults>`)
		for _, a := range actions {
			decision := "allowed"
			if isGlobalAction(a) && requested != region {
				decision = "implicitDeny"
			}
			b.WriteString(member(a, decision, ""))
		}
		b.WriteString(`</EvaluationResults></SimulatePrincipalPolicyResult></SimulatePrincipalPolicyResponse>`)
		return http.StatusOK, b.String()
	}
}

// False READY: the policy allows iam:* only under RequestedRegion=eu-west-1.
// Real IAM calls always carry us-east-1, so they would be denied; an eu-west-1
// only Connection must therefore not be accepted.
func TestIAMAllowedOnlyUnderTheDeployRegionIsNotReady(t *testing.T) {
	f := newFakeAWS()
	f.Simulate = iamOnlyUnder("eu-west-1")
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"eu-west-1"}); !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("an eu-west-1 only key whose IAM permission applies only under eu-west-1 was accepted: %v", err)
	}
	for _, request := range f.SimulationRequests() {
		for _, action := range request.Actions {
			if isGlobalAction(action) && request.Region != "us-east-1" {
				t.Fatalf("%s simulated under %s", action, request.Region)
			}
		}
	}
}

// False rejection: the policy allows iam:* when the request region is
// us-east-1 (where real IAM calls go) and regional services everywhere. A
// multi-region key must be accepted; simulating IAM under eu-west-1 would
// have rejected it.
func TestIAMAllowedUnderUSEast1AcceptsMultiRegionKeys(t *testing.T) {
	f := newFakeAWS()
	f.Simulate = iamOnlyUnder("us-east-1")
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"eu-west-1", "ap-south-1"}); err != nil {
		t.Fatalf("a valid multi-region key was rejected: %v", err)
	}
	if _, err := client(t, newFakeAWSWith(iamOnlyUnder("us-east-1"))).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err != nil {
		t.Fatalf("a us-east-1 key was rejected: %v", err)
	}
}

func newFakeAWSWith(sim func(string, []string) (int, string)) *awsfake.Server {
	f := newFakeAWS()
	f.Simulate = sim
	return f
}

func TestRegionalCreateDenyFailsEvenWhenReadsAndOtherRegionsPass(t *testing.T) {
	f := newFakeAWS()
	f.DenyRegionCapabilities["eu-west-1"] = true // e.g. an SCP or condition denying creates there
	_, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1", "eu-west-1"})
	if !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("a region whose creates are denied was accepted: %v", err)
	}
	// The same key is fine when that region is not requested.
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err != nil {
		t.Fatalf("unrequested region must not matter: %v", err)
	}
}

func member(action, decision, extra string) string {
	return `<member><EvalActionName>` + action + `</EvalActionName><EvalDecision>` + decision + `</EvalDecision>` + extra + `</member>`
}

// rewrite builds an answer where mutate may change the first result of the first call.
func answerWith(extra string) func(string, []string) (int, string) {
	var first atomic.Bool
	return func(_ string, actions []string) (int, string) {
		var b strings.Builder
		b.WriteString(`<SimulatePrincipalPolicyResponse><SimulatePrincipalPolicyResult><IsTruncated>false</IsTruncated><EvaluationResults>`)
		for _, a := range actions {
			e := ""
			if first.CompareAndSwap(false, true) {
				e = extra
			}
			b.WriteString(member(a, "allowed", e))
		}
		b.WriteString(`</EvaluationResults></SimulatePrincipalPolicyResult></SimulatePrincipalPolicyResponse>`)
		return http.StatusOK, b.String()
	}
}

func TestMissingContextValuesAreUnprovenNotAllowed(t *testing.T) {
	f := newFakeAWS()
	f.Simulate = answerWith(`<MissingContextValues><member>aws:SourceIp</member></MissingContextValues>`)
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrCapabilityUnverifiable) {
		t.Fatalf("missing context values: %v", err)
	}
}

func TestOrganizationDenialIsADenial(t *testing.T) {
	f := newFakeAWS()
	f.Simulate = answerWith(`<OrganizationsDecisionDetail><AllowedByOrganizations>false</AllowedByOrganizations></OrganizationsDecisionDetail>`)
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("organization denial: %v", err)
	}
	f = newFakeAWS()
	f.Simulate = answerWith(`<OrganizationsDecisionDetail><AllowedByOrganizations>true</AllowedByOrganizations></OrganizationsDecisionDetail>`)
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err != nil {
		t.Fatalf("organization allow: %v", err)
	}
}

func TestSimulationPagingFailsClosed(t *testing.T) {
	truncated := func(marker string) func(string, []string) (int, string) {
		return func(_ string, actions []string) (int, string) {
			_, body := awsfake.AllowAll("", actions)
			body = strings.Replace(body, "<IsTruncated>false</IsTruncated>", "<IsTruncated>true</IsTruncated>"+marker, 1)
			return http.StatusOK, body
		}
	}
	// Truncated without a marker: results are missing.
	f := newFakeAWS()
	f.Simulate = truncated("")
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrCapabilityUnverifiable) {
		t.Fatalf("truncated without marker: %v", err)
	}
	// A server that never stops paging is cut off, not trusted.
	f = newFakeAWS()
	f.Simulate = truncated("<Marker>again</Marker>")
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrCapabilityUnverifiable) {
		t.Fatalf("endless paging: %v", err)
	}
	// A real continuation is followed and its results count.
	f = newFakeAWS()
	var mu sync.Mutex
	pages := map[string]int{}
	f.Simulate = func(region string, actions []string) (int, string) {
		mu.Lock()
		defer mu.Unlock()
		key := region + actions[0]
		pages[key]++
		half := len(actions) / 2
		if pages[key]%2 == 1 {
			_, body := awsfake.AllowAll("", actions[:half])
			return http.StatusOK, strings.Replace(body, "<IsTruncated>false</IsTruncated>", "<IsTruncated>true</IsTruncated><Marker>m2</Marker>", 1)
		}
		return awsfake.AllowAll("", actions[half:])
	}
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err != nil {
		t.Fatalf("paged simulation: %v", err)
	}
}

func TestAnyDeniedResultForAnActionDenies(t *testing.T) {
	f := newFakeAWS()
	f.Simulate = func(_ string, actions []string) (int, string) {
		_, body := awsfake.AllowAll("", actions)
		extra := member(actions[0], "explicitDeny", "")
		return http.StatusOK, strings.Replace(body, "</EvaluationResults>", extra+"</EvaluationResults>", 1)
	}
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrPermissionDenied) {
		t.Fatalf("a later deny for an allowed action: %v", err)
	}
}

func TestSignedRequestsNeverFollowRedirects(t *testing.T) {
	var landed atomic.Int32
	var landedAuth atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		landed.Add(1)
		landedAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"GetCallerIdentityResponse":{"GetCallerIdentityResult":{"Account":"123456789012","Arn":"` + fakeUserARN + `"}}}`))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	c := &Client{
		HTTP:     redirector.Client(),
		STSURL:   func(string) string { return redirector.URL + "/" },
		EKSURL:   func(string) string { return redirector.URL },
		QueryURL: func(string, string) string { return redirector.URL + "/" },
		IAMURL:   func() string { return redirector.URL + "/" },
	}
	if _, err := c.VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err == nil {
		t.Fatal("a redirected verification succeeded")
	}
	if landed.Load() != 0 {
		t.Fatalf("the redirect destination received %d signed request(s), Authorization %v", landed.Load(), landedAuth.Load())
	}
	// The other signed calls refuse redirects too.
	if _, err := c.CallerAccount(context.Background(), conn()); err == nil || landed.Load() != 0 {
		t.Fatalf("CallerAccount followed a redirect: %v (landed %d)", err, landed.Load())
	}
	if _, err := c.DescribeCluster(context.Background(), conn(), "orc-cluster"); err == nil || landed.Load() != 0 {
		t.Fatalf("DescribeCluster followed a redirect: %v (landed %d)", err, landed.Load())
	}
	if _, err := c.QueryStatus(context.Background(), conn(), EC2Vpc, "vpc-123"); err == nil || landed.Load() != 0 {
		t.Fatalf("QueryStatus followed a redirect: %v (landed %d)", err, landed.Load())
	}
	// A redirecting caller-supplied client is not mutated.
	if redirector.Client().CheckRedirect != nil {
		t.Fatal("the shared client was modified")
	}
}

func TestCapabilityGroupNamesAreTheConnectionContract(t *testing.T) {
	var names []string
	for _, group := range capabilityGroups {
		names = append(names, group.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(connection.RequiredAWSCapabilities, ",") {
		t.Fatalf("groups %v, contract %v", names, connection.RequiredAWSCapabilities)
	}
	seen := map[string]bool{}
	for _, action := range requiredActions() {
		if seen[action] || !regexp.MustCompile(`^[a-z0-9-]+:[A-Za-z0-9]+$`).MatchString(action) {
			t.Fatalf("duplicate or malformed action %q", action)
		}
		seen[action] = true
	}
}

// moduleActions maps every resource type of the aws modules to the actions
// Terraform calls for it (create, update, delete, read, tag). Adding a
// resource to a module without extending this table fails the test, so the
// capability list cannot silently fall behind the modules.
var moduleActions = map[string][]string{
	"aws_vpc":                           {"ec2:CreateVpc", "ec2:DeleteVpc", "ec2:DescribeVpcs", "ec2:DescribeVpcAttribute", "ec2:ModifyVpcAttribute", "ec2:CreateTags"},
	"aws_default_security_group":        {"ec2:DescribeSecurityGroups", "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress"},
	"aws_internet_gateway":              {"ec2:CreateInternetGateway", "ec2:DeleteInternetGateway", "ec2:AttachInternetGateway", "ec2:DetachInternetGateway", "ec2:DescribeInternetGateways"},
	"aws_subnet":                        {"ec2:CreateSubnet", "ec2:DeleteSubnet", "ec2:DescribeSubnets", "ec2:ModifySubnetAttribute", "ec2:DescribeNetworkInterfaces"},
	"aws_route_table":                   {"ec2:CreateRouteTable", "ec2:DeleteRouteTable", "ec2:DescribeRouteTables", "ec2:CreateRoute", "ec2:DeleteRoute"},
	"aws_route_table_association":       {"ec2:AssociateRouteTable", "ec2:DisassociateRouteTable"},
	"aws_security_group":                {"ec2:CreateSecurityGroup", "ec2:DeleteSecurityGroup", "ec2:DescribeSecurityGroups", "ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress", "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress"},
	"aws_iam_role":                      {"iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:TagRole", "iam:UntagRole", "iam:ListRoleTags", "iam:ListRolePolicies", "iam:ListInstanceProfilesForRole", "iam:PassRole"},
	"aws_iam_role_policy_attachment":    {"iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:ListAttachedRolePolicies"},
	"aws_eks_cluster":                   {"eks:CreateCluster", "eks:DeleteCluster", "eks:DescribeCluster", "eks:UpdateClusterConfig", "eks:UpdateClusterVersion", "eks:TagResource", "eks:UntagResource"},
	"aws_eks_access_entry":              {"eks:CreateAccessEntry", "eks:DeleteAccessEntry", "eks:DescribeAccessEntry"},
	"aws_eks_access_policy_association": {"eks:AssociateAccessPolicy", "eks:DisassociateAccessPolicy", "eks:ListAssociatedAccessPolicies"},
	"aws_eks_node_group":                {"eks:CreateNodegroup", "eks:DeleteNodegroup", "eks:DescribeNodegroup", "eks:UpdateNodegroupConfig"},
	"aws_eks_addon":                     {"eks:CreateAddon", "eks:DeleteAddon", "eks:DescribeAddon", "eks:UpdateAddon", "eks:CreatePodIdentityAssociation", "eks:DeletePodIdentityAssociation", "eks:DescribePodIdentityAssociation", "eks:ListPodIdentityAssociations"},
	"aws_elasticache_subnet_group":      {"elasticache:CreateCacheSubnetGroup", "elasticache:DeleteCacheSubnetGroup", "elasticache:DescribeCacheSubnetGroups"},
	"aws_elasticache_user":              {"elasticache:CreateUser", "elasticache:DeleteUser", "elasticache:DescribeUsers", "elasticache:ModifyUser"},
	"aws_elasticache_user_group":        {"elasticache:CreateUserGroup", "elasticache:DeleteUserGroup", "elasticache:DescribeUserGroups", "elasticache:ModifyUserGroup"},
	"aws_elasticache_replication_group": {"elasticache:CreateReplicationGroup", "elasticache:DeleteReplicationGroup", "elasticache:DescribeReplicationGroups", "elasticache:ModifyReplicationGroup", "elasticache:DescribeCacheClusters", "elasticache:AddTagsToResource", "elasticache:RemoveTagsFromResource", "elasticache:ListTagsForResource"},
	"aws_db_subnet_group":               {"rds:CreateDBSubnetGroup", "rds:DeleteDBSubnetGroup", "rds:DescribeDBSubnetGroups", "rds:AddTagsToResource", "rds:RemoveTagsFromResource", "rds:ListTagsForResource"},
	"aws_rds_cluster":                   {"rds:CreateDBCluster", "rds:DeleteDBCluster", "rds:DescribeDBClusters", "rds:ModifyDBCluster"},
	"aws_rds_cluster_instance":          {"rds:CreateDBInstance", "rds:DeleteDBInstance", "rds:DescribeDBInstances", "rds:ModifyDBInstance"},
}

func TestCapabilityListCoversModuleResources(t *testing.T) {
	have := map[string]bool{}
	for _, action := range requiredActions() {
		have[action] = true
	}
	resource := regexp.MustCompile(`(?m)^resource "([a-z0-9_]+)"`)
	used := map[string]bool{}
	for _, module := range []string{"vpc", "eks", "elasticache", "aurora"} {
		raw, err := os.ReadFile(filepath.Join("..", "terraform", "modules", "modules", module, "main.tf"))
		if err != nil {
			raw, err = os.ReadFile(filepath.Join("..", "terraform", "modules", module, "main.tf"))
		}
		if err != nil {
			t.Fatalf("module %s: %v", module, err)
		}
		for _, m := range resource.FindAllStringSubmatch(string(raw), -1) {
			if !strings.HasPrefix(m[1], "aws_") {
				continue
			}
			actions, known := moduleActions[m[1]]
			if !known {
				t.Errorf("module %s uses %s which has no entry in moduleActions: review the capability list", module, m[1])
				continue
			}
			used[m[1]] = true
			for _, action := range actions {
				if !have[action] {
					t.Errorf("%s (%s) needs %s which the capability list lacks", m[1], module, action)
				}
			}
		}
	}
	// Every action in the list is justified by a module resource: no
	// permission expansion beyond what the modules call.
	justified := map[string]bool{"iam:CreateServiceLinkedRole": true, "ec2:DescribeAvailabilityZones": true, "ec2:DescribeAccountAttributes": true, "ec2:DeleteTags": true, "ec2:CreateTags": true, "ec2:DescribeNetworkInterfaces": true}
	for _, actions := range moduleActions {
		for _, action := range actions {
			justified[action] = true
		}
	}
	for action := range have {
		if !justified[action] {
			t.Errorf("%s is in the capability list but no module resource needs it", action)
		}
	}
	for resourceType := range moduleActions {
		if !used[resourceType] {
			t.Errorf("moduleActions lists %s which no module uses any more", resourceType)
		}
	}
}
