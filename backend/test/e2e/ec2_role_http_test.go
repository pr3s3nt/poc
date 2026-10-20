package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/application/connection"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/seed/seedtest"
)

type roleHTTPProvider struct{ calls int }

func (p *roleHTTPProvider) ResolveEC2Role(context.Context, string, string) (terraformexec.AWSConnection, error) {
	panic("HTTP registration must call verifier")
}
func (p *roleHTTPProvider) VerifyEC2Role(_ context.Context, arn string, regions []string) (connection.AWSVerification, error) {
	p.calls++
	return connection.AWSVerification{AccountID: "123456789012", PrincipalARN: arn, RoleID: "AROAHTTP", Regions: regions, Capabilities: connection.RequiredAWSCapabilities}, nil
}

func TestEC2RoleHTTPStrictScopedVersionedAndSafe(t *testing.T) {
	provider := &roleHTTPProvider{}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{Seed: seedtest.Defaults(), Adapters: bootstrap.AdapterFake, EC2RoleProviderOverride: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(app.Server)
	defer server.Close()
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	base := server.URL + "/api/v1/driver-accounts"
	body := `{"name":"EC2 role","roleArn":"arn:aws:iam::123456789012:role/Orchestrator","verificationRegions":["us-east-1"]}`
	if resp, _ := call(t, dev, http.MethodPost, base+"/aws/ec2-role", body); resp.StatusCode != http.StatusForbidden {
		t.Fatal("developer admitted")
	}
	for _, bad := range []string{
		`{"name":"x","roleArn":null,"verificationRegions":["us-east-1"]}`,
		`{"name":"x","roleArn":"arn:aws:iam::123456789012:role/Orchestrator","name":"y"}`,
		strings.TrimSuffix(body, "}") + `,"accessKeyId":"fixture-key"}`,
		strings.TrimSuffix(body, "}") + `,"metadataEndpoint":"http://untrusted"}`,
	} {
		if resp, _ := call(t, pe, http.MethodPost, base+"/aws/ec2-role", bad); resp.StatusCode != http.StatusBadRequest {
			t.Fatal("invalid body admitted", resp.StatusCode)
		}
	}
	if provider.calls != 0 {
		t.Fatal("gates bypassed")
	}
	status, out := requestJSON(t, pe, http.MethodPost, base+"/aws/ec2-role", map[string]any{"name": "EC2 role", "roleArn": "arn:aws:iam::123456789012:role/Orchestrator", "verificationRegions": []string{"us-east-1"}})
	if status != 201 || out["authenticationType"] != "AWS_EC2_ROLE" {
		t.Fatal("role not created", status, out)
	}
	key := out["key"].(string)
	resp, raw := call(t, pe, http.MethodGet, base+"/"+key, "")
	if resp.StatusCode != 200 || !strings.Contains(raw, "AROAHTTP") || strings.Contains(raw, "secretRef") {
		t.Fatal("unsafe/missing metadata", raw)
	}
	if resp, _ := call(t, pe, http.MethodPut, base+"/"+key+"/credentials", `{"version":1,"accessKeyId":"x","secretAccessKey":"y","verificationRegions":["us-east-1"]}`); resp.StatusCode != 422 {
		t.Fatal("role credential replacement admitted", resp.StatusCode)
	}
	if resp, _ := call(t, pe, http.MethodPost, base+"/"+key+"/verify", `{"version":1}`); resp.StatusCode != 200 {
		t.Fatal("verify failed", resp.StatusCode)
	}
	if resp, _ := call(t, pe, http.MethodPost, base+"/"+key+"/verify", `{"version":1}`); resp.StatusCode != 409 {
		t.Fatal("stale mutation admitted", resp.StatusCode)
	}
}
