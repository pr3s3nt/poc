package http

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/seed/seedtest"
)

type inspectionRegistry struct {
	moduleregistry.Registry
	calls  int
	bundle tfmodule.Bundle
	err    error
}

func (r *inspectionRegistry) InspectGit(context.Context, moduleregistry.GitInspection) (tfmodule.Bundle, error) {
	r.calls++
	return r.bundle, r.err
}

func TestInspectTerraformModuleAuthorizationScopeAndNoDefinitionWrites(t *testing.T) {
	server, ts, st := bundleTestServer(t, nil)
	modules := registrytest.Open(t, st)
	bundles, err := modules.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spy := &inspectionRegistry{Registry: modules, bundle: bundles[0]}
	server.catalog = catalog.NewService(st, spy)
	const endpoint = "/api/v1/terraform-modules/inspect"
	post := func(client *http.Client, body string) (int, string) {
		t.Helper()
		response, err := client.Post(ts.URL+endpoint, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(data)
	}
	const body = `{"resourceType":"vpc","url":"https://example.com/repo","path":".","ref":"v1"}`
	for _, test := range []struct {
		client *http.Client
		want   int
	}{{&http.Client{}, 401}, {bundleClient(t, ts.URL, "developer"), 403}, {bundleClient(t, ts.URL, "other-admin"), 400}} {
		code, _ := post(test.client, body)
		if code != test.want {
			t.Fatalf("status %d want %d", code, test.want)
		}
	}
	if spy.calls != 0 {
		t.Fatal("unauthorized/unscoped request reached Git")
	}
	pe := bundleClient(t, ts.URL, "platform-engineer")
	for _, invalid := range []string{`null`, `[]`, `{} {}`, `{"resourceType":"vpc","credential":"private-key"}`} {
		code, response := post(pe, invalid)
		if code != 400 || strings.Contains(response, "private-key") {
			t.Fatalf("invalid body: %d %s", code, response)
		}
	}
	before, _ := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey)
	code, response := post(pe, body)
	if code != 200 || !strings.Contains(response, `"capability"`) || spy.calls != 1 {
		t.Fatalf("inspect: %d %s", code, response)
	}
	after, _ := st.ListResourceDefinitions(context.Background(), seedtest.Defaults().OrganizationKey)
	if len(before) != len(after) {
		t.Fatal("inspection changed Definitions")
	}
	spy.err = moduleregistry.ErrUnavailable
	code, response = post(pe, body)
	if code != 400 || strings.Contains(response, "https://example.com") {
		t.Fatalf("unsafe fetch failure: %d %s", code, response)
	}
}
