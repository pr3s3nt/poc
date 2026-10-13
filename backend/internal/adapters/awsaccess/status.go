package awsaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"orchestrator/internal/ports/terraformexec"
)

// Query describes one read-only Query-protocol status call: a Describe action
// whose response carries a status element for the named object.
type Query struct {
	Service       string // signing name
	Host          string // host prefix, "<host>.<region>.amazonaws.com"
	Action        string
	Version       string
	IDParameter   string
	StatusElement string
}

// Status queries used by readiness. Each is a single Describe call by ID.
var (
	EC2Vpc                      = Query{Service: "ec2", Host: "ec2", Action: "DescribeVpcs", Version: "2016-11-15", IDParameter: "VpcId.1", StatusElement: "state"}
	RDSCluster                  = Query{Service: "rds", Host: "rds", Action: "DescribeDBClusters", Version: "2014-10-31", IDParameter: "DBClusterIdentifier", StatusElement: "Status"}
	RDSInstance                 = Query{Service: "rds", Host: "rds", Action: "DescribeDBInstances", Version: "2014-10-31", IDParameter: "DBInstanceIdentifier", StatusElement: "DBInstanceStatus"}
	ElastiCacheReplicationGroup = Query{Service: "elasticache", Host: "elasticache", Action: "DescribeReplicationGroups", Version: "2015-02-02", IDParameter: "ReplicationGroupId", StatusElement: "Status"}
	ElastiCacheCluster          = Query{Service: "elasticache", Host: "elasticache", Action: "DescribeCacheClusters", Version: "2015-02-02", IDParameter: "CacheClusterId", StatusElement: "CacheClusterStatus"}
)

var objectID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_./-]{0,254}$`)

// QueryStatus runs one Describe call and returns the first status element of
// the response, for example "available". An object that does not exist is an
// error, never a ready status. The response body is never returned.
func (c *Client) QueryStatus(ctx context.Context, conn terraformexec.AWSConnection, q Query, id string) (string, error) {
	creds, err := credentials(conn)
	if err != nil || !objectID.MatchString(id) {
		return "", ErrCall
	}
	base := "https://" + q.Host + "." + conn.Region + ".amazonaws.com/"
	if c.QueryURL != nil {
		base = c.QueryURL(q.Service, conn.Region)
	}
	form := url.Values{"Action": {q.Action}, "Version": {q.Version}, q.IDParameter: {id}}
	payload := []byte(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(payload))
	if err != nil {
		return "", ErrCall
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	Sign(req, payload, creds, conn.Region, q.Service, c.now())
	body, err := c.do(req)
	if err != nil {
		return "", err
	}
	return firstElement(body, q.StatusElement)
}

// firstElement returns the text of the first element with the given local name.
func firstElement(body []byte, name string) (string, error) {
	decoder := xml.NewDecoder(io.LimitReader(bytes.NewReader(body), 1<<20))
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", ErrCall
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != name {
			continue
		}
		var text string
		if decoder.DecodeElement(&text, &start) != nil {
			return "", ErrCall
		}
		return strings.TrimSpace(text), nil
	}
}

// NodegroupStatus reads the status of one EKS managed node group.
func (c *Client) NodegroupStatus(ctx context.Context, conn terraformexec.AWSConnection, cluster, nodegroup string) (string, error) {
	creds, err := credentials(conn)
	if err != nil || !clusterPattern.MatchString(cluster) || !clusterPattern.MatchString(nodegroup) {
		return "", ErrCall
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.eks(conn.Region), "/")+"/clusters/"+cluster+"/node-groups/"+nodegroup, nil)
	if err != nil {
		return "", ErrCall
	}
	req.Header.Set("Accept", "application/json")
	Sign(req, nil, creds, conn.Region, "eks", c.now())
	body, err := c.do(req)
	if err != nil {
		return "", err
	}
	var doc struct {
		Nodegroup struct {
			Status string `json:"status"`
		} `json:"nodegroup"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.Nodegroup.Status == "" {
		return "", ErrCall
	}
	return doc.Nodegroup.Status, nil
}

// Status implements terraformexec.AWSStatusAPI.
func (c *Client) Status(ctx context.Context, conn terraformexec.AWSConnection, query terraformexec.StatusQuery, id string) (string, error) {
	switch query {
	case terraformexec.StatusEC2Vpc:
		return c.QueryStatus(ctx, conn, EC2Vpc, id)
	case terraformexec.StatusRDSCluster:
		return c.QueryStatus(ctx, conn, RDSCluster, id)
	case terraformexec.StatusRDSInstance:
		return c.QueryStatus(ctx, conn, RDSInstance, id)
	case terraformexec.StatusElastiCacheReplicationGroup:
		return c.QueryStatus(ctx, conn, ElastiCacheReplicationGroup, id)
	case terraformexec.StatusElastiCacheCluster:
		return c.QueryStatus(ctx, conn, ElastiCacheCluster, id)
	case terraformexec.StatusEKSCluster:
		endpoint, err := c.DescribeCluster(ctx, conn, id)
		if err != nil {
			return "", err
		}
		return endpoint.Status, nil
	case terraformexec.StatusEKSNodegroup:
		cluster, nodegroup, ok := strings.Cut(id, "/")
		if !ok {
			return "", ErrCall
		}
		return c.NodegroupStatus(ctx, conn, cluster, nodegroup)
	}
	return "", ErrCall
}

var _ terraformexec.AWSStatusAPI = (*Client)(nil)
