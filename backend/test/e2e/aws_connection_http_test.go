package e2e

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	connectionapp "orchestrator/internal/application/connection"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed/seedtest"
)

// FAKE verifier (and in-memory credential store): these tests prove the HTTP
// contract, role/organization gates, strict bounded decoding, safe DTOs/errors
// and persistence/rollback behavior of AWS registration. They are not
// evidence that AWS accepts a key.

const (
	awsHTTPKeyID  = "AKIAHTTPFIXTUREKEY01"
	awsHTTPSecret = "http/fixture/secret/value/00000000000000" // 40 chars
)

type awsHTTPVerifier struct {
	mu    sync.Mutex
	err   error
	calls int
	// other, when set, is the account ID the verifier reports.
	other string
}

func (v *awsHTTPVerifier) VerifyAWS(_ context.Context, _ connectionapp.AWSAccessKey, regions []string) (connectionapp.AWSVerification, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	if v.err != nil {
		return connectionapp.AWSVerification{}, v.err
	}
	account := "123456789012"
	if v.other != "" {
		account = v.other
	}
	return connectionapp.AWSVerification{
		AccountID: account, PrincipalARN: "arn:aws:iam::" + account + ":user/orch",
		Regions: regions, Capabilities: []string{"Aurora", "EKS", "ElastiCache", "VPC"},
	}, nil
}

func awsServer(t *testing.T, withVerifier, withStore bool) (*httptest.Server, *awsHTTPVerifier, *credentialmemory.Store, *bootstrap.App) {
	t.Helper()
	verifier := &awsHTTPVerifier{}
	store := credentialmemory.New()
	options := bootstrap.Options{Seed: seedtest.Defaults(), Adapters: bootstrap.AdapterFake}
	if withVerifier {
		options.AWSVerifierOverride = verifier
	}
	if withStore {
		options.ConnectionCredentialsOverride = store
	}
	app, err := bootstrap.Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server, verifier, store, app
}

func awsBody(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	body := map[string]any{"name": "Tài khoản AWS lab", "accessKeyId": awsHTTPKeyID, "secretAccessKey": awsHTTPSecret, "allowedRegions": []string{"us-east-1", "eu-west-1"}}
	if mutate != nil {
		mutate(body)
	}
	return jsonBody(t, body)
}

func assertNoAWSSecret(t *testing.T, what, text string) {
	t.Helper()
	for _, leaked := range []string{awsHTTPKeyID, awsHTTPSecret, "secretRef", "memory://local", "kv2://", "sessionToken"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("%s exposes %q: %s", what, leaked, text)
		}
	}
}

func TestAWSConnectionHTTPContract(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	server, verifier, store, app := awsServer(t, true, true)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	register := server.URL + "/api/v1/connections/aws"

	if resp, _ := call(t, newJarClient(t), http.MethodPost, register, awsBody(t, nil)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", resp.StatusCode)
	}
	if resp, body := call(t, dev, http.MethodPost, register, awsBody(t, nil)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("developer: %d %s", resp.StatusCode, body)
	}
	if verifier.calls != 0 || store.Len() != 0 {
		t.Fatal("a refused caller reached verification or the credential store")
	}

	rejected := map[string]struct {
		body   string
		status int
	}{
		"unknown field":     {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"],"sessionToken":"t"}`, 400},
		"backend config":    {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"],"backend":"s3"}`, 400},
		"host profile":      {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"],"profile":"default"}`, 400},
		"duplicate field":   {`{"name":"x","name":"y","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"]}`, 400},
		"duplicate regions": {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"],"allowedRegions":["eu-west-1"]}`, 400},
		"wrong case":        {`{"Name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"]}`, 400},
		"non-string key":    {`{"name":"x","accessKeyId":1,"secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"]}`, 400},
		"region string":     {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":"us-east-1"}`, 400},
		"region object":     {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":[{"a":1}]}`, 400},
		"region number":     {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":[1]}`, 400},
		"nested array":      {`{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":[["us-east-1"]]}`, 400},
		"trailing json":     {awsBody(t, nil) + ` {}`, 400},
		"not an object":     {`["x"]`, 400},
		"empty body":        {``, 400},
		"no regions":        {awsBody(t, func(m map[string]any) { m["allowedRegions"] = []string{} }), 400},
		"missing key":       {awsBody(t, func(m map[string]any) { delete(m, "accessKeyId") }), 400},
		"short secret":      {awsBody(t, func(m map[string]any) { m["secretAccessKey"] = "short" }), 400},
		"temporary key":     {awsBody(t, func(m map[string]any) { m["accessKeyId"] = "ASIAHTTPFIXTUREKEY01" }), 400},
		"too many regions": {awsBody(t, func(m map[string]any) {
			m["allowedRegions"] = strings.Split(strings.Repeat("us-east-1,", 11), ",")[:11]
		}), 400},
		"envelope too large": {`{"name":"` + strings.Repeat("x", 17<<10) + `"}`, 413},
	}
	for name, tc := range rejected {
		resp, body := call(t, pe, http.MethodPost, register, tc.body)
		if resp.StatusCode != tc.status {
			t.Errorf("%s: %d %s", name, resp.StatusCode, body)
		}
		assertNoAWSSecret(t, name, body)
	}
	if verifier.calls != 0 || store.Len() != 0 {
		t.Fatalf("rejected bodies verified (%d) or stored (%d)", verifier.calls, store.Len())
	}

	// A field error names only the field to correct.
	resp, body := call(t, pe, http.MethodPost, register, awsBody(t, func(m map[string]any) { m["allowedRegions"] = []string{"nowhere"} }))
	if resp.StatusCode != 400 || !strings.Contains(body, `"field":"allowedRegions"`) || strings.Contains(body, "nowhere") {
		t.Fatalf("field error: %d %s", resp.StatusCode, body)
	}

	resp, body = call(t, pe, http.MethodPost, register, awsBody(t, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d %s", resp.StatusCode, body)
	}
	assertNoAWSSecret(t, "register response", body)
	created := decode(t, body)
	cfg, _ := created["config"].(map[string]any)
	ver, _ := created["verification"].(map[string]any)
	regions, _ := cfg["allowedRegions"].([]any)
	if created["key"] != "tai-khoan-aws-lab" || created["kind"] != "AWS" || created["authenticationType"] != "AWS_ACCESS_KEY" || created["status"] != "READY" ||
		cfg["accountId"] != "123456789012" || cfg["region"] != "us-east-1" || len(regions) != 2 || regions[1] != "eu-west-1" ||
		ver["identity"] != "VERIFIED" || ver["regionCheck"] != "VERIFIED" || ver["capabilityCheck"] != "VERIFIED" {
		t.Fatalf("created DTO: %s", body)
	}
	resp, list := call(t, pe, http.MethodGet, server.URL+"/api/v1/connections", "")
	assertNoAWSSecret(t, "list", list)
	if resp.StatusCode != http.StatusOK || !strings.Contains(list, `"key":"tai-khoan-aws-lab"`) || !strings.Contains(list, `"allowedRegions":["us-east-1","eu-west-1"]`) {
		t.Fatalf("list: %d %s", resp.StatusCode, list)
	}
	// The same name never overwrites; the default never moves.
	resp, body = call(t, pe, http.MethodPost, register, awsBody(t, nil))
	if resp.StatusCode != http.StatusCreated || decode(t, body)["key"] != "tai-khoan-aws-lab-2" {
		t.Fatalf("collision: %d %s", resp.StatusCode, body)
	}
	org, err := app.Store.GetOrganization(context.Background(), "acme")
	if err != nil || org.DefaultConnectionKey != "internal-cluster" {
		t.Fatalf("organization default changed: %#v %v", org, err)
	}
	stored, err := app.Store.GetConnection(context.Background(), "acme", "tai-khoan-aws-lab")
	if err != nil || stored.SecretRef == "" || store.Len() != 2 {
		t.Fatalf("stored %#v %v (credentials %d)", stored, err, store.Len())
	}
	// Developers do not see the AWS Connection through the Console read paths
	// that exist for them either way; the PE list is the only metadata route.
	if resp, _ := call(t, dev, http.MethodGet, server.URL+"/api/v1/connections", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("developer list: %d", resp.StatusCode)
	}
	assertNoAWSSecret(t, "server log", logs.String())
}

func TestAWSConnectionHTTPErrorCategoriesAreFixedAndSafe(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	server, verifier, store, app := awsServer(t, true, true)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	register := server.URL + "/api/v1/connections/aws"
	for name, tc := range map[string]struct {
		cause error
		code  string
	}{
		"rejected key":    {connectionapp.ErrAuthentication, "VERIFICATION_CREDENTIAL_REJECTED"},
		"missing right":   {connectionapp.ErrPermissionDenied, "VERIFICATION_PERMISSION_DENIED"},
		"region":          {connectionapp.ErrRegionUnavailable, "VERIFICATION_REGION_UNAVAILABLE"},
		"unproven rights": {connectionapp.ErrCapabilityUnverifiable, "VERIFICATION_CAPABILITY_UNVERIFIABLE"},
		"root or session": {connectionapp.ErrUnsupportedPrincipal, "VERIFICATION_PRINCIPAL_UNSUPPORTED"},
		"aws unreachable": {connectionapp.ErrClusterUnreachable, "VERIFICATION_UNREACHABLE"},
		"raw provider":    {errors.New("AccessDenied SECRET-AWS-DETAIL " + awsHTTPSecret), "VERIFICATION_FAILED"},
	} {
		verifier.err = tc.cause
		resp, body := call(t, pe, http.MethodPost, register, awsBody(t, nil))
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"`+tc.code+`"`) {
			t.Errorf("%s: %d %s", name, resp.StatusCode, body)
		}
		assertNoAWSSecret(t, name, body)
		if strings.Contains(body, "SECRET-AWS-DETAIL") {
			t.Errorf("%s: raw provider text in the response", name)
		}
	}
	list, _ := app.Store.ListConnections(context.Background(), "acme")
	for _, conn := range list {
		if conn.Kind == "AWS" && conn.Key != "aws-account" {
			t.Fatalf("a failed verification saved %s", conn.Key)
		}
	}
	if store.Len() != 0 {
		t.Fatalf("a failed verification wrote %d credentials", store.Len())
	}
	assertNoAWSSecret(t, "server log", logs.String())
}

func TestAWSConnectionHTTPFailsClosedWithoutVerifierOrCredentialStore(t *testing.T) {
	server, _, store, app := awsServer(t, false, true)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	// Fake adapters have no AWS verifier: a fixture key can never reach AWS.
	resp, body := call(t, pe, http.MethodPost, server.URL+"/api/v1/connections/aws", awsBody(t, nil))
	if resp.StatusCode != http.StatusUnprocessableEntity || store.Len() != 0 {
		t.Fatalf("no verifier: %d %s (credentials %d)", resp.StatusCode, body, store.Len())
	}
	assertNoAWSSecret(t, "no verifier", body)

	server2, verifier, _, app2 := awsServer(t, true, false)
	pe2 := authenticatedClientAs(t, server2.URL, "platform-engineer")
	resp, body = call(t, pe2, http.MethodPost, server2.URL+"/api/v1/connections/aws", awsBody(t, nil))
	if resp.StatusCode != http.StatusServiceUnavailable || verifier.calls != 0 {
		t.Fatalf("no credential store: %d %s", resp.StatusCode, body)
	}
	for _, a := range []*bootstrap.App{app, app2} {
		list, _ := a.Store.ListConnections(context.Background(), "acme")
		for _, conn := range list {
			if conn.Kind == "AWS" && conn.Key != "aws-account" {
				t.Fatalf("READY AWS metadata without a credential: %s", conn.Key)
			}
		}
	}
}
