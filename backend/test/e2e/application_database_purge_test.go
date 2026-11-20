package e2e

import (
	"context"
	"net/http"
	"testing"

	"orchestrator/internal/adapters/postgres"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestDatabasePurgeOverHTTP(t *testing.T) {
	f := newRCFixture(t)
	ctx := context.Background()
	admin := f.signInAdmin()
	appPath := f.server.URL + "/api/v1/applications"
	path := appPath + "/" + f.appKey + "/database-purge"
	expect := func(client *http.Client, method, url string, body any, want int) map[string]any {
		t.Helper()
		status, raw, data := f.do(client, method, url, body)
		if status != want {
			t.Fatalf("%s %s = %d, expected %d: %s", method, url, status, want, raw)
		}
		return data
	}

	expect(admin, http.MethodGet, path, nil, http.StatusNotFound)
	if expect(admin, http.MethodGet, appPath, nil, http.StatusOK)["databasePurgeEnabled"] == true {
		t.Fatal("purge exposed before opt-in")
	}
	if err := postgres.SetWebApplicationPurge(ctx, f.db.Owner, true); err != nil {
		t.Fatal(err)
	}
	expect(f.dev, http.MethodGet, path, nil, http.StatusForbidden)
	expect(f.foreign, http.MethodGet, path, nil, http.StatusNotFound)
	for _, client := range []*http.Client{admin, f.pe} {
		if expect(client, http.MethodGet, appPath, nil, http.StatusOK)["databasePurgeEnabled"] != true {
			t.Fatal("purge capability missing")
		}
	}

	preview := expect(admin, http.MethodGet, path, nil, http.StatusOK)["preview"].(map[string]any)
	if preview["displayName"] != "Payments" || len(preview["environments"].([]any)) != 2 {
		t.Fatal(preview)
	}
	originalID := preview["applicationId"]
	body := map[string]any{"applicationId": ids.New(), "confirmationName": "Payments"}
	expect(f.pe, http.MethodPost, path, body, http.StatusBadRequest)
	body["applicationId"], body["confirmationName"] = originalID, "payments"
	expect(f.pe, http.MethodPost, path, body, http.StatusBadRequest)
	body["confirmationName"] = "Payments"

	const typeKey, defKey = "test-purge-type", "test-purge-def"
	if err := f.app.Store.CreateResourceType(ctx, f.orgKey, resource.Type{Key: typeKey}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Store.CreateResourceDefinition(ctx, f.orgKey, resource.Definition{
		Key: defKey, Name: "Purgeable definition", Version: 1,
		ResourceTypeKey: typeKey, DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.Store.UpsertActiveResource(ctx, resource.ActiveResource{
		OrganizationKey: f.orgKey, Descriptor: resource.Descriptor{Type: typeKey, Class: "default", ID: "shared.data"},
		Scope: resource.Scope{Type: resource.ScopeShared, ID: f.appKey + ".staging"}, DefinitionKey: defKey,
		Status: resource.StatusReady, Outputs: map[string]any{}, ExecutorState: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	definitionPath := f.server.URL + "/api/v1/resource-definitions/" + defKey
	expect(f.pe, http.MethodDelete, definitionPath, nil, http.StatusConflict)

	_, op := f.claim("staging")
	expect(f.pe, http.MethodPost, path, body, http.StatusConflict)
	if err := f.app.Store.SuspendOperation(ctx, persistence.Owner{OperationID: op.ID, Owner: op.Owner, Fence: op.Fence}, "fixture stop"); err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE applications SET status='DELETING',deletion='{"status":"FAILED"}' WHERE id=$1::uuid`, originalID)
	stalled := expect(f.pe, http.MethodGet, path, nil, http.StatusOK)["preview"].(map[string]any)
	if stalled["busy"] == true || stalled["priorStopRequired"] != true {
		t.Fatal(stalled)
	}
	expect(f.pe, http.MethodPost, path, body, http.StatusConflict)
	body["priorExecutionStopped"] = true
	expect(f.foreign, http.MethodPost, path, body, http.StatusNotFound)
	expect(f.dev, http.MethodPost, path, body, http.StatusForbidden)

	for _, name := range []string{"Admin disposable", "Retained other app"} {
		expect(admin, http.MethodPost, appPath, map[string]string{"name": name}, http.StatusCreated)
	}
	expect(f.pe, http.MethodPost, path, body, http.StatusNoContent)
	expect(f.pe, http.MethodGet, path, nil, http.StatusNotFound)
	expect(f.pe, http.MethodDelete, definitionPath, nil, http.StatusNoContent)
	expect(f.pe, http.MethodDelete, f.server.URL+"/api/v1/resource-types/"+typeKey, nil, http.StatusNoContent)
	if f.count("SELECT count(*) FROM applications WHERE id=$1::uuid", originalID) != 0 ||
		f.count("SELECT count(*) FROM environment_operations WHERE id=$1::uuid", op.ID) != 0 ||
		f.count("SELECT count(*) FROM applications WHERE name='Retained other app'") != 1 {
		t.Fatal("app scope was not purged correctly")
	}

	var secondKey string
	err := f.conn().QueryRow(ctx, "SELECT application_key FROM applications WHERE name='Admin disposable'").Scan(&secondKey)
	if err != nil {
		t.Fatal(err)
	}
	secondPath := appPath + "/" + secondKey + "/database-purge"
	second := expect(admin, http.MethodGet, secondPath, nil, http.StatusOK)["preview"].(map[string]any)
	expect(admin, http.MethodPost, secondPath, map[string]any{"applicationId": second["applicationId"], "confirmationName": "Admin disposable"}, http.StatusNoContent)
	if err := postgres.SetWebApplicationPurge(ctx, f.db.Owner, false); err != nil {
		t.Fatal(err)
	}
	expect(admin, http.MethodGet, secondPath, nil, http.StatusNotFound)
}
