package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed/seedtest"
)

func getCapabilities(t *testing.T, c *http.Client, base, query string) (int, string) {
	t.Helper()
	resp, err := c.Get(base + "/api/v1/resource-definition-capabilities" + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

// TestDefinitionCapabilitiesAuthShapeAndNoWrites covers OC-04d: session auth,
// PE/Admin only, sorted safe metadata, no mutation, no org override.
func TestDefinitionCapabilitiesAuthShapeAndNoWrites(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	jar, _ := cookiejar.New(nil)
	if code, body := getCapabilities(t, &http.Client{Jar: jar}, ts.URL, ""); code != http.StatusUnauthorized || strings.Contains(body, "capabilities") {
		t.Fatalf("anonymous: %d %s", code, body)
	}
	if code, body := getCapabilities(t, bundleClient(t, ts.URL, "developer"), ts.URL, ""); code != http.StatusForbidden || strings.Contains(body, "capabilities") {
		t.Fatalf("developer: %d %s", code, body)
	}
	org := seedtest.Defaults().OrganizationKey
	typesBefore, _ := st.ListResourceTypes(context.Background(), org)
	defsBefore, _ := st.ListResourceDefinitions(context.Background(), org)

	pe := bundleClient(t, ts.URL, "platform-engineer")
	code, body := getCapabilities(t, pe, ts.URL, "?organization=other-org")
	if code != http.StatusOK {
		t.Fatalf("platform engineer: %d %s", code, body)
	}
	var got struct {
		Capabilities []struct {
			ResourceType  string           `json:"resourceType"`
			DriverType    string           `json:"driverType"`
			Module        string           `json:"module"`
			Variables     []map[string]any `json:"variables"`
			ExecutorOwned []string         `json:"executorOwned"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	var pairs []string
	for _, c := range got.Capabilities {
		pairs = append(pairs, c.ResourceType+"/"+c.DriverType+"/"+c.Module)
		if c.Variables == nil || c.ExecutorOwned == nil {
			t.Fatalf("%s: arrays must not be null", c.ResourceType)
		}
		for _, v := range c.Variables {
			for key := range v {
				switch key {
				case "name", "type", "required", "hasDefault", "default", "resourceParam", "preset":
				default:
					t.Fatalf("unexpected public field %q", key)
				}
			}
			if name := v["name"]; name == "region" || name == "tags" || name == "master_password" {
				t.Fatalf("executor-owned %v exposed", name)
			}
		}
	}
	if want := []string{"k8s-cluster/terraform/eks", "k8s-namespace/kubernetes/", "k8s-namespace/terraform/k8s-namespace", "postgres/kubernetes/", "postgres/terraform/aurora", "postgres/terraform/postgres-k8s", "redis/terraform/elasticache", "redis/terraform/redis-k8s", "vpc/terraform/vpc"}; !reflect.DeepEqual(pairs, want) {
		t.Fatalf("pairs %v", pairs)
	}
	// Image defaults are public digest pins ("name@sha256:..."); an internal
	// module fingerprint would be a bare "sha256:..." value.
	for _, leak := range []string{"fingerprint", "\"sha256:", ".tf", "existing-cluster", "score-k8s"} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaks %q", leak)
		}
	}
	if code, _ := getCapabilities(t, bundleClient(t, ts.URL, "other-admin"), ts.URL, ""); code != http.StatusOK {
		t.Fatalf("admin of another organization: %d", code)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, _ := http.NewRequest(method, ts.URL+"/api/v1/resource-definition-capabilities", strings.NewReader(`{}`))
		resp, err := pe.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Fatalf("%s accepted: %d", method, resp.StatusCode)
		}
	}
	typesAfter, _ := st.ListResourceTypes(context.Background(), org)
	defsAfter, _ := st.ListResourceDefinitions(context.Background(), org)
	if !reflect.DeepEqual(typesBefore, typesAfter) || !reflect.DeepEqual(defsBefore, defsAfter) {
		t.Fatal("capability metadata changed the catalog")
	}
}

// failingTypesStore fails the Resource Type read with a marker that must never
// reach the client and counts every write path.
type failingTypesStore struct {
	persistence.Store
	writes int
}

const leakMarker = "/srv/secret-path/AKIA-LEAK-MARKER"

func (s *failingTypesStore) ListResourceTypes(context.Context, string) ([]resource.Type, error) {
	return nil, errors.New("inspector failed at " + leakMarker)
}
func (s *failingTypesStore) Transact(ctx context.Context, fn func(context.Context) error) error {
	s.writes++
	return s.Store.Transact(ctx, fn)
}
func (s *failingTypesStore) CreateResourceType(ctx context.Context, org string, typ resource.Type) error {
	s.writes++
	return s.Store.CreateResourceType(ctx, org, typ)
}
func (s *failingTypesStore) CreateResourceDefinition(ctx context.Context, org string, def resource.Definition) error {
	s.writes++
	return s.Store.CreateResourceDefinition(ctx, org, def)
}

// TestDefinitionCapabilitiesInternalErrorIsSafeAndWriteFree: a store/inspection
// failure is the generic T20 500 envelope, with no raw text and no write.
func TestDefinitionCapabilitiesInternalErrorIsSafeAndWriteFree(t *testing.T) {
	st := store.New()
	opts := seedtest.Defaults()
	if err := seedtest.Apply(context.Background(), st, opts); err != nil {
		t.Fatal(err)
	}
	spy := &failingTypesStore{Store: st}
	s := NewServer(Config{Authentication: authentication.NewService(st), Store: spy, SeedOptions: opts})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	code, body := getCapabilities(t, bundleClient(t, ts.URL, "platform-engineer"), ts.URL, "")
	if code != http.StatusInternalServerError {
		t.Fatalf("status %d %s", code, body)
	}
	for _, leak := range []string{leakMarker, "secret-path", "AKIA", "inspector failed", "capabilities"} {
		if strings.Contains(body, leak) {
			t.Fatalf("500 body leaks %q: %s", leak, body)
		}
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(body), &envelope); err != nil || envelope["code"] != "INTERNAL_ERROR" {
		t.Fatalf("not the generic envelope: %s", body)
	}
	if spy.writes != 0 {
		t.Fatalf("metadata read performed %d writes", spy.writes)
	}
}
