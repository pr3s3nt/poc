---
id: VERIFY-20261007-APPLICATION-CONNECTION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-07
related: UC-01, UC-03, UC-04, UC-05, UC-06, UC-08
---

# Application connection selection — local verification

## Scope and design

[UC-01 BR-07/08](../usecase/UC-01/specification.md) adds creation-time
Application connection selection. UC-06 BR-20 and UC-08 BR-07 prevent internal
cluster/Kubernetes and AWS Terraform VPC/EKS Definitions from overriding that
binding. Schema reuses applications.connection_id; no migration required.
Existing Applications retain stored bindings; retargeting is outside scope.

Coordinator edited documentation and reviewed code. Claude Code ran via
`clauded` in tmux `app_connection_coding`, editing product code/tests only.
Coordinator polled approximately every four minutes, apart from initial trust
prompt recovery and final handoff, and sent four review rounds. Corrections
included stale contract comments, clearing a removed selection instead of
silently using default, meaningful late-response/default-change/workload-target
tests, and a design-first AWS VPC/EKS binding correction. No unresolved design
feedback remained in the final handoff.

## Checks

| Check | Result |
|---|---|
| `cd backend && go test ./... && go build ./...` | PASS; conformance unchanged, credential propagation/restart/remove tests retained |
| `cd frontend && npm run typecheck && npm run lint && npm test && npm run build` | PASS; 121 tests in 14 files |
| `bash -n` new local runner and kubectl stand-in | PASS |
| Local onboarding Playwright regression | PASS create and restart |
| Dedicated connection-selection Playwright | PASS create and restart |
| Documentation checker, PlantUML syntax, regenerated diagram PNGs and diff whitespace | PASS |

Coordinator independently reran backend/frontend gates and inspected final
code plus create-form/production-deploy screenshots. Private local logs and
screenshots are in `/tmp/poc-application-connection-review/`; Claude's result
is `RESULT.md`, browser evidence is `browser-final/` (run ID
`conn-20261007100113-12396`). These temporary files are not repository artifacts.

## Browser observations

1. Platform session registers `lab-cluster` via authenticated API, using the
   read-only kubectl stand-in; registration does not change Organization default.
2. Developer sees default selected, chooses lab-cluster, creates an Application,
   and sends exactly name/subdomain/connectionKey. Application home shows
   lab-cluster/internal-k8s for both staging and production.
3. Before a matching Definition is registered, standalone Score Preview and
   pending Preview fail and produce no deployment; default cluster is not used.
4. Platform registers a matching existing-cluster Definition with resource ID
   `connections.lab-cluster`. Score Preview matches it, and pending Preview/
   fake Deploy succeed in both Environments, showing the saved target.
5. Backend restarts from the same JSON state. The Application binding and
   matched Definition remain; persisted Active Resources use lab-cluster.

## Limits

Runtime adapters are fake; this run proves browser/API/planning/persistence
behavior, not live Kubernetes/cloud workload deployment. Connection/Definition
setup uses platform-authenticated API calls rather than registration UI forms.
Recorded deployment target application is also checked with recording executor/
deployer test doubles. AWS target guards are verified with local planner and
provisioning tests only; AWS registration/credential resolution remain deferred.
PostgreSQL integration, kind and AWS external checks were not run because no
external verification was requested. No credentials, local state, binaries,
build outputs or browser artifacts were added to Git. Initial working tree was
clean; there were no user-owned changes to preserve.
