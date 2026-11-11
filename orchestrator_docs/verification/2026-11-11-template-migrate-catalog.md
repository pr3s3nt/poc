---
id: VERIFY-2026-11-11-TEMPLATE-MIGRATE-CATALOG
artifact: verification
status: evidence
last_reviewed: 2026-11-11
related: [ADR-042, ADR-041, UC-16]
---

# Shared template catalog with "Backend API có migrate" — issue #71

## Scope

Branch `features/template-migrate-catalog`. The shared template repository
`pr3s3nt/orchestrator-workload-templates` received `templates/standard`
(`bundle.yaml`, `resources/migrate.tpl`) from `poc` `dfa6470c` with a `SOURCE.md`
note, commit `84f8d9f`, tag `v0.5.0`. Guide:
[workload authoring](../operations/workload-authoring.md). Scenario:
[environment-bundle](../../e2e/scenarios/environment-bundle/README.md).

## Shared Console

As `platform-engineer`, in the Console: **Cấu hình tài nguyên → Sửa
git-dev-workload → Đổi nguồn → Git** with `templates/standard` at `v0.5.0`;
**Kiểm tra template** returned the source AVAILABLE at the tagged commit; saving
created revision r4, promoted with **Phiên bản → Đặt làm mặc định**. No other
Definition changed. Existing Environments kept their pins (`verify55b` r3,
`app 1` and `dap` r1); `verify55b` staging stayed Ready.

## Application run

`environment-bundle` with `ORCH_WORKLOAD_CLASS=git-dev` (the shared workload
Definition matches that class) and `ORCH_ENVIRONMENT=production`: in staging the
shared Console has three equally specific `persistent-storage` Definitions, so
Preview rejects the PVC as `resource-ambiguous`; that pre-existing configuration
was left unchanged. No Definition was added for the application.

`migrate71b` (production, r4): the "Backend API có migrate" card was offered;
one Deploy of backend, worker, frontend, scheduled SUCCEEDED; Fleet watched
`app` and `migrate`; `backend-migrate-<hash>` was Complete before the backend
Deployment was created; app checks (connection, environment, secret, PostgreSQL,
Redis) PASS, worker job DONE, PVC value kept after Pod recreation, scheduled
receipt written. Deletion is tested by the same scenario in
[the issue #67 run](2026-11-11-environment-bundle.md).

## Findings outside this scope

- A first attempt (`migrate71`) ran while the acceptance image was missing on the
  node: the migrate Job hit the readiness deadline, the app unit was never
  published, and Preview then reported no change, so the same desired state
  could not be deployed again.
- Deleting that Application stopped with `CLEANUP_INCOMPLETE`: the migrate unit
  had been published but failed, so it is the unit's base for composition yet
  is not withdrawn (removal considers only acknowledged executions). The Fleet
  GitRepo keeps the `migrate` path and the namespace remains; the Application
  stays `DELETING`.
