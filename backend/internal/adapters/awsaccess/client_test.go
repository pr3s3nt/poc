package awsaccess

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/ports/terraformexec"
)

// FAKE AWS endpoints (httptest): these tests prove request signing, scoping and
// response handling only; they never reach AWS.
func conn() terraformexec.AWSConnection {
	return terraformexec.AWSConnection{ConnectionKey: "acct", AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret", SessionToken: "tok", AccountID: "123456789012", Region: "us-east-1"}
}

func TestCallerAccountSignsWithTheExplicitKeyOnly(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "HOSTKEY")
	var auth, token, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, token, ctype = r.Header.Get("Authorization"), r.Header.Get("X-Amz-Security-Token"), r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"GetCallerIdentityResponse":{"GetCallerIdentityResult":{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/x","UserId":"U"}}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), STSURL: func(string) string { return srv.URL + "/" }, Now: func() time.Time { return time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC) }}
	account, err := c.CallerAccount(context.Background(), conn())
	if err != nil || account != "123456789012" {
		t.Fatalf("account %q: %v", account, err)
	}
	if !strings.Contains(auth, "Credential=AKIDEXAMPLE/20261013/us-east-1/sts/aws4_request") || strings.Contains(auth, "HOSTKEY") || token != "tok" || !strings.HasPrefix(ctype, "application/x-www-form-urlencoded") {
		t.Fatalf("request auth=%q token=%q", auth, token)
	}
}

func TestCallerAccountFailsClosedWithoutLeakingResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("AccessDenied SECRET-DETAIL"))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), STSURL: func(string) string { return srv.URL + "/" }}
	if _, err := c.CallerAccount(context.Background(), conn()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("denied call: %v", err)
	}
	bad := conn()
	bad.Region = "not-a-region"
	if _, err := c.CallerAccount(context.Background(), bad); err == nil {
		t.Fatal("a malformed region was accepted")
	}
}

func TestDescribeClusterReturnsEndpointAndPEM(t *testing.T) {
	pem := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"cluster":{"endpoint":"https://abc.eks.example","status":"ACTIVE","certificateAuthority":{"data":"` + base64.StdEncoding.EncodeToString([]byte(pem)) + `"}}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), EKSURL: func(string) string { return srv.URL }}
	ep, err := c.DescribeCluster(context.Background(), conn(), "orc-cluster")
	if err != nil || ep.Endpoint != "https://abc.eks.example" || ep.CAPEM != pem || ep.Status != "ACTIVE" || path != "/clusters/orc-cluster" {
		t.Fatalf("endpoint %+v path %q: %v", ep, path, err)
	}
	if _, err := c.DescribeCluster(context.Background(), conn(), "../escape"); err == nil {
		t.Fatal("a path-like cluster name was accepted")
	}
}
