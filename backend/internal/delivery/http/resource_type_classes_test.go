package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/seed/seedtest"
)

func getTypes(t *testing.T, c *http.Client, base string) (int, string) {
	t.Helper()
	resp, err := c.Get(base + "/api/v1/resource-types")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// GET /resource-types is the all-authenticated read boundary of the class
// selector (UC-16 API mapping, I04-07): every authenticated role sees computed
// `classes` (sorted, unique, `[]` when none); the endpoint stays read-only and
// the registration POST refuses a `classes` field.
func TestResourceTypesExposeComputedClassesToEveryAuthenticatedRole(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	org := seedtest.Defaults().OrganizationKey
	if code, body := getTypes(t, &http.Client{Jar: func() *cookiejar.Jar { j, _ := cookiejar.New(nil); return j }()}, ts.URL); code != http.StatusUnauthorized || strings.Contains(body, "classes") {
		t.Fatalf("anonymous: %d %s", code, body)
	}
	for index, class := range []string{"in-cluster", "cloud-managed", "in-cluster"} {
		def := resource.Definition{Key: "pg-" + class + "-" + string(rune('a'+index)), Name: "pg-" + class + "-" + string(rune('a'+index)), Version: 1, ResourceTypeKey: "postgres", DriverType: resource.DriverKubernetes,
			DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"image": "postgres:16-alpine"}}}, Criteria: []resource.Criterion{{Class: class}}}
		if err := st.CreateResourceDefinition(context.Background(), org, def); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []string{"developer", "platform-engineer"} {
		code, body := getTypes(t, bundleClient(t, ts.URL, user), ts.URL)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", user, code, body)
		}
		var got struct {
			ResourceTypes []struct {
				Key     string   `json:"key"`
				Classes []string `json:"classes"`
			} `json:"resourceTypes"`
		}
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatal(err)
		}
		classes := map[string][]string{}
		for _, typ := range got.ResourceTypes {
			if typ.Classes == nil {
				t.Fatalf("%s: classes must be an array, not null", typ.Key)
			}
			classes[typ.Key] = typ.Classes
		}
		if strings.Join(classes["postgres"], ",") != "cloud-managed,in-cluster" || len(classes["vpc"]) != 0 {
			t.Fatalf("%s: classes %v", user, classes)
		}
		for _, leak := range []string{"sha256", "moduleId", "contentSha", "providerRequirements"} {
			if strings.Contains(body, leak) {
				t.Fatalf("%s: Type read exposes privileged registry pins (%s)", user, leak)
			}
		}
	}
	// `classes` is computed, never part of Type registration.
	pe := bundleClient(t, ts.URL, "platform-engineer")
	resp, err := pe.Post(ts.URL+"/api/v1/resource-types", "application/json", strings.NewReader(`{"key":"queue","inputs":[],"outputs":[],"classes":["x"]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST with classes: %d", resp.StatusCode)
	}
}

// A metadata failure is the safe generic 500; no empty or guessed class list.
func TestResourceTypesClassFailureIsSafeAndWriteFree(t *testing.T) {
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	spy := &failingTypesStore{Store: st}
	ts := httptest.NewServer(NewServer(Config{Authentication: authentication.NewService(st), Store: spy, SeedOptions: opts}))
	t.Cleanup(ts.Close)
	code, body := getTypes(t, bundleClient(t, ts.URL, "developer"), ts.URL)
	if code != http.StatusInternalServerError || strings.Contains(body, "classes") || strings.Contains(body, "secret-path") {
		t.Fatalf("status %d %s", code, body)
	}
	if spy.writes != 0 {
		t.Fatalf("read performed %d writes", spy.writes)
	}
}
