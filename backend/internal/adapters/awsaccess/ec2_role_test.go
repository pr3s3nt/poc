package awsaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/awsaccess/awsfake"
	"orchestrator/internal/application/connection"
)

// Local HTTP fixtures only. No metadata or AWS request leaves these servers.
func roleFixture(t *testing.T) (*EC2RoleProvider, *bool, *bool) {
	t.Helper()
	metadataRejected, wrongRole := false, false
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if metadataRejected {
			http.Error(w, "metadata-secret", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/latest/api/token" {
			if r.Method != "PUT" || r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				t.Error("not IMDSv2")
			}
			fmt.Fprint(w, "metadata-token")
			return
		}
		if r.Header.Get("X-aws-ec2-metadata-token") != "metadata-token" {
			t.Error("no metadata token")
		}
		if strings.HasSuffix(r.URL.Path, "security-credentials/") {
			fmt.Fprint(w, "Orchestrator")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Code": "Success", "AccessKeyId": "ASIAFIXTURE", "SecretAccessKey": "fixture-secret", "Token": "fixture-token", "Expiration": time.Now().Add(time.Hour)})
	}))
	t.Cleanup(metadata.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Security-Token") != "fixture-token" {
			t.Error("session token missing")
		}
		_ = r.ParseForm()
		switch r.Form.Get("Action") {
		case "GetCallerIdentity":
			role := "Orchestrator"
			if wrongRole {
				role = "Other"
			}
			fmt.Fprintf(w, `{"GetCallerIdentityResponse":{"GetCallerIdentityResult":{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/%s/i-fixture","UserId":"AROAID:i-fixture"}}}`, role)
		case "GetRole":
			fmt.Fprint(w, `<GetRoleResponse><GetRoleResult><Role><Arn>arn:aws:iam::123456789012:role/platform/Orchestrator</Arn><RoleId>AROAID</RoleId></Role></GetRoleResult></GetRoleResponse>`)
		case "SimulatePrincipalPolicy":
			if r.Form.Get("PolicySourceArn") != "arn:aws:iam::123456789012:role/platform/Orchestrator" {
				t.Error("simulation did not use canonical role ARN")
			}
			var actions []string
			for i := 1; ; i++ {
				a := r.Form.Get(fmt.Sprintf("ActionNames.member.%d", i))
				if a == "" {
					break
				}
				actions = append(actions, a)
			}
			status, body := awsfake.AllowAll("us-east-1", actions)
			w.WriteHeader(status)
			fmt.Fprint(w, body)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(api.Close)
	p := NewEC2RoleProvider()
	p.metadataURL = metadata.URL
	p.Client.STSURL = func(string) string { return api.URL }
	p.Client.IAMURL = func() string { return api.URL }
	p.Client.EKSURL = func(string) string { return api.URL }
	p.Client.QueryURL = func(string, string) string { return api.URL }
	return p, &metadataRejected, &wrongRole
}

func TestEC2RoleIMDSv2IdentityAndCapabilities(t *testing.T) {
	p, rejected, wrong := roleFixture(t)
	ctx := context.Background()
	arn := "arn:aws:iam::123456789012:role/platform/Orchestrator"
	t.Setenv("AWS_ACCESS_KEY_ID", "host-key")
	t.Setenv("AWS_PROFILE", "host-profile")
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", "http://invalid")
	v, err := p.VerifyEC2Role(ctx, arn, []string{"us-east-1", "eu-west-1"})
	if err != nil || v.RoleID != "AROAID" || len(v.Capabilities) != 4 {
		t.Fatalf("verification: %+v %v", v, err)
	}
	*wrong = true
	if _, err = p.ResolveEC2Role(ctx, arn, "us-east-1"); err != connection.ErrAccountIncompatible {
		t.Fatal("wrong role accepted", err)
	}
	*wrong = false
	if _, err = p.ResolveEC2Role(ctx, "arn:aws:iam::123456789012:role/other/Orchestrator", "us-east-1"); err != connection.ErrAccountIncompatible {
		t.Fatal("wrong role path accepted", err)
	}
	*rejected = true
	if _, err = p.ResolveEC2Role(ctx, arn, "us-east-1"); err == nil {
		t.Fatal("fell back to host credentials")
	}
}

func TestEC2MetadataRedirectAndIMDSv1Refused(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	p := NewEC2RoleProvider()
	p.metadataURL = redirect.URL
	if _, err := p.ResolveEC2Role(context.Background(), "arn:aws:iam::123456789012:role/Orchestrator", "us-east-1"); err == nil || hit {
		t.Fatal("metadata redirect followed")
	}
}
