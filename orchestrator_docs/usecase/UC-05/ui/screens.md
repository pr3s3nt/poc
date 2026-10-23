---
id: UC-05-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-23
related: UC-05
---

# UC-05 screens

## Nhãn Console tiếng Việt — T13

Trang standalone dùng nhãn **Xem trước Score**, **Thao tác**, **ID Workload**,
**ID lần chạy**, **Score trước (YAML hoặc JSON)**, **Score sau (YAML hoặc JSON)**,
**Xem trước** và **Thử lại**. Tab giữ nguyên `staging`/`production`; các lựa chọn
Thêm/Cập nhật/Xóa Workload vẫn gửi `deploy`/`update`/`remove`.
Toàn bộ hướng dẫn, aria-label, trạng thái và lỗi hiển thị dùng tiếng Việt theo
[glossary](../../../GLOSSARY.md#nhãn-web-console-vi-01). YAML/JSON keys,
placeholder, technical IDs, graph descriptors, output keys và dữ liệu planning
giữ nguyên. Chỉ đổi thông báo parser, không thay quy tắc parse Score.

Kết quả có tóm tắt Delta, thay đổi tài nguyên, thứ tự tạo tài nguyên, liên kết
thực thi, đồ thị và tài liệu Delta/Candidate Deployment Set. Nhãn giải thích
bản đã lọc dữ liệu nhạy cảm và giá trị `***redacted***`; không diễn giải marker
thành dữ liệu thực. Kết nối ngầm được gọi là **Kết nối của môi trường**.
Lỗi API chưa có mapping code đầy đủ dùng fallback Việt an toàn, không hiện
raw backend/parser error hoặc dịch bằng regex; mapping code theo contract lỗi T20.

Application home provides **Preview Score** for the selected Environment.
The standalone page visibly identifies Application, Environment and the persisted Environment
connection key/profile, provides
action, Workload ID, Run ID and labeled Score before/after JSON/YAML editors.
Action hides irrelevant editors; validation never silently changes input.
Warn users to use configuration placeholders instead of literal secrets.

**Preview** performs only the read-only API operation. Back returns to the
Application; it does not save input. The result shows base Set/version, plan
hash, add/update/remove Delta summary, resource classification, provider-first
batches, resource execution bindings, graph nodes/edges and expandable formatted Delta
and Candidate Set. An empty change is explicitly explained as no workload
change; planning artifacts may still contain the Environment graph.

No standalone Save or Deploy control. Existing pending-change panel remains
unchanged in responsibility. Long artifacts must remain readable/scrollable.

The implicit internal cluster row identifies **Environment connection** and
the selected Connection key, rather than presenting it as a criteria-matched
user Definition. Other rows continue showing their selected Resource Definition.

## Nội dung Preview tài nguyên

From Application → Environment → saved drafts, show per dependency Type/class,
selected Definition/module ID/immutable revision, target/account/region, owner
and intended action; show required score-k8s bundle per Workload. Internal
existing cluster remains system Connection binding. No module selector for
Developer; no secret/state/backend-token details. Explain retained/unreferenced
resources are not deleted by removing Workload. Input/no-match/tie blocks Deploy.
Replacement intent is explicit; ordinary Deploy cannot authorize destructive
replacement. No exact Terraform plan/manifests claim for unresolved outputs.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.


## V04–V06 current target/dependency behavior

[ADR-017](../../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces remaining profile/Environment-target requirements above for the
replacement path.

Show Definition/driver/account/region per resource and the explicit cluster
source per namespace/workload. Graph shows real consumer→provider edges and
ordering reasons separately from selection-only relations. Unknown EKS output
values show waiting/provenance, never fabricated Ready/endpoint values. Reuse
existing authoring, Preview and progress views; no new managed-only execution
entry or multi-cluster/patch-authoring screen.

## Desktop visual review — 2026-10-23

Use [accepted prototype](../../../../frontend/design/deployment-preview/README.md)
and [shared review contract](../../../architecture/contracts/deployment-review.md).
Keep existing AppShell/sidebar, application title/environment tabs, buttons,
typography and panels. Graph on left, selected node inspector on right; Overview,
Changes and Code stay linked to the same canonical node ID. Code shows red/green
line diff, old/new line numbers, after-only/copy and expand/collapse.

Cloud placement groups actual AWS/VPC/EKS namespace identities; Aurora/ElastiCache
remain outside EKS when so selected. Internal shows retained cluster and authored
namespace/resources. No hardcoded seven/six-node diagram or region/account fixture.
Canvas arrows may reverse canonical consumer→provider to show execution order,
but label direction clearly and preserve all reasons in dependency detail.
Shared nodes appear once; unknown placement is labeled unresolved.

Pending Preview uses existing explicit Deploy; standalone remains read-only.
During execution show observed node states, stage/time/wait reasons and counts;
never render prototype scenario/failure selectors, fake timer, pause or reset.
No separate mobile design is required by this task; preserve existing shell.

### Prototype fidelity correction — 2026-10-23

Pending review occupies its own full-width panel beside the other Application
sections, never a nested Workload card. Preserve the approved 340px inspector
(310px at the smaller desktop breakpoint), node/type icons, intent counts,
readiness card, readable safe workload properties and dependency cards. Group
multiple reasons for one provider without losing those reasons. Technical pins,
descriptors and provenance remain available in disclosure sections.

Code uses per-node file selection and one active artifact, diff/after controls,
line counts, safe copy and expand. Selecting another node resets its file context.
Preview and observed deployment use one graph at a time; show the disabled busy
Deploy action while admission is in flight. The dependency strip includes
workload nodes and describes graph ordering, not scheduler concurrency or ETA.
Unsupported placement is labeled unresolved. See IMP-030 for visual acceptance.

Connector correction: one visible connector per provider/consumer pair retains
all original reasons. Nonadjacent dependencies route outside intervening node
cards and use dashed strokes; immediate dependencies stay solid. Separate
attachment ports and selected/hovered-node highlighting prevent a direct
Cluster→Workload relation from appearing to originate at Namespace. No edge is
removed by transitive reduction and no api↔web relation is invented.

Desktop space/color follow-up: the shared shell supports a collapsible icon rail
without clearing Preview, selected node/file or drafts. Preview uses the newly
available main-content width and recalculates graph scale through its existing
resize observer. Node/inspector illustrations share resource-type colors and
distinct cluster/namespace glyphs under the
[shared shell design](../../../architecture/ui/README.md). Colors do not replace
status text, intent or the actual runtime progress. No planner/graph relation,
execution behavior or mobile redesign changes.
