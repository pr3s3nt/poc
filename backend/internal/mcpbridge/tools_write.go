package mcpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"
)

func confirmArgument(arguments map[string]any) bool {
	confirm, _ := arguments["confirm"].(bool)
	return confirm
}

func (b *bridge) addMutationTool(server *mcpserver.MCPServer, name, description string, destructive bool, options []mcp.ToolOption, fn toolFunc) {
	options = append([]mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithBoolean("confirm", mcp.Description("Execute the mutation only when true; false performs the documented dry run"), mcp.DefaultBool(false)),
		mcp.WithDestructiveHintAnnotation(destructive),
		mcp.WithOpenWorldHintAnnotation(true),
	}, options...)
	server.AddTool(mcp.NewTool(name, options...), b.handle(name, fn))
}

func (b *bridge) registerMutationTools(server *mcpserver.MCPServer) {
	b.addMutationTool(server, "orch_create_application", "Create an application after explicit confirmation.", false, []mcp.ToolOption{
		mcp.WithString("name", mcp.Required(), mcp.Description("Application display name")),
		mcp.WithString("subdomain", mcp.Required(), mcp.Description("Application subdomain")),
	}, func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
		name, err := requiredString(arguments, "name")
		if err != nil {
			return toolResponse{}, err
		}
		subdomain, err := requiredString(arguments, "subdomain")
		if err != nil {
			return toolResponse{}, err
		}
		if !confirmArgument(arguments) {
			return confirmationRequired()
		}
		return b.call(ctx, http.MethodPost, "/api/v1/applications", nil, map[string]string{"name": name, "subdomain": subdomain})
	})

	b.addMutationTool(server, "orch_save_workload", "Validate and, with confirmation, save a workload Score draft.", false, append(environmentOptions(),
		mcp.WithString("workload", mcp.Required(), mcp.Description("Workload identifier")),
		mcp.WithInteger("version", mcp.Required(), mcp.Min(0), mcp.Description("Current draft version")),
		mcp.WithObject("score", mcp.Required(), mcp.Description("Complete Score document"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			if err := b.checkEnvironment(arguments); err != nil {
				return toolResponse{}, err
			}
			workload, err := requiredString(arguments, "workload")
			if err != nil {
				return toolResponse{}, err
			}
			version, err := requiredVersion(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			score, err := requiredObject(arguments, "score")
			if err != nil {
				return toolResponse{}, err
			}
			path := environmentPath(application, environment) + "/workloads/" + url.PathEscape(workload)
			if !confirmArgument(arguments) {
				content, err := yaml.Marshal(score)
				if err != nil {
					return toolResponse{}, newToolError("INVALID_ARGUMENT", "score cannot be serialized as YAML")
				}
				response, err := b.call(ctx, http.MethodPost, environmentPath(application, environment)+"/workloads/parse", nil, map[string]string{"content": string(content)})
				if err != nil {
					return response, err
				}
				return dryRun(response.body, response.status)
			}
			return b.call(ctx, http.MethodPut, path, nil, map[string]any{"version": version, "score": score})
		})

	b.addMutationTool(server, "orch_delete_workload_draft", "Delete a workload draft after explicit confirmation.", true, append(environmentOptions(),
		mcp.WithString("workload", mcp.Required(), mcp.Description("Workload identifier")),
		mcp.WithInteger("version", mcp.Required(), mcp.Min(0), mcp.Description("Current draft version"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			if err := b.checkEnvironment(arguments); err != nil {
				return toolResponse{}, err
			}
			workload, err := requiredString(arguments, "workload")
			if err != nil {
				return toolResponse{}, err
			}
			version, err := requiredVersion(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			if !confirmArgument(arguments) {
				return confirmationRequired()
			}
			path := environmentPath(application, environment) + "/workloads/" + url.PathEscape(workload)
			return b.call(ctx, http.MethodDelete, path, nil, map[string]any{"version": version})
		})

	b.addMutationTool(server, "orch_deploy_pending", "Deploy a previously previewed pending draft after explicit confirmation.", false, append(environmentOptions(),
		mcp.WithString("token", mcp.Description("Single-use token returned by orch_preview_pending; required with confirm=true"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, environment, err := applicationEnvironment(arguments)
			if err != nil {
				return toolResponse{}, err
			}
			if err := b.checkEnvironment(arguments); err != nil {
				return toolResponse{}, err
			}
			if !confirmArgument(arguments) {
				return confirmationRequired()
			}
			token, err := requiredString(arguments, "token")
			if err != nil {
				return toolResponse{}, err
			}
			return b.call(ctx, http.MethodPost, environmentPath(application, environment)+"/deploy", nil, map[string]string{"token": token})
		})

	b.addMutationTool(server, "orch_delete_application", "Preview application deletion and, with confirmation and typed name, delete it.", true, append(applicationOptions(),
		mcp.WithString("confirmation_name", mcp.Description("Exact application name; required with confirm=true"))),
		func(ctx context.Context, arguments map[string]any) (toolResponse, error) {
			application, err := requiredString(arguments, "application")
			if err != nil {
				return toolResponse{}, err
			}
			preview, err := b.call(ctx, http.MethodPost, applicationPath(application)+"/deletion-preview", nil, map[string]any{})
			if err != nil || !confirmArgument(arguments) {
				if err != nil {
					return preview, err
				}
				return dryRun(preview.body, preview.status)
			}
			confirmationName, err := requiredString(arguments, "confirmation_name")
			if err != nil {
				return toolResponse{status: preview.status}, err
			}
			var envelope struct {
				Preview struct {
					Token string `json:"token"`
				} `json:"preview"`
			}
			if err := json.Unmarshal(preview.body, &envelope); err != nil || envelope.Preview.Token == "" {
				return toolResponse{status: preview.status}, newToolError("DELETION_PREVIEW_BLOCKED", "deletion preview did not issue a token; resolve its blockers before deletion")
			}
			return b.call(ctx, http.MethodPost, applicationPath(application)+"/deletions", nil, map[string]string{
				"token": envelope.Preview.Token, "confirmationName": confirmationName,
			})
		})
}
