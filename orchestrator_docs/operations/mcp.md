---
id: MCP-OPERATIONS
artifact: operations-runbook
status: current
last_reviewed: 2026-10-28
related: [ADR-028, MCP-CONTRACT]
---

# Operate the orchestrator from Claude Code through MCP

Start the normal orchestrator API first. The MCP process uses an ordinary
orchestrator user and inherits that user's Organization and role; it does not
need database, Kubernetes or cloud credentials. Supply credentials through a
local secret manager or the process environment, never a tracked file.

```bash
cd backend
ORCHESTRATOR_API_URL=http://127.0.0.1:8080 \
ORCHESTRATOR_USERNAME='<username>' \
ORCHESTRATOR_PASSWORD='<password>' \
MCP_LISTEN_ADDR=127.0.0.1:8090 \
MCP_ALLOWED_ENVIRONMENTS=staging \
MCP_AUDIT_PATH=/secure/local/path/orchestrator-mcp-audit.jsonl \
go run ./cmd/mcp-server
```

`MCP_ALLOWED_ENVIRONMENTS` is a comma-separated mutation allowlist; empty allows
every environment visible to the user. It does not restrict application-scoped
creation/deletion or read tools. `MCP_AUDIT_PATH` is optional and defaults to
stderr. The process creates an audit file with mode `0600`; protect existing
audit files and their directory as operational records.

Keep the MCP endpoint on loopback. The V1 endpoint has no separate MCP-client
authentication or TLS; do not expose it to a shared/network interface. Register
it in Claude Code while the process is running:

```bash
claude mcp add --transport http orchestrator http://127.0.0.1:8090/mcp
```

The [tool contract](../architecture/contracts/mcp.md) is authoritative for all
15 tools and arguments. A typical draft deployment is:

1. Inspect applications/resources with read tools.
2. Call `orch_save_workload` with `confirm: false` to validate Score, inspect the
   result, then call again with `confirm: true` to save.
3. Call `orch_preview_pending` and inspect the delta. Its deploy token is
   single-use and must go only to the subsequent deploy call.
4. Call `orch_deploy_pending` with that token and `confirm: true` only after the
   user approves the preview. Use deployment/operation/observability read tools
   to inspect progress and outcome.

Other mutations follow the same rule: `confirm: false` never performs the
mutation. Application deletion additionally requires the exact
`confirmation_name`; its preview token cannot be reused. API authorization,
planning guards, ownership/UID checks and cleanup behavior remain authoritative.

Validate a source checkout without external mutation:

```bash
cd backend
go test ./internal/mcpbridge ./cmd/mcp-server
go build ./cmd/mcp-server
```

These checks use a fake API. A real Claude-driven deployment is a separate,
explicitly authorized environment operation, not part of MCP startup.
