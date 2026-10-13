package awsaccess

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/internal/adapters/awsaccess/awsfake"
	"orchestrator/internal/application/connection"
)

// FAKE AWS (httptest). These tests prove request shape, read-only behavior,
// classification and secret hygiene of the verifier. They are not evidence
// that a real AWS account accepts the checks (V2-A01/A02/A03 need a real run).

const (
	fakeAccount = "123456789012"
	fakeUserARN = "arn:aws:iam::123456789012:user/orchestrator"
	fakeKeyID   = "AKIAFAKEFIXTURE00001"
	fakeSecret  = "fixture/secret/key/0000000000000000000x"
)

func newFakeAWS() *awsfake.Server {
	f := awsfake.New()
	f.Accept = func(keyID string) bool { return keyID == fakeKeyID }
	return f
}

func xmlError(code string) string { return awsfake.XMLError(code) }

func allowAll(actions []string) (int, string) { return awsfake.AllowAll("", actions) }

func client(t *testing.T, f *awsfake.Server) *Client {
	t.Helper()
	srv := f.Start()
	t.Cleanup(srv.Close)
	return &Client{
		HTTP:     srv.Client(),
		STSURL:   func(string) string { return srv.URL + "/" },
		EKSURL:   func(string) string { return srv.URL },
		QueryURL: func(string, string) string { return srv.URL + "/" },
		IAMURL:   func() string { return srv.URL + "/" },
		Now:      func() time.Time { return time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC) },
	}
}

func fakeKey() connection.AWSAccessKey {
	return connection.AWSAccessKey{AccessKeyID: fakeKeyID, SecretAccessKey: fakeSecret}
}

func TestVerifyAccessKeySucceedsOnlyWithIdentityRegionsAndCapabilities(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAHOSTCHAINKEY00001")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "host-chain-secret")
	t.Setenv("AWS_PROFILE", "host")
	f := newFakeAWS()
	got, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1", "eu-west-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountID != fakeAccount || got.PrincipalARN != fakeUserARN || len(got.Regions) != 2 || got.Regions[0] != "us-east-1" {
		t.Fatalf("verification: %+v", got)
	}
	if strings.Join(got.Capabilities, ",") != "Aurora,EKS,ElastiCache,VPC" {
		t.Fatalf("capabilities: %v", got.Capabilities)
	}
	for _, auth := range f.Authorizations() {
		if !strings.Contains(auth, "Credential="+fakeKeyID+"/") || strings.Contains(auth, "HOSTCHAIN") || strings.Contains(auth, fakeSecret) {
			t.Fatalf("authorization: %q", auth)
		}
	}
}

func TestVerifyAccessKeyIsReadOnly(t *testing.T) {
	f := newFakeAWS()
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"GetCallerIdentity": true, "DescribeVpcs": true, "DescribeReplicationGroups": true,
		"DescribeDBClusters": true, "SimulatePrincipalPolicy": true,
	}
	sawSimulate := false
	for _, call := range f.Calls() {
		fields := strings.Fields(call)
		if fields[0] == "eks" {
			if strings.Join(fields, " ") != "eks GET /clusters" {
				t.Fatalf("eks call %q", call)
			}
			continue
		}
		if len(fields) != 4 || fields[1] != "POST" {
			t.Fatalf("unexpected call shape %q", call)
		}
		if !allowed[fields[2]] {
			t.Fatalf("a non read-only call was made: %q", call)
		}
		if fields[2] == "SimulatePrincipalPolicy" {
			sawSimulate = true
		}
		for _, verb := range []string{"Create", "Delete", "Put", "Run", "Modify", "Attach", "Update"} {
			if strings.Contains(fields[2], verb) {
				t.Fatalf("mutating call %q", call)
			}
		}
	}
	if !sawSimulate {
		t.Fatal("capabilities were never evaluated")
	}
}

func TestSTSAloneIsNotEnough(t *testing.T) {
	// The key is valid and every probe works, but the principal may not
	// simulate its policy: identity and regions are proven, capabilities are
	// not, so there is no result.
	f := newFakeAWS()
	f.Simulate = func(string, []string) (int, string) { return http.StatusForbidden, xmlError("AccessDenied") }
	got, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"})
	if !errors.Is(err, connection.ErrCapabilityUnverifiable) || got.AccountID != "" {
		t.Fatalf("%+v %v", got, err)
	}
	// Incomplete simulation output is unproven, not allowed.
	f = newFakeAWS()
	f.Simulate = func(_ string, actions []string) (int, string) { return allowAll(actions[:len(actions)-1]) }
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrCapabilityUnverifiable) {
		t.Fatalf("incomplete simulation: %v", err)
	}
	// Garbage output is unproven too.
	f = newFakeAWS()
	f.Simulate = func(string, []string) (int, string) { return http.StatusOK, "not xml" }
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrCapabilityUnverifiable) {
		t.Fatalf("garbage simulation: %v", err)
	}
}

func TestMissingCapabilityIsPermissionDenied(t *testing.T) {
	for _, decision := range []string{"implicitDeny", "explicitDeny"} {
		f := newFakeAWS()
		f.Simulate = func(_ string, actions []string) (int, string) {
			_, body := allowAll(actions)
			return http.StatusOK, strings.Replace(body, "<EvalDecision>allowed</EvalDecision>", "<EvalDecision>"+decision+"</EvalDecision>", 1)
		}
		_, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"})
		if !errors.Is(err, connection.ErrPermissionDenied) {
			t.Fatalf("%s: %v", decision, err)
		}
	}
}

func TestSimulationIsPagedAndCoversEveryModuleAction(t *testing.T) {
	f := newFakeAWS()
	var asked []string
	var mu sync.Mutex
	f.Simulate = func(_ string, actions []string) (int, string) {
		mu.Lock()
		asked = append(asked, actions...)
		mu.Unlock()
		if len(actions) > simulateChunk {
			t.Errorf("chunk of %d actions", len(actions))
		}
		return allowAll(actions)
	}
	if _, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, a := range asked {
		have[a] = true
	}
	for _, group := range capabilityGroups {
		for _, action := range group.Actions {
			if !have[action] {
				t.Errorf("%s action %s never simulated", group.Name, action)
			}
		}
	}
	for _, mustHave := range []string{"ec2:CreateVpc", "eks:CreateCluster", "elasticache:CreateReplicationGroup", "rds:CreateDBCluster", "iam:PassRole", "iam:CreateRole"} {
		if !have[mustHave] {
			t.Errorf("required action %s is not part of the capability list", mustHave)
		}
	}
}

func TestVerificationFailureCategories(t *testing.T) {
	cases := map[string]struct {
		mutate  func(*awsfake.Server)
		regions []string
		want    error
	}{
		"rejected key":    {func(f *awsfake.Server) { f.Rules[fakeKeyID] = awsfake.RuleReject }, []string{"us-east-1"}, connection.ErrAuthentication},
		"disabled region": {func(f *awsfake.Server) { f.Disabled["ap-east-1"] = true }, []string{"us-east-1", "ap-east-1"}, connection.ErrRegionUnavailable},
		"other account":   {func(f *awsfake.Server) { f.RegionAccount = map[string]string{"eu-west-1": "999999999999"} }, []string{"us-east-1", "eu-west-1"}, connection.ErrRegionUnavailable},
		"ec2 read denied": {func(f *awsfake.Server) { f.DenyRead["ec2"] = "UnauthorizedOperation" }, []string{"us-east-1"}, connection.ErrPermissionDenied},
		"eks read denied": {func(f *awsfake.Server) { f.DenyRead["eks"] = "AccessDeniedException" }, []string{"us-east-1"}, connection.ErrPermissionDenied},
		"rds read denied": {func(f *awsfake.Server) { f.DenyRead["rds"] = "AccessDenied" }, []string{"us-east-1"}, connection.ErrPermissionDenied},
		"opt-in required": {func(f *awsfake.Server) { f.DenyRead["ec2"] = "OptInRequired" }, []string{"us-east-1"}, connection.ErrRegionUnavailable},
		"root principal":  {func(f *awsfake.Server) { f.ARN = "arn:aws:iam::123456789012:root" }, []string{"us-east-1"}, connection.ErrUnsupportedPrincipal},
		"assumed role":    {func(f *awsfake.Server) { f.ARN = "arn:aws:sts::123456789012:assumed-role/r/s" }, []string{"us-east-1"}, connection.ErrUnsupportedPrincipal},
	}
	for name, tc := range cases {
		f := newFakeAWS()
		tc.mutate(f)
		_, err := client(t, f).VerifyAccessKey(context.Background(), fakeKey(), tc.regions)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
		if err != nil && (strings.Contains(err.Error(), "SECRET-AWS-DETAIL") || strings.Contains(err.Error(), fakeSecret)) {
			t.Errorf("%s: error leaks: %v", name, err)
		}
	}
}

func TestVerificationUnreachableAndInputGuards(t *testing.T) {
	c := &Client{HTTP: &http.Client{Timeout: 200 * time.Millisecond}, STSURL: func(string) string { return "http://127.0.0.1:1/" }}
	if _, err := c.VerifyAccessKey(context.Background(), fakeKey(), []string{"us-east-1"}); !errors.Is(err, connection.ErrClusterUnreachable) {
		t.Fatalf("unreachable: %v", err)
	}
	f := newFakeAWS()
	cl := client(t, f)
	for name, args := range map[string]struct {
		key     connection.AWSAccessKey
		regions []string
	}{
		"empty key":  {connection.AWSAccessKey{}, []string{"us-east-1"}},
		"no regions": {fakeKey(), nil},
		"bad region": {fakeKey(), []string{"../x"}},
	} {
		if _, err := cl.VerifyAccessKey(context.Background(), args.key, args.regions); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if len(f.Calls()) != 0 {
		t.Fatalf("rejected input reached AWS: %v", f.Calls())
	}
}

func TestSimulateResponseParsing(t *testing.T) {
	var doc simulateResponse
	body := `<SimulatePrincipalPolicyResponse><SimulatePrincipalPolicyResult><IsTruncated>true</IsTruncated><Marker>M1</Marker><EvaluationResults><member><EvalActionName>a:B</EvalActionName><EvalDecision>allowed</EvalDecision></member></EvaluationResults></SimulatePrincipalPolicyResult></SimulatePrincipalPolicyResponse>`
	if err := xml.Unmarshal([]byte(body), &doc); err != nil || len(doc.Results) != 1 || doc.Marker != "M1" || doc.Truncated != "true" {
		t.Fatalf("%+v %v", doc, err)
	}
}
