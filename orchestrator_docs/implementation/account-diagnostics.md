---
id: IMP-HANDOFF-ACCOUNT-DIAGNOSTICS
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-25
---

# Account diagnostics implementation handoff

Implementation task authorized 2026-10-25. This document routes implementation;
it does not claim delivery before code and evidence exist.

## Sources

1. [UC-04 DIAG requirements](../usecase/UC-04/specification.md#account-diagnostics--diag-0114).
2. [ADR-022](../architecture/decisions/ADR-022-account-diagnostics.md) and
   [protocol/probe contract](../architecture/contracts/account-diagnostics.md).
3. [Schema](../architecture/database/schema.md#account-diagnostics--adr-022),
   [ERD](../architecture/database/erd.puml),
   [shared UI](../architecture/ui/README.md), UC-04 UI/state/API packages.
4. [Approved mock direction](../../frontend/design/diagnose/README.md): default
   account-integrated revision 2, not the standalone v1 or fabricated findings.

## Work ownership and implementation routes

The user explicitly asked one local Claude Code session launched with `clauded`
to implement and create a PR, and Codex to review, run independent end-user E2E,
merge to main and repair post-merge issues. This task-specific instruction
supersedes historical no-writer/no-writer-commit guidance. Keep existing sessions.
Claude owns code/tests/scripts and its PR; Codex owns canonical docs and acceptance.

Coordinator checkout `/home/thanhnt1/projects/diagnose/poc`, branch
`features/diagnose`; writer checkout
`/home/thanhnt1/projects/diagnose/claude-diagnose`, branch `claude/diagnose`.
Base `2879106` includes the mock and current main (`346cee9`). Session
`claude-diagnose-20261025`, tmux `$18`, pane `%18`, launched with actual alias
`clauded` = `claude --dangerously-skip-permissions`. CLI reported Sonnet 5.5,
high effort, bypass permissions on. Claude session ID
`3882893a-3f53-48bd-9300-875a2dfecd2d`. Brief is private task control at
`/tmp/diagnose-claude-brief-20261025.md`; terminal markers/handoff also live in
`/tmp`, never credentials or product state in Git. Normal writer monitoring
follows the existing 15-minute cadence, with early reads for errors/completion
or user requests. No additional writers are started.

Expected product areas: diagnostic domain/application/ports and adapters,
PostgreSQL baseline/grants and memory/JSON parity, HTTP/bootstrap, scoped account
Console/route/catalogs, meaningful unit/integration tests, focused real browser
runner. Reuse the existing credential resolver, Kubernetes CLI/materialization,
AWS verifier/action groups and planner contracts. Do not expose private resolver
data to the frontend or copy mock fixture logic into production.

Implement backend reports before wiring the UI; build UI with real DTOs. Preserve
all existing account registration, rename/CAS, verify, same-target credential
replacement and PE/Admin gates. Diagnostic scope fields are not account policy
or Environment deployment bindings. Fresh schema only, no retained DB reset.

## Acceptance

| ID | Required user-visible proof |
|---|---|
| DG-E01 | Sign in through Console; onboard a run-owned Kubernetes account using the real form and scoped Vault credentials. Existing connection/rename/credential paths remain working. |
| DG-E02 | Account list/detail separates verified connection from real diagnostics; GENERAL runs without Application and shows real TLS/API/RBAC/node/storage facts. |
| DG-E03 | Select use cases and optional app/environment; missing context stays UNKNOWN. With context, compare matching/storage/source/secret requirements to actual configuration. |
| DG-E04 | Inspect an actual failed owned-fixture condition and safe remediation; fix the fixture, rerun through the UI, confirm actual facts and correct dependent results. |
| DG-E05 | Explicit Kubernetes execution creates only run-owned probe objects, observes actual readiness and verifies cleanup; interrupted/denied/unavailable proof never becomes PASS. |
| DG-E06 | Switch account/purpose/region while checks run; no cross-scope result, stale response or false account status. History/export retain exact scope/time and truthful counts. |
| DG-E07 | Reload browser and restart backend with the same run-owned database; completed reports persist, in-flight reports/cleanup remain truthful. |
| DG-E08 | Existing org/role boundaries, strict request fields, stale versions, safe output/secret exclusion and owned-resource cleanup are covered by relevant automated checks. |
| DG-E09 | AWS adapter tests use actual signed request behavior/typed failures; live read-only AWS validation is recorded separately when a usable scoped account is available. No fake test proves real cloud permission/provisioning. |
| DG-E10 | After PR review and required checks, merge to main; build and run the meaningful real browser acceptance from the merged source, repair any attributable regression and preserve evidence. |

The acceptance runner must use actual product HTTP, PostgreSQL, Vault and an
isolated owned kind cluster, with no intercepted product API or fake diagnostic
adapter. User operations go through visible forms/buttons; API/native reads may
verify outcomes. Setup of cluster/addons/fixture faults is separate and recorded.
No mandatory slow typing, captions or video upload is inferred; assertions,
screenshots and actual backend/native facts are required. Preserve failure logs.

Use a unique run ID and an evidence directory outside Git. Record source SHA,
target identities/UIDs, created cluster/Compose project/DB/volumes and cleanup.
Cleanup only exact run-owned resources and verify native absence; any uncertain
cleanup fails acceptance. Retained clusters/services, user sessions and existing
applications are untouched. Paid AWS writes require a concrete authorized plan;
this task's default acceptance is isolated local Kubernetes plus truthful AWS
read-only/capability evidence. Never hide missing live AWS proof.

## Checks and handoff

Required changed-scope Go tests/build, frontend typecheck/lint/tests/build,
PostgreSQL integration with `REQUIRE_POSTGRES=1`, shell syntax for changed runners,
docs/diagram checks, diff review and real E2E. Code/test assertion changes must
not weaken deployed guards. Record exact commands, source, pass/fail/skip,
remaining capability limits and cleanup state in a dated verification record.

Completion requires the PR and merge SHA, post-merge check result and preserved
evidence. If external identity or tools block a part, continue independent work
and state the exact remaining gap rather than declaring unsupported checks PASS.

## Delivery record — completed2026-10-25

[PR #17](https://github.com/pr3s3nt/poc/pull/17) merged to main as `7f3886b`.
[Independent pre-merge verification](../verification/2026-10-25-account-diagnostics.md)
and [merged-main verification](../verification/2026-10-25-diagnostics-main.md)
record37/37 real browser assertions each, automated checks, capability limits,
actual AWS read-only observations and verified teardown. Source navigation is in
[the code map](code-map.md#account-diagnostics--adr-022). The earlier task/session
notes above preserve provenance; they are not remaining implementation work.
