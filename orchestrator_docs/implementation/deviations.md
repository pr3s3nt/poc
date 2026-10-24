---
id: IMPLEMENTATION-DEVIATIONS
artifact: design-implementation-deviations
status: current
last_reviewed: 2026-10-24
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

## IMP-021 — Terraform v2 design/implementation gap (resolved 2026-10-16)

**Đã đóng trong phạm vi M04.** I04-01..12 đã implement/review/verify; năm
kịch bản A01/E01/E02/A02/A03 PASS/exit0/CLOSED_CLEAN với video, SQL sau
restart, cleanup native độc lập và tài nguyên giữ lại không đổi. R15 đạt
1482Go/race/0skip,649frontend/fullchecks/hash/fixture.
[Biên bản hoàn tất](../verification/2026-10-16-m04-completion.md).
D12–D14/TLSissue#2/M02 giữ ngoài phạm vi, không coi là được implement.
Các đoạn dưới đây là checkpoint lịch sử, không phải gap còn mở hiện hành.

### Lịch sử checkpoint

I04-08 đã review/verified local: scoped resource queries, PE/Admin selected-set
admission nguyên tử, REQUESTED cùng claim, replay từ persistence và proof của
đúng operation; UI/history/confirmation/progress đã có. 1171 Go/0 skip,
609 frontend và headed engine/Terraform/encrypted SQL/Vault scenario đạt.
[Biên bản](../verification/2026-10-14-i04-08-resource-delete-ui.md).
DESTROYED không còn bị báo RETAIN; marker Active/current Set/SUCCEEDED vẫn
nguyên tử trong Console. State mirror sau commit dưới owner là projection riêng;
không được xác nhận thì safe count diagnostic/METADATA_SYNC_PENDING và chặn xóa,
không nới quyền hoặc tự recovery. Objects/provider absence video giả lập;
IMP-021 còn mở cho Redis app, wiring và live proof I04-09..12.

I04-07 đã review/verified local: source/mapping/class UI, required renderer,
safe resource intents/progress và một snapshot cho pending/standalone Xem trước.
1121 Go/0 skip, 549 frontend tests và headed video đạt với SQL/Vault thật;
executors và attempts video giả lập, không thay live proof. Public registration
không còn static Kubernetes driver pairs. [Biên bản](../verification/2026-10-14-i04-07-definition-workload-ui.md).
IMP-021 còn mở cho selected-set deletion, Redis app, wiring và live acceptance
I04-08..12; các checkpoint bên dưới mô tả kết quả ở từng thời điểm.

I04-05 AWS modules đã review/verified với 1042 tests trên PostgreSQL thật,
không fail/skip, build/vet và cleanup đạt. Xem
[evidence](../verification/2026-10-14-i04-05-aws-modules.md).
Người dùng chốt PE-owned `/32` EKS API allowlist và VPC sở hữu private subnets
chung qua Resource References. Revision/addon pins, node type và ACU validation
đã sửa; multi-consumer graph/cleanup tests đạt. Provider plan tests mock, không
có AWS apply. Egress check còn wiring I04-10, PE Definition registration I04-07,
node capacity/real network/ElastiCache RBAC+TLS/Aurora compatibility I04-11/12.
Không coi AWS pricing/read-only preflight là live provisioning proof.

I04-04 modules Kubernetes và chặn secret binding đã review/verified: 1021 tests
trên PostgreSQL thật, không fail/skip; build/vet, readonly init/validate và
cleanup đạt. Xem [evidence](../verification/2026-10-13-i04-04-kubernetes-modules.md).
Catalog kiểm Type classification, planner kiểm Type/module nguồn đã match,
provisioning kiểm thêm runtime-sensitive flag trước lưu inputs/chạy consumer.
Static Kubernetes-driver Definitions không thuộc guards Terraform này.
Chưa có module apply/live pod/PVC/storage proof; UI và live integration
vẫn chưa hoàn tất. Module names lấy stable executor tag; timeout readiness dùng
cấu hình chung executor. Không seed Definitions trùng criteria cũ; UI registration
và matching fixtures còn I04-07/11.

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
not yet fully conformant: AWS credential onboarding đã implement/verified local
ở [I04-06](../verification/2026-10-14-i04-06-aws-connection.md), kể cả real
SQL/Vault/browser và IAM global/regional context guards; live onboarding và
execution capability preflight còn I04-10/12. Environment-only AWS provider
binding có backend guards; form Definition còn I04-07;
I04-07 required score-k8s và module/source/class UI đã verified local;
I04-09 Redis app cache đã verified PG/Redis thật (1254Go/0skip,609frontend),
6 ảnh có manifest digest, chưa real delivery wiring hoặc live acceptance. Durable encrypted state/locks và executor cleanup provenance đã có. PE/Admin detail/delete preview/exact confirmation/selected-set admission/
progress/operation-bound proof đã verified local I04-08; live A01 còn I04-11.
Đây là gap code, không phải các luật baseline còn hiệu lực.

This is a planned implementation gap, not permission to alter canonical design
to match old code. Codex updates docs; Claude implements code/tests through
Claude cloud theo AGENTS.md; Codex chạy thật local. Delivery gates: backend/frontend checks, real
PostgreSQL state/locking tests, module validate/plan review, three real UI/app
happy paths + two exceptions với video/assertions, A01 xóa qua UI và verified
owned cleanup. M04 chặng 1 đã duyệt; điểm duyệt 2 bỏ ngày 2026-10-14. A01/E01/E02 pass,
video/estimate đã lưu và k8s-4f sạch thì tự chạy I04-12; giữ duyệt bàn giao cuối.
D12–D14 remain deferred after v2; do not mark them implemented as part of this gap.
No existing evidence or read-only STS check proves v2. Close only after review,
validation, required live acceptance and cleanup; update CURRENT_STATE/code map/
traceability with actual paths/tests/evidence then.

Các mô tả implementation bị loại khỏi specs/realizations được chuyển vào
CURRENT_STATE (checkpoint a897d6b/3367f7e), gồm catalog/renderer/planner
fixtures, current HTTP/draft/Preview/Deploy/Fleet/history/credential/SQL
behavior và old state/name limitations. Bằng chứng cũ vẫn giữ nguyên và
không được dùng để đóng các test obligations M04 mới.

## IMP-024 — V02 Definition contract replacement — closed 2026-10-17

At base96eb602 the accepted UC-03 V02 contract was designed but unimplemented.
Definition Name/account/CAS edit, shared matching, explicit AWS account/region,
fresh baseline and all affected consumers now implement that contract. One full
post-writer review batch and its cleanup-evidence remainder were resolved before
independent final validation. [Checks and limitations](../verification/2026-10-17-v02-handoff.md).
No old-data migration or Generic Workload/V03 completion is claimed.


## IMP-022 — V03 Definition-selected workload/target gap — resolved

[ADR-016](../architecture/decisions/ADR-016-definition-selected-workload-targets.md)
was accepted before code. The V03 delivery replaces baseline7a00e43 Internal
Environment/system binding and render-only workload registration with matched
cluster Definitions and early workload references/rules, typed per-consumer
placement, persisted target pins and strict lifecycle guards. R6 replacement
passed before V03.10 removal; final r7 after removal passed35/35assertions,
including update/restart/negative/product cleanup/reopen. Independent final
Go1580/race526/frontend700tests and build/vet/type/lint passed without individual
skips. [Handoff](../verification/2026-10-18-v03-handoff.md) records video, cleanup,
failed attempts and valid result reuse. Backend/jobs/PG/Redis business coverage
remains explicitly deferred V04 by user decision; it is not an unresolved V03
implementation gap or a claimed PASS.


## IMP-025 — V06 retirement removed retained state-generation metadata — resolved 2026-10-20

The isolated unintegrated S5 full PostgreSQL run found that its fresh baseline
removed environments.generation_high along with deployment-profile fields.
ADR-017 preserves state/ownership and generation semantics; the unchanged
State grant/owner query still requires the existing generation metadata. The
schema table/ERD now explicitly name this retained column before the corrective
code change. Restore the original column/default in S5; do not broaden grants,
remove owner checks, migrate/reset retained stores or edit requirements to accept
the deletion. Focused real PostgreSQL setup/reopen/roles/state checks and affected
full regression are required. This patch remains gated behind V06 replacement
acceptance; V04 current code and runtime are unaffected.


Staged correction now passes focused27 fresh role/setup/reopen checks and final
S5 full1667Go/0FAIL/0SKIP, build/vet/conformance with real PostgreSQL. Grants and
owner checks are unchanged. This is preparation evidence only; resolution in the
product remains gated on V06 replacement/runtime/removal integration and commit.


## IMP-026 — unused Organization default-choice metadata after selector retirement — resolved 2026-10-20

Final S5 audit found the removed application-choice/Environment selector still
left Organization.DefaultConnectionKey, two SQL columns/FK, seed marker and a
SaveConnection side-effect updating that marker. None is used by current
Definition-driven execution. This is remaining V06.08 legacy-path removal,
not a new account-selection feature. Canonical schema/ERD and ADR-017 now state
the final boundary before code: remove the marker/update/seed, keep Organization
Driver Account ownership and safe credentials unchanged; preserve operation and
generation guards. Replace historical default-marker tests with whole-Organization
immutability, retired-snapshot refusal and current account/target contracts.
Fresh real-PG role/baseline/reopen/registration checks and final broad regression
are required; do not migrate/reset retained stores. S5 remains unapplied until
replacement coverage passes. V04 active source is unaffected.

## V05 runtime defect — platform-produced cluster output mapping (2026-10-19)

Canonical ADR-017/module contract and catalog validation correctly reserve the
k8s-cluster Type output `cluster` for the platform, never a Terraform module
mapping. V05 live UI incorrectly demanded a mapping for every required Type
output, so EKS Definition registration was blocked before provisioning. Fix the
Web Console mapping guidance/editor to mirror that existing special output
contract; do not fabricate a module output or change planner/catalog rules.
Existing server planning tests already cover platform production; add positive
UI mapping coverage and run affected frontend checks before retry. Preserve the
failed run/video and verify exact ECR/API/local cleanup before fresh run.

V05 R3 exposed a second UI catalog defect after the output fix: creating the EKS
Definition adds class `eks` to computed Resource Type metadata, but the page only
reloaded Definitions, leaving the namespace provider class selector empty.
Reload authoritative Type metadata after successful create/update as well; do
not invent selectable classes or force a hidden value. Verify fresh class appears
in positive UI test and actual headed scenario before retrying infrastructure.

V05 R4 reached full authored Preview; unknown-cluster assertion incorrectly
matched the word `endpoint` in the explanatory UI sentence saying access data
is unavailable until creation. Check actual placement JSON for endpoint/token/
kubeconfig properties and URLs, alongside exact provider/account/region/reference
pins; prose mentioning an unavailable output is not a leaked output value.
Retain mandatory assertion and secret scanning. No product behavior change.

V05 R5 passed all12 setup/Preview assertions. Runner then navigated back to the
Application before clicking Deploy, clearing the transient Preview panel and its
confirmation button. Remove that redundant navigation; deploy from the verified
Preview on the same page, matching the existing UI contract and V04 kit flow.
Product code unchanged; keep failed evidence/cleanup and all required assertions.

R6 corrected the earlier incomplete diagnosis: graph assertions additionally
opened the standalone Score Preview route after Pending Preview; removing only
back navigation left the browser on Score Preview, which has no Deploy button.
Order Score Preview graph inspection first, return to Application, then create/
verify Pending Preview last and Deploy without route changes. All graph/purity/
identity assertions retained. R5 navigation fix alone was insufficient; failures
and cleanup evidence remain recorded, no repeated retry on unchanged source.

V05 R7 actual Deploy stopped at VPC PLANNING. Native verified registry/runner
read-only reproduction fails AWS plugin schema handshake; direct plan with same
module/vars/key passes. Re-running the exact native CLI invocation changing only
TMPDIR to a short owned directory passes. Deep evidence/cache/work TMPDIR exceeds
Unix-socket path limits. Fix runner temporary transport directory ownership and
cleanup/retention, preserving verified binary/module/state/credential isolation.
Keep a private pointer in invocation workspace for retained recovery; confirmed
Close cleans it, unconfirmed process retains it. Extend existing positive scrubbed
environment test with actual socket bind/private mode/Close absence, and run real
provider readonly plan plus affected runner/executor/cleanup checks before retry.

### V05 R8 — platform cluster output checked before platform completion (2026-10-19)

Actual EKS apply reached OUTPUTS/FAILED/OUTPUT_CONTRACT after VPC READY.
ADR-017 correctly reserves typed `cluster` for platform completion from pinned
AWS identity, after module outputs; module mapping cannot author it. Provisioning
passed the full Type (including required platform `cluster`) into Terraform
module validation before completeEKSTarget could publish it. Existing service
fake did not enforce this intermediate contract. Fix the caller projection for
the EKS module only: module contract retains all module fields, platform output
is validated by the unchanged full Type after completion. Do not weaken full
Type/mapping/identity/readiness checks or alter module pins. Existing positive
same-execution service test will assert both intermediate module validation and
unchanged canonical required cluster contract; focused checks and retry pending.

### V05 R11 — CSI derived volume after original cluster destroy (2026-10-20 local)

Original Terraform reverse destroy completed, but native EKS layer absence found
one available1GiB PostgreSQLCSI volume. Supervisor correctly blocked laterlayers
and retained recovery. Exact originalmanifest cluster/run namespace/PVC/PVUID/
creation/state/attachment/account proof enabled one exact nativeAPI deletion and
NotFound proof; remaining steps resumed with original binaries/deadline. Operator
cleanup needs that guarded derived-volume reconciliation before unchanged EKS
absence verification. Do not weaken verifier or remove finalizers/force detach.
No product/cloud scope or general ownership bypass added.

### V05 prepared restart assertion — historical worker attribution

Self-review of AWS acceptance runner found that the post-restart persisted-job
assertion matched processed_by against newly rolled out worker Pods. Completed
job attribution correctly remains the original worker, whose Pod may be gone.
Compare the complete stored original job fields and UI row against original
observations/recorded worker UID. Independently require the new post-restart job
to be processed by a current worker Pod/UID. No product behavior or weaker
persistence requirement; keep both old attribution and current-worker proof.
R12 source36 is frozen; defer its stop until original provision/readiness
completes to avoid interrupting an in-flight Terraform write, then original
supervisor cleanup. Do not count this intentionally stopped run as app PASS.

### V05 R13 post-cleanup reopen — resource-list projection

R13 real46/46main assertions, original native cleanup/status all8steps and
CLOSED_CLEAN passed, but final reader expected5Terraforminstances in the public
Environment resources list. Current resource-list implementation excludes cluster entries;
changing delete/list eligibility is outside this fix. Preserve all5-instance
status/identity/module proof with labelled read-only SQL technical inventory after
restart, and check the API's4-row projection separately. Current fixture returned
all5mock rows, hiding the mismatch. Correct its positive shape, keep existing
negative tests, add no replacement exception tests. Preserve safe actual snapshot/
counts on failure and retain original DB/private for read-only diagnosis if reopen
fails, rather than discarding the only persisted evidence. R13 originalFAIL stays;
independent API/SQL/process/private closurePASS, no old result relabelled.


## IMP-027 — unreachable transition adapter/ports after V06.08 — resolved 2026-10-20

ADR-017 and current UC-01/04 contracts retire the deployment-target transition
entry point. Final removal review found its Kubernetes Transition adapter,
execution.WorkloadScaler/PostgresTransfer/RouteInspector/NamespaceCleaner/TargetProbe
ports and fake transition methods remain without any product caller. The only
real adapter constructor is the old integration-tagged kindtransition test.
Remove these leftover legacy contracts and that retired-only test; retain current
fake workload/database/route observation and all current guards/tests. The
dnsLabel regex currently declared in the dead adapter is used by current
UID-preconditioned deletion: move that exact declaration to kubectl.go, keeping
its pattern and every consumer unchanged. No replacement backup/transition feature.

A02 and A03 replacement gates already passed47/47 and independent cleanup.
Final-R1 stopped on stale generated frontend artifact before any creation; manual
closure is independently native/API/SQL/process proved, originalFAIL retained.
Correct build artifact was materialized from actual passing S5 build with source/
config124file equality and3dist SHA matches. No current cloud run. Before final
A03 retry, review this removal, compile all current Go test packages and run
focused current Kubernetes/fake/bootstrap/deployment tests/build/vet as affected.
Existing full/focused checks are reused only for unchanged behavior; no new
exception tests or live fault injection. Historical sources remain in Git.


## IMP-028 — stale diagram participants after Definition-driven replacement — resolved

Existing ADR-017 and UC-05/06 current specification/realization already require
Definition/reference fixed-point expansion without profile enrichment, and retire
deployment-target transition. Final diagram audit found an unused old participant
in UC-05 sequence, old implicit-enricher boxes/notes in UC-05 VOPC/shared classes,
and a transition/backup component that no longer belongs to the current design.
Align those diagrams to the existing approved collaboration: graph expansion and
matching remain, configuration/Secret Store/operation components remain, retired
profile/transfer participants disappear. The old Environment selector state
diagram is preserved explicitly HISTORICAL, not reused as a current lifecycle.
No product requirement or runtime source changes. Five affected PlantUML sources
rendered to valid SVG without syntax-error diagrams;34unchanged sources exactly
match validated snapshot02 and are reused. Docs352Markdown/304IDs and diff PASS.
Current actual final A03 source51 remains frozen.


### Final V06 resolution evidence for IMP-025/026/027

The staged descriptions above retain discovery history. S5 was applied only after
A02R3 and A03R1 complete47/47/independent cleanup. Final source51 A03
`a03f-1020041903-32d7` passed47/47/exit0/CLOSED_CLEAN/reopen/secret scan/native,
SQL, process and private absence. Current generation metadata is retained; obsolete
Organization default-choice fields and unused transition ports/adapters are gone.
Source49 composedGo117+115/FE74/build/vet and source51 focused164/race76/all-test
compilation cover the affected final content, with unchanged prior checks reused.
No current tests/guards removed to force pass; old tagged transition-only test
retired with its unsupported feature. No further work remains for these deviations.

## IMP-023 — Resource Definition v3 visual fidelity (follow-up correction 2026-10-21)

Review of commit `1b4a486` against the preserved v3 HTML found the editor still
embedded beneath catalog/cluster sections, without configuration cards/overview
or the fixed Save bar, with stacked mapping controls and excessive help text.
Functional acceptance did not establish visual fidelity. Correct the Console
composition against v3 while retaining existing metadata/converters/contracts;
verify configuration, matching and advanced views at the same viewport.

Resolution: focused catalog/detail views, numbered configuration cards and real
overview, aligned mapping rows, optional help and fixed action bar. Functional
guards/converters retained; desktop/mobile browser geometry and same-viewport
visual review complement existing tests. See [visual correction evidence](../verification/2026-10-21-resource-definition-v3-visual.md).

Follow-up user review: `59e7dc5` improves composition but still has boxed variable
options, separate advanced mode toolbars, duplicate companion section chrome,
and a bullet-only scope summary. Reopen visual acceptance; compare all three
preserved views and live tailnet rendering before reporting the correction.

Follow-up correction: one atomic advanced Form/JSON toolbar, compact variable
fields with read-only metadata defaults/provenance, grouped case summary, quiet
review/help and the existing light sidebar wordmark. The user-supplied live
reference `http://100.77.224.24:18088/?review=v3` was opened and exercised; its
HTML differs from the preserved file only in the advanced.js URL. Existing
contract tests and real HTTP browser checks pass; user visual review is pending.

## IMP-029 — Shared primary-action color mismatch — open 2026-10-23

The [shared UI design](../architecture/ui/README.md) says primary actions are
blue, while [Console styles](../../frontend/src/styles/app.css) render the
shared Button primary action green, with a blue sign-in override. The accepted
scope-specific origin of that difference has not been reconciled. The platform
effectiveness prototype reuses the existing Button/styles as requested; it does
not resolve this mismatch or redefine the canonical color requirement. Resolve
against approved design history in a separate scoped change, not by silently
restating current CSS as the requirement.

## Git copies of release modules — resolved 2026-10-21

Inspection initially generated opaque module IDs for Git copies of packaged
modules and dropped release-owned executor outputs. EKS planning recognizes
`eks`; namespace creation baseline recognizes `k8s-namespace`. A byte-identical
Git copy therefore lost required semantics. Resolved with registry content/Type checks and seven-module/restart tests: reuse
the release module ID and executor outputs only when both inspected content
SHA-256 and requested Resource Type match the trusted release entry. Keep the
Git source/commit/path/digest; never infer identity from repository/path/name.

## IMP-029 — Visual deployment review implementation and remaining format gap

Core implementation delivered on 2026-10-23: safe pinned `.tf` source and inputs,
selected symbolic Deployment/Service rendering, exact applied baselines,
persistence/correlation and observed per-node progress in the existing Console.
Independent code review fixed original-batch baselines, stable cross-revision
paths, removed artifacts, scalar redaction, symbolic namespace substitution,
read-only exact operation lookup, UUID parity and contradictory progress pins.
See [implementation](visual-deployment-review.md) and
[verification](../verification/2026-10-23-visual-preview-implementation.md).

Remaining explicit gap: the review contract mentions `.tf.json`, while the
existing registry rejects Terraform JSON configuration at registration
(`registry/tree.go`). This task preserves that guard and does not expand registry
format support. Consequently `.tf.json` source review remains unimplemented;
there are no admitted JSON modules silently omitted from a supported bundle.
Real cloud/cluster acceptance is unverified for this feature, not inferred from
fake-adapter browser execution. Existing database upgrade is an operator task;
only a fresh disposable baseline was initialized here.

## IMP-030 — Deployment review visual fidelity reopened

User review on 2026-10-23 identified that the product integration did not retain
the approved prototype's visual quality. Functional verification did not establish
visual fidelity. The integration nested the review inside the Workload card,
reduced graph/inspector space, omitted icon hierarchy/intent counts/runtime cards
and rendered dense technical metadata and all code files at once.

Reopen visual acceptance against `frontend/design/deployment-preview/desktop.png`,
`code-diff.png` and cloud screenshots. Restore a full-width review panel, graph
and inspector proportions, visual node hierarchy, clear properties/dependencies,
file selection and deliberate action/progress layout using existing Console tokens.
Keep real DTOs, safe artifacts and actual progress; no prototype fixtures/timers,
scenario controls or new mobile work. Compare real screenshots at 1512 and 1280px,
then validate affected Console behavior before updating the running demo.

Correction implemented: the review is now a sibling full-width panel, with the
approved icon/summary/runtime/property hierarchy, grouped provider reasons,
per-node file tabs, one graph across admission and an explicit busy action.
Coordinator compared actual Internal/cloud/code captures at 1512px and 1280px;
functional and Tailscale HTTP checks pass. User re-review is pending. See
[visual fidelity evidence](../verification/2026-10-23-visual-preview-fidelity.md).

## IMP-031 — Preview edge routing and Application availability diagnosis

User reported confusing Namespace/api/web connectors and repeated Application
errors on 2026-10-23. Actual HTTP/browser inspection found three independent
causes: (1) a long Cluster→Workload edge is drawn behind the Namespace card and
shares segments with Namespace→Workload, (2) resource inventory returns scoped
503 SERVICE_UNAVAILABLE because the isolated memory/fake backend has no Terraform
ResourceReader, (3) legacy seeded `acceptance` has only `dev`, but the Console
invents empty staging/production targets and sends requests to nonexistent scopes.
Initial pre-login session 401 is expected; no browser page exception was observed.

Fix rendering without changing the planner: route long edges around intervening
cards, group reasons for one endpoint pair, identify and highlight selected
connections. Preserve all reasons and ordering. Keep service-unavailable distinct
from a successful empty inventory, with a clear unavailable/retry state. Preserve
actual Environment keys and block unsupported Application routes before issuing
requests; do not create/migrate/delete legacy Environments or user data. Verify
all existing Applications read-only, preserve the running backend and its drafts.

Routing, unsupported-scope and favicon defects are corrected. Tests and actual
33-page audit pass; inventory HTTP503 remains a documented demo configuration
limitation with explicit unavailable presentation, not a resolved state backend.
The live backend was not restarted or its user data reset. See
[diagnosis evidence](../verification/2026-10-23-preview-errors-diagnosis.md).

## IMP-032 — Persistent kind runtime enablement for the Preview checkout

The reviewed Console on port 18093 uses memory/fake execution, so its UI success
does not establish live deployment. User requested a complete usable kind runtime.
Existing deployment contracts already require real matched Definitions, pinned
Terraform/score-k8s, scoped credentials, state persistence and readiness; no new
business behavior or AWS verification is implied. Preserve that demo and all
retained development data. Prepare an isolated persistent `preview-kind` project
and verify the existing flow on `kind-k8s-4f` with new owned resources only.
The new overlay explicitly supplies the installed score-k8s path (the base
Compose does not select a renderer), durable module/provider caches and separate
ports/volumes. Shared browser helpers also need current Definition v3 criteria
controls rather than the retired advanced-mode radio.

Operational procedure: [Preview on kind](../operations/preview-kind.md).
Completion requires dated actual runtime evidence, with any residual limitation
kept explicit rather than treating fake UI checks as live acceptance.

Resolved for kind: the new persistent runtime completed browser onboarding,
real three-workload/PostgreSQL/Redis deployment, inventory200, exact applied code
baseline/update, backend restart/job/resource persistence and independent
product-cleanup absence. The sample is deliberately retained; no AWS acceptance
or retained-data upgrade is claimed. See
[dated live evidence](../verification/2026-10-23-preview-kind-live.md).

## IMP-033 — No-change Preview crashes the Console

User reported a white screen when Previewing an existing Application. Browser
reproduction on port 18094 found HTTP200 with zero changes and the pending
service's zero-value review (`version:0`, null collections, no last plan).
DeploymentReview called `nodes.find` before checking the review version, causing
an uncaught render exception and an empty React root. Existing no-op tests omitted
the serialized review field, so they missed the actual response shape.

The existing no-change specification remains correct. Guard version/collection
shape in the public component before mounting the hook-owning plan renderer;
unsupported/missing review shows the existing unavailable text. Preserve the
parent's no-change message and disabled Deploy. Regression tests use the actual
zero-value response and unavailable→valid→unavailable transitions. No backend,
admission, data, deployment or lifecycle behavior is changed. See
[white-screen diagnosis](../verification/2026-10-23-preview-no-change-white-screen.md).

## IMP-034 — Git-owned render bundles

Accepted design on 2026-10-24: [ADR-021](../architecture/decisions/ADR-021-git-render-bundles.md)
and UC-03/05/06/07 RB requirements move normalization/defaults/output-only
provisioners/patches into a Git bundle while preserving infrastructure and current
object lifecycle. Current code embeds renderer.go/provisioners.yaml, normalizes
and produces policy patches in Go, exposes only the startup bundle map and lacks
Git inspection, scoped persistent render registry, typed render_inputs and UI.

**Resolved 2026-10-24** (issue #14): registry, persistence, engine, semantic
guards, planner/Preview pins, HTTP, Console, reference bundle and runner are
implemented; real end-user RB-E01..E10 passed
([verification](../verification/2026-10-24-git-render-bundles.md)). The cleanup
gate needed UC-07 VSO-01/02 (user decision 2026-10-24): removed workloads used to
leave VSO objects that blocked confirmed namespace cleanup.

Residual differences, kept deliberately:

- RB-C07 names `correlationId` in the T20 envelope, but the implemented T20
  envelope has no correlation ID; Git render errors use the existing
  `{error, code, field, params}` shape (`params.file` for bundle files).
- The Git path does not restore an explicit empty plain value through Go; the
  reference bundle patch restores it and the guard treats an omitted value as
  the empty string.
- Bundle files cannot contain Terraform-reserved names (`*.tf.json`,
  `override.tf`, `.terraform*`) because the shared hardened Git tree reader
  rejects them; irrelevant to render bundles.
