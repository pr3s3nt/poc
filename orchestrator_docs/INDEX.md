---
id: DOC-INDEX
artifact: documentation-index
status: current
last_reviewed: 2026-09-21
---

# Documentation index

Index này giúp người và AI tìm bộ tài liệu nhỏ nhất, đúng thẩm quyền cho một
task. Nó chỉ điều hướng; không tự sở hữu product behavior, architecture hay
implementation status.

## Project entry points

- [Repository overview](../README.md)
- [AI agent workflow](../AGENTS.md)
- [Current project state](CURRENT_STATE.md)
- [Project glossary](GLOSSARY.md)
- [Documentation rules](DOCUMENTATION_RULES.md)

## Route by task

| Task | Read first |
|---|---|
| Hiểu scope, mức hoàn thành hoặc limitation | [Current project state](CURRENT_STATE.md) |
| Tra cứu thuật ngữ | [Glossary](GLOSSARY.md) |
| Thay đổi UC-01 | [UC-01 context](usecase/UC-01/README.md) |
| Thay đổi UC-02 | [UC-02 context](usecase/UC-02/README.md) |
| Thay đổi UC-03 | [UC-03 context](usecase/UC-03/README.md) |
| Thay đổi UC-04 | [UC-04 context](usecase/UC-04/README.md) |
| Thay đổi UC-05 | [UC-05 context](usecase/UC-05/README.md) |
| Thay đổi UC-06 | [UC-06 context](usecase/UC-06/README.md) |
| Thay đổi UC-07 | [UC-07 context](usecase/UC-07/README.md) |
| Thay đổi UC-08 | [UC-08 context](usecase/UC-08/README.md) |
| Thay đổi UC-09 | [UC-09 context](usecase/UC-09/README.md) |
| Thay đổi shared architecture/domain | [Architecture index](architecture/README.md) |
| Thay đổi schema/constraint | [Database schema](architecture/database/schema.md) |
| Hiểu rationale của quyết định | [ADR index](architecture/decisions/README.md) |
| Làm hạng mục đang hoạt động | [Iteration index](iterations/README.md) |
| Làm vấn đề deferred/open | [Backlog index](backlog/README.md) |
| So thiết kế với code | [Implementation index](implementation/README.md) |
| Điều tra khác biệt design/code | [Known deviations](implementation/deviations.md) |
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

## Major artifact indexes

- [Use cases](usecase/README.md)
- [Architecture](architecture/README.md)
- [Implementation](implementation/README.md)
- [Traceability and design gate](traceability/design-gate.md)
- [Operations](operations/README.md)
- [Verification](verification/README.md)
- [Backlog](backlog/README.md)
- [Iterations](iterations/README.md)
