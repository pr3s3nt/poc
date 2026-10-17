package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	connectionapp "orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed/seedtest"
)

// FAKE verifiers and in-memory credential store: these tests prove the V01
// /driver-accounts HTTP contract (role gates, strict bounded decoding, version
// CAS, safe fixed DTOs and codes). They are not evidence that AWS or a cluster
// accepts a credential.

const (
	accNewKeyID = "AKIAHTTPFIXTUREKEY02"
	accNewKey   = "http/fixture/secret/value/11111111111111"
	accToken    = "account-token-0123456789-secret"
)

func accountsServer(t *testing.T) (*httptest.Server, *awsHTTPVerifier, *uploadVerifier, *credentialmemory.Store) {
	t.Helper()
	aws, kube, store := &awsHTTPVerifier{}, &uploadVerifier{}, credentialmemory.New()
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: seedtest.Defaults(), Adapters: bootstrap.AdapterFake,
		AWSVerifierOverride: aws, KubeconfigVerifierOverride: kube, ConnectionCredentialsOverride: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Server)
	t.Cleanup(server.Close)
	return server, aws, kube, store
}

func assertSafeAccount(t *testing.T, what, body string) {
	t.Helper()
	for _, leaked := range []string{awsHTTPKeyID, awsHTTPSecret, accNewKeyID, accNewKey, accToken, "secretRef", "memory://", "kv2://", "allowedRegions", "tlsTrust", "executionProfile", "applicab"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("%s exposes %q: %s", what, leaked, body)
		}
	}
}

func TestDriverAccountsHTTPContract(t *testing.T) {
	server, aws, kube, store := accountsServer(t)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	anon := newJarClient(t)
	base := server.URL + "/api/v1/driver-accounts"
	doc := ct.Single("lab", "https://lab.example:6443", accToken)

	// Gates: session, then PE/Admin, before any body parsing or provider work.
	for name, tc := range map[string]struct{ method, path, body string }{
		"list":    {http.MethodGet, "", ""},
		"detail":  {http.MethodGet, "/x", ""},
		"inspect": {http.MethodPost, "/internal/inspect", `{"kubeconfig":"a"}`},
		"create":  {http.MethodPost, "/aws", awsAccountBody(t, nil)},
		"rename":  {http.MethodPut, "/x", `{"name":"n","version":1}`},
		"verify":  {http.MethodPost, "/x/verify", `{"version":1}`},
		"replace": {http.MethodPut, "/x/credentials", `{"version":1}`},
	} {
		if resp, _ := call(t, anon, tc.method, base+tc.path, tc.body); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s: %d", name, resp.StatusCode)
		}
		if resp, body := call(t, dev, tc.method, base+tc.path, tc.body); resp.StatusCode != http.StatusForbidden {
			t.Errorf("developer %s: %d %s", name, resp.StatusCode, body)
		}
	}
	if aws.calls != 0 || kube.calls != 0 || store.Len() != 0 {
		t.Fatal("a refused caller reached a verifier or the credential store")
	}

	// Strict create bodies: nothing is verified or stored.
	rejected := map[string]string{
		"legacy allowedRegions": `{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","allowedRegions":["us-east-1"]}`,
		"unknown field":         awsAccountBody(t, func(m map[string]any) { m["executionProfile"] = "aws-eks" }),
		"environment":           awsAccountBody(t, func(m map[string]any) { m["environmentId"] = "e" }),
		"secretRef":             awsAccountBody(t, func(m map[string]any) { m["secretRef"] = "kv2://x" }),
		"duplicate field":       `{"name":"x","name":"y","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","verificationRegions":["us-east-1"]}`,
		"duplicate regions key": `{"name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","verificationRegions":["us-east-1"],"verificationRegions":["us-east-1"]}`,
		"null member":           `{"name":null,"accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","verificationRegions":["us-east-1"]}`,
		"null regions":          awsAccountBody(t, func(m map[string]any) { m["verificationRegions"] = nil }),
		"regions string":        awsAccountBody(t, func(m map[string]any) { m["verificationRegions"] = "us-east-1" }),
		"wrong case":            `{"Name":"x","accessKeyId":"` + awsHTTPKeyID + `","secretAccessKey":"` + awsHTTPSecret + `","verificationRegions":["us-east-1"]}`,
		"trailing":              awsAccountBody(t, nil) + `{}`,
		"mixed internal":        `{"name":"x","kubeconfig":"a","context":"c","accessKeyId":"` + awsHTTPKeyID + `"}`,
		"empty regions":         awsAccountBody(t, func(m map[string]any) { m["verificationRegions"] = []string{} }),
		"bad region":            awsAccountBody(t, func(m map[string]any) { m["verificationRegions"] = []string{"nowhere"} }),
		"duplicate region":      awsAccountBody(t, func(m map[string]any) { m["verificationRegions"] = []string{"us-east-1", "us-east-1"} }),
		"eleven regions": awsAccountBody(t, func(m map[string]any) {
			m["verificationRegions"] = strings.Split(strings.Repeat("us-east-1,", 11), ",")[:11]
		}),
		"temporary key": awsAccountBody(t, func(m map[string]any) { m["accessKeyId"] = "ASIAHTTPFIXTUREKEY01" }),
	}
	for name, body := range rejected {
		resp, text := call(t, pe, http.MethodPost, base+"/aws", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, resp.StatusCode, text)
		}
		assertSafeAccount(t, name, text)
	}
	if resp, _ := call(t, pe, http.MethodPost, base+"/aws", `{"name":"`+strings.Repeat("x", 17<<10)+`"}`); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("aws envelope: %d", resp.StatusCode)
	}
	if resp, body := call(t, pe, http.MethodPost, base+"/internal", `{"name":"x","kubeconfig":"a","context":"c","namespace":"n"}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("internal unknown: %d %s", resp.StatusCode, body)
	}
	resp, body := call(t, pe, http.MethodPost, base+"/aws", awsAccountBody(t, func(m map[string]any) { m["verificationRegions"] = []string{"nowhere"} }))
	if !strings.Contains(body, `"field":"verificationRegions"`) || strings.Contains(body, "nowhere") {
		t.Errorf("field error: %d %s", resp.StatusCode, body)
	}
	if aws.calls != 0 || kube.calls != 0 || store.Len() != 0 {
		t.Fatalf("rejected bodies verified (%d/%d) or stored (%d)", aws.calls, kube.calls, store.Len())
	}

	// Create AWS: 201, version 1, verification regions evidence, safe DTO.
	resp, body = call(t, pe, http.MethodPost, base+"/aws", awsAccountBody(t, nil))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create aws: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "create aws", body)
	created := decode(t, body)
	cfg, _ := created["config"].(map[string]any)
	regions, _ := cfg["verificationRegions"].([]any)
	if created["type"] != "AWS" || created["authenticationType"] != "AWS_ACCESS_KEY" || created["status"] != "READY" || created["version"] != float64(1) ||
		created["key"] != "tai-khoan-aws-lab" || cfg["accountId"] != "123456789012" || len(regions) != 2 {
		t.Fatalf("aws account: %s", body)
	}
	for _, field := range []string{"kind", "secretRef", "region", "allowedRegions"} {
		if _, present := created[field]; present {
			t.Errorf("DTO carries %s", field)
		}
	}

	// Inspect then create Internal.
	resp, body = call(t, pe, http.MethodPost, base+"/internal/inspect", jsonBody(t, map[string]string{"kubeconfig": doc}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"name":"lab"`) || strings.Contains(body, accToken) {
		t.Fatalf("inspect: %d %s", resp.StatusCode, body)
	}
	resp, body = call(t, pe, http.MethodPost, base+"/internal", jsonBody(t, map[string]string{"name": "Cụm lab", "kubeconfig": doc, "context": "lab"}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create internal: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "create internal", body)
	internal := decode(t, body)
	icfg, _ := internal["config"].(map[string]any)
	tls, _ := icfg["tls"].(map[string]any)
	if internal["type"] != "INTERNAL" || internal["authenticationType"] != "KUBECONFIG" || internal["version"] != float64(1) || icfg["endpoint"] != "https://lab.example:6443" || tls["mode"] != "SYSTEM_ROOTS" {
		t.Fatalf("internal account: %s", body)
	}

	// List + detail are safe, scoped and versioned; a missing key is 404.
	resp, body = call(t, pe, http.MethodGet, base, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"accounts":[`) || strings.Count(body, `"version":1`) != 4 {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "list", body)
	if resp, body = call(t, pe, http.MethodGet, base+"/ghost", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing detail: %d %s", resp.StatusCode, body)
	}
	if resp, body = call(t, pe, http.MethodGet, base+"/tai-khoan-aws-lab", ""); resp.StatusCode != http.StatusOK || !strings.Contains(body, `"version":1`) {
		t.Fatalf("detail: %d %s", resp.StatusCode, body)
	}

	// Rename: strict body, version CAS, identity preserved.
	acct := base + "/tai-khoan-aws-lab"
	for name, body := range map[string]string{
		"no version":     `{"name":"n"}`,
		"no name":        `{"version":1}`,
		"zero version":   `{"name":"n","version":0}`,
		"string version": `{"name":"n","version":"1"}`,
		"float version":  `{"name":"n","version":1.5}`,
		"extra":          `{"name":"n","version":1,"type":"INTERNAL"}`,
		"duplicate":      `{"name":"n","version":1,"version":1}`,
		"trailing":       `{"name":"n","version":1}{}`,
	} {
		if resp, text := call(t, pe, http.MethodPut, acct, body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("rename %s: %d %s", name, resp.StatusCode, text)
		}
	}
	resp, body = call(t, pe, http.MethodPut, acct, `{"name":"Tên mới","version":1}`)
	renamed := decode(t, body)
	if resp.StatusCode != http.StatusOK || renamed["name"] != "Tên mới" || renamed["version"] != float64(2) || renamed["key"] != created["key"] || renamed["id"] != created["id"] {
		t.Fatalf("rename: %d %s", resp.StatusCode, body)
	}
	resp, body = call(t, pe, http.MethodPut, acct, `{"name":"Stale","version":1}`)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, `"code":"STALE_VERSION"`) {
		t.Fatalf("stale rename: %d %s", resp.StatusCode, body)
	}
	if _, body = call(t, pe, http.MethodGet, acct, ""); !strings.Contains(body, "Tên mới") {
		t.Fatalf("stale write overwrote the winner: %s", body)
	}
	if resp, _ = call(t, pe, http.MethodPut, base+"/ghost", `{"name":"n","version":1}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("rename missing: %d", resp.StatusCode)
	}
	if resp, body = call(t, pe, http.MethodPut, acct, `{"name":"`+strings.Repeat("x", 101)+`","version":2}`); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"field":"name"`) {
		t.Fatalf("long name: %d %s", resp.StatusCode, body)
	}

	// Verify: version required; failure is 422 with a stable code and no change;
	// Internal rejects the AWS-only field; success bumps once.
	verify := acct + "/verify"
	if resp, _ = call(t, pe, http.MethodPost, verify, `{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("verify without version: %d", resp.StatusCode)
	}
	if resp, _ = call(t, pe, http.MethodPost, base+"/cum-lab/verify", `{"version":1,"verificationRegions":["us-east-1"]}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("internal verify with regions: %d", resp.StatusCode)
	}
	aws.err = connectionapp.ErrAuthentication
	resp, body = call(t, pe, http.MethodPost, verify, `{"version":2}`)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"VERIFICATION_CREDENTIAL_REJECTED"`) {
		t.Fatalf("verify failure: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "verify failure", body)
	if _, body = call(t, pe, http.MethodGet, acct, ""); !strings.Contains(body, `"version":2`) || !strings.Contains(body, `"status":"READY"`) {
		t.Fatalf("failed verify changed the record: %s", body)
	}
	aws.err = nil
	resp, body = call(t, pe, http.MethodPost, verify, `{"version":2,"verificationRegions":["us-east-1"]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"version":3`) || !strings.Contains(body, `"verificationRegions":["us-east-1"]`) {
		t.Fatalf("verify: %d %s", resp.StatusCode, body)
	}
	if resp, _ = call(t, pe, http.MethodPost, verify, `{"version":2}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale verify: %d", resp.StatusCode)
	}
	if resp, body = call(t, pe, http.MethodPost, base+"/cum-lab/verify", `{"version":1}`); resp.StatusCode != http.StatusOK || !strings.Contains(body, `"version":2`) {
		t.Fatalf("internal verify: %d %s", resp.StatusCode, body)
	}

	// Credential replacement is kind-specific; mixed forms and secret refs fail.
	replace := acct + "/credentials"
	awsReplace := func(mutate func(map[string]any)) string {
		m := map[string]any{"version": 3, "accessKeyId": accNewKeyID, "secretAccessKey": accNewKey, "verificationRegions": []string{"us-east-1"}}
		if mutate != nil {
			mutate(m)
		}
		return jsonBody(t, m)
	}
	objects := store.Len()
	for name, body := range map[string]string{
		"kubeconfig form": `{"version":3,"kubeconfig":"a","context":"c"}`,
		"mixed":           awsReplace(func(m map[string]any) { m["kubeconfig"] = "a" }),
		"secretRef":       awsReplace(func(m map[string]any) { m["secretRef"] = "kv2://x" }),
		"no regions":      awsReplace(func(m map[string]any) { delete(m, "verificationRegions") }),
		"no version":      awsReplace(func(m map[string]any) { delete(m, "version") }),
		"null key":        awsReplace(func(m map[string]any) { m["accessKeyId"] = nil }),
	} {
		resp, text := call(t, pe, http.MethodPut, replace, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("replace %s: %d %s", name, resp.StatusCode, text)
		}
		assertSafeAccount(t, name, text)
	}
	if resp, _ = call(t, pe, http.MethodPut, base+"/cum-lab/credentials", awsReplace(nil)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("aws body on internal account: %d", resp.StatusCode)
	}
	if store.Len() != objects {
		t.Fatalf("a rejected replacement wrote an object")
	}
	resp, body = call(t, pe, http.MethodPut, replace, awsReplace(nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"version":4`) {
		t.Fatalf("replace: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "replace", body)
	if store.Len() != objects+1 {
		t.Fatalf("expected the prior object to be kept: %d -> %d", objects, store.Len())
	}
	resp, body = call(t, pe, http.MethodPut, replace, awsReplace(nil))
	if resp.StatusCode != http.StatusConflict || store.Len() != objects+1 {
		t.Fatalf("stale replace: %d %s (objects %d)", resp.StatusCode, body, store.Len())
	}

	// Another AWS account is refused with a stable code; the old record stays.
	aws.other = "999999999999"
	resp, body = call(t, pe, http.MethodPut, replace, awsReplace(func(m map[string]any) { m["version"] = 4 }))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"ACCOUNT_TARGET_MISMATCH"`) {
		t.Fatalf("redirect: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "redirect", body)
	if store.Len() != objects+1 {
		t.Fatalf("a redirected replacement wrote an object")
	}
	aws.other = ""

	// Internal: same target replaces, another endpoint is refused.
	icred := base + "/cum-lab/credentials"
	resp, body = call(t, pe, http.MethodPut, icred, jsonBody(t, map[string]any{"version": 2, "kubeconfig": ct.Single("lab", "https://elsewhere.example:6443", accToken), "context": "lab"}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"ACCOUNT_TARGET_MISMATCH"`) {
		t.Fatalf("internal redirect: %d %s", resp.StatusCode, body)
	}
	resp, body = call(t, pe, http.MethodPut, icred, jsonBody(t, map[string]any{"version": 2, "kubeconfig": ct.Single("lab", "https://lab.example:6443", accToken+"-2"), "context": "lab"}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"version":3`) {
		t.Fatalf("internal replace: %d %s", resp.StatusCode, body)
	}
	assertSafeAccount(t, "internal replace", body)

	// The legacy Connections DTO keeps its contract and shares the records.
	resp, body = call(t, pe, http.MethodGet, server.URL+"/api/v1/connections", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"kind":"AWS"`) || !strings.Contains(body, `"allowedRegions"`) {
		t.Fatalf("legacy list: %d %s", resp.StatusCode, body)
	}
}

func awsAccountBody(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	body := map[string]any{"name": "Tài khoản AWS lab", "accessKeyId": awsHTTPKeyID, "secretAccessKey": awsHTTPSecret, "verificationRegions": []string{"us-east-1", "eu-west-1"}}
	if mutate != nil {
		mutate(body)
	}
	return jsonBody(t, body)
}

// An unverified seed marker has no stored credential: verify and replacement
// fail safely with a stable code and leave it unchanged (never READY).
func TestDriverAccountsSeedMarkerCannotBeVerifiedOrReplaced(t *testing.T) {
	server, _, kube, store := accountsServer(t)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	base := server.URL + "/api/v1/driver-accounts/internal-cluster"
	resp, body := call(t, pe, http.MethodPost, base+"/verify", `{"version":1}`)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"CONNECTION_UNAVAILABLE"`) {
		t.Fatalf("verify seed: %d %s", resp.StatusCode, body)
	}
	doc := ct.Single("kind-idp-internal", "https://kind.example:6443", accToken)
	resp, body = call(t, pe, http.MethodPut, base+"/credentials", jsonBody(t, map[string]any{"version": 1, "kubeconfig": doc, "context": "kind-idp-internal"}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"ACCOUNT_TARGET_MISMATCH"`) {
		t.Fatalf("replace seed: %d %s", resp.StatusCode, body)
	}
	if kube.calls != 0 || store.Len() != 0 {
		t.Fatal("a seed marker reached the verifier or the store")
	}
	if _, body = call(t, pe, http.MethodGet, base, ""); !strings.Contains(body, `"status":"VERIFYING"`) || !strings.Contains(body, `"version":1`) {
		t.Fatalf("seed changed: %s", body)
	}
}
