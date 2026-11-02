package http

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/internal/adapters/renderbundles"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/seed/seedtest"
)

type uploadRuntime struct{}

func (uploadRuntime) BinaryDigest() string { return strings.Repeat("b", 64) }
func (uploadRuntime) CheckBinary() error   { return nil }

func sourceArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range files {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func multipartUpload(t *testing.T, client *http.Client, url string, fields [][2]string, data []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, field := range fields {
		if err := w.WriteField(field[0], field[1]); err != nil {
			t.Fatal(err)
		}
	}
	part, err := w.CreateFormFile("file", "source.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(url, w.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("non-JSON response %d", resp.StatusCode)
	}
	return resp.StatusCode, result
}
func TestUploadedSourcesUseRealInspectorsAndNeverRegisterDefinitions(t *testing.T) {
	server, ts, st := bundleTestServer(t, nil)
	modules := registrytest.Open(t, st)
	server.catalog = catalog.NewService(st, modules)
	cache, err := renderbundles.OpenCache(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	server.catalog.SetRenderRegistry(&renderbundles.Registry{Cache: cache, Store: st, Runtime: uploadRuntime{}})
	pe := bundleClient(t, ts.URL, "platform-engineer")
	ctx := context.Background()
	org := seedtest.Defaults().OrganizationKey
	before, err := st.ListResourceDefinitions(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	// Use an exact packaged module to prove ZIP retains trusted content identity.
	release, err := registry.DefaultRelease()
	if err != nil {
		t.Fatal(err)
	}
	module := release.Packaged.Modules[0]
	files := map[string][]byte{}
	err = fs.WalkDir(terraform.PackagedModules(), module.Subdirectory, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(terraform.PackagedModules(), name)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(name, module.Subdirectory+"/")
		files["repo/modules/test/"+rel] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data := sourceArchive(t, files)
	fields := [][2]string{{"resourceType", module.ResourceTypes[0]}, {"path", "repo/modules/test"}}
	endpoint := ts.URL + "/api/v1/terraform-modules/upload"
	code, body := multipartUpload(t, pe, endpoint, fields, data)
	if code != 200 {
		t.Fatalf("module: %d %v", code, body)
	}
	capability := body["capability"].(map[string]any)
	pin := capability["source"].(map[string]any)
	if !strings.HasPrefix(pin["sourceId"].(string), "zip-") || pin["moduleId"] != module.ID {
		t.Fatalf("module pin %v", pin)
	}
	// Uploaded module schemas and their pins are private to the session org.
	if err := st.SaveOrganization(ctx, application.Organization{ID: "other-org-id", Key: "other-org", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	types, err := st.ListResourceTypes(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range types {
		if typ.Key == module.ResourceTypes[0] {
			if err := st.SaveResourceType(ctx, "other-org", typ); err != nil {
				t.Fatal(err)
			}
		}
	}
	foreign := bundleClient(t, ts.URL, "other-admin")
	code, discovery := call(t, foreign, "GET", ts.URL+"/api/v1/resource-definition-capabilities", "")
	if code != 200 {
		t.Fatalf("foreign discovery %d", code)
	}
	for _, item := range discovery["capabilities"].([]any) {
		if source, ok := item.(map[string]any)["source"].(map[string]any); ok && source["sourceId"] == pin["sourceId"] {
			t.Fatal("ZIP module metadata crossed Organizations")
		}
	}
	foreignDraft := map[string]any{"key": "foreign-zip", "name": "Foreign ZIP", "resourceType": module.ResourceTypes[0], "driverType": "terraform", "criteria": []any{map[string]any{}}, "driverInputs": map[string]any{"values": map[string]any{
		"source":    map[string]any{"source_id": pin["sourceId"], "module_id": pin["moduleId"], "revision": pin["revision"], "subdirectory": pin["subdirectory"], "content_sha256": pin["contentSha256"]},
		"variables": map[string]any{}, "input_mapping": map[string]any{}, "output_mapping": map[string]any{}, "provider_bindings": map[string]any{},
	}}}
	foreignJSON, err := json.Marshal(foreignDraft)
	if err != nil {
		t.Fatal(err)
	}
	code, rejected := call(t, foreign, "POST", ts.URL+"/api/v1/resource-definitions", string(foreignJSON))
	if code != 400 || rejected["field"] != "definition.driverInputs.values.source" {
		t.Fatalf("foreign ZIP pin admitted or rejected for wrong reason: %d %v", code, rejected)
	}
	code, own := multipartUpload(t, foreign, endpoint, fields, data)
	if code != 200 || own["capability"].(map[string]any)["source"].(map[string]any)["sourceId"] == pin["sourceId"] {
		t.Fatalf("same-content Organization separation: %d %v", code, own)
	}
	againCode, again := multipartUpload(t, pe, endpoint, fields, data)
	if againCode != 200 || again["capability"].(map[string]any)["source"].(map[string]any)["revision"] != pin["revision"] {
		t.Fatalf("repeat %d %v", againCode, again)
	}
	bundleFiles := map[string][]byte{}
	root := "../../../render-bundles/standard"
	err = filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, name)
		data, err := os.ReadFile(name)
		bundleFiles["repo/bundles/standard/"+filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	zip := sourceArchive(t, bundleFiles)
	endpoint = ts.URL + "/api/v1/render-bundles/upload"
	code, body = multipartUpload(t, pe, endpoint, [][2]string{{"path", "repo/bundles/standard"}}, zip)
	if code != 200 {
		t.Fatalf("bundle: %d %v", code, body)
	}
	bundle := body["renderBundle"].(map[string]any)
	if bundle["source"].(map[string]any)["kind"] != "zip" || bundle["status"] != "AVAILABLE" {
		t.Fatalf("bundle pin %v", bundle)
	}
	id := bundle["id"].(string)
	if code, _ := call(t, bundleClient(t, ts.URL, "other-admin"), "GET", ts.URL+"/api/v1/render-bundles/"+id, ""); code != 404 {
		t.Fatalf("foreign access %d", code)
	}
	after, err := st.ListResourceDefinitions(ctx, org)
	if err != nil || len(after) != len(before) {
		t.Fatalf("upload registered a Definition: %v", err)
	}
	for _, endpoint := range []string{"terraform-modules/upload", "render-bundles/upload"} {
		url := ts.URL + "/api/v1/" + endpoint
		for _, client := range []*http.Client{{}, bundleClient(t, ts.URL, "developer")} {
			code, _ := multipartUpload(t, client, url, nil, []byte("invalid"))
			if code != 401 && code != 403 {
				t.Fatalf("RBAC %d", code)
			}
		}
		for _, invalid := range [][][2]string{{{"unknown", "secret"}}, {{"path", "."}, {"path", "other"}}} {
			code, out := multipartUpload(t, pe, url, invalid, zip)
			if code != 400 || out["code"] != "INVALID_REQUEST" {
				t.Fatalf("strict multipart %d %v", code, out)
			}
		}
	}
	code, out := multipartUpload(t, pe, ts.URL+"/api/v1/render-bundles/upload", nil, []byte("secret malformed zip"))
	if code != 400 || out["code"] != "RENDER_BUNDLE_INVALID" {
		t.Fatalf("malformed ZIP %d %v", code, out)
	}
}

func TestUploadTooLargeIsBoundedAndReturnsPublicCode(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "source.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(bytes.Repeat([]byte{'x'}, (32<<20)+1)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	if _, _, ok := readSourceUpload(response, request, false); ok {
		t.Fatal("accepted oversized upload")
	}
	if response.Code != 413 || !strings.Contains(response.Body.String(), `"code":"REQUEST_TOO_LARGE"`) {
		t.Fatalf("unexpected public error %d %s", response.Code, response.Body.String())
	}
}

// A context timeout alone leaves a slow request body blocked. Exercise the
// actual HTTP connection to ensure incomplete uploads release the handler.
func TestUploadTransportDeadlineLimitsIncompleteMultipart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
		defer cancel()
		if _, _, ok := readSourceUpload(w, r.WithContext(ctx), false); ok {
			writeJSON(w, 200, map[string]any{"unexpected": true})
		}
	}))
	defer server.Close()
	connection, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(connection, "POST /upload HTTP/1.1\r\nHost: local\r\nContent-Type: multipart/form-data; boundary=slow\r\nContent-Length: 4096\r\nConnection: close\r\n\r\n--slow\r\nContent-Disposition: form-data; name=\"file\"; filename=\"source.zip\"\r\n\r\npartial")
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("upload deadline did not release the handler: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatalf("incomplete upload status %d", response.StatusCode)
	}
}
