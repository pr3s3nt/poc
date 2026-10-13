package awsaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FAKE AWS endpoints: request shape and response parsing only.
func TestQueryStatusSignsAndParsesTheStatusElement(t *testing.T) {
	var body, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, 512)
		n, _ := r.Body.Read(raw)
		body, auth = string(raw[:n]), r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`<DescribeDBClustersResponse><DescribeDBClustersResult><DBClusters><DBCluster><DBClusterIdentifier>orc-1</DBClusterIdentifier><Status>available</Status></DBCluster></DBClusters></DescribeDBClustersResult></DescribeDBClustersResponse>`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), QueryURL: func(service, region string) string { return srv.URL + "/" }}
	status, err := c.QueryStatus(context.Background(), conn(), RDSCluster, "orc-1")
	if err != nil || status != "available" {
		t.Fatalf("status %q: %v", status, err)
	}
	if !strings.Contains(body, "Action=DescribeDBClusters") || !strings.Contains(body, "DBClusterIdentifier=orc-1") || !strings.Contains(auth, "/us-east-1/rds/aws4_request") {
		t.Fatalf("body %q auth %q", body, auth)
	}
	if _, err := c.QueryStatus(context.Background(), conn(), RDSCluster, "id with spaces"); err == nil {
		t.Fatal("an unsafe object ID was queried")
	}
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`<R></R>`)) }))
	defer empty.Close()
	c.QueryURL = func(string, string) string { return empty.URL + "/" }
	if _, err := c.QueryStatus(context.Background(), conn(), RDSCluster, "orc-1"); err == nil {
		t.Fatal("a response without a status counted as a status")
	}
}

func TestNodegroupStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clusters/orc-cluster/node-groups/orc-ng" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"nodegroup":{"status":"ACTIVE"}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), EKSURL: func(string) string { return srv.URL }}
	if status, err := c.NodegroupStatus(context.Background(), conn(), "orc-cluster", "orc-ng"); err != nil || status != "ACTIVE" {
		t.Fatalf("nodegroup %q: %v", status, err)
	}
	if _, err := c.NodegroupStatus(context.Background(), conn(), "orc-cluster", "missing"); err == nil {
		t.Fatal("a missing node group is not ready")
	}
}
