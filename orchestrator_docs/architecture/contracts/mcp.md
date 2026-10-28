---
id: MCP-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-28
related: [ADR-028, UC-01, UC-05, UC-06, UC-09, UC-12, UC-16, UC-18]
---

# MCP tool contract

[ADR-028](../decisions/ADR-028-mcp-integration.md) owns architecture and
gating policy. This contract owns the tool catalog: every MCP tool maps
one-to-one onto an existing orchestrator JSON API operation and inherits that
operation's authorization, guards and error semantics. The MCP server never
adds behavior the API does not already have.

## Process and transport

The MCP server is a separate Go process (`backend/cmd/mcp-server`, package
`orchestrator/internal/mcpbridge`). It exposes MCP over streamable HTTP at
`MCP_LISTEN_ADDR` (default `127.0.0.1:8090`) and calls the orchestrator API at
`ORCHESTRATOR_API_URL`. Configuration comes from environment only and is never
committed: `ORCHESTRATOR_USERNAME`/`ORCHESTRATOR_PASSWORD` sign in through
`POST /api/v1/auth/sign-in`; the returned session cookie is kept in-process,
refreshed by a single re-sign-in and retry on a 401, and is never exposed
through a tool result. The process holds no cluster, cloud or database
credentials.

## Gating, allowlist and audit

Every mutation tool takes a boolean `confirm` argument (default `false`).
With `confirm: false` the tool performs only the read-only step associated
with the operation — the available preview, parse or a structured
"confirmation required" result — and never changes runtime state. With
`confirm: true` the tool executes the mapped API operation, including the
API's own second-factor requirements (deploy preview token, typed deletion
confirmation name). `confirm` is an MCP-side gate; it narrows but does not
replace API authorization.

When `MCP_ALLOWED_ENVIRONMENTS` is non-empty, environment-scoped mutation
tools refuse environments outside the comma-separated allowlist before
calling the API. Application-scoped mutations (create/delete application) are
not environment-scoped and are not restricted by the allowlist.

Every tool call appends one JSONL record to `MCP_AUDIT_PATH` (stderr when
empty): timestamp, tool name, application/environment keys, `confirm` flag,
API status and outcome code. Audit records never contain Score bodies,
credential material or session tokens.

Non-2xx API responses become tool errors carrying the API error code and
field; error text adds no secret material. Tool results are API response
passthrough and disclose no more than the authenticated API already does
(UC-12 secret values are never part of configuration payloads).

## Read tools

| Tool | API operation | UC |
|---|---|---|
| `orch_list_applications` | `GET /api/v1/applications` | UC-01/09 |
| `orch_get_application` | `GET /api/v1/applications/{id}` | UC-09 |
| `orch_list_deployments` | `GET /api/v1/applications/{id}/environments/{env}/deployments` (`status`) | UC-09 |
| `orch_get_deployment` | `GET .../deployments/{deployment}` | UC-09 |
| `orch_list_resources` | `GET .../resources` | UC-09 |
| `orch_get_resource` | `GET .../resources/{resourceId}` | UC-09 |
| `orch_list_operations` | `GET .../operations` | UC-09 |
| `orch_preview_score` | `POST .../score-preview` (`workload_id`, `action`, `run_id`, `score_before`, `score_after`) | UC-05 |
| `orch_preview_pending` | `POST .../preview` (pending draft delta; response carries the deploy token) | UC-05 |
| `orch_observability` | `GET .../observability/{kind}`, `kind` = `overview`/`logs`/`metrics`; params `from,to,workload,q,level,limit,cursor` per the [observability contract](observability.md) | UC-09 TEL |

All read tools pass required path keys (`application`, `environment` and
resource/deployment identifiers) as explicit arguments.

## Mutation tools

| Tool | API operation | UC | `confirm: false` behavior |
|---|---|---|---|
| `orch_create_application` | `POST /api/v1/applications` (`name`, `subdomain`) | UC-01 | Returns "confirmation required"; creates nothing |
| `orch_save_workload` | `PUT .../workloads/{workload}` (`version`, `score`) | UC-16 | Serializes `score` as YAML and validates it via `POST .../workloads/parse` (`content`); saves nothing |
| `orch_delete_workload_draft` | `DELETE .../workloads/{workload}` (`version`) | UC-16 | Returns "confirmation required"; deletes nothing |
| `orch_deploy_pending` | `POST .../deploy` (`token` from `orch_preview_pending`) | UC-06 | Returns "confirmation required"; the API token requirement stays in force |
| `orch_delete_application` | `POST /api/v1/applications/{id}/deletion-preview`, then `POST .../deletions` (`token`, `confirmationName`) | UC-18 | Runs the deletion preview and returns it marked as a dry run |

`orch_deploy_pending` requires the deploy token issued by a prior
`orch_preview_pending` call for the same environment; the token is consumed
by the API and cannot be replayed. `orch_delete_application` with
`confirm: true` additionally requires `confirmation_name` typed exactly as
the application name (the API rejects mismatches).

## Change rule

Adding, removing or renaming a tool, changing its mapped endpoint or changing
gate semantics is a contract change: update this document first, then the
implementation, then the traceability links. Drift between this catalog and
the implementation is a defect.
