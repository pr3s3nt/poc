# Project instructions for AI agents

## Purpose and scope

File này quy định cách AI agent làm việc trong toàn repository. Yêu cầu cụ thể
của người dùng quyết định mục tiêu và phạm vi của từng task; tài liệu này không
tự mở rộng phạm vi sang hạng mục roadmap khác.

Tài liệu này không sở hữu product requirement, architecture, project status hay
runbook. Dùng [documentation index](orchestrator_docs/INDEX.md) để tìm nguồn
chuẩn tương ứng.

## Starting a task

Trước khi sửa file:

1. Phân loại task:
   - review/explanation/status: chỉ đọc và báo cáo, không sửa nếu người dùng
     không yêu cầu;
   - diagnosis: xác định nguyên nhân, chỉ sửa khi remediation thuộc yêu cầu;
   - change/build: implement, validate và hand off trong phạm vi được yêu cầu.
2. Chạy `git status --short --branch`; coi mọi thay đổi và file untracked có sẵn
   là công việc của người dùng.
3. Đọc `orchestrator_docs/INDEX.md`, chọn route phù hợp và chỉ nạp bộ tài liệu
   nhỏ nhất đủ cho task.
4. Với thay đổi theo use case, bắt đầu từ
   `orchestrator_docs/usecase/UC-xx/README.md`, rồi đọc specification và artifact
   liên quan.
5. Đọc tài liệu để hiểu intent trước; sau đó đọc code và test để hiểu trạng thái
   thực tế. Không suy product requirement chỉ từ code hiện tại.
6. Dùng change-impact map bên dưới để xác định artifact cần kiểm tra.
7. Nếu một quyết định còn thiếu có thể thay đổi behavior, architecture, data,
   security hoặc scope, dừng phần phụ thuộc và hỏi người dùng.

Chỉ khi người dùng yêu cầu “tiếp tục roadmap” mới đọc active iteration trong
`orchestrator_docs/iterations/`. Không tự động bỏ qua yêu cầu hiện tại để làm
hạng mục roadmap kế tiếp.

## Canonical change order

| Change | Required sequence |
|---|---|
| Required product behavior | Specification → realization/diagram → shared design → code/test → traceability |
| Shared architecture/domain | Architecture/ADR → affected use cases → code/test |
| Database design | `architecture/database/schema.md` → ERD → adapter/migration/test |
| Code defect against correct docs | Giữ canonical docs; sửa code và test |
| Designed but unimplemented behavior | Xác nhận docs đầy đủ → implement → cập nhật current state/code map |
| Behavior-preserving refactor | Chỉ cập nhật code map/path/runbook bị ảnh hưởng |
| Unexplained design/code mismatch | Ghi vào `orchestrator_docs/implementation/deviations.md` trước khi giải quyết |
| Chưa thể quyết định | Ghi backlog nếu task cho phép; không biến option thành requirement |

Không sửa canonical documentation chỉ để hợp thức hóa code hiện tại. Khi intent
thay đổi, sửa nguồn chuẩn trước rồi mới sửa dependent artifacts.

## Source and reference boundaries

- `backend/` chứa Go product module, tests và acceptance workloads.
- `frontend/` là Orchestrator Web Console.
- `backend/examples/acceptance-app/` là workload kiểm chứng, không phải
  Web Console.
- `orchestrator_docs/` chứa documentation; không chứa runtime product code.
- `orchestrator_reference/` là reference tracked, read-only. Không sửa hoặc copy
  nguyên domain/code vào sản phẩm. Các local duplicate repository ở root đã
  được retire theo ADR-005 và không được tạo lại.
- Planner, API, executor, CLI, acceptance workload và backend tests dùng Go.
  Web Console dùng React + TypeScript strict. Không thêm Python runtime vào sản
  phẩm.

## Change-impact map

“Inspect” không đồng nghĩa với “edit”; chỉ sửa artifact thực sự bị ảnh hưởng.

| Change | At minimum, inspect |
|---|---|
| Use-case flow/business rule | Specification, realization, sequence, contracts, traceability, tests |
| Operation/participant | Realization, sequence, VOPC, shared classes, contracts |
| Domain object/relationship | Domain model, schema, persistence, tests |
| State/transition | Specification, state machine, contracts, schema, code, tests |
| API/UI | Specification, realization, handler/client, UI states, tests |
| Planner contract | UC-05/06/07/08, planner reference, conformance tests, traceability |
| External provider | ADR/architecture, adapter, configuration, runbook, verification |
| Source path/module name | Imports, build tools, runbooks, code map, links |
| Implemented scope | `CURRENT_STATE.md`, code map/deviations, tests, verification |
| Documentation structure | `INDEX.md`, this file, `DOCUMENTATION_RULES.md`, checker/CI |

## Repository and external-state safety

1. Không sửa, xóa, revert hoặc commit thay đổi có sẵn ngoài task.
2. Không đọc/hiển thị credential, secret, Terraform state hoặc local `.env` nếu
   task không thực sự cần.
3. Thay đổi code không tự cấp quyền deploy hoặc mutate cluster/cloud.
4. Chỉ chạy kind/AWS integration khi người dùng đặt external verification vào
   scope. AWS run phải có preflight, run ID/tag, cost estimate, cleanup trap và
   xác minh cleanup theo `orchestrator_docs/operations/aws.md`.
5. Không commit `node_modules`, `dist`, coverage, binary, kubeconfig, local
   snapshot hoặc Terraform state.
6. Không commit hay push nếu người dùng không yêu cầu.

## Validation

Chạy validation tương ứng với phạm vi thay đổi:

| Scope | Minimum validation |
|---|---|
| Documentation | `python3 scripts/check_docs.py` và `git diff --check` |
| Go backend/product | `cd backend && go test ./... && go build ./...` |
| Web Console | `cd frontend && npm run typecheck && npm run lint && npm test && npm run build` |
| Shell | `bash -n <changed scripts>` |
| PlantUML | checker tài liệu; CI yêu cầu PlantUML |
| Renamed path/ID | Search toàn repository để tìm stale references |

Test làm thay đổi database, Kubernetes hoặc cloud chỉ chạy khi môi trường có sẵn
và external mutation đã được cho phép.

## Definition of done and handoff

Task chỉ hoàn thành khi outcome đã tồn tại, canonical docs/code/test/traceability
nhất quán hoặc deviation còn lại được ghi rõ, validation phù hợp đã pass, và
không làm mất công việc có sẵn của người dùng.

Handoff phải nêu ngắn gọn: outcome, vùng thay đổi, checks đã chạy, checks bỏ qua
và lý do, cùng mọi user-owned change còn lại.
