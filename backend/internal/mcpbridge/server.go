package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type Config struct {
	APIURL              string
	Username            string
	Password            string
	AllowedEnvironments []string
	Audit               io.Writer
}

type bridge struct {
	api     *apiClient
	allowed map[string]struct{}
	audit   *auditLogger
}

type toolResponse struct {
	body    json.RawMessage
	status  int
	outcome string
}

type toolFunc func(context.Context, map[string]any) (toolResponse, error)

type toolError struct {
	code    string
	message string
}

func (e *toolError) Error() string { return e.code + ": " + e.message }

func newToolError(code, message string) error {
	return &toolError{code: code, message: message}
}

func NewHandler(config Config) (http.Handler, error) {
	client, err := newAPIClient(config.APIURL, config.Username, config.Password)
	if err != nil {
		return nil, err
	}
	audit := config.Audit
	if audit == nil {
		audit = os.Stderr
	}
	b := &bridge{
		api:     client,
		allowed: make(map[string]struct{}),
		audit:   &auditLogger{writer: audit, now: time.Now},
	}
	for _, environment := range config.AllowedEnvironments {
		if environment = strings.TrimSpace(environment); environment != "" {
			b.allowed[environment] = struct{}{}
		}
	}
	server := mcpserver.NewMCPServer("orchestrator", "0.1.0", mcpserver.WithToolCapabilities(false), mcpserver.WithRecovery())
	b.registerReadTools(server)
	b.registerMutationTools(server)
	return mcpserver.NewStreamableHTTPServer(server), nil
}

func (b *bridge) handle(name string, fn toolFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := request.GetArguments()
		response, err := fn(ctx, arguments)
		record := auditRecord{
			Tool:        name,
			Application: stringArgument(arguments, "application"),
			Environment: stringArgument(arguments, "environment"),
			Confirm:     request.GetBool("confirm", false),
			Status:      response.status,
			Outcome:     response.outcome,
		}
		if err != nil {
			var apiErr *apiError
			var toolErr *toolError
			switch {
			case errors.As(err, &apiErr):
				record.Status = apiErr.Status
				record.Outcome = apiErr.Code
				if record.Outcome == "" {
					record.Outcome = "API_ERROR"
				}
			case errors.As(err, &toolErr):
				record.Outcome = toolErr.code
			default:
				record.Outcome = "INVALID_ARGUMENT"
			}
			b.audit.record(record)
			return mcp.NewToolResultError(err.Error()), nil
		}
		if record.Outcome == "" {
			record.Outcome = "OK"
		}
		b.audit.record(record)
		if len(response.body) == 0 {
			response.body = json.RawMessage(`{}`)
		}
		return mcp.NewToolResultText(string(response.body)), nil
	}
}

func (b *bridge) call(ctx context.Context, method, path string, query map[string]string, body any) (toolResponse, error) {
	values := make(map[string][]string, len(query))
	for key, value := range query {
		if value != "" {
			values[key] = []string{value}
		}
	}
	raw, status, err := b.api.do(ctx, method, path, values, body)
	return toolResponse{body: raw, status: status, outcome: "OK"}, err
}

func (b *bridge) checkEnvironment(arguments map[string]any) error {
	if len(b.allowed) == 0 {
		return nil
	}
	environment := stringArgument(arguments, "environment")
	if _, ok := b.allowed[environment]; !ok {
		return newToolError("ENVIRONMENT_NOT_ALLOWED", fmt.Sprintf("environment %q is not in MCP_ALLOWED_ENVIRONMENTS", environment))
	}
	return nil
}

func confirmationRequired() (toolResponse, error) {
	return toolResponse{
		body:    json.RawMessage(`{"outcome":"confirmation_required","confirmationRequired":true}`),
		outcome: "CONFIRMATION_REQUIRED",
	}, nil
}

func dryRun(body json.RawMessage, status int) (toolResponse, error) {
	encoded, err := json.Marshal(map[string]any{
		"outcome":              "dry_run",
		"confirmationRequired": true,
		"result":               body,
	})
	if err != nil {
		return toolResponse{}, fmt.Errorf("encode dry-run result: %w", err)
	}
	return toolResponse{body: encoded, status: status, outcome: "DRY_RUN"}, nil
}
