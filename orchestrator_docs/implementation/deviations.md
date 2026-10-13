---
id: IMPLEMENTATION-DEVIATIONS
artifact: design-implementation-deviations
status: current
last_reviewed: 2026-10-13
---

# Known design and implementation deviations

File này ghi khác biệt hiện tại mà AI không được tự suy diễn mất. Hạng mục thuần
roadmap/deferred capability nằm trong [backlog](../backlog/README.md).

| ID | Difference | Required interpretation/action |
|---|---|---|
| IMP-002 | UC-01 đã hoàn tất. UC-02..04 insert-only registration, safe errors, no-restart catalog và PostgreSQL race/rollback/reopen đã verified local ngày 2026-09-30. AWS registration/credential flow chưa có. Kubernetes kubeconfig credential store và executor resolution đã được review/verified ngày 2026-10-06; upload recording xác minh kind read-only, không phải live deployment. Identifier policy và Driver Inputs shape boundary đã implemented/verified local ngày 2026-10-02; UC-16 parse/draft Save cũng đã validate toàn bộ non-virtual resource params trước mutation. AWS access key + Vault storage đã chốt theo ADR-009 nhưng onboarding chưa có; arbitrary new Type chưa có runtime-supported Definition pair. | Không tuyên bố toàn bộ UC-02..04 complete từ lát cắt local; giữ AWS credential onboarding/execution và arbitrary-Type/runtime-pair limitation tách biệt; validation gaps đã đóng theo evidence ngày 2026-10-02. UC-01 không còn thuộc deviation này; production RBAC ngoài MVP. |
| IMP-006 | Six rejected challenge fixtures mới chỉ so rejection status, chưa so structured `phase/code/path`. | D02 vẫn deferred; không tuyên bố full rejection-contract conformance. |
| IMP-007 | Terraform inspector hiểu remote source identity nhưng runtime chỉ execute embedded `vpc`/`eks`/`aurora`. | D03 đã chọn vào v2 theo ADR-014 nhưng implementation chưa có; giữ gap tới khi verified immutable registry/runtime hoàn tất. |
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

## IMP-019 — Implicit existing-cluster binding (resolved 2026-10-09)

User accepted [ADR-013](../architecture/decisions/ADR-013-implicit-existing-cluster.md).
Canonical design removes internal per-cluster Definition registration/matching.
Claude implemented code/tests via tmux; Codex reviewed and independently validated
planner/provisioning, scoped targets, reserved-key/collision handling, admission
ordering, pure Preview, JSON reopen/retry/remove and Console projection.
Cloud/PostgreSQL matching remains unchanged. Go tests/build, targeted race suites,
frontend gates and fake-adapter browser create/deploy/restart passed.
See [local evidence](../verification/2026-10-09-implicit-existing-cluster-local.md).
No live cluster/cloud or PostgreSQL mutation was performed; local FK simulation
is not real PostgreSQL integration proof.

## IMP-020 — Resource account reuse guard (resolved 2026-10-11)

UC-08 BR-07 requires validating persisted resource connection identity before
reuse. T16 review found `provisioning.Service.provisionNode` loads prior executor
state without comparing ActiveResource.ConnectionKey to the effective account.
T16 adds fail-closed account validation before executor invocation and preserves
stored identity on mismatch. `TestProvision_ReuseNeverCrossesAccounts` proves
same-account reuse and VPC/EKS/Aurora cross-account rejection; repository failures
other than NotFound also stop execution. See
[T16 local evidence](../verification/2026-10-11-T16-environment-aws-account.md).

<a id="imp-021-terraform-v2-designimplementation-gap"></a>

## IMP-021 — Terraform v2 design/implementation gap

I04-03 executor đã qua review và xác minh độc lập: 970 tests trên PostgreSQL
thật, không fail/skip, build/vet đạt và cleanup đã xác minh. Xem
[evidence executor](../verification/2026-10-13-i04-03-terraform-executor.md).
Private artifacts, provider graph/scoped target, HTTP state, retry/hold và
original-state absence verification đã có code/tests. Không coi provider seam
fake trong fixtures là live proof. IMP-021 vẫn mở cho modules, UI và integration.

I04-01 encrypted state/roles/fencing và hai final review corrections đã
implement/verified với mandatory PostgreSQL tests: 835 PASS, zero skipped.
[Completion evidence](../verification/2026-10-13-i04-01-complete.md).
TLS deferred theo [issue #2](https://github.com/pr3s3nt/poc/issues/2); không còn
blocker hoặc substitute constraints. IMP-021 vẫn mở cho UI/
module/live integration còn lại.

I04-02 registry/caller wiring đã qua review và xác minh độc lập: 884 tests
trên PostgreSQL thật, không skip; build/vet đạt. Xem
[registry evidence](../verification/2026-10-13-i04-02-module-registry.md).
Source shape v2 khiến form Definition cũ chưa tương thích; I04-07 phải sửa.
I04-03 đã kiểm provider-binding graph edges/scoped target resolution, durable
state, private invocation copies và transitive execution. Inspector
chưa hỗ trợ module count/for_each, object inputs hoặc `.tf.json`; nguồn HTTPS
hiện dùng anonymous Git. Không coi backend tests là live UI proof.

Accepted [ADR-014](../architecture/decisions/ADR-014-terraform-v2-lifecycle.md)
và current BR/MS/VAR/OOS trong specifications thay required behavior.
Rule mapping/supersession: [reconciliation](v2-rule-reconciliation.md). Current code is
not yet conformant: Packaged namespace/Redis/PostgreSQL modules chưa hoàn tất; AWS credential
onboarding and Environment-only Aurora/ElastiCache provider binding need implementation;
score-k8s is optional/internal-only; no Redis app cache or v2 module/source/class
UI và real acceptance còn thiếu. Durable encrypted state/locks và executor cleanup provenance đã có. PE/Admin resource detail/delete preview/name confirmation/selected-set
eligibility/confirmed-scope admission/progress/absence proof chưa có.
Đây là gap code, không phải các luật baseline còn hiệu lực.

This is a planned implementation gap, not permission to alter canonical design
to match old code. Codex updates docs; Claude implements code/tests through
clauded/tmux under AGENTS.md. Delivery gates: backend/frontend checks, real
PostgreSQL state/locking tests, module validate/plan review, three real UI/app
happy paths + two exceptions với video/assertions, A01 xóa qua UI và verified
owned cleanup. M04 ba chặng cần user gates: docs/plan trước code; video A01
và estimate AWS trước cloud create; final handoff/evidence để người dùng duyệt.
D12–D14 remain deferred after v2; do not mark them implemented as part of this gap.
No existing evidence or read-only STS check proves v2. Close only after review,
validation, required live acceptance and cleanup; update CURRENT_STATE/code map/
traceability with actual paths/tests/evidence then.

Các mô tả implementation bị loại khỏi specs/realizations được chuyển vào
CURRENT_STATE (checkpoint a897d6b/3367f7e), gồm catalog/renderer/planner
fixtures, current HTTP/draft/Preview/Deploy/Fleet/history/credential/SQL
behavior và old state/name limitations. Bằng chứng cũ vẫn giữ nguyên và
không được dùng để đóng các test obligations M04 mới.
