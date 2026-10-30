package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	appdelivery "orchestrator/internal/application/delivery"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/seed/seedtest"
)

func TestDeliverySelectionHTTPIsScopedRoleGuardedAndIntentOnly(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: opts.ApplicationKey, Key: "staging", NamespaceIdentity: "delivery-staging"}); err != nil {
		t.Fatal(err)
	}
	service := appdelivery.NewBindings(st, port.NewRegistry())
	server := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts, Delivery: service}))
	defer server.Close()
	developer := bundleClient(t, server.URL, "developer")
	engineer := bundleClient(t, server.URL, "platform-engineer")
	endpoint := server.URL + "/api/v1/applications/" + opts.ApplicationKey + "/environments/staging/delivery"
	get := func(client *http.Client) deliveryView {
		t.Helper()
		resp, err := client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("get: %d", resp.StatusCode)
		}
		var view deliveryView
		if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	before := get(developer)
	if before.Desired.Mode != delivery.Direct || before.Applied.Mode != delivery.Direct || before.Pending {
		t.Fatal("legacy environment enabled CD")
	}
	put := func(client *http.Client, body string) int {
		t.Helper()
		request, _ := http.NewRequest(http.MethodPut, endpoint, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	body := fmt.Sprintf(`{"expectedEnvironmentVersion":%d,"expectedConfigurationRevision":0,"mode":"DIRECT","selfHeal":false}`, before.EnvironmentVersion)
	if code := put(developer, body); code != 403 {
		t.Fatalf("developer selection mutation %d", code)
	}
	if code := put(engineer, body); code != 200 {
		t.Fatalf("platform no-op %d", code)
	}
	for _, bad := range []string{`{"mode":"DIRECT"}`, body + ` {}`, strings.Replace(body, `"selfHeal":false`, `"selfHeal":null`, 1), strings.Replace(body, `"mode":"DIRECT"`, `"mode":"fleet"`, 1), strings.Replace(body, `"selfHeal":false`, `"selfHeal":true`, 1)} {
		if code := put(engineer, bad); code != 400 {
			t.Fatalf("invalid selection got %d", code)
		}
	}
	after := get(developer)
	if after != before {
		t.Fatal("invalid/no-op request changed selection")
	}
}

func TestReconciliationGETIsStoredScopedAndUnavailableWithoutRuntime(t *testing.T) {
	ctx := context.Background()
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(ctx, st, opts); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEnvironment(ctx, environment.Environment{ApplicationKey: opts.ApplicationKey, Key: "staging", NamespaceIdentity: "reconciliation-staging"}); err != nil {
		t.Fatal(err)
	}
	coordinator := &appdelivery.Coordinator{Store: st, Registry: port.NewRegistry()}
	server := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: st, SeedOptions: opts, Reconciliation: coordinator}))
	defer server.Close()
	client := bundleClient(t, server.URL, "developer")
	endpoint := server.URL + "/api/v1/applications/" + opts.ApplicationKey + "/environments/staging/reconciliation"
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	var result struct {
		Units    []any  `json:"units"`
		Next     string `json:"nextCursor"`
		Interval int    `json:"intervalSeconds"`
	}
	if json.NewDecoder(response.Body).Decode(&result) != nil || result.Units == nil || len(result.Units) != 0 || result.Interval != 30 {
		t.Fatal("missing observation was not explicit")
	}
	// No provider is registered. A successful stored read cannot have contacted one.
	for _, suffix := range []string{"?limit=201", "?limit=0", "?cursor=invalid"} {
		r, err := client.Get(endpoint + suffix)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal("invalid pagination admitted", r.StatusCode)
		}
	}
	r, err := client.Get(strings.Replace(endpoint, opts.ApplicationKey, "foreign", 1))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 404 {
		t.Fatal("foreign scope exposed", r.StatusCode)
	}
}
