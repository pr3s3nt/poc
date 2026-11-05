---
id: VERIFY-2026-11-05-INFRASTRUCTURE-DRIFT-REPAIR
artifact: verification-record
status: evidence
last_reviewed: 2026-11-05
related: [UC-08, ADR-030, INFRASTRUCTURE-DRIFT-CONTRACT]
---

# Issue #34 — full reviewed Terraform Repair

Source task: [issue #34](https://github.com/pr3s3nt/poc/issues/34), updated by the
user's direction to exclude Accept Drift and support reviewed Terraform
create/update/delete/replace. Branch: `features/drift`, starting at `48035f75`.
No user-owned changes existed at task start. No staging/cloud/Kubernetes
application was deployed or mutated by this task.

## Implemented scope

- Normal refreshed plan against the acknowledged pinned configuration; exact
  private semantic/variable/target comparison before saved-plan apply.
- Complete managed create/update/delete/replace, output/inventory validation,
  explicit destructive acknowledgement, namespace owned-content checks and
  existing State capability/lineage/serial/owner/fence protocol.
- Provider-first dependent scope, including edges through workloads/templates.
  Durable checkpoints retain completed instance/consumer pins. A remaining
  Terraform plan requires a new actor-bound ten-minute review, rather than an
  unreviewed expansion or replay of completed work.
- Consumer inputs resolve from frozen applied plan snapshots and current
  acknowledged provider outputs. Active Resource outputs/secret references,
  typed cluster metadata, rendering, delivery and routes use existing paths.
  Consumer refresh creates separate execution/empty Delta records; the current
  Desired State and historical Deployment records remain unchanged.
- Restricted operational Desired/Actual scalars only; arbitrary values, tags,
  secret/state/plan content and private comparison digests remain private.
- Existing VI/EN dialog/panel: full action names, consumer/deferred/completed
  scope, separate deletion/replacement acknowledgement and next-stage action.
  Refresh restores pending continuation from persisted operation/review records.
- No PostgreSQL schema or runtime privilege change: extended review metadata
  fits existing safe/private JSON columns. Frozen migrations/references remain
  untouched.

## Checks and evidence boundaries

The local test tools were Go 1.25.0, Terraform 1.9.8 (final pinned run; earlier runs used 1.5.7), Node 24.19.0, PostgreSQL 16
and Redis 7.0.15. Tool downloads, test credentials, databases, browser sessions,
logs and screenshots are outside Git. PostgreSQL testing uses a task-owned
container with password authentication, disposable owner/Console/State roles and
fresh test databases. No retained installation was upgraded.

Passing focused checks (final seven-package run with Terraform 1.9.8 passed;
Terraform package 78.494 seconds):

- `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test` covering `TestRepair*`,
  `TestReviewed*`, `TestComparison*`, private digest/parser tests and the existing
  infrastructure scan HTTP scope/RBAC test in drift/tfexec/provisioning/deployment,
  domain, PostgreSQL and HTTP packages.
- Real Terraform saved-plan recreate and replace, changed output acknowledgement,
  converged subsequent plan; the external provider observation is explicitly
  simulated on the disposable State copy using built-in `terraform_data`.
  This is real Terraform/PostgreSQL execution, not AWS/Kubernetes conformance.
- Real Console/State PostgreSQL roles and encryption through production Repair
  coordination: persisted review, one-time confirmation, idempotent retry and
  successful rescan; simulated missing-resource observation as above.
- Coordinator tests cover deferred dependent review, restart-readable checkpoint
  pins, no completed-provider/consumer replay, destructive consent, foreign scope,
  cycles, stale dependent/completed pins and dependencies through non-Terraform
  consumers. These tests explicitly use a coordinator fake.
- Pinned input binding/secret refusal and rendered consumer output propagation,
  preserving Desired State/history even after the current Definition changes.
  Resource/delivery adapters in this consumer test are explicit fakes.
- `go build ./...`; acceptance workload package tests with real PostgreSQL and
  Redis pass after supplying the required local tools/authenticated database.
- Frontend typecheck, lint and build pass. Lint retains one unrelated existing
  warning in PlatformSettingsPage; build retains the existing chunk-size warning.
- Frontend full rerun: 955 passed, 3 failed (958 tests). The three failures are
  reproduced on untouched `48035f75`: App deep-link selection for `workload`
  under the concurrent subset, old Terraform-v2 scenario labels and old V04
  assertion IDs. App's isolated 36-test run passes on both checkouts. The initial
  unconstrained run also had transient observability/matching-preview failures;
  both passed the focused rerun with two workers. Drift panel/dialog tests pass,
  including destructive consent, next-stage confirmation and continuation after
  reload (10 focused component tests). HTTP projection tests ensure private
  operation Detail remains hidden.
- Chromium at 1440px renders actual AppShell, shared styles/components and the
  actual Repair dialog with fixture API data. The browser flow checks consent,
  safe comparison, deferred stage, next preview and final success; no raw token
  appears in the UI. This proves component interaction, not live backend access.
- Documentation checker and `git diff --check`; the changed PlantUML sequence is
  regenerated and visually inspected using the PlantUML container through stdin.
  Local `plantuml` is absent, so the checker skips its repository-wide local
  syntax invocation; CI's required PlantUML check remains applicable.

The whole backend `go test ./...` was attempted and is **not green**. It exposes
existing Generic Workload/legacy render-bundle/Score fixture failures, old schema
upgrade assertions and integration scenario IDs, plus a Terraform package timeout
under the concurrent full run. A separate whole drift/domain/tfexec run also
reached the default ten-minute limit in the 93-test Terraform package; it
reported a CLI-version mismatch and retained-release destroy assertion. The
retained-release assertion is reproduced unchanged on `48035f75` with Terraform
1.9.8; the final focused Repair/VPC run uses that required CLI version.
Representative provisioning/preview/PostgreSQL
failures are reproduced on untouched `48035f75` with the same tools. The initial
full run also lacked Redis/client tooling and used a trust-auth database; the
acceptance workloads were rerun successfully with Redis and an authenticated
isolated database. These results are not represented as a passing full suite.

## UI review and remaining runtime gate

UI source/reference: existing AppShell, ApplicationsPage, shared Dialog/Button,
InfrastructureDriftPanel/InfrastructureRepairDialog and shared UI design. No new
shell/navigation/theme is introduced. Final review removed empty action tables
for deferred/completed members and combined comparison address/path in one column.
Destructive risk remains at confirmation; explanatory definitions are not added
as repeated page text. Screenshots/logs are under `/tmp/poc-drift-*` on the task
host, outside Git, and contain fixture data only.

A real cloud/Kubernetes Repair of recreated/replaced infrastructure and dependent
workloads remains unverified. Historical E09/E10 verifies only the earlier
in-place slice and cannot substitute for this runtime gate. Running such a gate
requires an explicitly authorized owned environment, credentials, run identity,
cleanup and (for AWS) cost controls from the existing runbooks.

No live Console handoff URL is claimed. The task environment's default
`tailscale serve status` cannot reach its local daemon/socket. The loopback Vite
browser fixture is not evidence of Tailscale or product backend availability and
is not offered as a usable Console link. Any later live handoff must follow the
[Tailscale runbook](../operations/tailscale-handoff.md).
