---
id: I06-10
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: UC-02, UC-03, UC-04, IMP-002
---

# I06-10 — Complete remaining UC-02..04 management flows

## Objective

Hoàn thiện application services, API và Web Console còn thiếu cho Resource
Type, Resource Definition và Connection lifecycle đã thiết kế.

## In scope

- UC-02 register Resource Type schemas.
- UC-03 register valid Resource Definition/criteria/contracts.
- UC-04 register/verify AWS or Kubernetes Connection và persist only secret refs.
- Management API/UI, validation, repository tests và planner integration.

## Out of scope

- Update/delete/versioning, fine-grained RBAC, audit và credential rotation.
- Humanitec public boundary D06 hoặc remote Terraform execution D03.
- Production-grade authorization, credential lifecycle và remote source
  execution.

## Exit criteria

- Ba management happy paths chạy qua API/UI và dữ liệu mới dùng được bởi
  planner mà không cần restart/seed edit.
- Invalid schemas/criteria/contracts/connections bị từ chối trước persistence.
- Secret value không vào database/state/API/logs.
- Backend/frontend validations pass và IMP-002 được xóa.

## First next action

Lập gap matrix UC-02..04 `MS-nn` → application service/repository/API/UI/test,
sau đó hoàn thiện theo dependency order UC-02 → UC-03 → UC-04. UC-01 đã hoàn
thành ở I00-00 và không được mở lại trong iteration này.

## Outcome

Current handoff after I06-09. Registration gap audit is in progress; AWS
credential storage/runtime binding requires an explicit user decision before
implementing the dependent branch. No live AWS/kind verification is authorized
by this local implementation milestone.

## M04 reprioritization — 2026-10-13

Người dùng ưu tiên [M04](../../M04-terraform-v2/README.md) cho Terraform v2.
Record này chưa hoàn thành, chỉ deferred; status/handoff cũ ở a897d6b/3367f7e.
AWS credential decision đã chốt ADR-009/014 và được implement trong M04,
không dùng ghi chú “decision pending” phía trên làm blocker hiện hành.

## Đối chiếu công việc còn dở với M04

M04 chỉ thay phần có bằng chứng tương ứng; không tự đóng I06-10/IMP-002.
IMP-002 ghi nhận ID/reserved-key, Driver Inputs boundary và UC-16 params
validation đã verified 2026-10-02; không mở lại như quyết định chưa chốt.

| Hạng mục / exit criterion I06-10 | Task M04 làm thay | Còn lại sau M04 |
|---|---|---|
| UC-02 đăng ký Resource Type/schema, validation trước persistence | I04-02/07 dùng và kiểm Resource Type contract; I04-10 kiểm integration | Luồng đăng ký type tùy ý và mọi MS/negative case ngoài type Redis/PostgreSQL/infra v2 vẫn cần gap audit. |
| UC-03 Definition/criteria/contracts; ít nhất một criterion | I04-02 registry, I04-07 Definition form, I04-10 matching | Các nhánh đăng ký Definition ngoài Terraform v2 chưa có evidence phải làm tiếp. Không đóng bằng kiểm thử seed. |
| UC-04 AWS Connection, opaque secret refs | I04-06 nhập key/Vault/verification, I04-10 binding | Credential rotation và các mục vốn ngoài scope không đưa vào M04. |
| UC-04 Kubernetes Connection registration/verification | I04-03/04 binding và module, I04-10/11 dùng Connection k8s-4f thật | Audit và hoàn thiện mọi trạng thái/negative case đăng ký Kubernetes chưa được test M04 bao phủ. |
| Management application services/repositories/API/UI/tests | I04-01 persistence, I04-02/06/07 catalog/Connection/UI, I04-10 integration | Management paths ngoài phạm vi v2; giữ regression UC-01 I00-00 và ID/reserved-key đã verified 2026-10-02. |
| Ba happy paths qua API/UI; dữ liệu dùng được không restart/seed edit | I04-06/07/10 và UI-only I04-11/12 cho Definition/Connection | Resource Type mới tùy ý chưa có UI evidence phải kiểm sau M04; giữ no-restart/reopen regression hiện có. |
| Invalid schemas/criteria/contracts/connections bị chặn trước persistence | I04-02/06/07 unit/API tests, I04-10 PostgreSQL thật; E01 ở I04-11 | Các UC-02..04 negative cases còn lại theo gap matrix MS/BR. |
| Secret value không vào DB/state/API/logs | I04-01 encryption; I04-03/06 Vault/bindings/redaction, I04-10 tests | Terraform state ciphertext được phép chứa secret theo ADR-014; không coi ciphertext là plaintext leak. Luồng management ngoài v2 vẫn cần audit. |
| Backend/frontend validation pass | Tất cả task M04 theo scope, tổng hợp I04-10/11/12 | Chạy lại checks khi làm nốt management gaps. |
| Xóa IMP-002 khi tất cả gaps đã giao | I04-10/12 cung cấp evidence cho phần M04 đã hoàn thành | Chỉ đóng IMP-002/I06-10 sau đối chiếu đầy đủ UC-02..04 và hoàn tất các dòng còn lại; không đóng chỉ vì M04 đạt. |

Sau M04, bước đầu của I06-10 là cập nhật gap matrix từng MS/BR với code/test và
verification M04, rồi tiếp tục các dòng chưa có bằng chứng. Quyết định AWS
credential đã chốt, không còn là blocker. Lịch sử safe slice giữ nguyên.
