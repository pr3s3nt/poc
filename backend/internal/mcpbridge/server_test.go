package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type recordedRequest struct {
	Method string
	Path   string
	Body   string
}

type requestLog struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (l *requestLog) add(request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	request.Body = io.NopCloser(bytes.NewReader(body))
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, recordedRequest{Method: request.Method, Path: request.URL.Path, Body: string(body)})
}

func (l *requestLog) snapshot() []recordedRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]recordedRequest(nil), l.requests...)
}

func newFakeAPI(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *requestLog) {
	t.Helper()
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		log.add(request)
		if request.URL.Path == "/api/v1/auth/sign-in" {
			var credentials map[string]string
			if err := json.NewDecoder(bytes.NewBufferString(log.snapshot()[len(log.snapshot())-1].Body)).Decode(&credentials); err != nil {
				t.Error(err)
			}
			if credentials["username"] != "operator" || credentials["password"] != "test-password" {
				writeTestJSON(response, http.StatusUnauthorized, map[string]any{"code": "INVALID_CREDENTIALS"})
				return
			}
			http.SetCookie(response, &http.Cookie{Name: "orchestrator_session", Value: "valid-session", Path: "/"})
			writeTestJSON(response, http.StatusOK, map[string]any{"user": map[string]any{"username": "operator"}})
			return
		}
		cookie, err := request.Cookie("orchestrator_session")
		if err != nil || cookie.Value != "valid-session" {
			writeTestJSON(response, http.StatusUnauthorized, map[string]any{"code": "UNAUTHORIZED"})
			return
		}
		handler(response, request)
	}))
	t.Cleanup(server.Close)
	return server, log
}

func newMCPClient(t *testing.T, apiURL string, allowed []string, audit *bytes.Buffer) *client.Client {
	t.Helper()
	handler, err := NewHandler(Config{
		APIURL: apiURL, Username: "operator", Password: "test-password",
		AllowedEnvironments: allowed, Audit: audit,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	mcpClient, err := client.NewStreamableHttpClient(server.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mcpClient.Close() })
	ctx := context.Background()
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "mcpbridge-test", Version: "1.0.0"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return mcpClient
}

func callTool(t *testing.T, mcpClient *client.Client, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := mcpClient.CallTool(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name: name, Arguments: arguments,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	return mcp.GetTextFromContent(result.Content[0])
}

func writeTestJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func TestToolCatalogAndMutationDryRun(t *testing.T) {
	api, requests := newFakeAPI(t, func(response http.ResponseWriter, _ *http.Request) {
		t.Error("dry-run create called the API")
	})
	var audit bytes.Buffer
	mcpClient := newMCPClient(t, api.URL, nil, &audit)

	tools, err := mcpClient.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 15 {
		t.Fatalf("expected 15 tools, got %d", len(tools.Tools))
	}
	result := callTool(t, mcpClient, "orch_create_application", map[string]any{
		"name": "Example", "subdomain": "example", "confirm": false,
	})
	if result.IsError || !strings.Contains(resultText(t, result), "confirmation_required") {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	if len(requests.snapshot()) != 0 {
		t.Fatalf("dry-run create made API requests: %#v", requests.snapshot())
	}
	if !strings.Contains(audit.String(), `"outcome":"CONFIRMATION_REQUIRED"`) {
		t.Fatalf("missing dry-run audit record: %s", audit.String())
	}
}

func TestSaveWorkloadDryRunParsesWithoutSaving(t *testing.T) {
	api, requests := newFakeAPI(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/applications/app/environments/dev/workloads/parse" {
			t.Errorf("unexpected API call: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.Contains(body["content"], "apiVersion: score.dev/v1b1") {
			t.Errorf("parse content is not Score YAML: %q", body["content"])
		}
		writeTestJSON(response, http.StatusOK, map[string]any{"score": map[string]any{"apiVersion": "score.dev/v1b1"}})
	})
	var audit bytes.Buffer
	mcpClient := newMCPClient(t, api.URL, nil, &audit)
	result := callTool(t, mcpClient, "orch_save_workload", map[string]any{
		"application": "app", "environment": "dev", "workload": "web", "version": 3,
		"score": map[string]any{"apiVersion": "score.dev/v1b1", "private": "score-secret"},
	})
	if result.IsError || !strings.Contains(resultText(t, result), `"outcome":"dry_run"`) {
		t.Fatalf("unexpected parse result: %s", resultText(t, result))
	}
	for _, request := range requests.snapshot() {
		if request.Method == http.MethodPut {
			t.Fatalf("dry-run saved a workload: %#v", request)
		}
	}
	if strings.Contains(audit.String(), "score-secret") || strings.Contains(audit.String(), "score") {
		t.Fatalf("audit leaked Score data: %s", audit.String())
	}
}

func TestMutationEnvironmentAllowlist(t *testing.T) {
	api, requests := newFakeAPI(t, func(response http.ResponseWriter, _ *http.Request) {
		t.Error("disallowed environment called the API")
	})
	var audit bytes.Buffer
	mcpClient := newMCPClient(t, api.URL, []string{"dev"}, &audit)
	result := callTool(t, mcpClient, "orch_deploy_pending", map[string]any{
		"application": "app", "environment": "prod", "token": "deploy-secret", "confirm": true,
	})
	if !result.IsError || !strings.Contains(resultText(t, result), "ENVIRONMENT_NOT_ALLOWED") {
		t.Fatalf("unexpected allowlist result: %s", resultText(t, result))
	}
	if len(requests.snapshot()) != 0 {
		t.Fatalf("allowlist rejection made API requests: %#v", requests.snapshot())
	}
	if strings.Contains(audit.String(), "deploy-secret") || !strings.Contains(audit.String(), `"environment":"prod"`) {
		t.Fatalf("invalid allowlist audit: %s", audit.String())
	}
}

func TestSessionRefreshRetriesOnce(t *testing.T) {
	var mu sync.Mutex
	signIns := 0
	applicationReads := 0
	log := &requestLog{}
	api := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		log.add(request)
		mu.Lock()
		defer mu.Unlock()
		switch request.URL.Path {
		case "/api/v1/auth/sign-in":
			signIns++
			http.SetCookie(response, &http.Cookie{Name: "orchestrator_session", Value: fmt.Sprintf("session-%d", signIns), Path: "/"})
			writeTestJSON(response, http.StatusOK, map[string]any{"user": map[string]any{}})
		case "/api/v1/applications":
			applicationReads++
			cookie, _ := request.Cookie("orchestrator_session")
			if applicationReads == 1 || cookie.Value != "session-2" {
				writeTestJSON(response, http.StatusUnauthorized, map[string]any{"code": "UNAUTHORIZED"})
				return
			}
			writeTestJSON(response, http.StatusOK, map[string]any{"applications": []any{}})
		default:
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
	}))
	t.Cleanup(api.Close)
	var audit bytes.Buffer
	mcpClient := newMCPClient(t, api.URL, nil, &audit)
	result := callTool(t, mcpClient, "orch_list_applications", nil)
	if result.IsError {
		t.Fatalf("session refresh failed: %s", resultText(t, result))
	}
	mu.Lock()
	defer mu.Unlock()
	if signIns != 2 || applicationReads != 2 {
		t.Fatalf("expected two sign-ins and reads, got %d and %d", signIns, applicationReads)
	}
	if strings.Contains(audit.String(), "session-") || !strings.Contains(audit.String(), `"status":200`) {
		t.Fatalf("invalid refresh audit: %s", audit.String())
	}
}

func TestDeleteApplicationUsesPreviewToken(t *testing.T) {
	api, requests := newFakeAPI(t, func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/applications/app/deletion-preview":
			writeTestJSON(response, http.StatusOK, map[string]any{"preview": map[string]any{"token": "deletion-secret"}})
		case "/api/v1/applications/app/deletions":
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["token"] != "deletion-secret" || body["confirmationName"] != "Example" {
				t.Errorf("unexpected deletion body: %#v", body)
			}
			writeTestJSON(response, http.StatusAccepted, map[string]any{"deletion": map[string]any{"operationId": "op-1"}})
		default:
			t.Errorf("unexpected deletion path: %s", request.URL.Path)
		}
	})
	var audit bytes.Buffer
	mcpClient := newMCPClient(t, api.URL, nil, &audit)
	result := callTool(t, mcpClient, "orch_delete_application", map[string]any{
		"application": "app", "confirmation_name": "Example", "confirm": true,
	})
	if result.IsError || !strings.Contains(resultText(t, result), "op-1") {
		t.Fatalf("unexpected deletion result: %s", resultText(t, result))
	}
	if len(requests.snapshot()) != 3 {
		t.Fatalf("expected sign-in, preview and deletion, got %#v", requests.snapshot())
	}
	if strings.Contains(audit.String(), "deletion-secret") {
		t.Fatalf("audit leaked deletion token: %s", audit.String())
	}
}
