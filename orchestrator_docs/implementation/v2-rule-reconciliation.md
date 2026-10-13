---
id: V2-RULE-RECONCILIATION
artifact: documentation-reconciliation
status: current
last_reviewed: 2026-10-13
---

# Hợp nhất luật Terraform v2

Đã đọc specification/realization/contract/schema/domain/ADR cũ trước sửa.
Nguồn lịch sử: `a897d6b` (trước Bước 1), `3367f7e` (Bước 1 hai lớp luật).
Dùng `git show <commit>:orchestrator_docs/<path>` để xem toàn bộ predecessor.
Specification hiện tại chỉ có một bộ luật bắt buộc. ID BR/MS/VAR/OOS giữ
ổn định; nội dung đổi được ghi dưới đây. V2 ID gộp không còn định nghĩa riêng.

| UC | ID tại 3367f7e | Canonical destination hiện tại |
|---|---|---|
| UC-02 | V2-01 | BR-02/BR-03 |
| UC-02 | V2-02 | BR-04 |
| UC-03 | V2-01 | BR-09/BR-11/BR-12/BR-14 |
| UC-03 | V2-02 | BR-08/VAR-01/VAR-02 |
| UC-03 | V2-03 | PRE-04/BR-08 |
| UC-03 | V2-04 | BR-02/BR-06/BR-17 |
| UC-03 | V2-05 | BR-15/BR-16 |
| UC-03 | V2-06 | BR-23..26 |
| UC-04 | V2-01 | VAR-02/BR-02/BR-09/BR-11 |
| UC-04 | V2-02 | BR-08/BR-12 |
| UC-04 | V2-03 | BR-10/BR-11 + UC-08 BR-07 |
| UC-04 | V2-04 | V2-04 (giữ) |
| UC-05 | V2-01 | BR-10 |
| UC-05 | V2-02 | MS-06/BR-04/BR-09 |
| UC-05 | V2-03 | BR-09/BR-11 |
| UC-05 | V2-04 | V2-04 (giữ) |
| UC-06 | V2-01 | UC-16 MS-03/BR-11 + V2-02 |
| UC-06 | V2-02 | VAR-01/VAR-02/BR-20 |
| UC-06 | V2-03 | MS-08/MS-09/BR-09 |
| UC-06 | V2-04 | MS-11/BR-17..19 |
| UC-06 | V2-05 | V2-05 (giữ) |
| UC-06 | V2-06 | V2-06 (giữ) |
| UC-07 | V2-01 | BR-03/VAR-02/POST-04 |
| UC-07 | V2-02 | V2-02 (giữ) |
| UC-07 | V2-03 | V2-03 + DEL-* (Xóa qua UI, mở rộng nghĩa) |
| UC-07 | V2-04 | V2-04 (phân biệt user deletion/test cleanup) |
| UC-08 | V2-01 | VAR-01/VAR-02/POST-05 |
| UC-08 | V2-02 | BR-07 |
| UC-08 | V2-03 | BR-02 |
| UC-08 | V2-04 | V2-04 + MS-02 (giữ locking rule) |
| UC-08 | V2-05 | BR-03/BR-05/MS-05/MS-06 |
| UC-08 | V2-06 | V2-06/OOS-01/OOS-02 |
| UC-08 | V2-07 | V2-07 (giữ) |
| UC-09 | V2-01 | BR-05 |
| UC-09 | V2-02 | BR-07 |
| UC-09 | V2-03 | BR-05 + RES-03 |
| UC-16 | V2-01 | MS-03/BR-07 |
| UC-16 | V2-02 | V2-02 (giữ) |
| UC-16 | V2-03 | V2-03 (giữ) |
| UC-16 | V2-04 | BR-11/BR-14 |

## Shared artifacts và semantic removal gate

| Khái niệm giữ lại | Canonical destination |
|---|---|
| Criteria/score/ties, descriptor/Delta invariants, fixed-point scheduling | UC-03 BR-01..07/17/20..26; UC-05 BR-01..12; UC-06 BR-01..21 và OC-04/07 |
| Insert-only IDs, typed params/defaults/placeholders, source/mapping trust | UC-02 BR-01..06; UC-03 BR-08..16; module contract; OC-03/04/04d |
| Native fallback/internal-only và explicit database account cũ | Đã thay trực tiếp: UC-03/05/06/08, OC-04/05/07/10; ADR-014 sở hữu ý định mới |
| Kubernetes normalization, scoped Vault refs, key collision/attempt rollback, ordinary Secret Store/bootstrap | UC-04 VAR/BR/SS; UC-04 realization và connection-credentials; không xóa các checks/security boundaries |
| Configuration revisions/key picker/aliases/Service/publicRoutes/T30/no-op/partial workload result | UC-16 BR-01..19, UC-05 BR-08, UC-06 BR-11..16, OC-12/16/17; preserved realizations/UI |
| Set-once/legacy Application backfill/run-derived naming/SQL migration ledger | Retire ADR-011 (superseded); current equivalents ADR-012/013/014 và schema; nguyên văn ở a897d6b/3367f7e |
| State/encryption/locks/fencing/lineage/serial, ownership/provenance, process interruption hold | UC-08 BR-02/03/05/07 + V2-04/06/07; OC-18; state contract; schema/state machines |
| Separate cleanup/retained target/dependency ownership | UC-07 V2-03/DEL-* và OC-19; user UI mở rộng scope mới, run filter giữ CREATED_BY_RUN |
| Rendering normalization/output-only bridge/Secret/protected manifests, bundle selector vs audit fingerprint | ADR-010 các boundary còn hiệu lực, workload-rendering contract; runtime policy hiện hành ở ADR-014 |
| Shared DB aggregates/transactions/constraints, secret vs encrypted state | schema/domain/ERD; loại bỏ prose baseline/add-then-override, giữ FK/CAS/immutable history |
| Baseline code/API/tests/Fleet/kind/AWS status trong specifications/realizations | CURRENT_STATE mục “Implementation mô tả được chuyển khỏi specification” + IMP-021; không copy thành requirement |
| Verification lịch sử, reference fixtures | Giữ nguyên evidence/read-only reference; M04 thực hiện các tests/scenarios mới |

Không có nguồn chuẩn bị xóa mà không destination hoặc rationale lịch sử.
Các ID V2 gộp chỉ được nhắc trong reconciliation/history, không giữ rule thứ hai.
Diagrams và traceability phải dùng ID/destination mới; nguồn chuẩn không bị
sửa để hợp thức hóa code tại 3367f7e.

## Stable ID có nội dung thay đổi

- UC-02 BR-02/03/04: common Redis/PostgreSQL/schema metadata.
- UC-03 PRE-04, VAR-01/02, POST-03/04, BR-02/08/09/11/12/14/15/21/23/26,
  OOS-07: registered Terraform modules, Environment AWS binding, renderer cả
  hai profiles, verified registry; không embedded-only/Aurora account riêng.
- UC-04 PRE-04, VAR-02, POST-04, BR-12, OOS-01: access-key UI/Vault flow
  giữ đúng quyết định người dùng, đưa vào required scope; state backend platform.
- UC-05 MS-06, BR-04/09/10/11: known-input validation, read-only metadata,
  required renderer/pins và safe module/target/ownership Preview.
- UC-06 MS-07/09/11, VAR-01/02, BR-05/09/17/20, OOS-01/06/07: Terraform cả
  hai provider, required score-k8s/Ready, admission và bounded retry/deletion scope.
- UC-07 BR-03/OOS-03 và V2-03/04: giữ hạ tầng khi Remove, thêm user-confirmed
  deletion/selected-set; run cleanup filter không bị dùng làm global ownership rule.
- UC-08 MS-02/05/06, VAR-01/02, POST-05, BR-02/03/05/07, OOS-01/02/04/05:
  durable state/outputs/locking/target, bounded retry và separate destroy.
- UC-09 BR-05/07: safe persisted Terraform progress; RES-* mới cho resource
  detail/cleanup. UC-16 MS-03/BR-11 gộp params/class form lifecycle.
- OC-01b/03/04/05/06/07/08/10/11/04c/04d sửa trực tiếp contract hiện hành;
  OC-19 đổi từ internal-only sang PE/Admin preview/confirmation/execute/progress.
  OC-09 Workload Remove vẫn không destroy, OC-18 state vẫn internal-only.

Luật thực sự mới giữ V2 ID (replacement, locking/unknown outcome, class/
shared metadata/ownership) hoặc DEL/RES IDs; không giữ V2 rule trùng BR/MS.
New dependency-edge persistence và confirmed-scope schema phục vụ selected-set deletion
đã được người dùng chọn, không tự mở thêm quyền xóa.
