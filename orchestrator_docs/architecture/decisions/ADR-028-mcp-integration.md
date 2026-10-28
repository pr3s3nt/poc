---
id: ADR-028
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
---

# ADR-028 — MCP adapter for agent-driven deployment operations

## Decision and scope

User decisions on 2026-10-28 (choices 1B, 2A, 3A): expose the orchestrator to
Claude through MCP so an agent can operate deployments; V1 includes mutation
tools behind explicit gates; the MCP server is a separate Go process; the
served client is Claude Code for developer/operations use, not an in-product
chat surface.

The MCP server is a separate deployable Go process (`backend/cmd/mcp-server`,
supporting package `backend/internal/mcpbridge`). It speaks MCP over streamable
HTTP and calls only the public orchestrator JSON API under `/api/v1/`,
authenticating with a user session obtained through sign-in (configuration
supplies username/password). It never holds cluster or cloud credentials,
never opens the database, and never imports application services directly.
Because every tool is an API call, the MCP surface inherits existing product
guards: ownership/UID cleanup, environment binding, secret non-disclosure and
deletion previews. No internal bypass path is introduced.

The tool catalog maps one-to-one onto existing use-case operations and is
owned by the [MCP tool contract](../contracts/mcp.md). V1 ships read tools
(application, deployment, resource, operation listing and detail; Score and
pending-draft preview per UC-05) and gated mutation tools (create application,
save/delete workload draft, deploy pending draft, delete application per
UC-01/06/16/18).

Mutation tools are gated three ways. First, every mutation tool takes an
explicit `confirm` argument: without it the tool performs only the read-only
preview available for that operation (for example the UC-18 deletion preview)
and returns what would happen; the agent must re-call with `confirm: true` to
execute. Second, an optional environment allowlist configuration restricts
mutations to named environments. Third, every tool call is appended to a local
JSONL audit log with tool name, application/environment keys, confirm flag and
outcome; secret values and credential material are never logged, and API
responses are passed through without adding secret disclosure beyond what the
authenticated API already returns.

No new use case is added. The MCP server is a new client type on existing UC
operations, operationally comparable to the CI/CD actor of UC-13; use-case
specifications do not change behavior for it. The Web Console remains the
human ownership surface; no console UI is built for MCP in this scope.

## Rationale and consequences

The platform's preview/delta/deploy discipline (UC-05 before UC-06) is exactly
the interaction model an agent needs: propose, inspect the delta, then apply.
A thin API-first adapter keeps that discipline instead of giving an agent a
second, weaker door; it also means MCP capabilities evolve only when the API
evolves, keeping one authorization and audit story. Explicit confirmation plus
audit bounds the blast radius of agent misunderstanding while still allowing
one chat turn to prepare, preview and — after confirmation — deploy.

Costs and rules: the MCP server is one more binary to build and run, and its
session token must be managed like any user credential (environment-provided,
never committed). The tool catalog contract must be updated whenever a tool or
its mapped endpoint changes; drift between contract and implementation is a
defect. Mutation gating configuration is local to the MCP server; it narrows
but does not replace API-side authorization.

## Delivery

Accepted design on 2026-10-28 on branch `features/mcp`. The
[MCP tool contract](../contracts/mcp.md) owns the tool catalog, gating semantics
and redaction rules; implementation lives in `backend/cmd/mcp-server` and
`backend/internal/mcpbridge`. In-product agent UI, multi-tenancy beyond the
signed-in organization, and non-Claude MCP clients are out of scope until
requested.
