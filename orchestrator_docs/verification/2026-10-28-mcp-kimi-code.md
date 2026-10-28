---
id: VER-2026-10-28-MCP-KIMI-CODE
artifact: verification-record
status: evidence
last_reviewed: 2026-10-28
related: [ADR-028, MCP-CONTRACT]
---

# Kimi Code MCP smoke verification

Tested commit `8c18944` with Kimi Code CLI `2.4.0` as the MCP client. A real
orchestrator process used fake adapters and in-memory state on loopback; the
MCP server was a second real process. The client used a temporary project
`.kimi-code/mcp.json` restricted to three tools. No Kubernetes, AWS, database or
persistent application state was touched.

An MCP `initialize` request returned protocol `2025-11-25` and serverInfo
`orchestrator/0.1.0`. A new non-interactive `kimi -p --output-format stream-json`
session then emitted these actual MCP tool calls and results:

| Step | Kimi tool call | Observed result | Audit |
|---|---|---|---|
| 1 | `mcp__orchestrator__orch_list_applications` | Empty initial application list | HTTP 200, `OK` |
| 2 | `orch_create_application`, `confirm:false` | `confirmation_required`; no creation | No API status, `CONFIRMATION_REQUIRED` |
| 3 | Same create, `confirm:true` | ACTIVE `MCP Kimi Probe` application | HTTP 201, `OK` |
| 4 | `orch_get_application` with the returned key | Same application with `staging` and `production` | HTTP 200, `OK` |

Kimi's final answer correctly reported the initial count, dry-run outcome,
created key and both Environments. The four JSONL audit records matched the
four model-issued calls. The test Application existed only in the fake process
memory.

Both owned processes were stopped and the temporary MCP configuration,
binaries, logs and audit were removed. This verifies Kimi Code connectivity,
tool discovery/calling, API authentication, dry-run gating, application creation
and read-back. It does not exercise the other 12 tools through Kimi or claim a
real workload deployment.
