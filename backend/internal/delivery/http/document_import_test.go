package http

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/application/catalog"
	"orchestrator/internal/seed/seedtest"
)

func TestDocumentImportStrictAndScoped(t *testing.T) {
	_, ts, _ := bundleTestServer(t, nil)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	dev := bundleClient(t, ts.URL, "developer")
	base := ts.URL + "/api/v1/resource-types/import"
	body := `{"key":"document-type","inputs":[],"outputs":[{"name":"host","type":"string"}]}`
	if code, out := request(t, dev, "POST", base, body); code != 403 {
		t.Fatal(code, out)
	}
	for _, status := range []string{"CREATED", "REUSED"} {
		if code, out := request(t, pe, "POST", base, body); code != 200 || !strings.Contains(out, status) {
			t.Fatal(code, out)
		}
	}
	if code, out := request(t, pe, "POST", base, strings.Replace(body, `"string"`, `"number"`, 1)); code != 200 || !strings.Contains(out, "CONFLICT") {
		t.Fatal(code, out)
	}
	for _, field := range []string{"version", "revisionId", "sourceFingerprint", "renderPin", "executionProfile", "connectionKey", "unknown"} {
		bad := strings.TrimSuffix(body, "}") + `,"` + field + `":1}`
		if code, out := request(t, pe, "POST", base, bad); code != 400 {
			t.Fatal(field, code, out)
		}
	}
	if code, out := request(t, pe, "POST", base, `{"key":"huge","unknown":"`+strings.Repeat("a", managementBodyLimit)+`"}`); code != 413 {
		t.Fatal(code, out)
	}
}

func TestDefinitionDocumentPrepareAndExportAreStrictScopedAndReadOnly(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	dev := bundleClient(t, ts.URL, "developer")
	other := bundleClient(t, ts.URL, "other-admin")
	const body = `{"key":"document-edge","name":"Document edge","resourceType":"endpoint","driverType":"endpoint","criteria":[{}],"provision":{},"driverInputs":{"values":{"variables":{"rootDomain":"apps.example.com","scheme":"https","gatewayName":"traefik","gatewayNamespace":"traefik"}}}}`
	typeBody := `{"key":"endpoint","outputs":[{"name":"hostname","type":"string"},{"name":"url","type":"string"}]}`
	if code, out := request(t, pe, "POST", ts.URL+"/api/v1/resource-types/import", typeBody); code != 200 {
		t.Fatal(code, out)
	}
	prepare := ts.URL + "/api/v1/resource-definitions/import/prepare"
	for _, test := range []struct {
		client *http.Client
		want   int
	}{{&http.Client{}, 401}, {dev, 403}, {other, 400}} {
		if code, out := request(t, test.client, "POST", prepare, body); code != test.want {
			t.Fatal(code, out)
		}
	}
	for _, invalid := range []string{
		strings.TrimSuffix(body, "}") + `,"version":1}`,
		strings.TrimSuffix(body, "}") + `,"unknown":"private-value"}`,
		`null`, `[]`, body + `{}`,
	} {
		if code, out := request(t, pe, "POST", prepare, invalid); code != 400 || strings.Contains(out, "private-value") {
			t.Fatal(code, out)
		}
	}
	if code, out := request(t, pe, "POST", prepare, `{"unknown":"`+strings.Repeat("a", managementBodyLimit)+`"}`); code != 413 {
		t.Fatal(code, out)
	}
	org := seedtest.Defaults().OrganizationKey
	before, _ := st.ListResourceDefinitions(context.Background(), org)
	code, out := request(t, pe, "POST", prepare, body)
	if code != 200 {
		t.Fatal(code, out)
	}
	var result catalog.PreparedDefinitionDocument
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	after, _ := st.ListResourceDefinitions(context.Background(), org)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("prepare wrote Definitions")
	}
	canonical, _ := json.Marshal(result.Definition)
	if code, out := request(t, pe, "POST", ts.URL+"/api/v1/resource-definitions/import", string(canonical)); code != 200 || !strings.Contains(out, "CREATED") {
		t.Fatal(code, out)
	}
	history, _ := st.GetDefinitionHistory(context.Background(), org, "document-edge")
	if len(history.Versions) != 1 {
		t.Fatalf("history %+v", history)
	}
	for _, query := range []string{"", "?version=1", "?revision=" + history.Versions[0].ID} {
		url := ts.URL + "/api/v1/resource-definitions/document-edge/document" + query
		if code, out := request(t, dev, "GET", url, ""); code != 403 {
			t.Fatal(code, out)
		}
		if code, out := request(t, other, "GET", url, ""); code != 404 {
			t.Fatal(code, out)
		}
		code, out := request(t, pe, "GET", url, "")
		if code != 200 {
			t.Fatal(code, out)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"version", "revisionId", "renderPin", "sourceFingerprint"} {
			if _, exists := doc[field]; exists {
				t.Fatalf("exported %s", field)
			}
		}
		if code, out := request(t, pe, "POST", prepare, out); code != 200 {
			t.Fatal(code, out)
		}
	}
	if code, out := request(t, pe, "GET", ts.URL+"/api/v1/resource-definitions/document-edge/document?revision=foreign", ""); code != 404 {
		t.Fatal(code, out)
	}
	for _, query := range []string{"?version=0", "?version=abc", "?version=1&version=2", "?version=1&revision=foreign"} {
		if code, out := request(t, pe, "GET", ts.URL+"/api/v1/resource-definitions/document-edge/document"+query, ""); code != 400 {
			t.Fatal(code, out)
		}
	}

}
