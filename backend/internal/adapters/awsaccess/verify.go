package awsaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/application/connection"
)

// This file implements connection.AWSVerifier (UC-04 VAR-02, BR-12) with
// signed, READ-ONLY AWS calls and the explicit access key only. It proves:
//
//  1. identity: STS GetCallerIdentity returns the account and the IAM user
//     principal (root and temporary principals are refused);
//  2. regions: in every allowed region STS answers for the same account and a
//     read-only Describe/List call of each service the modules use succeeds,
//     so a disabled region or an organization policy that blocks it fails;
//  3. capabilities: IAM SimulatePrincipalPolicy evaluates the create/modify/
//     delete actions the VPC, EKS, ElastiCache and Aurora modules need and
//     every one must be "allowed".
//
// STS success alone never produces a result. Nothing is created: the IAM
// simulation evaluates the principal's policies without calling the services.
// If the principal may not simulate its own policies the capabilities are
// unproven and verification fails closed with ErrCapabilityUnverifiable.
// IAM simulation covers identity-based policies and the permissions boundary;
// it does not evaluate organization SCPs, so the region probes and the
// execution-time STS/account check complement it. This is registration-time
// evidence only (BR-13); execution preflight must revalidate.

const (
	identityRegion = "us-east-1"
	iamVersion     = "2010-05-08"
	simulateChunk  = 50
	verifyTimeout  = 90 * time.Second
)

var (
	userARNPattern = regexp.MustCompile(`^arn:aws:iam::[0-9]{12}:user/[A-Za-z0-9+=,.@_/-]{1,512}$`)
	errorCodeXML   = regexp.MustCompile(`<Code>([A-Za-z0-9_.:-]{1,80})</Code>`)
	errorCodeJSON  = regexp.MustCompile(`"(?:Code|__type|code)"\s*:\s*"(?:[A-Za-z0-9_.#:-]*#)?([A-Za-z0-9_.:-]{1,80})"`)
)

// capabilityGroup is the set of IAM actions one family of modules needs.
type capabilityGroup struct {
	Name    string
	Actions []string
}

// capabilityGroups is derived from the resources of the aws modules (vpc, eks,
// elasticache, aurora): create, update, delete and the reads Terraform
// refreshes with. It is the list a READY AWS Connection proves.
var capabilityGroups = []capabilityGroup{
	{Name: "VPC", Actions: []string{
		"ec2:CreateVpc", "ec2:DeleteVpc", "ec2:DescribeVpcs", "ec2:DescribeVpcAttribute", "ec2:ModifyVpcAttribute",
		"ec2:CreateSubnet", "ec2:DeleteSubnet", "ec2:DescribeSubnets", "ec2:ModifySubnetAttribute",
		"ec2:CreateInternetGateway", "ec2:DeleteInternetGateway", "ec2:AttachInternetGateway", "ec2:DetachInternetGateway", "ec2:DescribeInternetGateways",
		"ec2:CreateRouteTable", "ec2:DeleteRouteTable", "ec2:CreateRoute", "ec2:DeleteRoute",
		"ec2:AssociateRouteTable", "ec2:DisassociateRouteTable", "ec2:DescribeRouteTables",
		"ec2:CreateSecurityGroup", "ec2:DeleteSecurityGroup", "ec2:DescribeSecurityGroups",
		"ec2:AuthorizeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress", "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress",
		"ec2:CreateTags", "ec2:DeleteTags", "ec2:DescribeAvailabilityZones", "ec2:DescribeNetworkInterfaces",
	}},
	{Name: "EKS", Actions: []string{
		"eks:CreateCluster", "eks:DeleteCluster", "eks:DescribeCluster", "eks:UpdateClusterConfig", "eks:UpdateClusterVersion", "eks:TagResource",
		"eks:CreateNodegroup", "eks:DeleteNodegroup", "eks:DescribeNodegroup", "eks:UpdateNodegroupConfig",
		"eks:CreateAddon", "eks:DeleteAddon", "eks:DescribeAddon", "eks:DescribeAddonVersions", "eks:UpdateAddon",
		"eks:CreateAccessEntry", "eks:DeleteAccessEntry", "eks:DescribeAccessEntry", "eks:AssociateAccessPolicy", "eks:DisassociateAccessPolicy", "eks:ListAssociatedAccessPolicies",
		"iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:PassRole", "iam:TagRole",
		"iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:ListAttachedRolePolicies", "iam:ListRolePolicies", "iam:ListInstanceProfilesForRole",
		"iam:CreateServiceLinkedRole",
	}},
	{Name: "ElastiCache", Actions: []string{
		"elasticache:CreateReplicationGroup", "elasticache:DeleteReplicationGroup", "elasticache:DescribeReplicationGroups", "elasticache:ModifyReplicationGroup",
		"elasticache:CreateCacheSubnetGroup", "elasticache:DeleteCacheSubnetGroup", "elasticache:DescribeCacheSubnetGroups",
		"elasticache:CreateUser", "elasticache:DeleteUser", "elasticache:DescribeUsers", "elasticache:ModifyUser",
		"elasticache:CreateUserGroup", "elasticache:DeleteUserGroup", "elasticache:DescribeUserGroups", "elasticache:ModifyUserGroup",
		"elasticache:DescribeCacheClusters", "elasticache:AddTagsToResource", "elasticache:ListTagsForResource",
	}},
	{Name: "Aurora", Actions: []string{
		"rds:CreateDBCluster", "rds:DeleteDBCluster", "rds:DescribeDBClusters", "rds:ModifyDBCluster",
		"rds:CreateDBInstance", "rds:DeleteDBInstance", "rds:DescribeDBInstances", "rds:ModifyDBInstance",
		"rds:CreateDBSubnetGroup", "rds:DeleteDBSubnetGroup", "rds:DescribeDBSubnetGroups",
		"rds:AddTagsToResource", "rds:ListTagsForResource",
	}},
}

// regionProbe is one read-only call per service and region.
type regionProbe struct {
	service string // signing name
	query   *probeQuery
	rest    string // EKS path and query
}

type probeQuery struct {
	action, version string
	extra           url.Values
}

var regionProbes = []regionProbe{
	{service: "ec2", query: &probeQuery{action: "DescribeVpcs", version: "2016-11-15", extra: url.Values{"MaxResults": {"5"}}}},
	{service: "eks", rest: "/clusters?maxResults=1"},
	{service: "elasticache", query: &probeQuery{action: "DescribeReplicationGroups", version: "2015-02-02", extra: url.Values{"MaxRecords": {"20"}}}},
	{service: "rds", query: &probeQuery{action: "DescribeDBClusters", version: "2014-10-31", extra: url.Values{"MaxRecords": {"20"}}}},
}

// Verifier adapts Client to connection.AWSVerifier.
type Verifier struct{ Client *Client }

var _ connection.AWSVerifier = Verifier{}

// VerifyAWS implements connection.AWSVerifier.
func (v Verifier) VerifyAWS(ctx context.Context, key connection.AWSAccessKey, regions []string) (connection.AWSVerification, error) {
	c := v.Client
	if c == nil {
		return connection.AWSVerification{}, connection.ErrClusterUnreachable
	}
	return c.VerifyAccessKey(ctx, key, regions)
}

// callResult is one signed call: HTTP status and a bounded body. The body is
// only inspected by this package and never returned to callers.
type callResult struct {
	status int
	body   []byte
	header http.Header
}

func (c *Client) iam() string {
	if c.IAMURL != nil {
		return c.IAMURL()
	}
	return "https://iam.amazonaws.com/"
}

func (c *Client) queryBase(service, host, region string) string {
	if c.QueryURL != nil {
		return c.QueryURL(service, region)
	}
	return "https://" + host + "." + region + ".amazonaws.com/"
}

func (c *Client) send(req *http.Request, payload []byte, creds Credentials, region, service string) (callResult, error) {
	Sign(req, payload, creds, region, service, c.now())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return callResult{}, connection.ErrClusterUnreachable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return callResult{}, connection.ErrClusterUnreachable
	}
	return callResult{status: resp.StatusCode, body: body, header: resp.Header}, nil
}

func (c *Client) post(ctx context.Context, endpoint string, form url.Values, creds Credentials, region, service string) (callResult, error) {
	payload := []byte(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return callResult{}, ErrCall
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	return c.send(req, payload, creds, region, service)
}

// errorCode extracts the AWS error code of a failed response for classification
// only. It is never returned.
func (r callResult) errorCode() string {
	if t := r.header.Get("X-Amzn-Errortype"); t != "" {
		t, _, _ = strings.Cut(t, ":")
		if _, after, ok := strings.Cut(t, "#"); ok {
			t = after
		}
		return t
	}
	if m := errorCodeXML.FindSubmatch(r.body); m != nil {
		return string(m[1])
	}
	if m := errorCodeJSON.FindSubmatch(r.body); m != nil {
		return string(m[1])
	}
	return ""
}

var (
	rejectedCodes = map[string]bool{
		"InvalidClientTokenId": true, "SignatureDoesNotMatch": true, "UnrecognizedClientException": true,
		"InvalidAccessKeyId": true, "AuthFailure": true, "IncompleteSignature": true, "ExpiredToken": true,
		"ExpiredTokenException": true, "InvalidSignatureException": true, "MissingAuthenticationToken": true,
		"InvalidToken": true,
	}
	deniedCodes = map[string]bool{
		"AccessDenied": true, "AccessDeniedException": true, "UnauthorizedOperation": true, "UnauthorizedAccess": true,
		"AuthorizationError": true, "NotAuthorized": true,
	}
	optInCodes = map[string]bool{"OptInRequired": true, "InvalidRegion": true}
)

// classify maps a non-200 result to the verification category. afterIdentity
// is true once the key itself was accepted by STS: a rejection from a
// regional service then means the region is not usable, not a wrong key.
func (r callResult) classify(afterIdentity bool) error {
	code := r.errorCode()
	switch {
	case optInCodes[code]:
		return connection.ErrRegionUnavailable
	case rejectedCodes[code]:
		if afterIdentity {
			return connection.ErrRegionUnavailable
		}
		return connection.ErrAuthentication
	case deniedCodes[code]:
		return connection.ErrPermissionDenied
	case r.status == http.StatusTooManyRequests || r.status >= 500:
		return connection.ErrClusterUnreachable
	case r.status == http.StatusUnauthorized:
		if afterIdentity {
			return connection.ErrRegionUnavailable
		}
		return connection.ErrAuthentication
	case r.status == http.StatusForbidden:
		return connection.ErrPermissionDenied
	}
	return ErrCall
}

type callerIdentity struct{ account, arn string }

func (c *Client) callerIdentity(ctx context.Context, creds Credentials, region string, afterIdentity bool) (callerIdentity, error) {
	form := url.Values{"Action": {"GetCallerIdentity"}, "Version": {"2011-06-15"}}
	payload := []byte(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sts(region), bytes.NewReader(payload))
	if err != nil {
		return callerIdentity{}, ErrCall
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	res, err := c.send(req, payload, creds, region, "sts")
	if err != nil {
		return callerIdentity{}, err
	}
	if res.status != http.StatusOK {
		return callerIdentity{}, res.classify(afterIdentity)
	}
	var doc struct {
		Response struct {
			Result struct {
				Account string `json:"Account"`
				Arn     string `json:"Arn"`
			} `json:"GetCallerIdentityResult"`
		} `json:"GetCallerIdentityResponse"`
	}
	if json.Unmarshal(res.body, &doc) != nil || !accountPattern.MatchString(doc.Response.Result.Account) || doc.Response.Result.Arn == "" {
		return callerIdentity{}, ErrCall
	}
	return callerIdentity{account: doc.Response.Result.Account, arn: doc.Response.Result.Arn}, nil
}

func (c *Client) probeRegion(ctx context.Context, creds Credentials, region string, probe regionProbe) error {
	var res callResult
	var err error
	if probe.query != nil {
		form := url.Values{"Action": {probe.query.action}, "Version": {probe.query.version}}
		for k, v := range probe.query.extra {
			form[k] = v
		}
		res, err = c.post(ctx, c.queryBase(probe.service, probe.service, region), form, creds, region, probe.service)
	} else {
		base := strings.TrimRight(c.eks(region), "/")
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, base+probe.rest, nil)
		if err == nil {
			req.Header.Set("Accept", "application/json")
			res, err = c.send(req, nil, creds, region, probe.service)
		}
	}
	if err != nil {
		return err
	}
	if res.status != http.StatusOK {
		return res.classify(true)
	}
	return nil
}

// VerifyAccessKey runs the three verification stages. Every error is one of
// the connection verification categories or ErrCall; none carries AWS output.
func (c *Client) VerifyAccessKey(ctx context.Context, key connection.AWSAccessKey, regions []string) (connection.AWSVerification, error) {
	if key.AccessKeyID == "" || key.SecretAccessKey == "" || len(regions) == 0 {
		return connection.AWSVerification{}, connection.ErrAuthentication
	}
	for _, region := range regions {
		if !regionPattern.MatchString(region) {
			return connection.AWSVerification{}, connection.ErrRegionUnavailable
		}
	}
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	creds := Credentials{AccessKeyID: key.AccessKeyID, SecretAccessKey: key.SecretAccessKey}

	identity, err := c.callerIdentity(ctx, creds, identityRegion, false)
	if err != nil {
		return connection.AWSVerification{}, err
	}
	if !userARNPattern.MatchString(identity.arn) {
		return connection.AWSVerification{}, connection.ErrUnsupportedPrincipal
	}
	for _, region := range regions {
		again, err := c.callerIdentity(ctx, creds, region, true)
		if err != nil {
			return connection.AWSVerification{}, err
		}
		if again.account != identity.account {
			return connection.AWSVerification{}, connection.ErrRegionUnavailable
		}
		for _, probe := range regionProbes {
			if err := c.probeRegion(ctx, creds, region, probe); err != nil {
				return connection.AWSVerification{}, err
			}
		}
	}
	groups, err := c.verifyCapabilities(ctx, creds, identity.arn)
	if err != nil {
		return connection.AWSVerification{}, err
	}
	return connection.AWSVerification{
		AccountID: identity.account, PrincipalARN: identity.arn,
		Regions: append([]string(nil), regions...), Capabilities: groups,
	}, nil
}

// simulateResponse is the part of SimulatePrincipalPolicy output that is read.
type simulateResponse struct {
	Results []struct {
		Action   string `xml:"EvalActionName"`
		Decision string `xml:"EvalDecision"`
	} `xml:"SimulatePrincipalPolicyResult>EvaluationResults>member"`
	Truncated string `xml:"SimulatePrincipalPolicyResult>IsTruncated"`
	Marker    string `xml:"SimulatePrincipalPolicyResult>Marker"`
}

// verifyCapabilities simulates every required action for the principal. Each
// action must be reported "allowed"; a missing result is unproven, not allowed.
func (c *Client) verifyCapabilities(ctx context.Context, creds Credentials, principalARN string) ([]string, error) {
	decisions := map[string]string{}
	var actions []string
	for _, group := range capabilityGroups {
		actions = append(actions, group.Actions...)
	}
	for start := 0; start < len(actions); start += simulateChunk {
		chunk := actions[start:min(start+simulateChunk, len(actions))]
		marker := ""
		for page := 0; page < 20; page++ {
			form := url.Values{
				"Action": {"SimulatePrincipalPolicy"}, "Version": {iamVersion},
				"PolicySourceArn": {principalARN}, "ResourceArns.member.1": {"*"}, "MaxItems": {"1000"},
			}
			for i, action := range chunk {
				form.Set("ActionNames.member."+strconv.Itoa(i+1), action)
			}
			if marker != "" {
				form.Set("Marker", marker)
			}
			res, err := c.post(ctx, c.iam(), form, creds, identityRegion, "iam")
			if err != nil {
				return nil, err
			}
			if res.status != http.StatusOK {
				switch err := res.classify(true); {
				case errors.Is(err, connection.ErrPermissionDenied), errors.Is(err, connection.ErrRegionUnavailable):
					// The principal may not simulate its own policy (or the
					// call is blocked): the capabilities are unproven.
					return nil, connection.ErrCapabilityUnverifiable
				default:
					return nil, err
				}
			}
			var doc simulateResponse
			if xml.Unmarshal(res.body, &doc) != nil {
				return nil, connection.ErrCapabilityUnverifiable
			}
			for _, r := range doc.Results {
				// A permission that is allowed only for some resources or
				// by a condition is reported differently; "allowed" is the
				// only accepted decision and an action is satisfied only if
				// every returned result for it is allowed.
				if previous, seen := decisions[r.Action]; !seen || previous == "allowed" {
					decisions[r.Action] = r.Decision
				}
			}
			if strings.TrimSpace(doc.Truncated) != "true" || doc.Marker == "" {
				break
			}
			marker = doc.Marker
		}
	}
	var denied bool
	for _, action := range actions {
		decision, seen := decisions[action]
		if !seen {
			return nil, connection.ErrCapabilityUnverifiable
		}
		if decision != "allowed" {
			denied = true
		}
	}
	if denied {
		return nil, connection.ErrPermissionDenied
	}
	names := make([]string, 0, len(capabilityGroups))
	for _, group := range capabilityGroups {
		names = append(names, group.Name)
	}
	sort.Strings(names)
	return names, nil
}
