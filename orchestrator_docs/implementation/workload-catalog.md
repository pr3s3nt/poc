---
id: WORKLOAD-CATALOG-IMPLEMENTATION
artifact: implementation-handoff
status: draft
last_reviewed: 2026-10-28
---

# Workload catalog — implementation entry, issue #24

Issue: https://github.com/pr3s3nt/poc/issues/24

## Trạng thái và quyết định

Người dùng đã chấp nhận triển khai và yêu cầu E2E staging trước merge main.
Target staging/namespace chờ người dùng chỉ rõ; chưa được mutate target bất kỳ. Có mock isolated; product chưa có catalog workload tổng quát.
Không migration dữ liệu/adopt/xóa hệ thống Passbolt cũ.

Người dùng đã chốt: catalog tổng quát; giữ trang ứng dụng, Thêm Workload mở picker;
MySQL Terraform resource, CronJob độc lập tùy chọn; giữ PVC khi remove workload,
chặn Environment deletion khi còn retained data, xóa dữ liệu riêng; backup Passbolt
ở object storage tương thích S3. Giữ hai CronJob backup/cleanup, không mặc định
thay cleanup bằng lifecycle bucket vì policy >7 ngày và count >=10 cần bảo toàn.

## Đọc theo thứ tự

1. [Spec extension](../usecase/UC-16/workload-catalog-specification.md).
2. [Contract draft](../architecture/contracts/workload-catalog.md).
3. [UI](../usecase/UC-16/ui/workload-catalog.md).
4. [Realization](../usecase/UC-16/workload-catalog-realization.md).
5. [Passbolt runbook](../operations/passbolt-catalog.md).

## Code map / gap đã xác nhận

| Area | Existing source | Gap |
|---|---|---|
| Home/editor | frontend/src/features/applications/ApplicationHomePage.tsx; frontend/src/features/workloads/ | Picker + schema driven typed extensions, Service/storage/job fields |
| Catalog | backend/internal/application/catalog/; backend/internal/domain/resource/ | Workload template/version/capability contract; reuse bundle registry |
| Desired | backend/internal/application/workloadconfig/; backend/internal/domain/deployment/ | Round-trip extension, draft/apply pin, compatibility |
| Render guard | backend/internal/adapters/scorek8s/guard.go | Primary allows Deployment/StatefulSet only, not CronJob |
| Delivery/inventory | backend/internal/application/deployment/inventory.go; backend/internal/adapters/kubernetes/ | Runtime-specific apply/readiness, scheduled status/owned Job tracking |
| Secrets | UC-12/VSO delivery | Nested job Pod, file projection, database resource credential references |
| Infrastructure | backend/internal/adapters/terraform/; registry/module contracts | Generic PVC and MySQL modules; S3 binding capability; delayed binding |
| Persistence | backend/internal/adapters/postgres/ | Update canonical schema/ERD before adapter/migration; no destructive reset |
| Observability | backend/internal/adapters/kubernetes/telemetry.go | CronJob/Job ownership traversal and projection |

## Ordered work packages / exit gates

1. **Contract closure:** review extension wire/API, template publication/matching,
   schema/ERD and active Job removal semantics. This document proposes them,
   does not silently approve architecture. Write ADR before coding shared changes.
2. **Desired/catalog:** schema validation, immutable versions/scoping, template
   picker source, full round-trip persistence. Existing workload backward compatibility.
3. **Resources:** PVC/MySQL/S3 definitions/modules; identity/retention; resources
   independently usable without backup jobs. No credentials in state by reference.
4. **Delivery:** controller variants, Service/routes/mounts, secret env/files,
   registry, health/resources, CronJob lifecycle/read status.
5. **UI/Preview:** product form, source pickers, safe previews, changes/draft state,
   scheduling details; mock approval does not equal full form approval.
6. **Acceptance:** generic cases then new Passbolt app, authorized environment;
   failure diagnosis/retry with owned cleanup, no lowering assertions.

## Planned traceability/tests

| Requirement | Planned check | Gate |
|---|---|---|
| WC-01..03, WC-CAT | Browser original home → picker, keyboard cancel/focus, catalog scope/version errors | Catalog |
| WC-04..10 | Go/TS lossless desired/import/edit/persistence/restart; explicit values; old Score compatibility | Desired |
| WC-11, WC-LIFE | Write/read after restart/update; delete workload retains data; Environment deletion blocked; explicit owned delete | Storage |
| WC-12, WC-DEP | Standalone CronJob with no Deployment; schedule applied != Job complete; nested secret refs; suspend/history/UID ownership | Jobs |
| WC-13 | Service disabled/enabled, named ports, ClusterIP/headless and route target | Network |
| WC-SEC | Scoped credential env/file binding, no bytes in preview/state/Git/logs; revision protected for running Job | Secrets |
| WC-RES | WaitForFirstConsumer test without readiness deadlock; MySQL readiness preserved | Resources |
| WC-PRE | Hash/CAS/stale scope; graph and outputs safe; retained storage impacts visible | Preview |
| Passbolt | Browser fresh setup, DB/image persistence, backup + cleanup + restore to disposable DB | Live |

New E2E under e2e/scenarios with README; evidence/credentials outside Git.
Required product guards/errors have automated tests; runtime happy-path scope
follows current AGENTS. No skip used to claim acceptance. Backend Go test/build,
frontend typecheck/lint/test/build, docs checker/diff; external mutation only
with separately authorized target/resources and cleanup ownership.

## Remaining decisions versus runtime inputs

ADR-025 chốt typed workloadExecution v1, active Job blocks deletion và single-claim
replica limit. Exact template API/version source, capability negotiation và
schema/ERD được cụ thể hóa trước mutation implementation tương ứng.
Runtime inputs still needed: manifests without secrets, image digest/entrypoint,
Passbolt/MySQL versions, registry ref, cluster/account/namespace, StorageClass,
URL/TLS, mail settings, GPG initialization procedure, S3 endpoint/bucket/prefix/
region/TLS/auth ref and timezone. They block only real config/run, not generic work.
No exact deploy command is claimed executable until these inputs and product exist.

## Validation tài liệu — 2026-10-28

`python3 scripts/check_docs.py`: pass (442 Markdown, 378 artifact IDs).
`git diff --check`: pass. Không chạy code tests/runtime vì lượt này chỉ sửa docs.
Giữ nguyên 8 file prototype đã staged từ task trước. Chưa commit/push do Git
identity chưa cấu hình; không tự chọn tên/email tác giả.

## Implementation checkpoint — 2026-10-28

- Branch `features/24-workload-catalog`; tác giả `Codex <codex@localhost>` theo
  authorization người dùng. User cho E2E trên cluster kind hiện có, namespace
  mới tùy chọn; preflight đọc được `kind-idp-internal` và `kind-k8s-4f`.
- Domain `WorkloadExecution` v1: template pin, runtime, schedule/timezone/policies,
  volume bindings; structural validation và deep clone. Cron grammar v1 chỉ
  numeric five fields; tên tháng/ngày và aliases bị từ chối rõ, không đoán.
- Score → module → JSON applied set → reconstructed Score giữ execution,
  kể cả explicit zero/false. Existing JSONB/ERD ghi extension, không DDL reset.
- Chưa bật feature: capability gate ở planner trước provisioning và renderers
  ngăn silently fallback legacy. IMP-036 theo dõi gap; gate chỉ được gỡ khi
  template capability, desired validation và renderer/delivery integration pass.
- Chưa mutate cluster, chưa deploy Passbolt, chưa merge main. Template API,
  storage resolver, job lifecycle, UI và real E2E vẫn là công việc tiếp theo.
