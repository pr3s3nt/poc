---
id: PROJECT-CURRENT-STATE
artifact: project-status
status: current
last_reviewed: 2026-10-26
---

# Current project state

## Generic Terraform resource completion — 2026-10-26

UC-08 BR-10 uses successful Terraform execution, acknowledged managed state
and validated outputs as default completion for every resource type accepted
by the verified provider/module contract. Existing supplemental Kubernetes/AWS
probes, EKS usability and namespace provenance still gate their objects,
including mixed modules. No additional provider or cleanup permission is added.
Resources whose provider schema has no conventional `id` attribute are also
supported, including `kubernetes_manifest`. Real headed Playwright acceptance
passed **57/57** checks (21 completion-specific): shared/private resources,
actual output consumption, valid in-place update, stable UIDs/state identities,
PostgreSQL/Redis job flow and product cleanup. Namespace, isolated kind cluster,
Compose containers/volumes, workload Vault and run image tags were verified
absent. No AWS execution is claimed. See the
[verification record](verification/2026-10-26-terraform-resource-completion.md)
and [runbook](operations/terraform-completion-kind.md).

## F3/F4/F5 corrections — accepted on kind — 2026-10-25

Shared Score aliases are preserved as private Deployment Set metadata, outside
planner/hash semantics, including multiple aliases for one ID. Historical sets
without this metadata keep generated names. Saved Git source initialization
survives metadata arriving after a source-mode switch and preserves user edits.
Namespace creation baselines are immutable per instance and namespace UID,
allowing CREATE after DESTROYED without reusing a previous incarnation's proof.
Existing missing provenance is not inferred from live objects; previously stuck
namespaces can remain protected by the ownership guard. The existing Claude
`kiemtra` session verified 85/85 Terraform executor tests on PostgreSQL, repeated
race checks, and 13 business checks on real kind: shared aliases survive Deploy,
no-op Save and restart; two same-instance namespace recreation cycles succeed
on their first attempt; delayed Git metadata preserves source/ref/focus.
New acceptance resources were removed through the product with namespace/PV
absence verified. The pre-fix namespace with missing incarnation proof remains
guarded. See the [verification record](verification/2026-10-25-workload-alias-namespace-git-fixes.md).
See [deviation record](implementation/deviations.md#imp-036--workload-aliases-and-namespace-recreation-2026-10-25)
and [code map](implementation/code-map.md#verified-feature-defect-fixes--f3f4f5).

## Basic application logging/monitoring — real Kubernetes accepted — 2026-10-25

Issue #1 basic slice follows [ADR-023](architecture/decisions/ADR-023-native-observability.md):
Orchestrator collects owned workload logs/pod state and Metrics Server CPU/RAM
through existing scoped internal/EKS credentials and stores bounded 7-day logs /
15-day metrics/events in PostgreSQL. Console has real stored-data overview,
filtered log details and history charts, using the approved UI. No extra telemetry
stack or application SDK is installed.

[Handoff](implementation/native-observability.md),
[runbook](operations/observability.md) and
[verification](verification/2026-10-25-native-observability.md) record local
SQL/HTTP checks and [real new-user Kubernetes acceptance](verification/2026-10-25-native-observability-live.md): 48/48 checks, including actual logs/load, container restart, pod update, backend restart, retained history and product UI cleanup. EKS-specific live verification remains separate.
The schema/snapshot baseline changed; retained DBs and preexisting clusters were
not changed. The new run-owned kind/Compose runtime was removed after acceptance. A
data-preserving rollout to an older retained DB needs a separate operator upgrade
procedure. Issue #1 remains open for provisioning/Definition integration,
resource-specific exporters, CloudWatch, alerting, tracing and advanced providers.

## Infrastructure Account diagnostics — implemented and accepted — 2026-10-25

[ADR-022](architecture/decisions/ADR-022-account-diagnostics.md) adds General and
Use-case diagnostics inside Infrastructure Account detail, with a separate latest
diagnostic column, persisted progress/history, cancel, safe JSON export and VI/EN
findings/remediation. Connection verification status/version stays independent.
The worker uses saved scoped credentials, bounded leases and repository revisions;
Application requirements come from the complete desired state on one planning
snapshot, including unchanged applied workloads. Kubernetes execution uses a
unique owned namespace and observed pod/PVC probes with UID-guarded cleanup.

Implementation and checks are mapped in [the delivery handoff](implementation/account-diagnostics.md)
and [code map](implementation/code-map.md#account-diagnostics--adr-022).
Independent real Console acceptance passed **37/37 before merge and 37/37 on
merged main** (`7f3886b`, [PR #17](https://github.com/pr3s3nt/poc/pull/17)).
Coverage includes actual missing-class repair/rerun, exact-class Bound PVC and
readypod/cleanup, stop, scope isolation, versioned export, history/restart and
SIGKILL recovery. Read-only AWS checks also ran against real signed endpoints.
See [pre-merge evidence](verification/2026-10-25-account-diagnostics.md) and
[main verification/cleanup](verification/2026-10-25-diagnostics-main.md).
A [post-merge CI correction](verification/2026-10-25-diagnostics-ci.md) removes
intermittent fencing/template-test races while preserving runtime guards.

Limits: conditions are observations at the report time; primitive pod/PVC probes
do not execute an Application image or prove SQL/cache/application behavior.
Workload-side Vault authentication remains UNKNOWN. AWS policy simulation is
identified separately from provisioning; quota comparison and paid AWS execution
are unsupported and remain UNKNOWN. No retained database is migrated or reset;
the repository's fresh-baseline database policy continues to apply.

## Open workload object set and Gateway API routes — implemented and accepted 2026-10-26

[ADR-024](architecture/decisions/ADR-024-open-workload-objects.md) is implemented
for Git render bundles: a new `resources` stage and open guard let a bundle emit
any namespaced object (HPA, PDB, ConfigMap, PVC, Job, CronJob, StatefulSet,
NetworkPolicy, HTTPRoute, operator CRs) and any pod/container field; cluster-scoped,
Secret and VSO objects stay refused and explicit Score intent/identity stay
protected. Score `metadata.annotations` reach bundles. The deployer checks API
discovery and ownership before writing, runs Jobs to completion before rollout
objects, persists the object inventory in `workload_instances.target_ref.objects`,
prunes dropped objects by UID after a successful update and removes all of them
with the workload. Public routes can be Gateway API HTTPRoutes
(`-public-route-kind httproute`, `-public-gateway`, `-public-base-domain`; see
[runbook](operations/public-routes.md)). Production bundle repository:
`https://github.com/pr3s3nt/workload-manifests` (`bundles/production`). Real
kind/Envoy Gateway browser acceptance passed 36/36:
[verification](verification/2026-10-26-open-workload-objects.md).
Limits: the staging Gateway, DNS and TLS were not verified (no access from this
machine); Vault Agent delivery gives bundle Jobs no Agent-injected files; Fleet
delivery stays unavailable; installed (legacy) bundles keep the ADR-010 envelope.

## Git-owned manifest rendering — implemented and accepted 2026-10-24

[ADR-021](architecture/decisions/ADR-021-git-render-bundles.md) is implemented:
PE/Admin inspect a credential-free HTTPS Git directory in Mẫu dựng ứng dụng
(new drafts default to Git), configure typed `render_inputs` and save; the
verified Organization-scoped pin lives in `render_bundles` and the durable
render bundle cache. Planning/Preview/Deploy pin source, engine, binary and
effective inputs and never fetch Git; a bundle- or input-only change is a
workload UPDATE. Normalization, defaults, output-only score-k8s provisioners and
ordered patches come from the bundle (killable template worker, sjson patch
semantics) and are checked by semantic invariants instead of a native render.
Installed bundles keep their legacy path. Reference bundle:
`backend/render-bundles/standard`. Workload removal now also deletes that
workload's VSO delivery objects (UC-07 VSO-01). Real end-user acceptance
RB-E01..E10 passed: [verification](verification/2026-10-24-git-render-bundles.md).
Limits: public HTTPS Git only; no bundle garbage collection; legacy Vault Agent
removal keeps its ServiceAccount behavior; error envelope has no correlation ID
(see [IMP-035](implementation/deviations.md#imp-035--git-owned-render-bundles)).

Post-merge verification: PR #16 merged into main with both literal provenance and
Git bundle metadata preserved; independent real-adapter A→B→restart→A plus UI
cleanup passed 47/47. Runner now fails if teardown cannot be verified. See
[main integration evidence](verification/2026-10-24-pr16-main-verification.md).

## Workload environment editor — implemented — 2026-10-24

Each container has one **Biến môi trường** table with five sources (entered
value, Application variable, Secret, Resource, other workload) and inline creation
of Application keys (create-only `POST .../configuration/keys`), the initial Secret
Store and workload dependencies, in Vietnamese and English. Ordinary strings
(empty, whitespace, escaped `$${...}`) round-trip through Save, Preview, Deploy,
Edit, restart and UC-12 rename; readback of a deployed literal needs a private
proof stored with the Deployment Set (`literalProofs`), written only by a UC-16
deploy of a validated draft. Historical and direct-API literals stay unreadable.
Evidence: [2026-10-24 DEVEX-ENV implementation](verification/2026-10-24-devex-env-implementation.md).
Limitations: browser acceptance used fake execution adapters; the editor never
reads secret values; an Application key created in place commits independently of
the workload Save; a literal accepted by Save is stored with the configuration and
is not a place for secrets. Design: [handoff](implementation/workload-environment-editor.md).

## Console Vietnamese/English message catalogs — 2026-10-24

All Console text formerly hard-coded in Vietnamese now comes from runtime JSON
catalogs (`frontend/public/locales/vi.json`, `en.json`, `manifest.json`) per
[ADR-020](architecture/decisions/ADR-020-console-message-catalogs.md). Vietnamese
stays the default and fallback; a “Ngôn ngữ”/“Language” selector sits in the
sidebar footer and on the sign-in page. Dates and numbers follow the selected
language. A new language is a catalog file plus a manifest entry, no code change.
Limitation: strings that were already English in source (none with Vietnamese
text) are still literal, and backend messages remain code-mapped only (T20).

## Readable Preview connectors and safe node facts — 2026-10-23

Preview cards now have factual rows and larger spacing. Routing uses independent
horizontal tracks, outer lanes and reserved attachment columns; real browser
geometry at 1512px/1280px found no card crossings or shared connector segments.
An optional selected-node connection view reduces clutter without changing the
graph. PostgreSQL shows configured database name and pinned literal port, Redis
its identity/port, namespace its name, and workloads safe image/ports with fuller
details in Overview. Unknowns stay explicit; summaries never expose secrets/raw
inputs or query providers. Backend/frontend runtime was updated after an idle
check, preserving metadata for seven Applications. User confirmed the existing
sidebar reopens, so its behavior was kept unchanged. See
[node facts/spacing verification](verification/2026-10-23-preview-node-facts.md).

## Desktop sidebar and colored resource illustrations — 2026-10-23

The signed-in shell can collapse from 260px to a 76px icon rail and restores
the local display preference. Named navigation, role gates, active route,
identity/sign-out and keyboard access remain. Collapse preserves mounted drafts
and selected review node/file; blocked browser storage does not break navigation.
Desktop Preview pages use the released width; real browser checks at 1512px and
1280px measured 184px additional graph width. Resource nodes/inspector and menu
icons now use coordinated type colors with native SVG, while state labels remain
independent. See [desktop UI verification](verification/2026-10-23-sidebar-preview-space.md).

## No-change Preview rendering fix — 2026-10-23

Previewing an already-deployed/empty Application exposed a frontend crash:
the zero-value review has null collections, but the renderer read nodes before
checking its version. IMP-033 guards the wire shape before mounting the renderer
and adds actual no-op payload/availability-transition regressions. No-change
notice and disabled Deploy remain; valid plans still show their graph. Both
running Consoles received the fixed assets without backend restart or data writes.
See [diagnosis and verification](verification/2026-10-23-preview-no-change-white-screen.md).

## Persistent Preview Console on kind — 2026-10-23

An isolated `preview-kind` Compose project now serves this checkout at Tailscale
port 18094 using real adapters, durable PostgreSQL/Vault/state and explicit
score-k8s. Browser onboarding → Preview → Deploy created real frontend/backend/
worker, shared PostgreSQL/Redis and two Bound PVCs on `kind-k8s-4f`. Inventory
HTTP200, persisted manifests, code diff with exact applied baseline, valid image
update, backend restart/resource reuse and old/new completed jobs passed.
A separate disposable namespace was deleted through the product and independently
verified absent. The sample remains usable; port 18093 stays the original fake
demo. No retained data reset, user session removal or AWS execution occurred.
See [runtime evidence](verification/2026-10-23-preview-kind-live.md),
[runbook](operations/preview-kind.md) and IMP-032.

## Visual deployment review — implemented 2026-10-23

Desktop Console now shows the final pending graph, dependencies and placement,
pinned sanitized Terraform source/inputs, selected score-k8s Deployment/Service
preview, red/green code diffs and persisted per-node progress. Existing Internal
clusters are labeled as bindings; cloud resources follow selected Definitions.
Per-click UUID correlation isolates operations/deployments and retries. Desired
review and rendered workload review survive persistence; only exact READY
associations provide an applied baseline. Historical missing data stays unavailable.

Initial independent review included product HTTP/browser flows using fake execution
adapters, a real disposable PostgreSQL fixture, backend and Console checks. The
subsequent real kind execution is recorded above; no new AWS execution is claimed.
Terraform source is a redacted view,
not a provider plan; registry-unsupported `.tf.json` remains outside supported
source coverage (IMP-029). Existing PostgreSQL installations require the normal
operator-managed schema upgrade; no retained database was reset or migrated.
See [implementation](implementation/visual-deployment-review.md) and
[verification](verification/2026-10-23-visual-preview-implementation.md).

The initial integration's visual acceptance was reopened by user feedback.
IMP-030 corrects it against the approved prototype: full-width review, original
column proportions, icons, runtime/configuration cards, grouped dependencies and
file-based Code. The Tailscale HTTP demo also has a cryptographic UUID fallback.
Coordinator screenshot comparison and browser checks pass; user re-review is
pending. See [fidelity correction](verification/2026-10-23-visual-preview-fidelity.md).

IMP-031 corrects connector routing around intermediate cards and the Console's
requests for nonexistent legacy staging/production Environments. The current
memory/fake demo still lacks the Terraform resource reader: inventory HTTP503
is shown explicitly as unavailable, never as an empty list or failed deployment.
[Diagnosis and 33-page audit](verification/2026-10-23-preview-errors-diagnosis.md)
record the remaining backend limitation separately from corrected UI defects.

## UC-17 Hiệu quả platform — issue #12 đợt 1, 2026-10-23

PE/Admin mở “Hiệu quả platform” (`/ui/platform/effectiveness`): mặc định Toàn
tổ chức, drill-down Application → Workload, không tab Dữ liệu; chi tiết chỉ số
có cách tính, độ phủ và bản ghi dẫn tới lịch sử triển khai; CSV; baseline
“Mốc so sánh” có phiên bản (append-only, PostgreSQL `effectiveness_baselines`
và JSON snapshot). Chỉ số platform tự đo từ Deployment và cleanup
`USER_CONFIRMED`: tần suất triển khai production, ứng dụng/người dùng hoạt
động, công việc hoàn thành (gộp retry), lượt chạy, thời gian thực hiện và chênh
lệch với baseline. Lead time, CFR, recovery, rework, yêu cầu → hoàn thành, tự
phục vụ, công sức, onboarding, giờ công là `NOT_COLLECTED`
([issue comment](https://github.com/pr3s3nt/poc/issues/12#issuecomment-6361990505)).

Limitations: cleanup history chỉ có trên PostgreSQL (memory/JSON báo nguồn
không khả dụng); báo cáo đọc toàn bộ lịch sử Organization mỗi request (chưa
tối ưu cho khối lượng lớn); bảng baseline mới đổi digest baseline T28 nên
database/snapshot cũ cần reset phát triển; chỉ desktop. Evidence:
[verification](verification/2026-10-23-uc17-platform-effectiveness.md).

## Git is the default Terraform source — 2026-10-21

New Resource Definition drafts, reset/cancel and successful registration start
with Git; registered modules require an explicit source switch. Edit preserves
the original verified pin. Seven unchanged modules are published in public
[pr3s3nt/terraform-modules](https://github.com/pr3s3nt/terraform-modules), tag
`v1.0.0`. See [source runbook](operations/terraform-module-sources.md).
Git inspection recognizes exact release content + Type to preserve EKS executor
outputs/target identity and namespace creation baseline. Original packaged pins,
state and cleanup snapshots remain retained. Arbitrary changed Git modules do
not gain those identities by name. Validation evidence is recorded in
[verification](verification/2026-10-21-git-module-default.md).

## Resource Definition module forms — issue #11, 2026-10-22

Form Resource Definition có vùng “Kết nối hạ tầng” (AWS tài khoản/vùng, nguồn
cluster Kubernetes) dùng cùng draft/serializer với provider binding; thông số
module hiển thị dạng thẻ nguồn với bộ chọn dùng chung (literal, mặc định/param,
ngữ cảnh, tài nguyên khác); preset theo tên biến đã bỏ. Tài nguyên đi kèm là thẻ
Loại/class/phạm vi + params theo schema Type + hai cờ có tên cụ thể; descriptor
và params thô nằm ở “Cấu hình kỹ thuật”. Chỉ desktop, không đổi backend/DTO.
Binding cluster luôn dùng phạm vi “cùng ứng dụng và môi trường” (contract hiện
hành), ghi vào dữ liệu nhưng không có ô chọn; danh sách context chỉ `env.namespace`, `infra.resourceName`. Các script
browser cũ (refactor-local, v02-*, v03-internal-frontend, definition-workload-local)
còn nhãn cũ; chưa chạy browser acceptance trên backend thật.
## Unified cluster Definition authoring — 2026-10-21

Resource Definition catalog/Create/Edit now includes existing-cluster alongside
Terraform. k8s-cluster selects its method from backend capabilities; existing
cluster uses Internal READY account, shared two tabs/criteria/payload review and
CAS/reload pipeline. Removed the separate cluster section/form. Infrastructure
Accounts, backend/API/matching and cluster lifecycle remain unchanged.
Focused frontend 152 tests, typecheck/build, changed-file lint, docs and mocked
HTTP browser desktop/mobile PASS; full lint retains 26 preexisting errors outside
this task. [Verification](verification/2026-10-21-unified-cluster-definition.md).

## Resource Definition v3 — issue #8, 2026-10-21

Nhánh `iam_role`: form Definition có hai tab Cấu hình / Điều kiện áp dụng,
Thiết lập nâng cao đóng mặc định giữ mapping/provider/provision và Form/JSON
hiện hành. Rule editor hỗ trợ đúng năm fields, AND trong trường hợp/OR giữa
trường hợp; phân biệt wildcard, zero cases và điều kiện chưa nhập. Copy/matching/
help thu gọn, tóm tắt phạm vi và full draft review; ẩn panel không unmount hay
vô hiệu guards. Criteria giữ nguyên literals, kể cả whitespace. Mẫu dựng ứng
dụng giữ projection cũ và được kiểm tra vì dùng chung editor/converters.

Không thay backend/API, source/module pins, secret/account/CAS hay execution
contract; không dùng v4/Usage/Humanitec DTO. Bản sửa visual tách catalog/detail,
card module + tài khoản/thông số, overview desktop, mapping theo hàng và Save
bar cố định; quay lại catalog có Tiếp tục bản nháp giữ CAS/draft.
Lượt đối chiếu URL v3 người dùng bổ sung: một thanh Form/JSON nâng cao chuyển
đồng thời mapping/provision, giữ JSON khi converter không biểu diễn được; ô
xem mặc định metadata chỉ đọc, ghi đè vẫn opt-in; scope summary theo trường hợp.
Kiểm tra lại 127 tests component/editor chịu ảnh hưởng và browser HTTP/Git PASS;
user visual review còn chờ phản hồi.
Frontend 215 tests component/editor và 6 tests điều hướng chịu ảnh hưởng,
typecheck, changed-file lint/build, docs và browser HTTP/Git create/edit/CAS/
reload/mapping/provision/copy/matching/keyboard/mobile PASS. Catalog browser dùng
in-memory và HTTPS Git fixture thật; không chứng minh database/deploy/cloud.
Không fullsuite hoặc acceptance ngoài scope. Full-directory lint còn lỗi có
sẵn ở prototype, script IAM và user-owned demo; không sửa các file đó.
[Functional verification](verification/2026-10-21-resource-definition-v3.md);
[visual correction](verification/2026-10-21-resource-definition-v3-visual.md).

## Direct Git module authoring — issue #7, 2026-10-20

Nhánh `iam_role`: PE/Admin thêm module qua HTTPS URL/thư mục/tag hoặc commit
ngay trong form Definition, không cần allowlist từng repository. Inspect resolve
commit, tự tính digest, kiểm/cache/persist verified bundle metadata; không tạo
Definition hoặc chạy Terraform/provider. Typed variables/mappings/account/criteria
và CAS hiện hành được reuse. Type chưa có module chọn được để thêm nguồn đầu tiên.
Digest/full pin chuyển vào Chi tiết kỹ thuật đóng mặc định; selector dùng tên và
revision rút gọn. Đổi source/Type/mode làm stale inspect; late replies không ghi đè.

Focused registry/catalog/HTTP tests + race, module-domain tests, bootstrap wiring,
backend build/vet, frontend107distinct tests (chỉ các file chịu ảnh hưởng), typecheck,
changed-file lint/build và browser Git main flow PASS. Không fullsuite hoặc
AWS/Kubernetes/deploy rerun. HTTPS Git fixture và product HTTP/catalog chạy thật;
browser catalog in-memory, không claim SQL persistence hoặc Terraform apply.
Private Git auth/SSH, provider mới và resource upgrade/destroy ngoài phạm vi.
Cần durable catalog + module cache để reuse qua process restart. Full repository
lint còn lỗi có sẵn ở `ec2-role-real.mjs` và user-owned untracked demo; task files
lint sạch. [Verification](verification/2026-10-20-git-module-authoring.md).

## V04–V06 hoàn tất — 2026-10-20

Đã hoàn tất Definition/matching/references → dependency graph → tài khoản/vùng
và đích theo từng resource/workload theo ADR-017. Không còn executionProfile,
Environment deployment selector/transition hoặc Organization default-choice
metadata trong domain/schema/API/UI/seed. Adapter/ports chuyển đích cũ không có
caller đã gỡ; giữ nguyên Secret Store, operation/fencing, generation và UID cleanup.
Mô hình placement theo workload/resource vẫn có giới hạn một cluster đã resolve
trong Candidate Set; không thêm multi-cluster execution hoặc issue5.

| Gate | Bằng chứng thực tế | Git |
|---|---|---|
| V04 | R18 `v04-1019082805-fb41fd`:54/54main assertions, native video đã kiểm, SQL/Vault/Kubernetes cleanup độc lập | `5413fa1` pushed |
| V05 | R14 `v05-1019191424-6833`:46/46, EKS tạo trong chính graph, readiness/job/cache/update/restart/reuse, SQL5/API4 và native cleanup | `c6dad76` pushed; AWS profile path gỡ sau replacement PASS |
| V06 replacement | A02 `a02-1020003113-c21a` rồi cleanup sạch, A03 `a03-1020024412-35bc`: mỗi run47/47, exit0/CLOSED_CLEAN | Thực hiện trước S5 removal |
| V06 final | `a03f-1020041903-32d7` trên source51/S5 sau removal:47/47,9marks,7SUCCEEDED deployments, SQL5/API4 reopen, secret scan, durable creationUID/atomic cleanup và native/SQL/process/private absence độc lập05:43UTC | Commit chứa báo cáo này hoàn tất V06; SHA push cuối trong issue4 |

Toàn bộ nghiệm thu dùng `backend/examples/acceptance-app/` thật: frontend proxy
đến backend, worker claim/DONE/result trên PostgreSQL; Redis worker write/hit,
TTL hết tự nhiên và DB refill; reload, valid update, restart, job mới, reuse và
reverse cleanup. A02 dùng Aurora/ElastiCache; A03 dùng Aurora/Redis trong EKS.
AWS thật chỉ452025861381/us-east-1. Mixed account/region và managed-only graph
là contract tests; managed-only không tự thêm EKS. Public no-workload execution
entry chưa được định nghĩa và ngoài scope issue4; không có UI/API mới cho nó.

Checks dùng PostgreSQL18.6 thật/REQUIRE_POSTGRES=1,0individualSKIP. V05 full
Go1715/race218; V06 reuse fullS5 Go1674/frontend722 ở phần không đổi và kiểm
composed delta: Go117+115/operator176/race48+148, frontend74+21; legacy closure
Go164/race76/build/vet và all-test compilation. Không cộng các lượt chồng lặp
thành một fullsuite mới. Frontend typecheck/lint/build, docs352/304, shell/diff
PASS. PlantUML5 nguồn đổi render PASS,34 nguồn giống snapshot đã kiểm được reuse.
Mapping chi tiết và các FAIL lịch sử nằm trong handoff; không suite để poll.

Mỗi run đã cleanup bằng original state/module/target và native API; fixture
PG62005/Vault66681 đã dừng, ports đóng, không còn DB/role/session phụ trước stop,
private fixture/credential copies đã xóa. Giữ metadata/evidence và billing subset
hạn chế quyền đọc.10namespace UID cùng reviewerSA/Secret của k8s-4f giữ nguyên;
host context `kind-idp-internal`, defaultVPC và11SLRs, usertmux `v456`/`tempt`,
Docker metadata và checkout người dùng `512d0d3` được giữ. Không còn owned cloud
resources/process/supervisor/recovery cần giữ cho task.

Giới hạn bằng chứng: optional ACCEPTANCE_SECRET trên EKS là NOT_CONFIGURED,
không tính PASS; V04 đã kiểm binding Vault thật và EKS vẫn có Secret refs thật
cho DB/cache. Intentional negatives không còn thuộc runtime gate theo quyết định
19-10; tests hiện có vẫn giữ/pass. Không video V05/V06 theo quyết định mới nhất;
native video V04 giữ lịch sử. A02R2 technical-read FAIL chưa đủ dữ liệu xác định
nguyên nhân; suy đoán cũ về expiry đã bị timestamp loại trừ, không đổi thành PASS.
Các run đã hoàn tất sau đó là bằng chứng độc lập, có đủ assertions/cleanup.

Cost Explorer read-only05:35UTC chỉ trả tagged subset với Estimated=true; không
đủ quy thuộc mọi EC2/EBS/chi phí và độ trễ billing. **Chi phí thực tế chưa xác định**,
không ghi0. Dự toán riêng5h: V05≈1.289USD, A02≈1.994USD, mỗi A03≈1.915USD;
không phải số thực chi hay trần chi phí mới.

Agent trực tiếp code/review/check trên luồng chính; không coding agent phụ.
[Handoff/coverage/reuse](verification/2026-10-18-v040506-handoff.md) và Current
handoff issue4 chứa head/session/quyền/evidence cuối. Dừng ở V06.

Các checkpoint roadmap bên dưới là lịch sử theo ngày, không là task đang chạy.

## V03 — Definition-selected Internal workload, 2026-10-18

Đã implement và nghiệm thu Generic Workload qua Definition/matching/references
trên cluster nội bộ có sẵn theo [ADR-016](architecture/decisions/ADR-016-definition-selected-workload-targets.md).
Workload Definition được chọn trước khi hoàn tất graph; namespace/companion/
workload dùng typed references và kế thừa cluster/account đúng. Pin đích theo
workload/resource; V03 từ chối nhiều cluster trong một Candidate Set trước
side effects, không cố định domain vào một target của Application/Environment.
Score metadata.class, provision flags/full DAG order, target agreement, readiness,
update/restart/cleanup có tests. Custom workload Type inputs/outputs ngoài
contract renderer bị từ chối; lifecycle metadata đi qua status hiện có.

R6 replacement gate PASS trước khi áp dụng V03.10. Sau đó bỏ Connection selector,
loại bỏ internal builtin/admission/deployment fallback và từ chối Internal
Connection transition/write. AWS legacy còn thuộc mốc sau; conformance reference
mode chỉ phục vụ read-only fixtures. R7 sau removal PASS35/35 assertions:
UI onboarding/Score/Preview/graph, namespace chính+companion, frontend thật,
update image/config, restart và thao tác tiếp, negatives, product cleanup/reopen.
Video headed14phút59.933giây có phụ đề Việt, đã review/decode/secret check và
[attach trực tiếp issue4](https://github.com/pr3s3nt/poc/issues/4#issuecomment-6245557915).

Final Go1580tests/59packages, race526tests/10packages, frontend700tests/45files,
build/vet/typecheck/lint/build/docs/PlantUML PASS, không individual fail/skip.
Các lượt lỗi runner/config r1–r5 được giữ là FAIL, đã sửa và cleanup từng lượt;
không giảm assertions. Hai chỉnh sửa cuối chỉ là comments; behavioral results
được tái sử dụng vì executable logic/config/environment không đổi.

[Coverage/handoff](verification/2026-10-18-v03-handoff.md) ghi rõ frontend-only:
backend/proxy/jobs/worker/PostgreSQL/Redis business tests **DEFERRED V04**, không
được tính PASS. V04 phải đọc lại coverage và bổ sung tests; V05 lặp trên EKS,
V06 trên managed resources. Hai image dùng cùng binary acceptance frontend
thật với metadata khác nhau; config update được kiểm trên trang. Platform
score-k8s patches hiện tại đủ; PE-authored patches deferred
[issue5](https://github.com/pr3s3nt/poc/issues/5).

Task runtime đã cleanup và kiểm độc lập: namespace/SQL/Vault/run processes,
auth fixture đúng UID và task image references; task PG/Vault dừng, private
material xóa. Hai cluster có sẵn Ready, namespace UID ban đầu được giữ; không có lệnh task
đóng tmux có sẵn. Inventory cuối và giới hạn quan sát session ghi trong handoff. Không AWS, không xóa cluster/reset retained data/Docker metadata.
Dừng tại V03; các mục dưới là checkpoints lịch sử, không ghi đè contract V03.

## V02 — Definition authoring và matching, 2026-10-17

Follow-up provision đã khép ngày2026-10-18: UI tạo/sửa descriptor/params/hai
flags, form↔JSON, reload/restart và real Score Preview graph/batches PASS trên
SQL/Vault thật. Bốn tổ hợp flags, mọi consumer của parent, provider-first order
và cycle rejection/recovery có assertions. Không deploy tài nguyên. Người dùng
miễn video bổ sung; original V02 rerun bị hủy theo yêu cầu, không claim pass.
Go1536tests, race190tests/2packages, frontend690tests/full gates, build/vet/docs
PASS, không individual skip. Task services/private material đã cleanup.

Đã triển khai tạo/sửa Definition với Name, version CAS, Driver Inputs, Driver
Account và region theo từng resource. ID/Resource Type bất biến; bản sửa chỉ
áp dụng cho Preview/Deploy sau, không sửa snapshot/resource đang chạy. Matcher
chung năm field, AND/OR và trọng số1/2/4/8/16 phân biệt wildcard, criteria rỗng,
hòa điểm và không khớp; không lọc executionProfile. Preview phía server hoạt động
với Environment chưa cấu hình và vô hiệu khi metadata/input đổi.

Các consumer planner/renderer/executor dùng cùng contract; account/version và
region được pin trước provision. Cleanup thiếu bằng chứng đích trả unknown,
không đoán account Environment. Kubernetes resource_ref kế thừa cluster; legacy
system/Environment binding còn giữ đến V03. Chưa triển khai Generic Workload
vào cluster có sẵn. Baseline SQL/JSON mới không migration dữ liệu cũ.

[Handoff/checks/video](verification/2026-10-17-v02-handoff.md): Go1528tests,
race530tests/8packages và closure99tests/2packages, frontend687tests, build/vet,
docs/PlantUML PASS, không bỏ test. UI headed8phút09giây/21phase dùng PostgreSQL
SCRAM và Vault file-backed thật, AWS verifier mô phỏng. T16 độc lập PASS.
Docker/kind vẫn không truy cập được; chưa reset retained instance hoặc chạy live
cloud/cluster. Các runner lịch sử lỗi sẵn được ghi rõ trong handoff.

## V01 — danh mục tài khoản hạ tầng, 2026-10-17

Đã implement catalog Organization cho PE/Admin: AWS access key và Internal
kubeconfig, UI/API tạo/xem/sửa tên, kiểm tra chỉ đọc và thay credential cùng đích.
Backing record vẫn là Connection; metadata có version/CAS, credential Vault bất
biến, giữ object đã commit. Commit chưa rõ kết quả trả lỗi an toàn, giữ object có
thể đã commit và yêu cầu GET/xem kết quả trước thao tác tiếp. Key AWS thay thế
được kiểm cả vùng legacy còn dùng. Developer không đọc catalog/credential.

[ADR-015](architecture/decisions/ADR-015-infrastructure-driver-accounts.md),
[UC-04 AC-01..12](usecase/UC-04/specification.md),
[checks/video/bàn giao](verification/2026-10-17-v01-handoff.md). Frontend665tests,
race PostgreSQL265tests và recording độc lập PASS; Go1510tests/58packages,
build/vet PASS, không bỏ test.
Video được attach trực tiếp issue#4; SQL và Vault file-backed thật, AWS/Kubernetes
verifier mô phỏng có nhãn. Không có bằng chứng live AWS/kind cho V01.

Tại mốc V01, consumers/liên kết Environment cũ được giữ đến V02–V03.
Đường Definition đã được thay thế tại V02 mô tả ở trên; legacy cluster binding
còn giữ đến V03.
V02–V03 không cần migration dữ liệu cũ theo quyết định người dùng. Baseline SQL/
JSON không tương thích phải fail rõ; không tự chuyển đổi. Người dùng cho reset
dữ liệu phát triển cũ, nhưng retained Docker/kind hiện không truy cập được nên
chưa reset/deploy retained instance. Không sửa metadata Docker hoặc xóa tài
nguyên ngoài task.

## M04 — hoàn tất ngày 2026-10-16

I04-01..12 đã hoàn tất trong phạm vi Terraform v2; IMP-021 đóng. Năm kịch
bản A01/E01/E02/A02/A03 đều PASS/exit0/CLOSED_CLEAN, có video, SQL sau restart
và cleanup độc lập. [Biên bản bàn giao](verification/2026-10-16-m04-completion.md).

| Kịch bản | Lượt đạt | Kiểm tra / mốc |
|---|---|---|
| A01 | a01-20261015-r4 | 42 / 8 |
| E01 | e01-20261015-r2 | 12 / 4 |
| E02 | e02-20261015-r3 | 15 / 5 |
| A02 | a02-20261016-r2 | 35 / 8 |
| A03 | a03-20261016-r2 | 35 / 8 |

A02 source `b083d97`, A03 source `a121642`: ba deployment/five Terraform
modules, job/Aurora/cache/TTL/refill/reload đạt. A03 PVC1Gi/gp2→PV→EBS được
đối chiếu native NotFound. Sáu lớp AWS verifier đạt,28UID k8s-4f/11SLR giữ
đúng baseline. Các database/role/Vault/private/run image alias đã dọn; fixture
PostgreSQL container/volume/admin credential cuối cùng cũng vắng. Video ở
`/home/thanhnt1/.m4/<run-id>/`, lỗi cũ và recovery evidence giữ nguyên.

R15 `1f3d3cb`:1482Go/race/0fail/0skip,649frontend/41file, build/vet/type/lint/
build/docs/PlantUML/diff đạt; hash trước/sau khớp, fixture riêng vắng độc lập.
Writer viết code/test; Codex review, kiểm độc lập và chạy thật. Dự toán mỗi
lượt A02~2,13USD/A03~2,05USD; Cost Explorer chưa phân bổ theo run tag, không
coi rỗng là0. Giữ nguyên user-owned node_modules tại runtime checkout.

Không còn iteration M04 đang thực thi. D12–D14, M02/I06-10 và TLS issue#2 vẫn
hoãn, không tự mở roadmap khác. Các checkpoint bên dưới là lịch sử triển khai;
không thay trạng thái hoàn tất hiện hành ở trên.

## M04 documentation review checkpoint — 2026-10-13

Người dùng bổ sung PE/Admin Xóa tài nguyên qua UI, cùng selected set qua ô tick với root được chọn mặc định; AWS access-key/Vault onboarding được giữ.
Specification/contracts nay đã hợp nhất trực tiếp, không hai lớp baseline/V2.
[Reconciliation](implementation/v2-rule-reconciliation.md) ghi ID mapping và
canonical destinations, nguồn a897d6b/3367f7e. Code tại checkpoint này chưa
đổi, IMP-021 vẫn mở.

[M04](iterations/M04-terraform-v2/README.md) current; M02/I06-10 deferred,
không complete. Người dùng đã duyệt chặng 1 sau sửa checkbox/coverage; I04-01 được
khởi động qua Claude/tmux. Chặng 2 là code/SQL thật và k8s-4f A01/E01/E02,
gate tiếp theo là video A01 + estimate AWS; chặng 3 chỉ sau duyệt mới chạy AWS.
Không có M04 source code/test mutation, live scenario hoặc cloud create trong
checkpoint tài liệu này. Các baseline status dưới đây vẫn là hiện trạng code.

## I04-01..05 hoàn tất, I04-06 hiện hành — 2026-10-14

Encrypted state/roles/fencing đã verified. Final corrections: invalid state key
trả 400, và REQUIRE_POSTGRES=1 chặn validation thiếu SQL. Claude/Codex độc lập
pass 835 tests, zero skipped, build/checks và cleanup. Xem
[completion evidence](verification/2026-10-13-i04-01-complete.md).
TLS PostgreSQL/state HTTP deferred theo [issue #2](https://github.com/pr3s3nt/poc/issues/2),
không bắt buộc TLS hoặc giới hạn thay thế. I04-02 registry và I04-03 executor
đã verified local; live k8s/AWS vẫn pending. Gate 2 video A01 + AWS estimate chưa đạt.

## I04-02 hoàn tất, chuyển executor I04-03

Registry/domain/persistence/inspector và catalog/planner wiring đã được Codex
review, sửa thêm mutation isolation và xác minh độc lập: 884 PASS, 0 FAIL/SKIP
trên PostgreSQL thật; build/vet đạt. Hạn mức Claude đã reset và không còn chặn.
Xem [evidence registry](verification/2026-10-13-i04-02-module-registry.md).
Cleanup test database đã xác minh. Form Definition vẫn gửi source shape cũ;
I04-07 phải sửa trước khi kiểm chứng registration qua Console.
I04-03 đã kiểm durable executor, provider target/graph edges, transitive execution
và runtime output checks. Không cloud/cluster mutation hoặc live proof ở I04-02.

## I04-03 hoàn tất, chuyển module Kubernetes I04-04

Executor v2 đã qua review và validation độc lập: **970 PASS, 0 FAIL, 0 SKIP**
với PostgreSQL thật, HTTP state backend thật và Terraform CLI thật; build/vet
đạt. Database và credential file của lượt xác minh đã dọn sạch. Xem
[evidence executor](verification/2026-10-13-i04-03-terraform-executor.md).

Executor dùng stable identity, scoped provider bindings, private verified
artifacts, durable state, bounded retry và hold khi outcome chưa rõ. Destroy
kiểm original module/target và bản state trước xóa, kể cả khi retry sau state
rỗng hoặc restart. Namespace kiểm UID/consumer/ownership trước cascade delete.
Provider/readiness ở các fixture local có seam fake; chưa có live k8s/AWS proof.

## I04-04 hoàn tất, chuyển module AWS I04-05

Packaged namespace/Redis/PostgreSQL modules đã qua review và validation độc
lập: **1021 PASS, 0 FAIL, 0 SKIP**, build/vet đạt trên PostgreSQL riêng; cleanup
đã xác minh. Modules được pin image/provider/digest, stable identity/password,
state-owned PVC và readiness sau acknowledged apply. Binding secret vào input
Terraform không bí mật bị chặn tại catalog/planner/provisioning, kể cả nested
và interpolated refs; nonsecret host fixture vẫn hoạt động. Xem
[evidence modules](verification/2026-10-13-i04-04-kubernetes-modules.md).

Chưa apply modules trên cluster; storage class và pod/PVC behavior còn cần live
verification. I04-05 chỉ chuẩn bị/validate AWS modules, không tạo hạ tầng AWS.
AWS Connection UI, class/Definition form, selected-set Delete UI/API, Redis app
và live scenarios thuộc các task sau. Gate 2 video A01 + dự toán AWS chưa đạt.

## I04-05 hoàn tất, chuyển AWS Connection I04-06

AWS modules VPC/EKS/Aurora/ElastiCache đã qua review và validation độc lập:
**1042 PASS, 0 FAIL, 0 SKIP**, build/vet đạt với PostgreSQL riêng và Terraform
CLI thật; cleanup đã xác minh. Release `v3-2026.10.13` pin AWS provider 6.60.0
và EKS addon builds. API EKS nhận danh sách IP `/32` do PE/Admin cấu hình,
không mặc định mở mọi IP. VPC sở hữu subnet private dùng chung; nhiều Aurora/
ElastiCache nhận IDs qua Resource References và không tạo subnet riêng trùng CIDR.
Xem [evidence AWS modules](verification/2026-10-14-i04-05-aws-modules.md).

Module plan-policy tests dùng mock provider; chưa có AWS apply/live proof.
I04-06 tiếp tục AWS Connection qua Console/Vault. Definition/class UI còn I04-07;
egress preflight wiring còn I04-10; node capacity, mạng, storage/addons, TLS/auth
và real cleanup còn live verification. Gate 2 chưa đạt, chưa được tạo AWS.

## I04-06 hoàn tất, chuyển Definition/class UI I04-07

Concealed AWS form, strict role/org/payload gates, read-only verifier, immutable
Vault credentials và allowed-region target/binder đã qua review/checks/video
local. Không host fallback, overwrite winner hoặc đổi Organization default.
IAM simulation cần `iam:SimulatePrincipalPolicy`, global actions dùng us-east-1,
regional actions theo allowed regions; simulation không thay execution preflight.
Initial flow hỗ trợ long-term IAM user key, không root/temporary credentials.
Vùng đầu tiên là default; UI Environment vẫn theo flow hiện hành, không thêm
region picker trong I04-06. Source/module/class forms và runtime capability
preflight còn I04-07/10; live A01/E01/E02 và A02/A03 còn I04-11/12.

## Terraform v2 — accepted design, implementation pending (2026-10-13)

User completed Step 1 decisions: [ADR-014](architecture/decisions/ADR-014-terraform-v2-lifecycle.md),
registered immutable Terraform modules for AWS/Kubernetes, required score-k8s on
both targets, same Environment AWS account/region, durable encrypted PostgreSQL
HTTP state, locking and ownership-based lifecycle. Specifications/contracts/UI/
schema/diagrams now describe required v2; baseline delivery rows below remain
actual implementation status, not v2 completion.

D01/D03 are selected for v2 with implementation pending. Full Terraform resume,
orphan reconciliation and platform backup/restore are deliberately deferred
D12–D14. Minimal interrupted-state/owner hold and independent verified acceptance
cleanup remain required. No v1 import/migration, retained reset or old resource
adoption. [IMP-021](implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap)
tracks remaining implementation and live verification gaps.

Acceptance will use existing frontend/backend/worker job app plus Redis:
Web Console forms/Preview/Deploy, then browser job completion/reload and actual
PostgreSQL/Redis cache hit/miss/refill. Three live happy paths + two exceptions;
[execution scope](operations/terraform-v2-acceptance.md) records AWS target,
confirmed k8s-4f test namespace boundary, small sizing, estimates, one scenario
retry and immediate cleanup. STS authentication was checked; no v2 provision,
permission completeness, Redis/browser acceptance or cleanup run is claimed.

## Lifecycle position

Phase 1–5 của Unified Process cho UC-01..UC-09 đã hoàn thành và design gate đã PASS.
UC-00 được bổ sung sau gate và đã có local/test implementation. Executable
architecture cho UC-06/UC-08 cùng query path tối thiểu của UC-09 đã được hiện
thực. Internal happy path đã được kiểm chứng trên kind; cloud happy path đã được
kiểm chứng trên AWS với VPC, EKS và Aurora rồi cleanup.

[I06-04 — Complete UC-09 observability](iterations/M02-usecase-completion/I06-04-uc09-observability/README.md)
thuộc [M02 — Use-case completion](iterations/M02-usecase-completion/README.md)
đã hoàn thành local verification ngày 2026-09-30. Hạng mục tiếp theo trong M02
là I06-08 (UC-05), nay cũng đã hoàn thành
[local/API/UI/PostgreSQL verification](verification/2026-09-30-uc05-preview-local.md).
I06-09 (UC-07 update/remove) cũng đã hoàn thành
[local API/UI/PostgreSQL verification](verification/2026-09-30-uc07-update-remove-local.md).
I06-10 đã hoàn thành [registration hardening safe slice](verification/2026-09-30-registration-hardening-local.md): insert-only duplicate guards,
safe errors, truthful UI reload states và no-restart catalog usage. I06-10/M02
chưa đóng; identifier/Driver Inputs policy đã được chốt và hoàn thành local code/API/UI
validation ngày 2026-10-02. AWS access key + Vault storage đã được chọn
theo ADR-009; phần AWS onboarding chưa triển khai.
[M00-a — Developer onboarding](iterations/M00-developer-onboarding/README.md) đã
hoàn thành 2026-09-30 với React UI, authenticated API, session, normalized
PostgreSQL persistence và local browser restart verification cho UC-00/UC-01.
[M01 — Contract hardening](iterations/M01-contract-hardening/README.md) đã hoàn
thành: I06-05 đóng IMP-010, I06-06 đóng IMP-008, I06-07 đóng IMP-009.

## Use-case delivery state

| UC | State | Current conclusion |
|---|---|---|
| UC-00 | Implemented; locally E2E verified | Fixed local/test accounts, opaque HttpOnly cookie session, restore/expiry/revocation và sign-out đã có; production profile không seed hoặc chấp nhận fixed credential từ state mới hay legacy. |
| UC-01 | Implemented; locally E2E verified | Authenticated Developer list/create/get Application qua API/UI; service tự sinh ID, staging/production, empty Deployment Sets và namespace identities. Create không deploy; browser restart test xác nhận durable local state. |
| UC-02 | Registration implemented | Platform Engineer/Admin có thể đăng ký và xem Resource Types trong Organization qua API/UI; seeded catalog và normalized PostgreSQL persistence dùng được. Production-grade RBAC chưa có. |
| UC-03 | Registration implemented | Platform Engineer/Admin có thể đăng ký Definition runtime-supported với criteria, driver inputs và provision rules qua API/UI; planner vẫn có profile guard và matching, catalog được persist trong PostgreSQL. Remote source và production-grade RBAC chưa có. |
| UC-04 | Kubernetes kubeconfig onboarding implemented and verified | Platform Engineer/Admin nhập tên, upload/dán kubeconfig, inspect/chọn context rồi verify API/RBAC read-only và lưu `READY`. Vault KV v2 lưu scoped credential; metadata/name/authentication type ở PostgreSQL, migration/race/reopen đã pass isolated local. Executor resolve credential bằng opaque target identity; T22 bỏ host-context/API body cũ, seed thiếu credential ở VERIFYING. Live kind upload và video đã pass ngày 2026-10-06; AWS access-key onboarding đã verified local qua Console/Vault ở I04-06; STS/region/IAM policy simulation và scoped resolver đã có, AWS transport browser local fake; live AWS Connection/deployment đã đạt A02/A03 ở I04-12. |
| UC-05 | Implemented; locally verified | Standalone Score Preview service, authenticated scoped API và Console trả Delta/Candidate/graph/matches/batches/classification, consistent snapshot chung với direct Deploy, safe public projection và no-mutation tests. Pending-change Preview vẫn riêng; UI-only human-paced recording đã pass. Không chạy Terraform plan thật. |
| UC-06 | Executable baseline; partially conformant | HTTP → plan → UC-08 → target workload apply đã pass kind/AWS; optional Fleet GitRepo adapter cho internal kind đã pass Harbor-image deployment. Environment Ingress nhiều path và Fleet `_routes` bundle đã pass kind test với hai workload BusyBox, Fleet revision/prune và cleanup; route failure có route-only retry (unit test), Backstage cụ thể chưa kiểm chứng. DNS/TLS/chuyển controller ra ngoài chưa có. Mỗi Deployment persist immutable `DeploymentDeltaSnapshot`; UC-12/16 pending changes deploy theo Preview token với per-workload result. |
| UC-07 | Implemented; locally verified | UC-16 scoped Preview → Deploy update/remove, before/shared invariants, final Service validation, confirmation/Undo và busy/stale/reload UI states. READY resource marking UNREFERENCED commit cùng current set/SUCCEEDED; PostgreSQL rollback/reopen và UI-only recording pass. Không destroy/rollback; Application-wide cleanup ngoài scope. Fleet remove kind evidence là lịch sử, chưa rerun live cho mốc này. |
| UC-08 | Implemented and E2E verified | Kubernetes và Terraform resource execution, output propagation và persistence baseline đã pass. |
| UC-09 | Implemented; locally E2E verified | Authenticated Organization/Application/Environment-scoped history/filter và detail; consistent read snapshots, per-Deployment workload history, fail-closed output redaction và không trả resolved inputs. React loading/error/not-found/retry và browser redeploy/restart checks đã pass. Comparison và live status ngoài scope. |
| UC-12 | MVP path implemented on kind | Settings UI/API, immutable desired/applied revisions, Vault KV v2 adapter, scoped backend/workload policies và VSO → namespace Secret → Pod `secretKeyRef` đã pass kind; secret không xuất hiện trong read API, snapshot hoặc Pod spec. Production secret lifecycle/HA chưa có. |
| UC-16 | MVP path implemented | Form/Score import, typed resource params, public path + Service port rows, draft save/delete/undo, references, edit deployed workload bằng reconstructed Score, Preview → Deploy và partial retry đã có. Preview bỏ qua no-op draft nhưng giữ UC-12 revision update. Kind đã kiểm tra multi-path BusyBox `/` + `/api`, Fleet route và no-op Pod UID; Backstage cụ thể và broader update/cloud path chưa kiểm chứng. |

## Refactor T16 — Environment AWS account (2026-10-11)

[Local verification](verification/2026-10-11-T16-environment-aws-account.md): public
Terraform VPC/EKS chỉ nhận ConnectionKey rỗng; form không có selector và execution
inherit READY AWS Connection của Environment. Aurora giữ account riêng bắt buộc.
Effective target/hash/executor/ActiveResource nhất quán; malformed winner và
cross-account state reuse fail closed. New seed bỏ VPC/EKS key, không migrate
catalog/profile cũ hoặc đổi matcher/ties/reference fixtures. Go/conformance/build,
378 frontend tests và headed fake T16/T02 pass; video decoded/frame reviewed.
SQL opt-in/cloud/cluster checks không chạy; không có external mutation/live proof.

## Console refactor T15 (2026-10-11)

[Local verification](verification/2026-10-11-T15-deployment-details.md): chi tiết
triển khai dùng tiếng Việt cho metadata, trạng thái, graph/batches và UI states;
giữ snapshot/IDs/API và thời gian vi-VN. Failure dùng hướng dẫn an toàn, output
bí mật hiện “Đã che”, nested outputs và resolved inputs không render. Scope/ID/
lượt đọc cũ không ghi đè kết quả mới. 375 frontend tests và headed fake T15 pass,
video decoded/reviewed; graph/redaction/failure/status/delay dùng projection mocks.
Không external mutation/live proof; mapping lỗi chi tiết thuộc T20.

## Console refactor T13 (2026-10-11)

[Local verification](verification/2026-10-11-T13-score-preview.md): standalone
Xem trước Score dùng tiếng Việt cho form/kết quả/lỗi/retry và accessible labels;
giữ parser semantics, IDs, planning data/payload và `staging`/`production`.
Không lưu draft/Deployment hoặc provision; stale/input/scope guards giữ nguyên.
344 frontend tests và headed fake T13 pass, video được decode/frame review.
413/404/500/401/delay dùng projection mocks; chi tiết error code thuộc T20.
Không external mutation hoặc live adapter proof.

## Console refactor T12 (2026-10-11)

[Local verification](verification/2026-10-11-T12-application-home.md): trang
ứng dụng, Workload pending actions/results, triển khai gần đây và tóm tắt đích
dùng tiếng Việt; giữ IDs/payload và staging/production. Preview chỉ đọc, Deploy
tường minh, confirmation/Undo và stale/scope guards giữ nguyên. Raw lỗi dùng
fallback Việt an toàn; mapping đầy đủ thuộc T20. 336 frontend tests và headed
fake T12/T09 regression pass. Partial/stale/delay là projection mocks ghi riêng;
BUSY kiểm bằng unit, không chứng minh live deployment. Không external mutation.

## Console refactor T11 (2026-10-11)

[Local verification](verification/2026-10-11-T11-applications-list.md): danh sách
ứng dụng, cards và loading/empty/error/retry dùng nhãn tiếng Việt. Tên, endpoint,
thứ tự dữ liệu, create/card routes và deep links giữ nguyên; nhãn môi trường là
`staging`/`production`. 324 frontend tests và headed fake T11/T01 regression pass.
Empty/loading/500 là GET projection mock được ghi riêng; tạo/mở ứng dụng qua
backend fake local, không external mutation. Application home đã hoàn tất ở T12; code-based
errors còn thuộc T20.

## Console refactor T09 (2026-10-11)

[Local verification](verification/2026-10-11-T09-create-application.md): trang
Tạo ứng dụng dùng tiếng Việt, gợi ý subdomain từ tên đến khi người dùng sửa
thủ công; giữ cả lựa chọn xóa trống. Validation/trùng/lỗi chung giữ form và
retry tường minh. Payload name/subdomain, endpoints và navigation staging giữ
nguyên. 309 frontend tests và headed fake T09 pass; lỗi async 500 là mock,
không external mutation. Shared shell/list/home đã hoàn tất ở T10/T11/T12;
code-based errors còn ở T20.

## Console refactor T08 (2026-10-11)

[Local verification](verification/2026-10-11-T08-workload-editor.md): editor/key
picker Việt hóa; CPU/memory, resource class và target-port override trong phần
nâng cao với tóm tắt defaults hiệu lực. Collapse giữ imported/edited overrides;
Score không tự thêm capacity. Thêm route chỉ tự chọn đúng một cổng hợp lệ;
port rename/delete/invalid giữ đích cũ và chặn lưu, không retarget. Typed params,
key aliases, stale/BUSY/scope guards giữ nguyên. Headed T08 và T03 regression
pass với fake adapters; Secret catalog/stale/BUSY là projection mock. Backend
error mapping T20 và publicPort removal T25 đã hoàn tất theo các record riêng;
không live mutation.

## Console refactor T07 (2026-10-11)

[Local verification](verification/2026-10-11-T07-environment-settings.md):
Cài đặt môi trường và các dialogs cấu hình/chuyển đích/recovery dùng nhãn Việt.
Một READY choice hợp lệ chỉ được gợi ý draft khi chưa có binding; nhiều lựa chọn
phải chọn tường minh, không tự persist/default/fallback. Chỉ Lưu đổi binding;
binding đã có không bị thay. Reload/stale/reply muộn giữ pending edits và không
đưa cấu hình sai app/env vào trang đang xem. Secret không đọc lại; T07 giữ
rename-only, T30 bên dưới bổ sung lựa chọn explicit rewrite desired references. Backend/API/version/downtime/recovery confirmations giữ
nguyên. Headed local dùng fake/memory adapters; transition/recovery và lỗi
copy/stale/BUSY dùng projection mock, không chứng minh migration thật. T20 error
mapping và T30 rename-reference behavior được ghi ở các mục delivery tương ứng.

## Console refactor T06 (2026-10-11)

[Local verification](verification/2026-10-11-T06-connections.md): trang Kết nối
dùng tiếng Việt, upload tự inspect sau đọc file và paste debounce 500 ms.
Một context tự chọn; nhiều context không đoán theo current-context. Tên gợi ý
theo cluster/context chỉ khi chưa sửa; tên custom hoặc xóa trống được giữ.
Đổi document/source vô hiệu summary và file-read/inspect pending. Inspect không
lưu; kiểm tra/lưu vẫn tường minh. Kubeconfig paste mặc định che, lỗi dùng hướng
dẫn Việt an toàn; T06 giữ API/backend/HOST_CONTEXT tại thời điểm đó, T22 đã bỏ HOST_CONTEXT. Headed local pass;
parser/422 được kiểm qua backend local, success 201/list là mock projection,
không chứng minh cluster/credential persistence thật. T20/T22 vẫn deferred.

## Console refactor T05 (2026-10-11)

[Local verification](verification/2026-10-11-T05-secret-stores.md): trang Kho bí
mật dùng tiếng Việt, phần nâng cao giữ mount/CA với tóm tắt defaults hiệu lực.
Checkbox dùng cùng địa chỉ là opt-in, giữ draft workload riêng khi tắt. Local
validation chặn POST sai; server lỗi xóa token và giữ metadata. Danh sách tách
xác minh backend với Kubernetes auth; READY không cam kết deploy-ready. API và
Developer choices giữ nguyên; code-based errors còn chờ T20. Video headed local
fake pass; ABSENT/list failure là projection mock, không chứng minh Vault thật.

## Console refactor T04 (2026-10-11)

[Local verification](verification/2026-10-11-T04-resource-types.md):
Trang Loại tài nguyên dùng tiếng Việt, xem schema GET và sao chép schema sang
draft độc lập với ID mới, giữ types/required/secret. Duplicate không ghi đè
catalog; submit lỗi giữ form. Metadata authoritative T31 dùng chung với form
Cấu hình tài nguyên xác nhận cặp hỗ trợ; custom type như postgres-ha chỉ có
contract. CTA chọn sẵn type sau khi xác nhận catalog và metadata; workload dẫn
sang Mẫu dựng ứng dụng. T31 bổ sung API metadata chỉ đọc, không thêm executor;
code error mapping đầy đủ đã có ở T20.

## Console refactor T03 (2026-10-10)

[Local verification](verification/2026-10-10-T03-rendering-templates.md): trang
Mẫu dựng ứng dụng tại `/ui/platform/rendering-templates` dành PE/Admin đã có
list workload-only, ID/bundle ID và editor criteria chung. POST giữ BR-15/16,
không connection override/provision/source. Form/error/help dùng tiếng Việt;
T18 đã thay nhập bundle ID bằng selector từ registry; xem mục T18 bên dưới.

Headed local scenario kiểm native/selected renderer Preview và selected render
failure/no fallback qua Deploy. Infrastructure/delivery fake, score-k8s là stub
local cố ý generate lỗi; không claim output CLI thật hoặc live provisioning.
Runner live template đã đổi route/locator, chỉ kiểm syntax, chưa rerun live.

## Console refactor T02B (2026-10-10)

[Local verification](verification/2026-10-10-T02B-criteria-editor.md): editor
Điều kiện áp dụng dùng chung đã có bốn chế độ, dropdown app/env IDs thực,
copy độc lập và giữ criteria nâng cao/nhiều dòng khi chuyển mode. Xem trước
phạm vi và cảnh báo overlap vẫn là trợ giúp frontend; matching authoritative
đã được bổ sung ở T19 (xem mục bên dưới). Payload/matcher không đổi; headed fake evidence không chứng minh
live provisioning. T03 dùng lại editor cho Mẫu dựng ứng dụng.

## Console refactor T02 (2026-10-10)

[Local verification](verification/2026-10-10-T02-resource-form.md): frontend
checks và headed fake scenario pass; video/visibility và payload reviewed.

Form Cấu hình tài nguyên dùng tiếng Việt và chỉ cho đăng ký
Terraform/Kubernetes; driver/module tự chọn theo type, Terraform khóa `aws-eks`,
Kubernetes có Cluster nội bộ/Dùng chung. Connection lấy từ list org-scoped,
lọc READY đúng kind; Kubernetes mặc định dùng Connection của Môi trường,
override nằm ở nâng cao. Terraform vẫn cần AWS Connection tường minh tới T16.
JSON/criteria và lựa chọn theo driver được giữ khi đổi controls hoặc submit lỗi.

Workload Definitions không còn ở list/form resource; trang Mẫu dựng ứng dụng
đã được triển khai riêng ở T03. Backend rendering/API vẫn giữ nguyên. Runner live
`frontend/test/e2e/template-engine-kind-human.mjs` đã cập nhật route/locator tại
T03 nhưng chưa chạy lại live. Exact
matching-preview đã bổ sung ở T19; code-based error mapping vẫn thuộc T20.

## Definition-selected rendering (2026-10-06)

[ADR-010](architecture/decisions/ADR-010-score-k8s-workload-rendering.md) is accepted.
Registration → standalone/pending Preview → Deploy supports optional score-k8s
0.15.0 on internal-k8s through an installed immutable bundle. The native renderer
remains the default. Plans pin Definition/bundle/binary content, pending detects
renderer-only updates, and preflight runs before UC-08 side effects. Resources
are still provisioned by their existing executors; output-only CLI bindings carry
non-secret values and Secret references. Shared policy preserves resources,
probes, replicas, pull references, VSO refs and Agent injection.

[Local evidence](verification/2026-10-06-score-k8s-rendering-local.md) covers the
real CLI with fake infrastructure/delivery, HTTP flow and JSON-state reopen.
After a renderer upgrade the same Definition selects the installed bundle; old
previews are stale and pending proposes a renderer-only update (registration
fingerprint is audit-only).
No live kind, Fleet/VSO readiness, PostgreSQL database mutation or AWS run was
performed for this extension. The configured CLI is an external process
prerequisite; current deployment images do not automatically install it.

## Executable baseline

- Go HTTP API, application services, planner, resource executors, Kubernetes
  deployer, normalized PostgreSQL adapter và local/test JSON snapshot store nằm
  dưới `backend/`.
- Web Console React/TypeScript có sign-in, Applications list/create/home,
  UC-12 Settings, UC-16 editor, Application Preview/Deploy panel và UC-09
  recent deployments/history/detail với authenticated scoped read API.
- Planner product conformance chạy đủ 33 fixture: 27 accepted cases so sánh
  Humanitec-shaped Delta, Candidate Set/graph/matching/batches/classification/
  Terraform artifacts và kiểm `base + delta = candidate`; 6 rejected cases hiện
  mới xác nhận rejection status. Bundle không có case container resource
  requests/limits hoặc array diff; hai vùng này có product tests riêng.
- Internal verification chạy trên cluster kind có sẵn, namespace riêng theo run
  ID và cleanup đã xác minh; run gần nhất `kind-20260922114440-17489` (I06-07,
  [evidence](verification/2026-09-22-imp009-kind-rerun.md)) xác nhận live
  container resources của ba seeded workload: backend declared, worker partial,
  frontend omitted.
- AWS verification đã tạo VPC/EKS/Aurora tối thiểu, chạy acceptance job flow,
  không tạo public LoadBalancer và cleanup 17/17 truy vấn theo run ID.
- Một lượt kiểm thử riêng đã deploy Backstage 1.53.1 qua UC-12/16 trên kind:
  PostgreSQL resource, VSO configuration, Preview → Deploy, Traefik Ingress,
  health và guest-auth API đều pass; namespace/PVC run-scoped đã cleanup.
  Đây là evidence thử nghiệm, không phải một deployment Backstage thường trực.
- Orchestrator có container image cho API (kèm kubectl) và Web Console (nginx).
  Trên kind, host Orchestrator đã tự deploy hai image này qua Web Console; bản
  in-cluster sau đó deploy acceptance app và cả 4 check đều pass. Bản
  self-hosted này chỉ là demo: nhận admin kubeconfig qua UC-12 secret; logical
  state hiện được giữ trong PostgreSQL, nhưng credential delivery vẫn là
  limitation của IMP-014.

Chi tiết từng lần chạy nằm trong [verification index](verification/README.md);
evidence lịch sử không chứng minh checkout hiện tại vẫn pass.

## Current architectural baseline

- Go product module nằm tại root `backend/`; Orchestrator Web Console nằm tại
  root `frontend/` theo ADR-005.
- Planning deterministic và side-effect free.
- UC-08 chỉ execute resource nodes; UC-06 apply workload sau khi có outputs.
- Historical `fleet-gitrepo` mode (T22 hiện chặn trước Git/cluster mutation vì
  chưa có explicit credential transport) ghi non-secret workload manifests vào private
  GitOps repo; Fleet triển khai workload, còn UC-08 vẫn trực tiếp quản lý
  namespace/resource. Harbor chứa image, không có GitHub source build trong
  đường này. Image-pull credential nằm ở namespace Secret, không nằm trong Git.
- Internal-kind public access dùng một Traefik Ingress cho mỗi Environment,
  được reconcile sau khi workload Ready. Traefik Service hiện chỉ là ClusterIP;
  muốn truy cập từ máy ngoài cluster cần port-forward/edge mapping và DNS hoặc
  hosts mapping. TLS và AWS ingress chưa được triển khai.
- Resource graph dùng edge `consumer -> provider`; provider được schedule trước.
- Application `aws-eks` sở hữu VPC/EKS application-scoped; `internal-k8s` dùng
  registered cluster và Environment ánh xạ namespace.
- Candidate Deployment Set chỉ trở thành current sau workload readiness.
- Resource provisioning hiện chạy theo full-style behavior cho mọi resource
  node trong plan; workload execution chỉ apply workload mục tiêu của request.
- UC-12 secret value lưu trong Vault KV và, ở Kubernetes VSO mode, được đồng bộ
  vào namespace-local Kubernetes Secret. State snapshot, read API, GitOps
  manifests, Pod spec và UC-09 view không chứa giá trị. Pod nhận biến qua
  `secretKeyRef` mà không cần startup script. Agent file delivery vẫn có thể
  chọn như legacy mode. Production cần etcd encryption, least-privilege RBAC,
  chính sách dọn Secret/bundle cũ và Vault HA/backup.
- Web Console và acceptance application frontend là hai artifact độc lập.

## Known limitations and release gate

- Backend hỗ trợ normalized PostgreSQL system of record qua
  `-database-url-file`; repository ghi trực tiếp dedicated tables, dùng foreign
  keys/unique constraints, fresh baseline identity/validation và PostgreSQL transaction (T28).
  In-memory/JSON adapter vẫn là lựa chọn mặc định cho local/test. PostgreSQL 16
  riêng đã Ready trên kind trong `orchestrator-system` với PVC/Secret; automated
  backup scheduling/retention vẫn là release gate.
- Terraform state chưa có durable backend; physical cloud names còn chứa run ID.
- Runtime chỉ execute ba embedded Terraform modules; remote source execution
  chưa thuộc baseline.
- `DeploymentDeltaSnapshot` chỉ giữ `deployment_id`; PostgreSQL FK/UNIQUE cấm
  orphan hoặc nhiều Snapshot cho một Deployment, và deferred constraint trigger
  bắt buộc Snapshot trước khi Deployment commit ở `PROVISIONING`, `DEPLOYING`
  hoặc `SUCCEEDED`.
- Container resources chỉ được validate theo shape (`cpu`/`memory`, non-empty
  string); Kubernetes quantity semantics và cặp request > limit khai báo chỉ
  được Kubernetes API kiểm khi apply. Nhánh request thiếu lấy limit cùng field
  (BR-11) và request vượt limit mới được unit-test, không có seeded workload
  nào chạy trên kind.
- Deploy API chạy đồng bộ. UC-12/16 pending Preview → Deploy hỗ trợ nhiều
  workload tuần tự, per-workload result và retry phần chưa hoàn thành, nhưng
  chưa có standalone Humanitec Delta/Set deployment lifecycle.
- Public Resource Definition/Score boundary chưa tương thích Humanitec/Score:
  driver ID/account/secret refs, remote source mapping, context extensions,
  nested probe, replicas extension và namespace output được phân loại tại
  [compatibility matrix](implementation/humanitec-compatibility.md).
- Conformance loader bỏ Definition thiếu criteria hoặc `criteria: []` khỏi
  challenge catalog và giữ criterion `{}` thành wildcard điểm 0; product planner
  vẫn từ chối catalog có Definition không có criterion.
- UC-00 có cookie session cho local/test, chưa có production-grade RBAC. UC-04
  chưa có AWS registration/durable credential flow. UC-05 standalone Preview và
  UC-07 update/remove đã có local verification. UC-02/03 đã có đăng
  ký catalog và PostgreSQL persistence nhưng chưa có đầy đủ lifecycle quản trị.
- UC-12 fake provider chỉ dành local/test và mất giá trị khi process restart.
  Vault trên kind dùng file storage PVC, cần unseal thủ công sau restart; chưa
  có HA, backup, rotation/revocation tự động hoặc production RBAC. Workload
  phải tự source Agent file bằng startup script; không tự động sửa image/entrypoint.
- Fleet GitRepo mode mới kiểm chứng trên một internal kind cluster với image
  BusyBox mẫu từ Harbor; GitOps writer dùng local clone và host Git credential,
  chưa có distributed lock hay production-grade reconciliation. Harbor hiện
  dùng Service IP và HTTP node config riêng cho kind; phải cập nhật cấu hình
  nếu Service IP đổi. AWS vẫn dùng direct Kubernetes delivery.
- Rollback, failure recovery, RBAC, audit và secret lifecycle nằm ngoài MVP.
- AWS happy path gần nhất là run `aws-20260921052038`, trước một số thay đổi
  planning cuối và trước khi seeded Scores khai báo container resources. Phải
  chạy lại `aws-verify.sh` trước release.

Các vấn đề deferred được theo dõi tại [backlog](backlog/README.md); khác biệt
thiết kế–implementation nằm tại
[known deviations](implementation/deviations.md).

## Accepted decisions and verified local hardening (2026-10-02)

New public Resource Type/Definition ID policy and Driver Inputs schema checking
are implemented in UC-02/03. UC-16 validates all non-virtual dependencies on
Save/import before mutation. Backend full test/build and frontend
typecheck/lint/72 tests/build passed; code review required and verified fixes
for malformed nested placeholders and namespace name supply. See
[local evidence](verification/2026-10-02-catalog-workload-validation-local.md). IMP-002 remains open for the residual
AWS onboarding and arbitrary-Type/runtime-pair boundaries.
AWS access key + Vault storage has been selected in
[ADR-009](architecture/decisions/ADR-009-aws-access-key-storage.md); the decision
resolves credential selection, not AWS registration/executor delivery. AWS
onboarding and live kind/AWS verification remain separate work.

## UC-16 Application-key picker delivered (2026-10-02)

Per-container Variables/Secrets checklists with same-name defaults, optional
aliases and separate resource/Service sources are accepted under UC-16
BR-15–BR-18 and implemented in the Console. Same-scope stale reload retains
edits; real Application/Environment/workload changes reset local selections
and ignore late old replies. Frontend typecheck/lint/107 tests/build and local
HTTP browser checks passed. See [picker evidence](verification/2026-10-02-workload-key-picker-local.md).
Runtime API, configuration storage and Preview/Deploy behavior are unchanged.

## Human UI verification on kind (2026-10-02)

The current UC-16 Application-key picker was exercised in a browser-driven
acceptance deployment on kind with Vault/VSO: backend/frontend deployed,
reference selections survived Edit and all diagnostic checks passed. A separate
Platform Engineer recording covered Type/Definition registration and validation,
actual Kubernetes connection verification to READY, then Developer Preview
consumption. Both human-paced full-window MP4s were reviewed and published;
see [execution evidence](verification/2026-10-02-human-ui-kind-recordings.md).
This adds live UI verification, not production RBAC or AWS onboarding delivery.

## UC-04 upload delivery (2026-10-06)

[Approved specification](usecase/UC-04/specification.md), realization/shared
design, contracts, schema, UI and traceability are aligned with the Kubernetes
upload implementation. [Verification](verification/2026-10-06-uc04-kubeconfig-upload.md)
records code review, Go/frontend gates, disposable PostgreSQL tests and the
published kind read-only upload video. Direct executor resolution and
restart/remove are locally tested with stub kubectl; live workload deployment
using uploaded credential was not exercised by this registration recording.

The [desktop UI review](verification/2026-10-06-uc04-connections-ui.md) covers
corrected Upload/Paste controls, readable form/action placement, aligned list
columns and keyboard focus, with a revised published registration demo.

Connection credential store is explicit configuration: durable Vault for real
execution/persistent state, memory only for fake/no-persistent-state tests,
default none returns 503 for upload registration. New registration does not
change Organization default; UC-03 matching Definition can consume the new
Connection. T22 removes host-context execution: Kubernetes seed without credential
is VERIFYING, upload is the only registration body, and context-only targets fail
closed. Fleet fixed-cluster mode is currently unavailable before Git/cluster
mutation for all targets; adding explicit Fleet credential delivery is separate
scope. Direct uploaded KUBECONFIG and Terraform-generated EKS path targets remain
supported. Existing HOST_CONTEXT PostgreSQL rows require the approved dev reset,
not silent conversion. AWS
registration and Terraform account credential resolution remain deferred.

Local Compose builds backend/frontend source by default and runs public
PostgreSQL/Vault images. Explicit bootstrap now registers `platform-vault` /
`Platform Vault` as an ordinary verified store, using the shared
Vault verifier and private platform credential store. Runtime resolves its opaque
credential reference; the application token file is bootstrap input only.
T23 removes legacy flags/global tokens, store backfill/conversion and legacy
domain/API/UI flags. New Environments select stores explicitly; retained old
dev data requires the separately approved T29 reset. Bootstrap is idempotent, checks endpoint identity and
can refresh only managed credentials after successful verification/CAS admission.
Workload delivery still requires Vault reachability and Kubernetes auth for the
selected cluster. [Normal-store verification](verification/2026-10-08-compose-vault-normal-store.md)
records the new implementation; the earlier legacy-bootstrap evidence is historical.

T24 removes store-backed Variable reads/materialization and the Legacy configuration
DTO/error code. Variables require metadata Value only; Secrets require explicit
owning store/ref only. JSON and PostgreSQL revision reads/writes reject mixed or
old Variable formats; no retained data conversion is performed. Variable CRUD/list
never resolves Vault, and store switching copies only desired Secrets while
preserving Variables and old immutable Secret refs. Local service/API tests prove
these boundaries; PostgreSQL opt-in/live Vault are not reverified. T28 owns the
fresh SQL constraint baseline and T29 the approved retained-dev reset.

## Historical Environment execution binding (2026-10-07; superseded by T26–T28)

Developer creates an Application from Name/Subdomain; staging/production start
UNCONFIGURED. Each Environment Settings explicitly sets one Organization READY
Connection once, then shows a locked binding even before deployment. Repeated
sets return 409; defaults never retarget or auto-bind. Independent profile/region
and mixed Kubernetes/AWS targets are supported. Preview/Deploy use the Environment
binding, reject unconfigured targets and Definition target conflicts safely.
Legacy SQL/JSON binding is backfilled/locked without changing existing namespace,
TargetRef or AWS application-scope state. New AWS VPC/EKS are Environment-scoped,
with bounded provider-safe names; AWS credential onboarding remains deferred.

[Verification/video](verification/2026-10-07-environment-connection-kind.md)
covers real kind upload, two independent logical bindings, refresh/restart lock,
unconfigured Preview rejection, staging backend/frontend/PostgreSQL deployment
and Vault/VSO diagnostic PASS. Both Connections use the same physical cluster;
production is bound but not deployed. AWS scope/name semantics are covered by
local tests, not a cloud run. Run-owned resources/private credentials were removed.
Final review and validation passed, including isolated PostgreSQL race tests,
deterministic seed preservation tests and run-owned browser cleanup checks.

Previous [Application-selection local evidence](verification/2026-10-07-application-connection-selection-local.md)
and [kind evidence](verification/2026-10-07-application-connection-selection-kind.md)
remain historical; their Application-wide selection is superseded by ADR-011.

## Environment secret stores and editable destinations — verified first delivery

[ADR-012](architecture/decisions/ADR-012-environment-stores-and-transitions.md)
replaces permanent selection lock with versioned editable per-Environment execution
and Secret Store Connections, atomic admission, Vault transfer and PostgreSQL
migration with downtime. The first direct-Kubernetes delivery is implemented and
verified by the [real-kind human recording](verification/2026-10-08-environment-stores-kind.md).

Backend/frontend code and canonical design are present. Initial compilation,
recovery fencing, PostgreSQL command, routing preflight and Preview impact defects
were corrected. Independent Go race/PostgreSQL integration/vet/build and frontend
typecheck/lint/153 tests/build checks passed on 2026-10-08. The full human UI flow
passed on real kind with two workload Vault stores, separate platform credential
storage, scoped tokens, distinct VSO auth mounts, PostgreSQL migration and actual
Ingress HTTP/data checks. The reviewed H.264 recording has 33 phase marks and
full decode/frame validation; test namespaces, temporary Vaults and credentials
were cleaned after the run.

Review corrected recovery polling after a failed attempt, idle-to-busy UI refresh,
configuration/draft submission guards, early application-ID capture, owned process
groups, bounded calls and deterministic transition ownership cleanup. Current
acceptance helpers explicitly select a store before secret writes and current
Connection runners follow editable ADR-012 behavior. IMP-017 is resolved for this
delivery. The live run proves two logical Connections on one physical kind
cluster; it does not prove distinct-cluster/DNS transfer or AWS/Aurora migration.
Recovery/fencing failure paths are covered by local tests rather than this happy-path
recording. Other updated legacy runners were syntax-checked, not all replayed live.
The pre-existing AWS integration-tag test has an EKSDescriptor argument mismatch; no live AWS run
is authorized or claimed. Prior set-once evidence remains historical.

## Implicit internal cluster binding — local and kind verified 2026-10-09

[ADR-013](architecture/decisions/ADR-013-implicit-existing-cluster.md) removes
per-cluster user Definition registration by binding an implicit existing-cluster
node to the Environment Connection. Product planner now uses this by default;
the reference harness explicitly retains reference matching. Execution admits a
reserved system Definition before FK-dependent writes; public registration and
foreign-content collisions fail closed. Existing descriptor/Application scope,
historical Definitions and target restoration remain compatible. New seeds no
longer create the authored internal cluster Definition. Console shows the
Environment Connection binding; namespace/PostgreSQL and cloud matching remain.

[Local evidence](verification/2026-10-09-implicit-existing-cluster-local.md) records
Go tests/build, targeted race suites, frontend typecheck/lint/154 tests/build,
syntax checks and fake-adapter browser create/deploy/restart with no per-cluster
Definition registration. IMP-019 is resolved for this delivery.
[Real kind evidence](verification/2026-10-09-k8s4f-live.md) additionally covers
retained `k8s-4f` registration in the Docker Console and a separate isolated
real-adapter Playwright FE/BE/PostgreSQL Deploy, diagnostic/job submission,
builtin binding and ownership-checked cleanup. No AWS or SQL-system-store
integration run was performed. FK ordering is locally simulated; existing
transition tests pass, without a new live transition proof. Persistent Console
workload Vault auth was not established by the isolated test.

The subsequent [retained dev deployment](verification/2026-10-09-k8s4f-dev-retained.md)
uses the persistent Docker Console to deploy sample FE/BE/PostgreSQL onto the
new physical `kind-k8s-4f` cluster. New Connection `K8S-4F` (`k8s-4f-2`), a
dedicated persistent workload Vault and VSO establish actual secret delivery;
readiness/PVC, diagnostics and reload persistence pass. At that checkpoint both sample attempt and
final Applications were deployed; T29 below supersedes their current retained state. The reviewed Vietnamese-captioned video
joins registration and successful deployment sessions explicitly. See the
[dev runbook](operations/k8s4f-dev.md) for URLs/restart and token lifetime limits.

## Console refactor foundation — T01

Thuật ngữ nhãn tiếng Việt đã chốt trong [glossary](GLOSSARY.md#nhãn-web-console-vi-01).
Runner refactor local và locator role/label theo trang đã implement; headed
sign-in/navigation/sign-out smoke, assertion-failure retention và signal cleanup
đã kiểm trên fake adapters. [Evidence T01](verification/2026-10-10-T01-refactor-foundation.md).
Các trang hiện giữ nhãn cũ tới task Việt hóa tương ứng; chưa có thay đổi matching,
API hoặc deploy retained K8S-4F trong T01.

## Public existing-cluster registration closed — T17 (2026-10-11)

Public Resource Definition registration rejects every `existing-cluster` driver
with validation 400 before insert; reserved system keys remain protected for all
drivers. Trusted builtin construction, executor and FK admission are unchanged.
Existing records/history remain readable; no retained database or cluster was
mutated. ReferenceCluster conformance remains unchanged.
See [T17 local evidence](verification/2026-10-11-T17-existing-cluster-registration.md).

## Console refactor T18 (2026-10-11)

GET `/api/v1/render-bundles` dành PE/Admin với session Organization trả danh
sách ID/version/status AVAILABLE an toàn từ registry bất biến của backend,
được sắp theo ID và dùng cùng availability predicate với registration.
Trang Mẫu dựng ứng dụng chọn từ API: một bundle tự chọn khi draft chưa chọn,
nhiều bundle cần chọn tường minh, rỗng/loading/error khóa đăng ký và có hướng
dẫn/retry Việt. Refresh không thay lựa chọn cũ; option mất availability vẫn
hiển thị và không submit. POST giữ nguyên payload và revalidate trước insert.

[Local verification](verification/2026-10-11-T18-render-bundles.md) ghi HTTP
permissions/metadata/no-insert, frontend selector guards và headed fake scenario.
Registry một bundle/registration/Preview dùng backend local thật với fake adapters
và binary stub; 0/n/error là projection mocks. Không chứng minh renderer binary
thật hoặc live cluster/cloud; PostgreSQL opt-in không chạy. Matching/ADR-010 giữ nguyên.

## Console refactor T19 (2026-10-11)

POST `/api/v1/resource-definitions/matching-preview` đã có PE/Admin/session-org
gate, bounded strict safe input, scoped read-only snapshot app/env/READY target/
catalog và candidate replacement chỉ trong memory. Trả eligibility, best
criterion/score, unique winner hoặc winning ties, winning Connection conflict,
incomplete/system-binding/native-renderer và deterministic snapshot marker/version.
Weights/profile/ties, match.go, reference fixtures và full graph Preview không đổi.
Không write, inspect, render, provision hoặc secret resolution; kết quả không
bảo đảm registration/deploy, không phát token.

Hai form resource/template có control chung “Kiểm tra matching” dùng draft chưa
lưu/context mẫu. Input/context/metadata/session đổi thì invalidate result/pending;
transport errors tách khỏi matching states, retry tường minh. INCOMPLETE không
bị diễn giải thành không khớp; system node không do authored criteria điều khiển.
[Local verification](verification/2026-10-11-T19-matching-preview.md):
Go/conformance/build và frontend checks pass local; browser dùng backend thật
với fake adapters/state tạm, transport 500 là projection mock. SQL/kind/AWS
opt-in và retained K8S-4F không được chạy hoặc thay đổi trong T19.

## Console error contract T20 (2026-10-12)

Các handler Console dùng stable error code và câu an toàn cố định; field và
planner params chỉ từ allowlist, không echo submitted values/raw provider/decode
text. Unknown cause thành lỗi nội bộ, không phân loại bằng prefix tiếng Anh.
Client chỉ dùng code/status và metadata đã kiểm để chọn hướng dẫn Việt.
BUSY không bị coi là stale; reload stale chỉ báo thành công sau GET thành công,
failed reload giữ edits và khóa writes tới khi reload/review, scope đổi reset gate.
Retry mutation vẫn tường minh; không đổi scoping/CAS/planner/renderer semantics.

[Verification T20](verification/2026-10-12-T20-console-errors.md) ghi checks và
headed local fake evidence; BUSY/unknown/copy failure là projection mocks.
Không PostgreSQL/kind/AWS opt-in hoặc retained K8S-4F mutation trong task này.

## T21 — Environment context aliases removed (2026-10-12)

Product context now exposes profile/region only as `env.profile`/`env.region`
from the selected Environment. `app.profile`/`app.region` return unknown context
errors, including whole and interpolated placeholders; no rewrite/fallback.
Application identity and infrastructure scope/naming remain unchanged. Go suite,
build and uncached planning/conformance passed locally; no SQL opt-in or external
mutation. See [T21 evidence](verification/2026-10-12-T21-context-aliases.md).

## T22 — Explicit Kubernetes credential boundary

[Local verification](verification/2026-10-12-T22-explicit-kubeconfig.md) covers
upload-only registration, missing/foreign credential rejection, seed VERIFYING,
scoped target restore/cancellation cleanup and Fleet rejection before side
effects. Go/conformance/build, frontend 425 tests/build and headed fake T01/T07
plus application-connection local restart pass. SQL integration/live checks
were not run. Retained dev resources remain untouched; old HOST_CONTEXT DB
requires approved reset. T28 removes the historical SQL startup migration chain.

## Public route cleanup T25 (2026-10-12)

[Local verification](verification/2026-10-12-T25-public-routes.md): product chỉ
nhận `service.publicRoutes`; field `publicPort` bị từ chối kể cả rỗng/null/cùng
routes, với HTTP `INVALID_SCORE` và hướng dẫn Việt an toàn. Không convert draft
cũ, không route fallback; dữ liệu cũ chứa alias cần sửa/reset dev theo T29.
Decoder giữ strict nested validation; literal binding security, nhiều path,
port ownership, no-op/route-only retry và T08 controls giữ nguyên. Full Go/build/
conformance, frontend 429 tests và headed T25/T08 pass với fake adapters.
Không kiểm PostgreSQL opt-in, kind/AWS/Vault thật hoặc mutate retained state.

## T26 — Application metadata cleanup (2026-10-12)

Application domain and SQL/JSON Save/Get/List now carry only identity/current
metadata; execution profile/Connection/region/runtime and configuration provider
fields are removed. Environment bindings remain independent; Application saves
and JSON restart preserve historical Deployment targets/generations and workload
TargetRef snapshots. Application-to-Environment backfill is removed; retained
meaningful/malformed old target data fails startup with a safe reset-required error.
At the T26 checkpoint, transitional SQL migration 9 prepared fresh state; PostgreSQL integration was
not executed (no authorized disposable DB). Full uncached Go/conformance/build,
targeted race and docs/PlantUML checks pass. No browser/UI change or external mutation.
See [T26 evidence](verification/2026-10-12-T26-application-metadata.md).
Retained dev state requires the separately approved T29 reset before use. T27
scope/seed cleanup is described below; T28 replaces the transitional SQL chain.

## T27 — Environment-only infrastructure scope (2026-10-12)

Removed the LEGACY_APPLICATION product enum and cloud identity/runtime branches.
Execution binding scope is ENVIRONMENT only; domain/planner/target and SQL/JSON
Save/Bind/reopen reject explicit invalid scopes without conversion or scope-value
leakage. Current generation-0 namespace/descriptor/provider-name identities and
later-generation isolation remain; trusted internal cluster Application resource
scope is preserved under ADR-013. Settings drops its migrated-binding label.

Acceptance seed binds ENVIRONMENT explicitly and inserts missing Definitions only;
exact old seeded templates are no longer upgraded. Conformance needs no additional
historical path/name adapter; all 33 fixtures retain their existing outcomes and
matching weights/ties. At the T27 checkpoint, SQL migration 10 supplied the strict CHECK after historical
ledger replay; the migration-6 literal is replaced before serving and removed with
T28 baseline. PostgreSQL integration was unexecuted at the T27 checkpoint;
the current baseline is verified by T28 below. Retained dev state requires the separately approved T29 reset before
use; no retained database, cluster, Vault or AWS resources were mutated.

Full uncached Go/conformance/build and targeted race suites pass; frontend 429
checks and headed fake T07 regression pass. Video decode/frame/caption review
completed. See [T27 evidence](verification/2026-10-12-T27-environment-scope.md).

## T28 — Fresh persistence baseline (2026-10-12)

Code replaces PostgreSQL migration replay/backfills with empty-schema atomic
initialization and current baseline/structural validation on boot. Older ledgers,
unmarked state and schema drift fail before serving without repair. JSON requires
the current complete format and explicit current metadata; no defaults, Connection
inference or history reconstruction. Current Snapshot/CAS/fencing/generation and
Variable/Secret boundaries remain.

Full uncached Go suite/build/conformance and targeted races pass both locally and
on authorized isolated PostgreSQL 16. Fresh/reopen/rollback, schema drift rejection,
SQL constraints, registration and multi-backend ownership pass with no skipped
tests. Synthetic dump/restore and owned cleanup pass; failed first-run fixture
checks remain in evidence. T28 is complete. No retained DB/Vault/cluster was mutated;
T29 below completes the approved reset/rebuild for the retained dev stack.
Other historical databases still require an explicitly scoped reset before use.
See [T28 evidence](verification/2026-10-12-T28-fresh-baseline.md).


## T29 — Approved dev K8S-4F reset/rebuild (2026-10-12)

Real IDP backup and both workload PostgreSQL backups passed isolated restore,
row-count/job-checksum checks before resetting only the IDP database. Current
fresh baseline serves the new application `k8s4f-t29-20261012-r1`, ID
`47fb34b4-2d6e-493e-a2c5-880837abfc1c`; staging uses KUBECONFIG Connection
`k8s-4f`, ordinary Store `k8s-4f-vault`, ENVIRONMENT scope, generation 0.
Frontend/backend/PostgreSQL are Ready, PVC Bound; empty DB before first job,
four diagnostics, job/reload persistence and IDP restart persistence passed.
App URL is <http://localhost:18481/>; Console remains <http://localhost:3001/ui/>.

Both old namespaces/PVC remain with FE/BE at zero and PostgreSQL 1/1; old port
18480 no longer serves, and their metadata/history is not in the new IDP.
Vault volumes, VSO, clusters and unrelated systems are preserved. Private backups
are retained until 2026-11-11T04:05:55Z with pinned old-image recovery prepared.
One pre-deploy runner locator failure and its video are preserved; explicit
same-run continuation passed without replaying Deploy. Headed real video has
Vietnamese captions; it is separate from the first Connection-registration phase.
See [T29 evidence](verification/2026-10-12-T29-dev-rebuild.md) and
[dev runbook](operations/k8s4f-dev.md). No production/AWS run or new recovery
execution is claimed; live backup restore-smoke succeeded, offline recovery guards
pass. The sample has no worker, so its diagnostic job stays PENDING.

## T30 — Key rename and desired reference update (2026-10-12)

[Local verification](verification/2026-10-12-T30-key-rename.md): Settings có
checkbox mặc định bỏ chọn “Đổi tên và cập nhật tham chiếu”. Opt-in commits
config revision + affected drafts nguyên tử dưới config/env/draft/current-set
CAS/BUSY; deployed consumers không có draft được reconstruct/round-trip verified.
Giữ aliases/many-to-one, advanced fields, unrelated refs/context/escaped literals
và opaque Secret refs, không đọc/copy Secret bytes. Pending deletes không bị
hồi sinh; rename-only/delete giữ references. Runtime/current/applied state không
đổi tới Preview → Deploy tường minh; Preview cũ stale. Unsafe reconstruction
fail closed với hướng dẫn Việt. Production Preview CTA giữ đúng Environment.

Full Go/conformance/build, targeted races, frontend 444 tests/gates, headed T30
và T07/T08 regression pass. Video dùng real product API/planner với fake adapters
và memory state/store; T30 BUSY là projection mock. PostgreSQL atomic command và
opt-in rollback/race/reopen tests đã có nhưng chưa được thực thi vì chưa có
approval DB disposable cho T30; không claim PostgreSQL/live rollout proof.

## T31 — Typed Definition form and capability metadata (2026-10-12)

[Local verification](verification/2026-10-12-T31-typed-definitions.md):
GET capability metadata role/org-gated và chỉ đọc cung cấp cặp type/driver/module,
types/defaults/required/param sources và preset từ policy/embedded inspector.
Namespace name source giữ BR-12 required string; executor-owned/credentials
không có controls editable. Loại tài nguyên và CTA dùng metadata authoritative,
không fallback whitelist frontend khi tải lỗi.

Form variables/provision typed giữ JSON types, placeholders, default/param omission,
presets opt-in và draft từng cặp. Advanced JSON giữ lossless known fields hoặc
giữ mode khi không biểu diễn an toàn; params workload vẫn ưu tiên hơn fallback
Definition. Registration/planner tiếp tục validate independently; matcher/ranking/
ties, providers/renderer và fixtures không đổi.

Go/build/conformance, catalog/HTTP race, frontend 464 tests/gates và headed
T31/T02/T04/T16/T19 pass. T31 full Score Preview kiểm consumer → provider và
provider-first batches, không Deploy. Fake adapters/disposable state; metadata
500 projection mock, bool/map dùng synthetic metadata unit coverage. Video
decoded/frame/caption reviewed, failed attempts và bản gốc được giữ. Không chạy
SQL opt-in hoặc mutate retained/cloud/cluster; không claim live provider proof.

## Implementation mô tả được chuyển khỏi specification

Các mô tả dưới đây được chuyển từ checkpoint `3367f7e`; chúng mô tả trạng
thái code/evidence trước M04, không phải yêu cầu v2 hoặc test vừa chạy lại.
Nguồn trước cập nhật nằm ở `git show 3367f7e:<path>` và trước Bước 1 ở
`git show a897d6b:<path>`. IMP-021 theo dõi gap ở checkpoint đó; nay đã đóng theo biên bản M04.

### usecase/UC-02/specification.md

- Seed catalog đã có Resource Types cho workload, VPC, cluster, namespace và PostgreSQL; planner dùng input contract để kiểm tra Score `params` và output contract để kiểm tra placeholder/executor output.
- API/UI đăng ký và nghiệp vụ `RegisterResourceType` cùng normalized PostgreSQL persistence đã có; production-grade RBAC chưa có.


### usecase/UC-03/specification.md

- Seed catalog đã có Definitions cho implicit VPC/EKS/namespace, existing cluster, Aurora và PostgreSQL StatefulSet; executor registry chọn Terraform/Kubernetes/existing-cluster adapter theo matched Definition.
- Planner đã hỗ trợ optional Execution Profile guard trước năm Matching Criteria chuẩn, Driver Inputs, Resource References, provision rules, fixed-point expansion và kiểm tra Terraform contract. Seeded PostgreSQL Definitions áp dụng cho mọi Environment cùng profile.
- Terraform execution của MVP chỉ hỗ trợ các module `vpc`, `eks` và `aurora` được nhúng trong binary. Conformance harness có thể inspect `source.url[@rev][/path]`, nhưng runtime chưa tải hoặc execute Terraform source từ xa.
- API/UI đăng ký và nghiệp vụ `RegisterResourceDefinition` cùng normalized
  PostgreSQL persistence đã có; production-grade RBAC còn thiếu. Catalog seed
  vẫn cung cấp Definition mặc định.
- Driver enum ngắn, `ConnectionKey`, `source.module` và context placeholder mở
  rộng là contract nội bộ của MVP, không phải Humanitec public contract. Mapping
  boundary và `driver_inputs.secret_refs` được phân loại tại
  [compatibility matrix](implementation/humanitec-compatibility.md) và D06.


### usecase/UC-05/specification.md

- `PlanningService` dùng chung đã triển khai Score validation, Candidate Set, graph/matching, contract inspection, classification và batches; 33 challenge fixture được chạy qua planner sản phẩm với các khác biệt đã tài liệu hóa.
- Planner sinh Humanitec-shaped Delta `modules.add/remove/update` và `shared`; patch `modules.update.<id>` relative với module, patch `shared` relative với object shared, array diff theo BR-06 có product tests riêng và conformance so Delta của 27 accepted fixtures. Deployment path của UC-06 persist Delta thành immutable Snapshot riêng; preview không persist Snapshot. Parser validate `containers.*.resources` theo BR-07 và planner giữ requests/limits nguyên văn trong Candidate Set (I06-07).
- UC-06 đang gọi pipeline này để deploy thật. UC-16 pending-change Preview,
  endpoint và Application home panel đã wire, gồm no-op filtering và route-only
  retry. Standalone Score Preview đã có service, scoped authenticated HTTP API
  và Web Console riêng; snapshot nhất quán, public response được sanitize,
  không save draft hoặc tạo Deployment.


### usecase/UC-06/specification.md

- **UC-06a Plan Deployment:** đã có Score conversion, Candidate Set, descriptor theo Deployment Set path, implicit infrastructure theo profile, fixed-point graph expansion, matching, Terraform contract inspection, Active Resource classification và provider-first batches. Planner sinh Humanitec-shaped Delta `modules.add/remove/update` và `shared` theo BR-10, với patch relative đúng module/shared scope và array semantics được test; mỗi Deployment persist immutable `DeploymentDeltaSnapshot` riêng và conformance so Delta cho 27 accepted fixtures. Score parser nhận `containers.*.resources.requests/limits` (`cpu`/`memory`) và giữ typed values nguyên văn qua Score fragment, Candidate Set, Delta Snapshot và persisted set theo BR-11 (I06-07).
- **UC-06b Execute Deployment:** đã có fake, Kubernetes và Terraform adapters; output propagation; workload render/apply; API/Web Console; in-memory store kèm JSON snapshot. Renderer ánh xạ declared requests/limits nguyên văn; request field thiếu lấy limit cùng field, nếu không có limit thì default `10m`/`32Mi` theo BR-11; kind verification của I06-07 chỉ xác nhận live Deployment cho ba seeded case declared/partial/omitted; nhánh limit fallback và request khai báo vượt limit (giữ nguyên, Kubernetes API reject khi apply) mới được unit-test. Internal happy path đã verify trên kind và cloud happy path đã verify trên AWS; AWS chưa chạy lại sau I06-07.
- **Fleet GitRepo mode:** internal kind có adapter tùy chọn ghi manifest vào repo GitOps private, đợi Fleet quan sát commit và đúng Deployment revision Ready. Image được kéo từ Harbor bằng namespace-local imagePullSecret; không có bước lấy mã nguồn hoặc build. Đường này đã pass kind create/remove với image mẫu; direct adapter vẫn là mặc định.
- Mỗi request xử lý đúng một Score/workload. Acceptance flow gọi tuần tự ba deployment `backend`, `worker`, `frontend`; database là shared resource được giữ/reuse qua cùng descriptor.
- Normalized PostgreSQL system-of-record đã được triển khai; in-memory/JSON vẫn là local/test option. Terraform state backend bền vững chưa được triển khai.


### usecase/UC-07/specification.md

- Planner đã validate module và từng shared entry trong `before Score`, từ chối shared conflict, tạo Candidate Set và giữ nguyên các module khác. Planner sinh Delta theo BR-07: module add/remove/update nằm trong `modules.add/remove/update`, patch relative với module hoặc object shared và array semantics theo UC-05 BR-06 được test; conformance so Delta cho 27 accepted fixtures.
- Shared entry chỉ bị loại khi workload thôi khai báo và không còn module khác tham chiếu. Final transaction persist `UNREFERENCED` cho READY resources thuộc Environment/shared/workload scope và không còn trong desired graph; không destroy, không mark Application-wide resources từ một Environment riêng lẻ.
- Candidate cuối cùng được kiểm tra Service references; pending batch có thể xóa cả consumer/provider mà không bị chặn vì trạng thái trung gian.
- UC-16 Preview → Deploy update/delete theo từng workload có scoped strict API và Console confirmation/Undo, busy/stale/reload states, per-workload report và history navigation. Memory/PostgreSQL rollback/restart tests và local fake-runtime UI recording kiểm chứng milestone này. Fleet GitRepo remove đã pass kind trong evidence cũ; image/resource update qua Fleet chưa có external verification riêng.


### usecase/UC-08/specification.md

- Planner đã dựng private/shared/implicit resource nodes, match Definition và tạo provider-first batches; VPC/EKS and namespace use Environment/generation scope only (T27/ADR-011/012).
- `ResourceProvisioningService` đã chọn fake, existing-cluster, Kubernetes hoặc Terraform executor theo matched Definition; outputs được validate, truyền sang node phụ thuộc/workload và lưu vào Active Resource/deployment-resource state.
- Internal path đã provision namespace/PostgreSQL StatefulSet trên kind; cloud path đã provision VPC/EKS/Aurora bằng Terraform trên AWS.
- Normalized PostgreSQL repository đã có cho logical orchestration state; in-memory/JSON vẫn dùng cho local/test. Terraform state vẫn nằm trong local run directory; remote state/locking và lifecycle reconcile đầy đủ chưa có.


### usecase/UC-09/specification.md

- Deployment, plan snapshot, resource progress và Active Resources đã được lưu
  trong state store. Detail đọc per-Deployment workload snapshot, không đọc
  mutable current Environment state.
- React Web Console có recent deployments, danh sách theo Environment với bộ
  lọc status và trang detail dùng persisted query API.
- Authenticated Organization scoping, không trả `resolvedInputs`, server-side
  status filter và loading/error/not-found tests đã được hiện thực. Local
  browser test xác nhận snapshot không đổi sau redeploy và backend restart.
  Cross-deployment comparison và live status không thuộc happy path.


### usecase/UC-16/specification.md

Đã có UI/API lưu draft, đánh dấu xóa/hoàn tác, import Score và validation
tham chiếu Application key, resource output, Service/cổng cùng Environment.
Save và import kiểm tra toàn bộ non-virtual resource params theo input contract
trước khi ghi draft; local code/API/UI tests đã pass ngày 2026-10-02.
Planner/executor resolve tham chiếu `environment` và `service`; Application home
có Preview → Deploy của các draft. Workload đã deploy không có draft được tái
tạo thành Score tham chiếu để sửa trên form. Kind đã kiểm chứng create,
configuration-only redeploy và remove; broader cloud path chưa kiểm chứng lại.


## Rationale/implementation chuyển khỏi shared design

Checkpoint `3367f7e` mô tả AWS process-configured seed chưa onboarding, native
rendering omitted selection, embedded/static inspector và EKS workspace paths.
Các ghi chú đó nay chỉ mô tả code chưa đáp ứng v2; required design dùng scoped
AWS Vault credential, required rendering selection, verified module registry
và opaque target. ADR-010 baseline score-k8s 0.15.0/internal bundle có evidence
đã liệt kê ở phần rendering phía trên; không bị diễn giải thành version v2.
Set-once/legacy migrations và tên chứa run ID của ADR-011 đã hết hiệu lực;
ADR-012 operation/transition features đã verified vẫn được giữ trong scope cũ.
Không claim Terraform full resume, orphan import hay platform DR đã implement.


Checkpoint trước khi M04 hoàn tất — getter probe `660fd5c` đạt độc lập1407Go/race/
0fail/0skip,634frontend/fullchecks, mã băm nguồn khớp vàfixtureclean (R7).
Helper boolean `214e37f` đạt635frontend/fullchecks, backend không đổi.
Cả hai đã hợp nhất; A02r3 chạy trên mã này, evidence sẽ ghi đúng commit.

## Issue #6 — EC2 IAM role accounts (2026-10-20)

AWS Driver Accounts now support explicit AWS_EC2_ROLE alongside access keys.
Expected canonical role ARN/unique RoleId are verified through IMDSv2 + STS/IAM;
role accounts store no credential object, retain Organization/version guards and
support rename/reverification. Terraform has an authenticated per-invocation
refresh source; readiness and EKS reuse explicit scoped identity resolution.
The Console hides access keys/replacement for EC2 accounts. Full-stack EC2 Compose
is available through `deploy/ec2/start.sh`.

Local Go/frontend/docs checks and real PostgreSQL persistence checks passed.
Live EC2 UI registration/rename/reverify and a production-binder Terraform VPC
apply/destroy passed; exact VPC native absence was confirmed. Restart/reverification and full run-owned EC2/EBS/network/IAM cleanup also passed;
see [issue #6 evidence](verification/2026-10-20-issue6-ec2-role.md).
EKS/Aurora/ElastiCache full provisioning and waiting for natural credential expiry
are outside this focused verification. Previous SQL baseline compatibility rules
remain: no automatic migration or authorized reset is introduced.
