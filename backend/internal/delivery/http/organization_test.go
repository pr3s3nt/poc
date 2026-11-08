package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"orchestrator/internal/domain/application"
)

func TestOrganizationReadUsesOnlySessionIdentity(t *testing.T) {
	ts, st := newUsersServer(t)
	code, _ := request(t, http.DefaultClient, http.MethodGet, ts.URL+"/api/v1/organization", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("anonymous %d", code)
	}
	if err := st.SaveOrganization(context.Background(), application.Organization{ID: "other-id", Key: "other", Name: "Other private organization"}); err != nil {
		t.Fatal(err)
	}
	c := signInClient(t, ts.URL, "admin", adminTestPassword)
	code, body := request(t, c, http.MethodGet, ts.URL+"/api/v1/organization?organization=other", "")
	var view struct {
		Organization application.Organization `json:"organization"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	if code != http.StatusOK || view.Organization.Key != "acme" || view.Organization.Name != "Acme" {
		t.Fatalf("wrong identity: %d %s", code, body)
	}
}
