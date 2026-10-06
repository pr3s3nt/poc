package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	connectionapp "orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
)

const uploadToken = "upload-token-0123456789-secret"

type uploadVerifier struct {
	err   error
	calls int
}

func (v *uploadVerifier) VerifyKubeconfig(_ context.Context, selected connectionapp.SelectedKubeconfig) (connectionapp.KubernetesVerification, error) {
	v.calls++
	return connectionapp.KubernetesVerification{Endpoint: selected.Context.Endpoint, Version: "v1.34.0"}, v.err
}

func uploadServer(t *testing.T, withStore bool) (*httptest.Server, *uploadVerifier, *credentialmemory.Store, *bootstrap.App) {
	t.Helper()
	verifier := &uploadVerifier{}
	store := credentialmemory.New()
	options := bootstrap.Options{Seed: seed.Defaults(), Adapters: bootstrap.AdapterFake, KubeconfigVerifierOverride: verifier, ConnectionVerifierOverride: approvedCluster{}}
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

func jsonBody(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUC04UploadHTTPContract(t *testing.T) {
	server, verifier, store, app := uploadServer(t, true)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	dev := authenticatedClient(t, server.URL)
	doc := ct.Document(ct.TokenContext("dev", "https://dev.example:6443", uploadToken), ct.CertificateContext("prod", "https://prod.example"))
	inspect := server.URL + "/api/v1/connections/kubernetes/inspect"
	register := server.URL + "/api/v1/connections/kubernetes"

	if resp, _ := call(t, newJarClient(t), http.MethodPost, inspect, jsonBody(t, map[string]string{"kubeconfig": doc})); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous inspect: %d", resp.StatusCode)
	}
	if resp, _ := call(t, dev, http.MethodPost, inspect, jsonBody(t, map[string]string{"kubeconfig": doc})); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("developer inspect: %d", resp.StatusCode)
	}
	resp, body := call(t, pe, http.MethodPost, inspect, jsonBody(t, map[string]string{"kubeconfig": doc}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"name":"dev"`) || !strings.Contains(body, `"endpoint":"https://prod.example"`) {
		t.Fatalf("inspect: %d %s", resp.StatusCode, body)
	}
	ca, cert, key := ct.TLSMaterial()
	for _, secret := range []string{uploadToken, ca, cert, key} {
		if strings.Contains(body, secret) {
			t.Fatal("inspect response leaks credential")
		}
	}
	if verifier.calls != 0 || store.Len() != 0 {
		t.Fatal("inspect verified or stored")
	}

	noAPIVersion := strings.Replace(doc, "apiVersion: v1\n", "", 1)
	nullKind := strings.Replace(doc, "kind: Config\n", "kind: null\n", 1)
	for name, tc := range map[string]struct {
		url, body string
		status    int
	}{
		"inspect no apiVersion":  {inspect, jsonBody(t, map[string]string{"kubeconfig": noAPIVersion}), http.StatusBadRequest},
		"inspect null kind":      {inspect, jsonBody(t, map[string]string{"kubeconfig": nullKind}), http.StatusBadRequest},
		"register no apiVersion": {register, jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": noAPIVersion, "context": "dev"}), http.StatusBadRequest},
		"register null kind":     {register, jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": nullKind, "context": "dev"}), http.StatusBadRequest},
		"mixed shapes":           {register, `{"name":"x","kubeconfig":"a","context":"dev","key":"x"}`, http.StatusBadRequest},
		"unknown field":          {register, `{"name":"x","kubeconfig":"a","context":"dev","namespace":"n"}`, http.StatusBadRequest},
		"duplicate field":        {register, `{"name":"x","name":"y","kubeconfig":"a","context":"dev"}`, http.StatusBadRequest},
		"wrong case field":       {register, `{"Name":"x","kubeconfig":"a","context":"dev"}`, http.StatusBadRequest},
		"non-string value":       {register, `{"name":1,"kubeconfig":"a","context":"dev"}`, http.StatusBadRequest},
		"trailing":               {inspect, `{"kubeconfig":"a"} {}`, http.StatusBadRequest},
		"inspect extra":          {inspect, `{"kubeconfig":"a","context":"dev"}`, http.StatusBadRequest},
		"missing context":        {register, jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": doc, "context": ""}), http.StatusBadRequest},
		"unknown context":        {register, jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": doc, "context": "ghost"}), http.StatusBadRequest},
		"exec":                   {register, jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": ct.Document(ct.Context{Name: "eks", Cluster: "c", User: "u", Server: "https://e.example", UserExtra: []string{"exec:", "  command: aws"}}), "context": "eks"}), http.StatusBadRequest},
		"document too large":     {inspect, jsonBody(t, map[string]string{"kubeconfig": strings.Repeat("#", connectionapp.MaxKubeconfigBytes+1)}), http.StatusRequestEntityTooLarge},
		"envelope too large":     {register, `{"name":"` + strings.Repeat("x", 8<<20) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		resp, body := call(t, pe, http.MethodPost, tc.url, tc.body)
		if resp.StatusCode != tc.status || strings.Contains(body, uploadToken) {
			t.Errorf("%s: %d %s", name, resp.StatusCode, body)
		}
	}
	if verifier.calls != 0 || store.Len() != 0 {
		t.Fatalf("rejected requests verified (%d) or stored (%d)", verifier.calls, store.Len())
	}

	// Escaped JSON near the document limit still fits the 8 MiB envelope.
	escaped := ct.Single("big", "https://big.example", uploadToken) + "# " + strings.Repeat("\"", connectionapp.MaxKubeconfigBytes-1000) + "\n"
	if resp, body := call(t, pe, http.MethodPost, inspect, jsonBody(t, map[string]string{"kubeconfig": escaped})); resp.StatusCode != http.StatusOK {
		t.Fatalf("escaped near-limit document: %d %s", resp.StatusCode, body)
	}

	verifier.err = connectionapp.ErrPermissionDenied
	resp, body = call(t, pe, http.MethodPost, register, jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": doc, "context": "prod"}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "permission") || store.Len() != 0 {
		t.Fatalf("permission rejection: %d %s", resp.StatusCode, body)
	}
	verifier.err = nil

	resp, body = call(t, pe, http.MethodPost, register, jsonBody(t, map[string]string{"name": "Lab cluster", "kubeconfig": doc, "context": "prod"}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d %s", resp.StatusCode, body)
	}
	created := decode(t, body)
	if created["key"] != "lab-cluster" || created["name"] != "Lab cluster" || created["authenticationType"] != "KUBECONFIG" || created["status"] != "READY" {
		t.Fatalf("created DTO: %s", body)
	}
	resp, list := call(t, pe, http.MethodGet, server.URL+"/api/v1/connections", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(list, `"key":"lab-cluster"`) {
		t.Fatalf("list: %s", list)
	}
	for _, leaked := range []string{"secretRef", "memory://", "kubeconfig", uploadToken, key} {
		if strings.Contains(body, leaked) || strings.Contains(list, leaked) {
			t.Fatalf("public DTO exposes %q", leaked)
		}
	}
	org, err := app.Store.GetOrganization(context.Background(), "acme")
	if err != nil || org.DefaultConnectionKey != "internal-cluster" {
		t.Fatalf("organization default changed: %#v %v", org, err)
	}
	resp, body = call(t, pe, http.MethodPost, register, jsonBody(t, map[string]string{"name": "Lab cluster", "kubeconfig": doc, "context": "dev"}))
	if resp.StatusCode != http.StatusCreated || decode(t, body)["key"] != "lab-cluster-2" {
		t.Fatalf("collision: %d %s", resp.StatusCode, body)
	}

	// The legacy host-context shape remains an explicit, separate contract.
	resp, body = call(t, pe, http.MethodPost, register, `{"key":"legacy-one","clusterId":"kind-a","kubeContext":"kind-idp-internal"}`)
	if resp.StatusCode != http.StatusCreated || decode(t, body)["authenticationType"] != "HOST_CONTEXT" || strings.Contains(body, "secretRef") {
		t.Fatalf("legacy register: %d %s", resp.StatusCode, body)
	}
}

func TestUC04UploadWithoutCredentialStoreFailsClosed(t *testing.T) {
	server, verifier, _, app := uploadServer(t, false)
	pe := authenticatedClientAs(t, server.URL, "platform-engineer")
	resp, body := call(t, pe, http.MethodPost, server.URL+"/api/v1/connections/kubernetes", jsonBody(t, map[string]string{"name": "Lab", "kubeconfig": ct.Single("ctx", "https://c.example", uploadToken), "context": "ctx"}))
	if resp.StatusCode != http.StatusServiceUnavailable || verifier.calls != 0 {
		t.Fatalf("no store: %d %s", resp.StatusCode, body)
	}
	list, _ := app.Store.ListConnections(context.Background(), "acme")
	for _, conn := range list {
		if conn.Key == "lab" {
			t.Fatal("READY connection saved without a credential store")
		}
	}
}
