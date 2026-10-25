---
id: DOC-INDEX
artifact: documentation-index
status: current
last_reviewed: 2026-10-25
---

# Documentation index

Index này giúp người và AI tìm bộ tài liệu nhỏ nhất, đúng thẩm quyền cho một
task. Nó chỉ điều hướng; không tự sở hữu product behavior, architecture hay
implementation status.

## Project entry points

- [Repository overview](../README.md)
- [AI agent workflow](../AGENTS.md)
- [Current project state](CURRENT_STATE.md)
- [Native application observability implementation](implementation/native-observability.md)
- [Native observability operations](operations/observability.md)
- [Project glossary](GLOSSARY.md)
- [Documentation rules](DOCUMENTATION_RULES.md)

## Route by task

| Task | Read first |
|---|---|
| Hiểu scope, mức hoàn thành hoặc limitation | [Current project state](CURRENT_STATE.md) |
| Tra cứu thuật ngữ | [Glossary](GLOSSARY.md) |
| Xây/sửa UI, mock hoặc prototype | [Shared UI design](architecture/ui/README.md), rồi UI của use case liên quan |
| Thay đổi UC-00 | [UC-00 context](usecase/UC-00/README.md) |
| Thay đổi UC-01 | [UC-01 context](usecase/UC-01/README.md) |
| Thay đổi UC-02 | [UC-02 context](usecase/UC-02/README.md) |
| Thay đổi UC-03 | [UC-03 context](usecase/UC-03/README.md) |
| Thay đổi UC-04 | [UC-04 context](usecase/UC-04/README.md) |
| Thay đổi UC-05 | [UC-05 context](usecase/UC-05/README.md) |
| Thay đổi UC-06 | [UC-06 context](usecase/UC-06/README.md) |
| Thay đổi UC-07 | [UC-07 context](usecase/UC-07/README.md) |
| Thay đổi UC-08 | [UC-08 context](usecase/UC-08/README.md) |
| Thay đổi UC-09 | [UC-09 context](usecase/UC-09/README.md) |
| Thay đổi UC-12 | [UC-12 context](usecase/UC-12/README.md) |
| Thay đổi UC-16 | [UC-16 context](usecase/UC-16/README.md) |
| Thay đổi UC-17 | [UC-17 context](usecase/UC-17/README.md) |
| Thay đổi shared architecture/domain | [Architecture index](architecture/README.md) |
| Thay đổi schema/constraint | [Database schema](architecture/database/schema.md) |
| Hiểu rationale của quyết định | [ADR index](architecture/decisions/README.md) |
| Làm hạng mục đang hoạt động | [Iteration index](iterations/README.md) |
| Làm vấn đề deferred/open | [Backlog index](backlog/README.md) |
| So thiết kế với code | [Implementation index](implementation/README.md) |
| So contract với Humanitec/Score | [Compatibility matrix](implementation/humanitec-compatibility.md) |
| Điều tra khác biệt design/code | [Known deviations](implementation/deviations.md) |
| Agent trực tiếp code/review/check; quy trình writer lịch sử | [Agent workflow runbook](operations/claude-tmux.md) |
| Build/run/troubleshoot | [Operations index](operations/README.md) |
| Xem bằng chứng execution | [Verification index](verification/README.md) |
| Kiểm requirement → design → test | [Traceability matrix](traceability/matrix.md) |
| Thay đổi cấu trúc tài liệu | [Documentation rules](DOCUMENTATION_RULES.md) |

## Canonical ownership

| Kind of information | Canonical owner | Không được thay thế bởi |
|---|---|---|
| Required use-case behavior | `usecase/UC-xx/specification.md` | Code, test, diagram, verification |
| Use-case collaboration | `usecase/UC-xx/realization.md` và diagrams | Implementation prose |
| Shared architecture/model | Artifact tương ứng dưới `architecture/` | Bản sao trong use case hoặc code comment |
| Database schema/constraint/enumeration | `architecture/database/schema.md` | Adapter hoặc migration prose |
| Accepted decision/rationale | ADR tương ứng | Plan, code hoặc discussion note |
| Current implemented scope/limitations | `CURRENT_STATE.md` | README, historical plan hoặc evidence |
| Open/deferred problem | Record tương ứng dưới `backlog/` | Current architecture |
| Design–implementation difference | `implementation/deviations.md` | Sửa ngầm canonical design |
| Design-to-code navigation | `implementation/code-map.md` | Product requirements |
| Actual implemented behavior | Source code và tests | Required behavior |
| Current work order | Active record dưới `iterations/` | Historical `plan.md` |
| Operational procedure | Record dưới `operations/` | Verification evidence |
| Result of one execution | Dated record dưới `verification/` | Current status hoặc requirement |
| Shared terminology | `GLOSSARY.md` | Định nghĩa cục bộ mâu thuẫn |
| AI working rules | Root `AGENTS.md` | Product docs |

## Conflict resolution

Khi hai nguồn mâu thuẫn, xác định loại thông tin rồi dùng bảng ownership; không
chọn nguồn chỉ vì mới hơn, dài hơn hoặc gần code hơn.

1. Xác định chính xác khái niệm mâu thuẫn.
2. Xác định canonical owner.
3. Phân loại nguồn là current, deferred, historical hay evidence.
4. Phân loại vấn đề: implementation defect, unrecorded requirement change,
   stale summary hay unresolved decision.
5. Sửa canonical owner trước, rồi cập nhật dependent artifacts/code/tests.
6. Nếu không đủ dữ kiện, ghi backlog khi thuộc scope và không tự đoán.

## DEVEX-ENV task entry

[Workload environment editor handoff](implementation/workload-environment-editor.md)
collects the approved UC-12/16 design, API contracts, implementation order and
acceptance scenarios for the unified source table (2026-10-24; not implemented).

## Major artifact indexes

- [Use cases](usecase/README.md)
- [Architecture](architecture/README.md)
- [Implementation](implementation/README.md)
- [Traceability and design gate](traceability/design-gate.md)
- [Operations](operations/README.md)
- [Verification](verification/README.md)
- [Backlog](backlog/README.md)
- [Iterations](iterations/README.md)

## Terraform v2 design route

Start with [ADR-014](architecture/decisions/ADR-014-terraform-v2-lifecycle.md),
then affected use-case specifications, [module contract](architecture/contracts/terraform-modules.md),
[state contract](architecture/contracts/terraform-state.md) and
[acceptance scope](operations/terraform-v2-acceptance.md). Full recovery, orphan
reconciliation and platform backup are explicitly deferred in D12–D14.

M04 đã hoàn tất ngày16-10-2026: [plan](iterations/M04-terraform-v2/README.md),
[rule reconciliation](implementation/v2-rule-reconciliation.md). Chặng 1 đã duyệt; quyết định 2026-10-14 bỏ điểm duyệt 2. I04-11
pass + video/estimate + cleanup sạch thì tự chạy AWS; quyết định2026-10-15
bỏ điểm duyệt cuối và yêu cầu sửa/check/chạy lại khi scenario thất bại.

Kết quả và cleanup cuối: [bàn giao M04](verification/2026-10-16-m04-completion.md).

## Git-owned Kubernetes manifest rendering

For the requested change after infrastructure provisioning, start with
[ADR-021](architecture/decisions/ADR-021-git-render-bundles.md), then
[Git bundle contract](architecture/contracts/git-render-bundles.md),
[implementation handoff](implementation/git-render-bundles.md) and
[real Playwright acceptance](operations/git-render-bundles-acceptance.md).
UC-03/05/06/07 RB requirements own behavior; implemented and accepted 2026-10-24
([verification](verification/2026-10-24-git-render-bundles.md)).
This route does not activate a roadmap iteration.

## Basic application logging and monitoring

[ADR-022](architecture/decisions/ADR-022-native-observability.md) and
[UC-09 TEL](usecase/UC-09/specification.md#native-application-observability--tel)
define the approved native collector slice of issue #1; read the
[shared contract](architecture/contracts/observability.md) for API/storage.
The mock is approved UI reference, not evidence of collection or retention.
[Real new-user acceptance](verification/2026-10-25-native-observability-live.md)
records the native Kubernetes end-to-end run, including product cleanup.
