---
id: VERIFY-2026-10-09-IMPLICIT-CLUSTER
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-09
---

# Implicit existing-cluster — local delivery verification

## Scope and execution ownership

User accepted [ADR-013](../architecture/decisions/ADR-013-implicit-existing-cluster.md)
and requested Codex coordination/documentation/review with Claude implementing
code/test through `clauded` in a task-owned tmux session. Codex created
`codex-cluster-20261009`; existing user sessions were left intact. Claude edited
code/tests/helpers; Codex edited canonical docs, reviewed changes and performed
independent checks. No external deployment was authorized.

## Result

Internal planning binds `k8s-cluster.internal#connections.<key>` directly to the
Environment Connection, with current Application scope preserved, no user
Definition matching/registration. The graph/batches still execute existing-cluster
before consumers. Reserved system Definition `builtin-existing-cluster` is
admitted insert-only before progress/Active Resource writes; no Preview writes.
Stored foreign-content collisions and public key registration fail closed.
Environment target/credential identity survives reopen/retry/remove; historical
cluster Definitions remain, while new seeds no longer add one. Cloud VPC/EKS and
Aurora and internal namespace/PostgreSQL retain their original matching/drivers.
Console displays the implicit node as Environment connection, with safe metadata.

## Reviewed coverage

- Planner: no cluster Definition, conflicting/tied authored Definitions ignored,
  deterministic hashes, READY/kind/org/identity rejection, reserved record checks,
  forged bindings and cluster params cannot redirect execution.
- Provisioning: idempotent/concurrent admission, concurrent foreign-content
  collision preservation, forged match rejection, stored Connection recheck and
  local FK-enforcing wrapper proves admission before dependent writes.
- Deploy: selected nondefault/independent targets, real failed-deploy retry,
  JSON reopen then redeploy/remove, stored target guard and legacy host-context.
- Preview: deterministic system binding metadata, no catalog admission, existing
  no-mutation/redaction tests; Console row distinguishes Connection from matching.
- Regression: ordinary Go suite includes existing pending/transition, cloud
  account guards, VPC/EKS/Aurora graph and cross-profile PostgreSQL output contracts.
  Reference conformance uses explicit compatibility rather than changing fixtures.

## Independent validation

| Check | Result |
|---|---|
| `cd backend && go test ./...` | PASS |
| `cd backend && go build ./...` | PASS |
| `go test -race ./internal/planning ./internal/application/provisioning ./internal/application/deployment ./internal/application/preview ./internal/application/transition` | PASS |
| Frontend `npm run typecheck && npm run lint && npm test && npm run build` | PASS; 18 test files, 154 tests |
| `bash -n` for three edited integration helper scripts | PASS |
| `node --check` for four edited browser helpers | PASS |
| `application-connection-playwright-local.sh` | PASS; create/deploy and backend restart phases |
| `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py` | PASS; 251 Markdown files, 204 artifact IDs |
| `git diff --check` | PASS |

Go test subprocesses explicitly unset `ORCHESTRATOR_POSTGRES_TEST_URL` and
`ORCH_KIND_VERIFY`; optional external tests do not run. Browser run ID:
`conn-20261009083340-3821`. It used fake runtime adapters, JSON state, a kubectl
stand-in and a run-owned local server/browser. Output reported both phases `ok`
and status 0; the runner cleaned its temporary directory/processes. No manual
cluster Definition was registered in this run. Production was configured and
previewed; staging was deployed through fake adapters.

Documentation validation and PlantUML syntax/PNG regeneration passed under
Codex as the final documentation gate. The repository was clean at task start;
no pre-existing user-owned changes were incorporated. The external guide in the
parent projects folder was also updated and is outside this repository's commit.

## Limits

No live kind/Kubernetes/AWS run or PostgreSQL database mutation. The FK wrapper
proves ordering locally, not actual PostgreSQL integration/reopen. Existing
transition tests pass; no new transition-specific test/live migration was added.
Changed kind/video helpers were syntax-checked, not replayed. Previous dated
verification records remain historical and were not rewritten. Credentials and
private raw tmux/task logs are not included in repository artifacts.
