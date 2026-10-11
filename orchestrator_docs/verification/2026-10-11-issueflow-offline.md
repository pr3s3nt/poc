---
id: VERIFY-2026-10-11-ISSUEFLOW
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-11
---

# Issueflow — offline verification

## Scope and ownership

User requested a sequential GitHub Issues worker: Codex coordinates and reviews,
Claude implements, normal progress is read every 240 seconds, decisions stop the
queue, quota schedules durable resumption, and local Claude can fall back to cloud.
Codex edited documentation; Claude implemented `tools/issueflow/`, the Issue
template and tool CI in task-owned tmux session `codex-design-workflow`.
The tool is a separate Go module; product backend/frontend were not changed.

## Observed checks

Codex independently ran with writable task cache
`GOCACHE=/tmp/design-workflow-survey/go-cache`:

| Check | Result |
|---|---|
| CLI smoke with fake command processes | PASS; claim → coder → review/checks → publication → DONE, no real model or GitHub writes. |
| `go test -count=1 ./...` | PASS; two real-tmux socket tests skipped by the Codex sandbox, remaining tests ran. |
| `go test -count=1 ./internal/sup ./internal/config ./internal/agent ./internal/gitx` | PASS, including automatic cloud protocol tests; agent package has no standalone tests. |
| `go test -race -count=1 ./...` | PASS after fixing a double-`Wait()` race in a process test; real-tmux smoke tests skipped because the Codex sandbox forbids socket connections. |
| `ISSUEFLOW_REQUIRE_TMUX=1 go test -race -count=1 -v ./internal/tmux` | BLOCKED by sandbox `Operation not permitted`; this is not a successful independent real-tmux run. |
| `go vet ./...` | PASS. |
| `go build -o /tmp/design-workflow-survey/issueflow ./cmd/issueflow` | PASS; binary outside Git. |

Claude's final report records test/race/vet/build PASS with
`ISSUEFLOW_REQUIRE_TMUX=1`, no skips; both real-tmux/fake-interactive smoke tests
passed there. Codex read those logs, but could not independently reproduce the
real socket tests in its sandbox.

Supervisor tests exercise sequential ownership, worktree isolation, answer
ID/actor, queue halt, durable quota wake, restart/ambiguous effects, stop proof,
stale generations/acceptance, content hashes stable across staging/commit,
publication retries and evidence confinement. Cloud tests use temporary Git
remotes and fake CLI responses: pinned report validation, two feedback rounds
with report exclusion during import, no-change completion, blocked/quota events,
quota wake preserving feedback, ambiguous sends, stale reports and late-write
gates before acceptance/publication. These are offline observations.

## Evidence and limits

Local execution logs and handoff are in `/tmp/design-workflow-survey/`:
`implementation-checks.txt`, `implementation-status.txt`,
`independent-tests-final.log`, `independent-race-final-2.log`. These temporary paths are not durable online
evidence. No browser scenario exists for this CLI-only change, so no browser
video was recorded. Browser validator tests use fake media/structured assertions;
they do not prove a real UI scenario or a video upload to GitHub.

No live GitHub Issue, release asset, cloud session, nested real coordinator/coder
smoke, database, kind or AWS mutation was performed. Real CLI response formats,
per-worktree Claude trust/readiness, cloud report adherence and account/repo
permissions need a designated live smoke before enabling the worker.
See [runbook](../operations/issueflow.md) and
[IMP-020](../implementation/deviations.md#imp-020--unattended-claude-cloud-completion-transport-open).
