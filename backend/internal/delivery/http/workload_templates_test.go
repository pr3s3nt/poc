package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/seed/seedtest"
)

func TestWorkloadTemplateCatalogIsScopedAndReadOnly(t *testing.T) {
	_, server, store := bundleTestServer(t, nil)
	org := seedtest.Defaults().OrganizationKey
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	template := renderbundle.WorkloadTemplate{ID: "stateful", Version: "1.0.0", Kind: "StatefulSet", Labels: map[string]string{"vi": "Stateful"}, InputsSchema: schema, Defaults: map[string]any{}}
	record := renderbundle.Bundle{ID: ids.New(), OrganizationKey: org, Source: renderbundle.Source{Kind: "git", URL: "https://example.com/r.git", Path: ".", Revision: strings.Repeat("a", 40), ContentSHA256: strings.Repeat("b", 64)}, EngineContract: renderbundle.EngineContract, RendererVersion: renderbundle.RendererVersion, RendererBinary: strings.Repeat("c", 64), Contract: renderbundle.Contract{InputContract: renderbundle.InputContract, InputsSchema: schema, Defaults: map[string]any{}}, CreatedAt: time.Now()}
	record.Contract.Manifest.WorkloadTemplates = []renderbundle.WorkloadTemplate{template}
	if _, err := store.InsertRenderBundle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/applications/" + seedtest.Defaults().ApplicationKey + "/environments/staging/workload-templates"
	client := bundleClient(t, server.URL, "developer")
	response, err := client.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	var result struct{ Templates []struct{ ID, SHA256 string } }
	if err = json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Templates) != 1 || result.Templates[0].ID != "stateful" || result.Templates[0].SHA256 != template.SHA256() {
		t.Fatalf("bad catalog: %s", body)
	}
	foreign := bundleClient(t, server.URL, "other-admin")
	reply, err := foreign.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	reply.Body.Close()
	if reply.StatusCode == http.StatusOK {
		t.Fatal("foreign organization saw app catalog")
	}
}
