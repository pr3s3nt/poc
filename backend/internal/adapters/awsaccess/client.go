package awsaccess

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"orchestrator/internal/ports/terraformexec"
)

var (
	regionPattern  = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]$`)
	clusterPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$`)
	accountPattern = regexp.MustCompile(`^[0-9]{12}$`)
)

// ErrCall means a signed AWS call failed. It never carries a response body,
// a key or a token.
var ErrCall = errors.New("awsaccess: the AWS call failed")

// Client implements terraformexec.AWSAPI over HTTPS. STSURL and EKSURL are
// overridden only by tests.
type Client struct {
	HTTP   *http.Client
	STSURL func(region string) string
	EKSURL func(region string) string
	// QueryURL overrides the base URL of a Query-protocol service.
	QueryURL func(service, region string) string
	// IAMURL overrides the global IAM endpoint.
	IAMURL func() string
	Now    func() time.Time
}

var _ terraformexec.AWSAPI = (*Client)(nil)

// New returns a client with bounded timeouts and no proxy fallback beyond the
// standard environment proxy settings.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// httpClient returns the transport client with redirects refused: a signed
// request (its Authorization header) must never follow a redirect to a
// destination nobody validated. A 3xx answer is then a failed call.
func (c *Client) httpClient() *http.Client {
	base := c.HTTP
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	guarded := *base
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &guarded
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) sts(region string) string {
	if c.STSURL != nil {
		return c.STSURL(region)
	}
	return "https://sts." + region + ".amazonaws.com/"
}

func (c *Client) eks(region string) string {
	if c.EKSURL != nil {
		return c.EKSURL(region)
	}
	return "https://eks." + region + ".amazonaws.com"
}

func credentials(conn terraformexec.AWSConnection) (Credentials, error) {
	if conn.AccessKeyID == "" || conn.SecretAccessKey == "" || !regionPattern.MatchString(conn.Region) {
		return Credentials{}, ErrCall
	}
	return Credentials{AccessKeyID: conn.AccessKeyID, SecretAccessKey: conn.SecretAccessKey, SessionToken: conn.SessionToken}, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, ErrCall
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w (status %d)", ErrCall, resp.StatusCode)
	}
	return body, nil
}

// CallerAccount calls STS GetCallerIdentity with the explicit key.
func (c *Client) CallerAccount(ctx context.Context, conn terraformexec.AWSConnection) (string, error) {
	creds, err := credentials(conn)
	if err != nil {
		return "", err
	}
	payload := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sts(conn.Region), bytes.NewReader(payload))
	if err != nil {
		return "", ErrCall
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	Sign(req, payload, creds, conn.Region, "sts", c.now())
	body, err := c.do(req)
	if err != nil {
		return "", err
	}
	var doc struct {
		Response struct {
			Result struct {
				Account string `json:"Account"`
			} `json:"GetCallerIdentityResult"`
		} `json:"GetCallerIdentityResponse"`
	}
	if json.Unmarshal(body, &doc) != nil || !accountPattern.MatchString(doc.Response.Result.Account) {
		return "", ErrCall
	}
	return doc.Response.Result.Account, nil
}

// DescribeCluster reads the endpoint and CA of an EKS cluster.
func (c *Client) DescribeCluster(ctx context.Context, conn terraformexec.AWSConnection, cluster string) (terraformexec.ClusterEndpoint, error) {
	creds, err := credentials(conn)
	if err != nil || !clusterPattern.MatchString(cluster) {
		return terraformexec.ClusterEndpoint{}, ErrCall
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.eks(conn.Region), "/")+"/clusters/"+cluster, nil)
	if err != nil {
		return terraformexec.ClusterEndpoint{}, ErrCall
	}
	req.Header.Set("Accept", "application/json")
	Sign(req, nil, creds, conn.Region, "eks", c.now())
	body, err := c.do(req)
	if err != nil {
		return terraformexec.ClusterEndpoint{}, err
	}
	var doc struct {
		Cluster struct {
			Endpoint             string `json:"endpoint"`
			Status               string `json:"status"`
			CertificateAuthority struct {
				Data string `json:"data"`
			} `json:"certificateAuthority"`
		} `json:"cluster"`
	}
	if json.Unmarshal(body, &doc) != nil || !strings.HasPrefix(doc.Cluster.Endpoint, "https://") {
		return terraformexec.ClusterEndpoint{}, ErrCall
	}
	ca, err := base64.StdEncoding.DecodeString(doc.Cluster.CertificateAuthority.Data)
	if err != nil || !bytes.Contains(ca, []byte("BEGIN CERTIFICATE")) {
		return terraformexec.ClusterEndpoint{}, ErrCall
	}
	return terraformexec.ClusterEndpoint{Endpoint: doc.Cluster.Endpoint, CAPEM: string(ca), Status: doc.Cluster.Status}, nil
}

// ClusterToken derives a short-lived EKS token with the explicit key.
func (c *Client) ClusterToken(conn terraformexec.AWSConnection, cluster string) (string, error) {
	creds, err := credentials(conn)
	if err != nil || !clusterPattern.MatchString(cluster) {
		return "", ErrCall
	}
	return EKSToken(creds, conn.Region, cluster, c.now()), nil
}
