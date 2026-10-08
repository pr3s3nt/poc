---
id: IMPLEMENTATION-DEVIATIONS
artifact: design-implementation-deviations
status: current
last_reviewed: 2026-10-08
---

# Known design and implementation deviations

File này ghi khác biệt hiện tại mà AI không được tự suy diễn mất. Hạng mục thuần
roadmap/deferred capability nằm trong [backlog](../backlog/README.md).

| ID | Difference | Required interpretation/action |
|---|---|---|
| IMP-002 | UC-01 đã hoàn tất. UC-02..04 insert-only registration, safe errors, no-restart catalog và PostgreSQL race/rollback/reopen đã verified local ngày 2026-09-30. AWS registration/credential flow chưa có. Kubernetes kubeconfig credential store và executor resolution đã được review/verified ngày 2026-10-06; upload recording xác minh kind read-only, không phải live deployment. Identifier policy và Driver Inputs shape boundary đã implemented/verified local ngày 2026-10-02; UC-16 parse/draft Save cũng đã validate toàn bộ non-virtual resource params trước mutation. AWS access key + Vault storage đã chốt theo ADR-009 nhưng onboarding chưa có; arbitrary new Type chưa có runtime-supported Definition pair. | Không tuyên bố toàn bộ UC-02..04 complete từ lát cắt local; giữ AWS credential onboarding/execution và arbitrary-Type/runtime-pair limitation tách biệt; validation gaps đã đóng theo evidence ngày 2026-10-02. UC-01 không còn thuộc deviation này; production RBAC ngoài MVP. |
| IMP-006 | Six rejected challenge fixtures mới chỉ so rejection status, chưa so structured `phase/code/path`. | D02 vẫn deferred; không tuyên bố full rejection-contract conformance. |
| IMP-007 | Terraform inspector hiểu remote source identity nhưng runtime chỉ execute embedded `vpc`/`eks`/`aurora`. | D03 vẫn deferred; không nhận remote module là supported runtime contract. |
| IMP-014 | The self-hosted Orchestrator on kind receives the host's admin kubeconfig and the scoped Vault token as UC-12 secrets injected as environment variables. Logical state is now durable in PostgreSQL, but cluster credential delivery remains over-privileged. | Treat the self-hosted instance as a kind demonstration only, not a production deployment model. Do not reuse the admin-kubeconfig delivery outside kind; replace it with a scoped ServiceAccount or the UC-04 credential store before any shared use. |

Humanitec standalone Delta API, asynchronous deploy, whole-set workload apply và
incremental deployment là compatibility scope deferred ở D05. Resource
Definition/Score public boundary được theo dõi ở D06. Đây không phải lỗi code
đối với happy-path API hiện hành.

Khi resolve deviation, cập nhật canonical docs, code/tests, current state và xóa
hoặc sửa dòng tương ứng trong cùng change.

UC-04 upload gap IMP-015 was resolved on 2026-10-06 after parser/error/restart
review corrections, isolated PostgreSQL verification and the
[reviewed upload recording](../verification/2026-10-06-uc04-kubeconfig-upload.md).
AWS onboarding remains in IMP-002 and the specification's later delivery scope.

## IMP-016 — Environment connection design transition (resolved 2026-10-07)

User replaced Application binding with Environment Settings set-once, including
AWS Environment scopes. Canonical [ADR-011](../architecture/decisions/ADR-011-environment-execution-binding.md)
and UC-01 specify the new behavior. Code, migration and UI implement that design;
live kind verification and final local gates passed. Seed preservation assertions
now compare deterministic complete JSON values; the recording runner terminates
its owned browser process group and bounds the flow/restart waits.
See [reviewed evidence](../verification/2026-10-07-environment-connection-kind.md).
No remaining design/code deviation is tracked by this transition record.

## IMP-017 — Editable Environment stores/transitions acceptance gap (resolved 2026-10-08)

User accepted ADR-012 on 2026-10-07 and requested the current implementation
checkpoint be committed on 2026-10-08 before final acceptance. Backend/frontend
replacement code and canonical specs/schema are present. Compilation, owner/fencing,
PostgreSQL commands, routing preflight and visible Preview impact were corrected;
independent Go race/PostgreSQL integration/vet/build and frontend gates passed.

Review corrected recovery polling, idle-to-busy refresh, configuration/draft
submission guards, failure cleanup/process ownership/timeouts and explicit store
selection. The [reviewed real-kind recording](../verification/2026-10-08-environment-stores-kind.md)
passed with two workload Vault stores/VSO, actual PostgreSQL data migration,
stale-preview rejection, Ingress traffic, restart persistence and explicit cleanup.
Final local Go race/PostgreSQL/vet/build and frontend 153-test gates passed.
This resolves the first direct-Kubernetes delivery acceptance gap; distinct-cluster
DNS transfer and AWS/Aurora migration are not claimed. Historical evidence remains
unchanged. Other legacy recorder changes have syntax checks, not fresh live evidence.
The existing integration-tag AWS test in the baseline passes a string to
EKSDescriptor's Context parameter; this delivery does not claim it passes or
perform an AWS run. See CURRENT_STATE for current limits. Do not weaken canonical
requirements or extend this recording beyond its documented live scope.

## IMP-018 — Compose store bootstrap correction (resolved 2026-10-08)

Compose initially seeded a configured-platform legacy store with an application
runtime token file. User clarified that the bundled Vault must be an ordinary
verified store, equivalent to one added by a Platform Engineer. UC-04 SS-07..08
and ADR-012 now require shared validation/verification/credential persistence,
idempotent startup and in-place conversion preserving existing secret references.
Shared normal-store bootstrap, managed CAS admission and regression tests now
implement that correction. [Real Docker/UI verification](../verification/2026-10-08-compose-vault-normal-store.md)
passed fresh seed, in-place upgrade, two-Vault browser writes, runtime without
bootstrap and token replacement. Historical legacy evidence remains unchanged.
