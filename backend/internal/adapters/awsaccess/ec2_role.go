package awsaccess

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/application/connection"
	"orchestrator/internal/ports/terraformexec"
)

// EC2RoleProvider never consults the SDK default chain or environment settings.
// Metadata endpoint/transport seams are unexported and used by local tests only.
type EC2RoleProvider struct {
	Client       *Client
	metadataURL  string
	metadataHTTP *http.Client
}

func NewEC2RoleProvider() *EC2RoleProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &EC2RoleProvider{Client: New(), metadataURL: "http://169.254.169.254", metadataHTTP: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

var metadataRoleName = regexp.MustCompile(`^[A-Za-z0-9+=,.@_-]{1,64}$`)

func (p *EC2RoleProvider) metadata(ctx context.Context, method, path, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.metadataURL+path, nil)
	if err != nil {
		return nil, connection.ErrAuthentication
	}
	if method == http.MethodPut {
		req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	} else {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}
	resp, err := p.metadataHTTP.Do(req)
	if err != nil {
		return nil, connection.ErrClusterUnreachable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || resp.StatusCode != http.StatusOK {
		return nil, connection.ErrAuthentication
	}
	return body, nil
}

func (p *EC2RoleProvider) ResolveEC2Role(ctx context.Context, arn, region string) (terraformexec.AWSConnection, error) {
	if !connection.ValidEC2RoleARN(arn) || !regionPattern.MatchString(region) || p.Client == nil || p.metadataHTTP == nil {
		return terraformexec.AWSConnection{}, connection.ErrAuthentication
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	raw, err := p.metadata(ctx, http.MethodPut, "/latest/api/token", "")
	if err != nil {
		return terraformexec.AWSConnection{}, err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return terraformexec.AWSConnection{}, connection.ErrAuthentication
	}
	raw, err = p.metadata(ctx, http.MethodGet, "/latest/meta-data/iam/security-credentials/", token)
	if err != nil {
		return terraformexec.AWSConnection{}, err
	}
	name := strings.TrimSpace(string(raw))
	if !metadataRoleName.MatchString(name) || !strings.HasSuffix(arn, "/"+name) {
		return terraformexec.AWSConnection{}, connection.ErrAccountIncompatible
	}
	raw, err = p.metadata(ctx, http.MethodGet, "/latest/meta-data/iam/security-credentials/"+name, token)
	if err != nil {
		return terraformexec.AWSConnection{}, err
	}
	var doc struct {
		Code, AccessKeyID, SecretAccessKey, Token string
		Expiration                                time.Time
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Code != "Success" || doc.AccessKeyID == "" || doc.SecretAccessKey == "" || doc.Token == "" || !doc.Expiration.After(p.Client.now().Add(time.Minute)) {
		return terraformexec.AWSConnection{}, connection.ErrAuthentication
	}
	creds := Credentials{AccessKeyID: doc.AccessKeyID, SecretAccessKey: doc.SecretAccessKey, SessionToken: doc.Token}
	identity, err := p.Client.callerIdentity(ctx, creds, region, false)
	if err != nil {
		return terraformexec.AWSConnection{}, err
	}
	expected := "arn:aws:sts::" + identity.account + ":assumed-role/" + name + "/"
	session := strings.TrimPrefix(identity.arn, expected)
	if !strings.HasPrefix(identity.arn, expected) || session == "" || strings.Contains(session, "/") || !strings.HasPrefix(arn, "arn:aws:iam::"+identity.account+":role/") {
		return terraformexec.AWSConnection{}, connection.ErrAccountIncompatible
	}
	// GetRole proves the complete path and stable role ID; STS assumed-role ARNs
	// omit role paths. Never infer a canonical ARN by replacing ARN prefixes.
	res, err := p.Client.post(ctx, p.Client.iam(), url.Values{"Action": {"GetRole"}, "Version": {iamVersion}, "RoleName": {name}}, creds, identityRegion, "iam")
	if err != nil {
		return terraformexec.AWSConnection{}, err
	}
	if res.status != http.StatusOK {
		return terraformexec.AWSConnection{}, fmt.Errorf("EC2 role IAM GetRole: %w", res.classify(true))
	}
	var role struct {
		Role struct {
			ARN string `xml:"Arn"`
			ID  string `xml:"RoleId"`
		} `xml:"GetRoleResult>Role"`
	}
	if xml.Unmarshal(res.body, &role) != nil || role.Role.ARN != arn || role.Role.ID == "" || !strings.HasPrefix(identity.userID, role.Role.ID+":") {
		return terraformexec.AWSConnection{}, connection.ErrAccountIncompatible
	}
	return terraformexec.AWSConnection{AccessKeyID: doc.AccessKeyID, SecretAccessKey: doc.SecretAccessKey, SessionToken: doc.Token, Expiration: doc.Expiration, AccountID: identity.account, RoleARN: arn, RoleID: role.Role.ID, Region: region}, nil
}

func (p *EC2RoleProvider) VerifyEC2Role(ctx context.Context, arn string, regions []string) (connection.AWSVerification, error) {
	if len(regions) == 0 {
		return connection.AWSVerification{}, connection.ErrRegionUnavailable
	}
	for _, region := range regions {
		if !regionPattern.MatchString(region) {
			return connection.AWSVerification{}, connection.ErrRegionUnavailable
		}
	}
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	conn, err := p.ResolveEC2Role(ctx, arn, regions[0])
	if err != nil {
		return connection.AWSVerification{}, err
	}
	creds := Credentials{AccessKeyID: conn.AccessKeyID, SecretAccessKey: conn.SecretAccessKey, SessionToken: conn.SessionToken}
	for _, region := range regions {
		id, err := p.Client.callerIdentity(ctx, creds, region, true)
		if err != nil {
			return connection.AWSVerification{}, fmt.Errorf("EC2 role regional STS: %w", err)
		}
		if id.account != conn.AccountID || !strings.HasPrefix(id.userID, conn.RoleID+":") {
			return connection.AWSVerification{}, connection.ErrAccountIncompatible
		}
		for _, probe := range regionProbes {
			if err := p.Client.probeRegion(ctx, creds, region, probe); err != nil {
				return connection.AWSVerification{}, fmt.Errorf("EC2 role %s regional probe: %w", probe.service, err)
			}
		}
	}
	groups, err := p.Client.verifyCapabilities(ctx, creds, arn, regions)
	if err != nil {
		return connection.AWSVerification{}, fmt.Errorf("EC2 role policy simulation: %w", err)
	}
	return connection.AWSVerification{AccountID: conn.AccountID, PrincipalARN: arn, RoleID: conn.RoleID, Regions: append([]string(nil), regions...), Capabilities: groups}, nil
}
