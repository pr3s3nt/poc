// Package awsfake is a deterministic FAKE of the AWS HTTP endpoints the
// read-only verifier calls (STS, EC2, EKS, ElastiCache, RDS and IAM policy
// simulation). It is test support only: it serves canned answers over plain
// HTTP on a local listener and never reaches AWS. A verification that passes
// against it proves request shape, classification and secret handling, not
// that a real AWS account accepts anything.
package awsfake

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Rule is the outcome the fake gives one access key ID.
type Rule string

// Rules by access key ID. A key without a rule is accepted when Accept allows
// it (every key when Accept is nil).
const (
	// RuleReject answers every call as an invalid access key.
	RuleReject Rule = "reject"
	// RuleNoCapabilities makes IAM simulation report implicit denies.
	RuleNoCapabilities Rule = "no-capabilities"
	// RuleNoSimulate denies the principal the right to simulate its policy.
	RuleNoSimulate Rule = "no-simulate"
)

// Server is the fake. Configure fields before use; they must not change while
// requests are in flight, except through Calls.
type Server struct {
	Account string
	ARN     string
	// Accept decides which access key IDs are valid; nil accepts every key.
	Accept func(keyID string) bool
	Rules  map[string]Rule
	// Disabled regions answer an invalid-token error to regional calls.
	Disabled map[string]bool
	// DenyRead maps a service signing name to the error code its read probe returns.
	DenyRead map[string]string
	// RegionAccount maps a region to the account its regional STS reports.
	RegionAccount map[string]string
	// KeyAccount maps an access key ID to the account its STS identity reports
	// (the principal ARN follows the account); other keys report Account.
	KeyAccount map[string]string
	// Simulate overrides the IAM simulation answer for one requested region.
	Simulate func(region string, actions []string) (status int, body string)
	// DenyRegionCapabilities makes simulation for these regions (the
	// aws:RequestedRegion context value) report implicit denies.
	DenyRegionCapabilities map[string]bool

	mu       sync.Mutex
	calls    []string
	auths    []string
	regions  []string
	requests []SimulationRequest
}

// SimulationRequest is one IAM simulation request: the aws:RequestedRegion
// context value and the actions asked.
type SimulationRequest struct {
	Region  string
	Actions []string
}

// SimulationRequests returns every IAM simulation request so far.
func (s *Server) SimulationRequests() []SimulationRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SimulationRequest(nil), s.requests...)
}

// New returns a fake for one account and user principal.
func New() *Server {
	return &Server{Account: "123456789012", ARN: "arn:aws:iam::123456789012:user/orchestrator", Rules: map[string]Rule{}, Disabled: map[string]bool{}, DenyRead: map[string]string{}, DenyRegionCapabilities: map[string]bool{}}
}

// Calls returns "service METHOD Action path" for every request so far.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// Authorizations returns the Authorization header of every request so far.
func (s *Server) Authorizations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.auths...)
}

// XMLError renders a Query-protocol error response.
func XMLError(code string) string {
	return `<ErrorResponse><Error><Type>Sender</Type><Code>` + code + `</Code><Message>SECRET-AWS-DETAIL</Message></Error></ErrorResponse>`
}

// AllowAll answers a simulation with every action allowed.
func AllowAll(_ string, actions []string) (int, string) { return decide(actions, "allowed") }

// SimulatedRegions returns the aws:RequestedRegion value of every simulation
// request so far.
func (s *Server) SimulatedRegions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.regions...)
}

func decide(actions []string, decision string) (int, string) {
	var b strings.Builder
	b.WriteString(`<SimulatePrincipalPolicyResponse><SimulatePrincipalPolicyResult><IsTruncated>false</IsTruncated><EvaluationResults>`)
	for _, a := range actions {
		b.WriteString(`<member><EvalActionName>` + a + `</EvalActionName><EvalDecision>` + decision + `</EvalDecision></member>`)
	}
	b.WriteString(`</EvaluationResults></SimulatePrincipalPolicyResult></SimulatePrincipalPolicyResponse>`)
	return http.StatusOK, b.String()
}

func (s *Server) record(entry, auth string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, entry)
	s.auths = append(s.auths, auth)
}

// Handler serves every service on one listener. The service and region are
// read from the Credential scope of the Authorization header.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		scope := ""
		if _, after, ok := strings.Cut(auth, "Credential="); ok {
			scope, _, _ = strings.Cut(after, ",")
		}
		parts := strings.Split(scope, "/")
		if len(parts) != 5 || !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(XMLError("IncompleteSignature")))
			return
		}
		keyID, region, service := parts[0], parts[2], parts[3]
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		action := form.Get("Action")
		s.record(service+" "+r.Method+" "+action+" "+r.URL.Path, auth)
		rule := s.Rules[keyID]
		if rule == RuleReject || (s.Accept != nil && !s.Accept(keyID)) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(XMLError("InvalidClientTokenId")))
			return
		}
		if service != "iam" && s.Disabled[region] {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(XMLError("InvalidClientTokenId")))
			return
		}
		if code := s.DenyRead[service]; code != "" && service != "sts" && service != "iam" {
			if service == "eks" {
				w.Header().Set("X-Amzn-Errortype", code)
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"SECRET-AWS-DETAIL"}`))
				return
			}
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(XMLError(code)))
			return
		}
		switch service {
		case "sts":
			account := s.Account
			if other, ok := s.RegionAccount[region]; ok {
				account = other
			}
			arn := s.ARN
			if other, ok := s.KeyAccount[keyID]; ok {
				account, arn = other, "arn:aws:iam::"+other+":user/orchestrator"
			}
			_, _ = w.Write([]byte(`{"GetCallerIdentityResponse":{"GetCallerIdentityResult":{"Account":"` + account + `","Arn":"` + arn + `","UserId":"U"}}}`))
		case "ec2":
			_, _ = w.Write([]byte(`<DescribeVpcsResponse><vpcSet/></DescribeVpcsResponse>`))
		case "eks":
			_, _ = w.Write([]byte(`{"clusters":[]}`))
		case "elasticache":
			_, _ = w.Write([]byte(`<DescribeReplicationGroupsResponse/>`))
		case "rds":
			_, _ = w.Write([]byte(`<DescribeDBClustersResponse/>`))
		case "iam":
			var actions []string
			for i := 1; ; i++ {
				a := form.Get("ActionNames.member." + strconv.Itoa(i))
				if a == "" {
					break
				}
				actions = append(actions, a)
			}
			requested := form.Get("ContextEntries.member.1.ContextKeyValues.member.1")
			principal := s.ARN
			if other, ok := s.KeyAccount[keyID]; ok {
				principal = "arn:aws:iam::" + other + ":user/orchestrator"
			}
			if form.Get("PolicySourceArn") != principal || form.Get("ResourceArns.member.1") != "*" ||
				form.Get("ContextEntries.member.1.ContextKeyName") != "aws:RequestedRegion" || requested == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			s.regions = append(s.regions, requested)
			s.requests = append(s.requests, SimulationRequest{Region: requested, Actions: append([]string(nil), actions...)})
			s.mu.Unlock()
			switch {
			case rule == RuleNoSimulate:
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(XMLError("AccessDenied")))
			case rule == RuleNoCapabilities || s.DenyRegionCapabilities[requested]:
				status, body := decide(actions, "implicitDeny")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			default:
				sim := s.Simulate
				if sim == nil {
					sim = AllowAll
				}
				status, body := sim(requested, actions)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// Start serves the fake on a local test listener.
func (s *Server) Start() *httptest.Server { return httptest.NewServer(s.Handler()) }
