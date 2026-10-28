package mcpbridge

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func requiredString(arguments map[string]any, key string) (string, error) {
	value := strings.TrimSpace(stringArgument(arguments, key))
	if value == "" {
		return "", newToolError("INVALID_ARGUMENT", key+" is required")
	}
	return value, nil
}

func optionalString(arguments map[string]any, key string) (string, error) {
	value, present := arguments[key]
	if !present || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", newToolError("INVALID_ARGUMENT", key+" must be a string")
	}
	return text, nil
}

func requiredObject(arguments map[string]any, key string) (map[string]any, error) {
	value, ok := arguments[key].(map[string]any)
	if !ok || value == nil {
		return nil, newToolError("INVALID_ARGUMENT", key+" must be an object")
	}
	return value, nil
}

func optionalObject(arguments map[string]any, key string) (map[string]any, error) {
	value, present := arguments[key]
	if !present || value == nil {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, newToolError("INVALID_ARGUMENT", key+" must be an object")
	}
	return object, nil
}

func requiredVersion(arguments map[string]any) (int64, error) {
	value, present := arguments["version"]
	if !present {
		return 0, newToolError("INVALID_ARGUMENT", "version is required")
	}
	var version int64
	switch number := value.(type) {
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < 0 || number >= float64(math.MaxInt64) {
			return 0, newToolError("INVALID_ARGUMENT", "version must be a non-negative integer")
		}
		version = int64(number)
	case int:
		if number < 0 {
			return 0, newToolError("INVALID_ARGUMENT", "version must be a non-negative integer")
		}
		version = int64(number)
	case int64:
		if number < 0 {
			return 0, newToolError("INVALID_ARGUMENT", "version must be a non-negative integer")
		}
		version = number
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		if err != nil || parsed < 0 {
			return 0, newToolError("INVALID_ARGUMENT", "version must be a non-negative integer")
		}
		version = parsed
	default:
		return 0, newToolError("INVALID_ARGUMENT", "version must be a non-negative integer")
	}
	return version, nil
}

func applicationEnvironment(arguments map[string]any) (string, string, error) {
	application, err := requiredString(arguments, "application")
	if err != nil {
		return "", "", err
	}
	environment, err := requiredString(arguments, "environment")
	if err != nil {
		return "", "", err
	}
	return application, environment, nil
}

func applicationPath(application string) string {
	return "/api/v1/applications/" + url.PathEscape(application)
}

func environmentPath(application, environment string) string {
	return applicationPath(application) + "/environments/" + url.PathEscape(environment)
}

func applicationOptions() []mcp.ToolOption {
	return []mcp.ToolOption{mcp.WithString("application", mcp.Required(), mcp.Description("Application key"))}
}

func environmentOptions() []mcp.ToolOption {
	return append(applicationOptions(), mcp.WithString("environment", mcp.Required(), mcp.Description("Environment key")))
}

func (b *bridge) addReadTool(server *mcpserver.MCPServer, name, description string, options []mcp.ToolOption, fn toolFunc) {
	options = append([]mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	}, options...)
	server.AddTool(mcp.NewTool(name, options...), b.handle(name, fn))
}

func (b *bridge) registerReadTools(server *mcpserver.MCPServer) {
	b.addReadTool(server, "orch_list_applications", "List applications visible to the configured orchestrator user.", nil,
		func(ctx context.Context, _ map[string]any) (toolResponse, error) {
			return b.call(ctx, http.MethodGet, "/api/v1/applications", nil, nil)
		})

	b.addReadTool(server, "orch_get_application", "Get one application and its environments.", applicationOptions(),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, err := requiredString(arguments, "application")
			if err != nil {
				return toolResponse{}, err
			}
			return b.call(ctx, http.MethodGet, applicationPath(application), nil, nil)
		})

	b.addReadTool(server, "orch_list_deployments", "List deployments in an environment, optionally filtered by status.", append(environmentOptions(),
		mcp.WithString("status", mcp.Description("Optional deployment status filter"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			status, err := optionalString(arguments, "status")
			if err != nil {
				return toolResponse{}, err
			}
			return b.call(ctx, http.MethodGet, environmentPath(application, environment)+"/deployments", map[string]string{"status": status}, nil)
		})

	b.addReadTool(server, "orch_get_deployment", "Get one deployment report.", append(environmentOptions(),
		mcp.WithString("deployment", mcp.Required(), mcp.Description("Deployment identifier"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			deployment, err := requiredString(arguments, "deployment")
			if err != nil {
				return toolResponse{}, err
			}
			path := environmentPath(application, environment) + "/deployments/" + url.PathEscape(deployment)
			return b.call(ctx, http.MethodGet, path, nil, nil)
		})

	b.addReadTool(server, "orch_list_resources", "List resources in an environment.", environmentOptions(),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			return b.call(ctx, http.MethodGet, environmentPath(application, environment)+"/resources", nil, nil)
		})

	b.addReadTool(server, "orch_get_resource", "Get one resource in an environment.", append(environmentOptions(),
		mcp.WithString("resource_id", mcp.Required(), mcp.Description("Resource identifier"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			resource, err := requiredString(arguments, "resource_id")
			if err != nil {
				return toolResponse{}, err
			}
			path := environmentPath(application, environment) + "/resources/" + url.PathEscape(resource)
			return b.call(ctx, http.MethodGet, path, nil, nil)
		})

	b.addReadTool(server, "orch_list_operations", "List operations in an environment.", environmentOptions(),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			return b.call(ctx, http.MethodGet, environmentPath(application, environment)+"/operations", nil, nil)
		})

	b.addReadTool(server, "orch_preview_score", "Preview a standalone Score deployment delta without deploying.", append(environmentOptions(),
		mcp.WithString("workload_id", mcp.Description("Workload identifier")),
		mcp.WithString("action", mcp.Description("Preview action")),
		mcp.WithString("run_id", mcp.Description("Preview run identifier")),
		mcp.WithObject("score_before", mcp.Description("Score before the change")),
		mcp.WithObject("score_after", mcp.Description("Score after the change"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			body := make(map[string]any)
			for _, item := range []struct{ argument, field string }{
				{"workload_id", "workloadId"}, {"action", "action"}, {"run_id", "runId"},
			} {
				value, err := optionalString(arguments, item.argument)
				if err != nil {
					return toolResponse{}, err
				}
				if value != "" {
					body[item.field] = value
				}
			}
			for _, item := range []struct{ argument, field string }{
				{"score_before", "scoreBefore"}, {"score_after", "scoreAfter"},
			} {
				value, err := optionalObject(arguments, item.argument)
				if err != nil {
					return toolResponse{}, err
				}
				if value != nil {
					body[item.field] = value
				}
			}
			return b.call(ctx, http.MethodPost, environmentPath(application, environment)+"/score-preview", nil, body)
		})

	b.addReadTool(server, "orch_preview_pending", "Preview the pending draft delta and obtain a single-use deploy token.", environmentOptions(),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			return b.call(ctx, http.MethodPost, environmentPath(application, environment)+"/preview", nil, map[string]any{})
		})

	b.addReadTool(server, "orch_observability", "Query environment observability overview, logs or metrics.", append(environmentOptions(),
		mcp.WithString("kind", mcp.Required(), mcp.Enum("overview", "logs", "metrics")),
		mcp.WithString("from"), mcp.WithString("to"), mcp.WithString("workload"),
		mcp.WithString("q"), mcp.WithString("level"), mcp.WithInteger("limit", mcp.Min(1)), mcp.WithString("cursor")),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			kind, err := requiredString(arguments, "kind")
			if err != nil {
				return toolResponse{}, err
			}
			if kind != "overview" && kind != "logs" && kind != "metrics" {
				return toolResponse{}, newToolError("INVALID_ARGUMENT", "kind must be overview, logs or metrics")
			}
			query := make(map[string]string)
			for _, key := range []string{"from", "to", "workload", "q", "level", "cursor"} {
				value, err := optionalString(arguments, key)
				if err != nil {
					return toolResponse{}, err
				}
				query[key] = value
			}
			if value, present := arguments["limit"]; present {
				number, ok := value.(float64)
				if !ok || math.Trunc(number) != number || number < 1 {
					return toolResponse{}, newToolError("INVALID_ARGUMENT", "limit must be a positive integer")
				}
				query["limit"] = strconv.FormatInt(int64(number), 10)
			}
			return b.call(ctx, http.MethodGet, environmentPath(application, environment)+"/observability/"+kind, query, nil)
		})
}
