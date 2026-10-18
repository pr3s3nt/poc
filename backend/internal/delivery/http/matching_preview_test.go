package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const previewPath = "/api/v1/resource-definitions/matching-preview"

func postPreview(t *testing.T, c *http.Client, base, body string) (int, string) {
	t.Helper()
	resp, err := c.Post(base+previewPath, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

const previewBody = `{"definition":{"key":"pg-new","resourceType":"postgres","driverType":"terraform","criteria":[{"class":"fast"}],"driverInputs":{"values":{"variables":{"password":"S3CRET-VALUE"}}}},` +
	`"context":{"applicationId":"acceptance","environmentId":"dev","resourceId":"db","class":"fast"}}`

func TestMatchingPreviewAuthRoleAndScope(t *testing.T) {
	_, ts, _ := bundleTestServer(t, nil)
	if code, _ := postPreview(t, &http.Client{}, ts.URL, previewBody); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	if code, body := postPreview(t, bundleClient(t, ts.URL, "developer"), ts.URL, previewBody); code != http.StatusForbidden || strings.Contains(body, "pg-new") {
		t.Fatalf("developer: %d %s", code, body)
	}
	// Admin of another organization cannot see this organization's Application.
	if code, _ := postPreview(t, bundleClient(t, ts.URL, "other-admin"), ts.URL, previewBody); code != http.StatusNotFound {
		t.Fatalf("other org: %d", code)
	}
	pe := bundleClient(t, ts.URL, "platform-engineer")
	missing := strings.Replace(previewBody, `"environmentId":"dev"`, `"environmentId":"nope"`, 1)
	if code, _ := postPreview(t, pe, ts.URL, missing); code != http.StatusNotFound {
		t.Fatalf("missing env: %d", code)
	}
}

func TestMatchingPreviewSuccessIsSafeAndReadOnly(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	before, _ := st.ListResourceDefinitions(context.Background(), "acme")
	code, body := postPreview(t, pe, ts.URL, previewBody)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	if strings.Contains(body, "S3CRET-VALUE") || strings.Contains(body, "driverInputs") {
		t.Fatalf("response leaks inputs: %s", body)
	}
	var got struct {
		Status   string `json:"status"`
		Winner   *struct{ Key string }
		Snapshot struct {
			Marker             string
			EnvironmentVersion int64
		}
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "MATCHED" || got.Winner == nil || got.Winner.Key != "pg-new" || got.Snapshot.Marker == "" {
		t.Fatalf("%s", body)
	}
	after, _ := st.ListResourceDefinitions(context.Background(), "acme")
	if len(after) != len(before) {
		t.Fatal("preview inserted a definition")
	}
	for _, d := range after {
		if d.Key == "pg-new" {
			t.Fatal("candidate persisted")
		}
	}
}

func TestMatchingPreviewStrictBoundedErrors(t *testing.T) {
	_, ts, _ := bundleTestServer(t, nil)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	cases := map[string]struct {
		body string
		code int
	}{
		"unknown field":   {strings.Replace(previewBody, `"context":`, `"organizationKey":"x","context":`, 1), http.StatusBadRequest},
		"unknown context": {strings.Replace(previewBody, `"class":"fast"}}`, `"class":"fast","profile":"aws-eks"}}`, 1), http.StatusBadRequest},
		"not object":      {`[]`, http.StatusBadRequest},
		"trailing":        {previewBody + `{}`, http.StatusBadRequest},
		"reserved":        {strings.Replace(previewBody, `"pg-new"`, `"builtin-existing-cluster"`, 1), http.StatusBadRequest},
		"unknown type":    {strings.Replace(previewBody, `"postgres"`, `"nope"`, 1), http.StatusBadRequest},
		"oversize":        {`{"definition":{"key":"` + strings.Repeat("a", 2<<20) + `"}}`, http.StatusRequestEntityTooLarge},
	}
	for name, c := range cases {
		code, body := postPreview(t, pe, ts.URL, c.body)
		if code != c.code {
			t.Fatalf("%s: %d %s", name, code, body)
		}
		if strings.Contains(body, "S3CRET-VALUE") {
			t.Fatalf("%s leaks: %s", name, body)
		}
	}
}

// UC-03 V02-05: ordinary matching needs no configured Environment target.
func TestMatchingPreviewWorksForUnconfiguredEnvironment(t *testing.T) {
	_, ts, st := bundleTestServer(t, nil)
	env, err := st.GetEnvironment(context.Background(), "acceptance", "dev")
	if err != nil {
		t.Fatal(err)
	}
	env.Key, env.ID, env.ConnectionKey, env.Profile = "unbound", "unbound-id", "", ""
	if err := st.SaveEnvironment(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	pe := bundleClient(t, ts.URL, "platform-engineer")
	code, body := postPreview(t, pe, ts.URL, strings.Replace(previewBody, `"environmentId":"dev"`, `"environmentId":"unbound"`, 1))
	if code != http.StatusOK || strings.Contains(body, "unbound-id") || !strings.Contains(body, `"status":"MATCHED"`) {
		t.Fatalf("%d %s", code, body)
	}
}

func TestMatchingPreviewErrorsNeverEchoSubmittedValues(t *testing.T) {
	_, ts, _ := bundleTestServer(t, nil)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	const secret = "SENTINEL-s3cret"
	bodies := []string{
		strings.Replace(previewBody, `"driverType":"terraform"`, `"driverType":"`+secret+`"`, 1),
		strings.Replace(previewBody, `"resourceType":"postgres"`, `"resourceType":"`+secret+`"`, 1),
		strings.Replace(previewBody, `"key":"pg-new"`, `"key":"`+secret+`_bad"`, 1),
		strings.Replace(previewBody, `"criteria":[`, `"`+secret+`":1,"criteria":[`, 1),
		strings.Replace(previewBody, `"class":"fast"}]`, `"class":{"x":"`+secret+`"}}]`, 1),
		`{"definition":` + secret,
	}
	for i, body := range bodies {
		code, resp := postPreview(t, pe, ts.URL, body)
		if code != http.StatusBadRequest || strings.Contains(resp, secret) {
			t.Fatalf("case %d: %d %s", i, code, resp)
		}
	}
}

func TestMatchingPreviewWhitespaceContext(t *testing.T) {
	_, ts, _ := bundleTestServer(t, nil)
	pe := bundleClient(t, ts.URL, "platform-engineer")
	code, body := postPreview(t, pe, ts.URL, strings.Replace(previewBody, `"resourceId":"db"`, `"resourceId":"  "`, 1))
	if code != http.StatusOK || !strings.Contains(body, `"status":"INCOMPLETE"`) || strings.Contains(body, `"winner"`) || strings.Contains(body, `"ties"`) {
		t.Fatalf("%d %s", code, body)
	}
	code, body = postPreview(t, pe, ts.URL, strings.Replace(previewBody, `"environmentId":"dev"`, `"environmentId":" "`, 1))
	if code != http.StatusBadRequest || !strings.Contains(body, "context.environmentId") {
		t.Fatalf("%d %s", code, body)
	}
}
