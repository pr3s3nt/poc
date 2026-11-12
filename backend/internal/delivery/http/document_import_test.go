package http

import (
	"strings"
	"testing"
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
