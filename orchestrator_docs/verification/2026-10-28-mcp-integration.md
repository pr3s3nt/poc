---
id: VER-2026-10-28-MCP-INTEGRATION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-28
related: [ADR-028, MCP-CONTRACT]
---

# MCP integration local verification

Verified the `features/mcp` implementation with the real
`mark3labs/mcp-go` streamable HTTP client and an `httptest` orchestrator API.
No live orchestrator database, Kubernetes cluster, AWS account or application
was changed.

Five MCP tests pass: the catalog contains 15 tools; create with `confirm:false`
makes no API call; workload dry-run parses YAML without a PUT; the environment
allowlist rejects before sign-in/API; a 401 causes one re-sign-in and retry;
confirmed application deletion uses the preview token and typed name. Audit
assertions reject Score, deploy-token, session-token and deletion-token leakage.

```text
go test -v ./internal/mcpbridge ./cmd/mcp-server
PASS (five mcpbridge tests; cmd/mcp-server builds)

go test ./... && go build ./...
PASS (all backend packages)

go test ./internal/mcpbridge ./cmd/mcp-server && go build ./cmd/mcp-server
PASS (focused loopback-default recheck)

python3 scripts/check_docs.py && git diff --check
PASS (457 Markdown files, 393 artifact IDs)
```

The tests prove protocol registration, request mapping and MCP-side safeguards.
They do not claim live Claude Code interaction or real deployment acceptance.
